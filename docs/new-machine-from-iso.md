# A new machine from the NixOS ISO

From a fresh install to a machine that comin keeps in step with the fleet and
that reports to nixos-manager. The console never connects to the machine: you
copy three things from it, and switch it once by hand.

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

## 1. Install NixOS

Install from the ISO as usual and reboot into it. The host name, user and
password you pick are fine as they are.

## 2. Copy three things from the machine

As root (`sudo -i`):

```sh
nixos-generate-config --show-hardware-config     # the hardware configuration
ssh-keygen -A                                     # makes the SSH host keys if the install has none
cat /etc/ssh/ssh_host_ed25519_key.pub             # the machine's key, for its secrets
grep -nE 'boot\.|stateVersion' /etc/nixos/configuration.nix   # boot loader and stateVersion
```

Paste them into the console from a browser on the machine, or turn on
`services.openssh.enable = true;` in the installer's configuration and copy them
over SSH.

## 3. Add it in the console

1. **Machines → New machine**: its name (it becomes the host name, which comin
   finds its configuration by), architecture and role; **Physical machine** (or
   **Proxmox VM**), **Installed by hand**; paste the hardware configuration.
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

3. The machine's page → **Users → Add user**: the user from the installer, same
   name, group `wheel`, with your SSH public key. Its password stays as set.
   The roles create no user, and **Server** allows SSH keys only.
4. **Settings → Secrets → Machine keys → Add key** next to the machine: paste
   the `.pub` line from step 2. With deployment reports on, click **Store every
   machine's token** again: it only covers machines that had a key when
   clicked.
5. Time zone, locale and keyboard from the installer are not carried over: set
   them in the machine's **Essentials** if they differ from the fleet's.
6. **Changes**: stage everything, run the check, commit, **Push**.

## 4. Switch it once

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

## 5. Check it is in

- On the machine: `journalctl -u comin -f` shows comin fetching the
  repository.
- In the console: **Deployments** lists the machine at the commit you pushed
  within about 15 minutes (its first report or heartbeat), and **Health** has
  nothing about it.
