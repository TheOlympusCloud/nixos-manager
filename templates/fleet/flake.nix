{
  description = "NixOS configurations for all machines";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
    home-manager = {
      url = "github:nix-community/home-manager/release-26.05";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    comin = {
      url = "github:nlewo/comin";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    # Add more inputs here (sops-nix, disko, nixos-hardware, ...), then run "Lock".
  };

  outputs =
    { nixpkgs, ... }@inputs:
    let
      mkHost =
        name:
        nixpkgs.lib.nixosSystem {
          specialArgs = { inherit inputs; };
          modules = [
            ./hosts/${name}
            # Packages and options changed by clicking in nixos-manager.
            (import ./modules/managed.nix ./hosts/${name}/managed.json)
            ./modules/common.nix
            inputs.comin.nixosModules.comin
            inputs.home-manager.nixosModules.home-manager
          ];
        };
      # Every folder in hosts/ with a default.nix is a machine named after the folder.
      # The name must match its networking.hostName: comin deploys nixosConfigurations.<hostname>.
      hosts = builtins.attrNames (
        nixpkgs.lib.filterAttrs (
          name: type: type == "directory" && builtins.pathExists (./hosts + "/${name}/default.nix")
        ) (builtins.readDir ./hosts)
      );
    in
    {
      nixosConfigurations = nixpkgs.lib.genAttrs hosts mkHost;
    };
}
