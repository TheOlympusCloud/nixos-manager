# NixOS configurations

- `flake.nix`: inputs and how machines are built. Every folder in `hosts/` with a
  `default.nix` becomes a machine (`nixosConfigurations.<folder>`), so adding one
  is adding a folder; nixos-manager's "New machine" does it for you.
- `hosts/<name>/`: one folder per machine, with its `hardware-configuration.nix`
  and `managed.json` (packages, removed and changed packages, home-manager users
  and options changed by clicking in nixos-manager; edit the other files by hand).
  A machine installed with nixos-anywhere also has `disk-config.nix`, its disk
  layout for disko. `modules/managed.nix` also applies, when they exist, the
  machine's `custom.nix`, `packages/<attribute>.nix` (how a package is built),
  and per home-manager user `home/<user>/custom.nix` and the dotfiles in
  `home/<user>/files/`.
- `modules/`: settings shared by several machines; `managed.json` (when it exists) is what
  nixos-manager sets for every machine, under each machine's own `managed.json`; `common.nix` enables comin everywhere,
  and connects the machines to nixos-manager: deployment reports, its binary cache and
  secrets (`secrets.nix`).
- `home/`: home-manager configurations for users.
- `homes/<name>/`: what nixos-manager sets for a standalone home-manager configuration
  (`homeConfigurations.<name>`, for a machine that is not NixOS), applied by
  `modules/managed-home.nix` once the home's modules load it:
  `(import ./modules/managed-home.nix ./homes/<name>/managed.json)`.
- `.forgejo/workflows/check.yml`: with Forgejo Actions, a runner checks every
  push to a branch (`nix flake check` and a build of every machine). nixos-manager's push
  webhook checks pushes on its own server too, so this file is optional.
- `secrets/` and `.sops.yaml`: the machines' secrets, encrypted with sops for nixos-manager
  and the machines that get them (`common.json` for every machine with a key,
  `hosts/<name>.json` for one), and the machines' SSH host keys (`hosts/<name>.pub`).
  nixos-manager writes them (Settings → Secrets); `modules/secrets.nix` puts each value on
  its machine as `/run/secrets/<name>`, once the file is encrypted for the machine's
  approved key (named in `.sops.yaml`). Never put a secret here in plain text.

Every machine runs comin: it pulls `main` from Forgejo and switches to
`nixosConfigurations.<its hostname>`. A branch named `testing-<hostname>` is
deployed to that machine only, with `switch-to-configuration test`, while it is
on top of `main`: nixos-manager's **Try on** in Changes pushes one, to try a
change on one machine before `main`. nixos-manager's scheduled update of the inputs
pushes `flake-update`, which no machine follows: it reaches them only once
merged into `main`.
