{
  description = "nixos-manager: a self-hosted console for a NixOS fleet kept in one flake repository";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
      version = self.shortRev or self.dirtyShortRev or "dev";
    in
    {
      packages = forAllSystems (pkgs: {
        default = pkgs.callPackage ./nix/package.nix { inherit version; };
      });

      overlays.default = final: _prev: {
        nixos-manager = final.callPackage ./nix/package.nix { inherit version; };
      };

      nixosModules.default =
        { lib, pkgs, ... }:
        {
          imports = [ ./nix/module.nix ];
          # Built with the system's own nixpkgs.
          services.nixos-manager.package = lib.mkDefault (
            pkgs.callPackage ./nix/package.nix { inherit version; }
          );
        };

      # Evaluates the module in the README's LXC configuration and checks the service, without building the app.
      checks = forAllSystems (pkgs: {
        module = import ./nix/module-check.nix {
          inherit nixpkgs pkgs;
          module = self.nixosModules.default;
        };
      });
    };
}
