# A machine whose managed.json takes hello and jq from the flake input "other"
# (from-input/other, a marked nixpkgs), passed in specialArgs.inputs as the
# starter fleet does. missing and noInputs are the same machine without that
# input, and without inputs at all: both fail. Read by
# TestManagedPackageFromAnotherInput (needs real Nix and network).
let
  nixpkgs = builtins.getFlake "github:NixOS/nixpkgs/nixos-26.05";
  machine =
    specialArgs:
    nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      inherit specialArgs;
      modules = [
        (import ../fleet/modules/managed.nix ./from-input/managed.json)
        {
          boot.loader.grub.enable = false;
          fileSystems."/" = {
            device = "nodev";
            fsType = "tmpfs";
          };
          system.stateVersion = "26.05";
          nixpkgs.config.allowUnfree = true;
        }
      ];
    };
  m = machine { inputs.other = ./from-input/other; };
  input = p: p.NIXMGR_INPUT or null;
in
{
  ok = {
    installed = map input (builtins.filter (p: (p.pname or "") == "hello") m.config.environment.systemPackages);
    hello = [
      (input m.pkgs.hello)
      m.pkgs.hello.NIXMGR_SYSTEM
      m.pkgs.hello.NIXMGR_UNFREE
    ];
    jq = [
      (input m.pkgs.jq)
      (m.pkgs.jq.NIXMGR_FROM or null)
    ];
    rsync = [
      (input m.pkgs.rsync)
      (m.pkgs.rsync.NIXMGR_FROM or null)
    ];
  };
  missing = input (machine { inputs = { }; }).pkgs.hello;
  noInputs = input (machine { }).pkgs.hello;
}
