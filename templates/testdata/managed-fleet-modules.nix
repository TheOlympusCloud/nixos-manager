# Machines a to d with fleet modules next to managed.nix: media (options port and
# name, a custom.nix) and desk (on for every machine in modules/managed.json).
# $NIXMGR_TEST_FLEET is a fleet copy made by TestManagedFleetModules (needs real
# Nix and network).
let
  dir = builtins.getEnv "NIXMGR_TEST_FLEET";
  root = /. + dir;
  nixpkgs = builtins.getFlake "github:NixOS/nixpkgs/nixos-26.05";
  inherit (nixpkgs) lib;
  try = v: let r = builtins.tryEval v; in if r.success then r.value else "failed";
  report =
    name:
    let
      m = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [
          (import (root + "/modules/managed.nix") (root + "/hosts/${name}/managed.json"))
          {
            boot.loader.grub.enable = false;
            fileSystems."/" = {
              device = "nodev";
              fsType = "tmpfs";
            };
            system.stateVersion = "26.05";
            # Hand-written: the modules' ports merge with it.
            networking.firewall.allowedTCPPorts = [ 22 ];
          }
        ];
      };
      c = m.config;
      has = ps: n: builtins.any (p: (p.pname or "") == n) ps;
    in
    {
      media = c.fleet.modules.media.enable;
      desk = c.fleet.modules.desk.enable;
      packages = builtins.filter (has c.environment.systemPackages) [
        "hello"
        "jq"
      ];
      ports = lib.sort lib.lessThan c.networking.firewall.allowedTCPPorts;
      conf = try (c.environment.etc."media.conf".text or null);
      custom = c.environment.variables.NIXMGR_MEDIA or null;
      git = c.programs.git.enable;
      timeZone = c.time.timeZone;
      version = c.nixosManager.version;
      # Where the model finds a module turned on, and an option's value set.
      enableFiles = map (d: lib.removePrefix "${dir}/" (toString d.file)) m.options.fleet.modules.media.enable.definitionsWithLocations;
      portFiles = map (d: lib.removePrefix "${dir}/" (toString d.file)) m.options.fleet.modules.media.port.definitionsWithLocations;
      portChecked = m.options.fleet.modules.media.port.type.check 70000;
      portDescription = m.options.fleet.modules.media.port.description;
    };
in
lib.genAttrs [
  "a"
  "b"
  "c"
  "d"
] report
