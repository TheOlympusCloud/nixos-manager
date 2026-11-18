# NixOS configurations

- `flake.nix`: inputs and the list of machines (`nixosConfigurations`).
- `hosts/<name>/`: one folder per machine, with its `hardware-configuration.nix`
  and `managed.json` (packages and options changed by clicking in nixos-manager;
  edit the other files by hand).
- `modules/`: settings shared by several machines; `common.nix` enables comin everywhere.
- `home/`: home-manager configurations for users.

Every machine runs comin: it pulls `main` from Forgejo and switches to
`nixosConfigurations.<its hostname>`. A branch named `testing-<hostname>` is
deployed to that machine only, with `switch-to-configuration test`.

Never put secrets in this repository; use sops-nix or agenix.
