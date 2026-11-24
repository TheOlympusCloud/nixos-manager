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

1. **Machines → New machine**: its name, architecture and role, your user
   with your SSH key, **Physical machine** (or **Proxmox VM**) with a disk
   layout (**ext4** or **Btrfs**) and the disk to erase. Commit and push it in **Changes**. A machine that is
   already in the repository with its real hardware configuration works too.
2. **Machines → Installer ISO**: pick it, **Build ISO**, and wait for the
   build (its log shows meanwhile). **Download** it, check its SHA-256, and
   write it to a USB stick: **Install steps** gives the `dd` command.
3. Boot the machine from the stick and run `sudo fleet-install`. It shows the
   machine's addresses and the SSH host key it will keep. Wi-Fi: `nmtui`
   first.
4. Under **Install steps** in the console: the address, **Fetch host key**,
   compare the fingerprint with the one on the machine, **Use it for secrets**
   (this stores its report token too). Commit and push in **Changes**.
5. On the machine, press Enter. It fetches the repository (a private one asks
   for your Forgejo user and a token), lists the disks, and erases the one in
   the disk layout once you type the machine's name. It installs, offers to
   set the users' passwords, and pushes the hardware configuration it wrote
   (a token that can write). Take out the stick and reboot.

Users come from the console (the wizard's **User** step, or **Users → Add
user** before step 2), with their SSH keys: the install sets no root password.
Then [check it is in](#5-check-it-is-in). What fleet-install does, step by step, and what it
needs: [Install from an installer ISO](../README.md#install-from-an-installer-iso).

## By hand, from the NixOS ISO

The console never connects to the machine: you copy three things from it, and
switch it once by hand.

### 1. Install NixOS

Install from the ISO as usual and reboot into it. The host name, user and
password you pick are fine as they are.

### 2. Copy three things from the machine

As root (`sudo -i`):

```sh
nixos-generate-config --show-hardware-config     # the hardware configuration
ssh-keygen -A                                     # makes the SSH host keys if the install has none
cat /etc/ssh/ssh_host_ed25519_key.pub             # the machine's key, for its secrets
grep -nE 'boot\.|networking|stateVersion' /etc/nixos/configuration.nix   # boot loader, network, stateVersion
```

Paste them into the console from a browser on the machine, or turn on
`services.openssh.enable = true;` in the installer's configuration and copy them
over SSH.

### 3. Add it in the console

1. **Machines → New machine**: its name (it becomes the host name, which comin
   finds its configuration by), architecture and role; under **User**, the
   user from the installer, same name, **Administrator** on, with your SSH
   public key; **Physical machine** (or **Proxmox VM**), **Installed by
   hand**; paste the hardware configuration.
   **Desktop** is Plasma: a machine installed with GNOME gets Plasma at step 4.
2. **Code**, `hosts/<name>/default.nix`: add the boot loader lines from step 2,
   and any `boot.initrd.luks` lines if the disk is encrypted. Set
   `system.stateVersion` to the install's value if it is not `26.05`. The wizard
   writes no boot loader, so until then the machine does not evaluate (`You must
   set the option 'boot.loader.grub.devices'…`). For the installer's UEFI
   default that is:

   ```nix
   boot.loader.systemd-boot.enable = true;
   boot.loader.efi.canTouchEfiVariables = true;
   ```

3. The machine's page → **Users**: the user from step 1 (or **Add user** if
   you left it out there), with your SSH public key. Add `networkmanager` to
   its groups where NetworkManager is on, as the installer had it. Its
   password stays as set. The roles create no user, and **Server** allows SSH
   keys only.
4. **Settings → Secrets → Machine keys → Add key** next to the machine: paste
   the `.pub` line from step 2. With deployment reports on, click **Store every
   machine's token** again: it only covers machines that had a key when
   clicked.
5. Nothing else in `/etc/nixos/configuration.nix` is carried over: the
   desktop, services, time zone, locale and keyboard picked in the installer
   are gone at step 4 unless you add them in the console (time zone, locale
   and keyboard are in the machine's **Essentials**). **Above all the
   network:** the installer turns NetworkManager on, and of the roles only
   **Desktop** has it. A Wi-Fi machine on **Server** or **Minimal** needs
   `networking.networkmanager.enable = true;` in `default.nix` before step 4
   (the Wi-Fi it already saved then keeps working). Without it the machine
   loses its network at the switch, comin can never fetch again, and only its
   own keyboard can bring it back (boot the previous generation).
6. **Changes**: stage everything, run the check, commit, **Push**.

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
