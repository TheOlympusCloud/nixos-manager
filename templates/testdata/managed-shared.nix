# Two machines whose managed.nix has modules/managed.json next to it: a
# manages pablo's home, b has no pablo. $NIXMGR_TEST_FLEET is a fleet copy
# (modules/, hosts/a, hosts/b) made by TestManagedSharedByEveryMachine (needs
# real Nix and network).
let
  dir = builtins.getEnv "NIXMGR_TEST_FLEET";
  root = /. + dir;
  nixpkgs = builtins.getFlake "github:NixOS/nixpkgs/nixos-26.05";
  home-manager = builtins.getFlake "github:nix-community/home-manager/release-26.05";
  inherit (nixpkgs) lib;
  report =
    name:
    let
      m = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [
          (import (root + "/modules/managed.nix") (root + "/hosts/${name}/managed.json"))
          home-manager.nixosModules.home-manager
          {
            boot.loader.grub.enable = false;
            fileSystems."/" = {
              device = "nodev";
              fsType = "tmpfs";
            };
            system.stateVersion = "26.05";
            time.timeZone = "UTC";
            home-manager.useGlobalPkgs = true;
          }
        ];
      };
      c = m.config;
      has = ps: n: builtins.any (p: (p.pname or "") == n) ps;
      pablo = c.home-manager.users.pablo or null;
    in
    {
      packages = builtins.filter (has c.environment.systemPackages) [
        "cowsay"
        "hello"
        "jq"
        "perl"
        "rsync"
      ];
      home = if pablo == null then null else builtins.filter (has pablo.home.packages) [ "cowsay" "hello" ];
      sessionVariable = if pablo == null then null else pablo.home.sessionVariables.NIXMGR or null;
      helloFrom = m.pkgs.hello.NIXMGR_FROM or null;
      helloPatch = builtins.any (p: lib.hasSuffix "/modules/patches/nixmgr.patch" (toString p)) (m.pkgs.hello.patches or [ ]);
      jqFrom = m.pkgs.jq.NIXMGR_FROM or null;
      timeZone = c.time.timeZone;
      ports = c.networking.firewall.allowedTCPPorts;
      version = c.nixosManager.version;
      # The model shows where a package comes from by its definition's file.
      helloFile = map (d: lib.removePrefix "${dir}/" (toString d.file)) (
        builtins.filter (d: has (lib.toList d.value) "hello") m.options.environment.systemPackages.definitionsWithLocations
      );
    };
in
{
  a = report "a";
  b = report "b";
}
