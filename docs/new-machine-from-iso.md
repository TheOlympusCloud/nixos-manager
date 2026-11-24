# A new machine from the NixOS ISO

From a fresh install to a machine that comin keeps in step with the fleet and
that reports to nixos-manager. There are two ways:

- **An installer ISO from the console** (below): the console builds a USB
  stick for the machine, and one command on it installs the machine already
  enrolled. Nothing to copy by hand.
- **By hand, from the NixOS ISO** (further down): install NixOS as usual, copy
  three things from it into the console, and switch it once.

## Once, before the first machine

- nixos-manager runs on the home server ([Proxmox
  LXC](../README.md#run-it-in-a-proxmox-lxc-recommended)), is connected to the
  fleet repository on Forgejo ([Forgejo access](../README.md#forgejo-access)),
  and **Push** has published it.
- In `modules/common.nix`, the comin remote's `url` is the repository's HTTPS
  address as the machines reach it, and `fleet.deployReport.url` is the
  console's address ([Deployments](../README.md#deployments)). Check, commit,
  push.
- A private repository: **Settings → Secrets → Private repository** holds
  comin's read token ([details](../README.md#private-repository-and-report-tokens)).

## The installer ISO from the console

1. **Machines → New machine**: its name (it becomes the host name, which comin
   finds its configuration by), architecture and role; under **User**, the
   user from the installer, same name, **Administrator** on, with your SSH
   public key; **Physical machine** (or **Proxmox VM**), **Installed by
   hand**; the boot loader the install's `configuration.nix` shows (**UEFI**
   for `boot.loader.systemd-boot`, **BIOS** for `boot.loader.grub.device`,
   with that disk); paste the hardware configuration. The wizard writes the
   boot loader into `hosts/<name>/default.nix`. Only if the install's
   `system.stateVersion` is not `26.05`, or it has `boot.initrd.luks` lines
   the hardware configuration lacks (an encrypted swap, say), copy those into
   that file in **Code**.
   **Desktop** is Plasma: a machine installed with GNOME gets Plasma at step 4.
2. The machine's page → **Users**: the user from step 1 (or **Add user** if
   you left it out there), with your SSH public key. Add `networkmanager` to
   its groups where NetworkManager is on, as the installer had it. Its
   password stays as set. The roles create no user, and **Server** allows SSH
   keys only.
3. **Settings → Secrets → Machine keys → Add key** next to the machine: paste
   the `.pub` line from step 2. With deployment reports on, click **Store every
   machine's token** again: it only covers machines that had a key when
   clicked.
4. Nothing else in `/etc/nixos/configuration.nix` is carried over: the
   desktop, services, time zone, locale and keyboard picked in the installer
   are gone at step 4 unless you add them in the console (time zone, locale
   and keyboard are in the machine's **Essentials**). **Above all the
   network:** the installer turns NetworkManager on, and of the roles only
   **Desktop** has it. A Wi-Fi machine on **Server** or **Minimal** needs
   `networking.networkmanager.enable = true;` in `default.nix` before step 4
   (the Wi-Fi it already saved then keeps working). Without it the machine
   loses its network at the switch, comin can never fetch again, and only its
   own keyboard can bring it back (boot the previous generation).
5. **Changes**: stage everything, run the check, commit, **Push**.

### 4. Switch it once

On the machine, as root:

```sh
nix-shell -p git
git clone https://forgejo.example.lan/pablo/nixos-config.git /root/nixos-config
nixos-rebuild switch --flake /root/nixos-config#<name>
exit
rm -rf /root/nixos-config
reboot
```

A private repository asks for your Forgejo user and a token for the clone.
From now on comin pulls `main` and switches the machine; `/etc/nixos` is no
longer used. Change the machine in the console only: a `nixos-rebuild` on the
machine shows as **Changed by hand**.

### 5. Check it is in

- On the machine: `journalctl -u comin -f` shows comin fetching the
  repository.
- In the console: **Deployments** lists the machine at the commit you pushed
  within about 15 minutes (its first report or heartbeat), and **Health** has
  nothing about it.
