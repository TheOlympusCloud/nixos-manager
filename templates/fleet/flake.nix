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
      # One entry per machine: the name must match its networking.hostName,
      # because comin deploys nixosConfigurations.<hostname>.
      mkHost =
        name:
        nixpkgs.lib.nixosSystem {
          specialArgs = { inherit inputs; };
          modules = [
            ./hosts/${name}
            ./modules/common.nix
            inputs.comin.nixosModules.comin
            inputs.home-manager.nixosModules.home-manager
          ];
        };
    in
    {
      nixosConfigurations = {
        example = mkHost "example";
      };
    };
}
