# NixOS configurations

- `flake.nix`: inputs and how machines are built. Every folder in `hosts/` with a
  `default.nix` becomes a machine (`nixosConfigurations.<folder>`), so adding one
  is adding a folder; nixos-manager's "New machine" does it for you.
- `hosts/<name>/`: one folder per machine, with its `hardware-configuration.nix`
  and `managed.json` (packages and options changed by clicking in nixos-manager;
  edit the other files by hand).
- `modules/`: settings shared by several machines; `common.nix` enables comin everywhere,
  and connects the machines to nixos-manager: deployment reports and its binary cache.
- `home/`: home-manager configurations for users.
- `.forgejo/workflows/check.yml`: with Forgejo Actions, a runner checks every
  push to a branch (`nix flake check` and a build of every machine). nixos-manager's push
  webhook checks pushes on its own server too, so this file is optional.

Every machine runs comin: it pulls `main` from Forgejo and switches to
`nixosConfigurations.<its hostname>`. A branch named `testing-<hostname>` is
deployed to that machine only, with `switch-to-configuration test`.

Never put secrets in this repository; use sops-nix or agenix.
