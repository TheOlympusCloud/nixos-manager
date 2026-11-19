# nixmgr:discover - the configurations the flake defines, by kind.
let
  f = builtins.getFlake (builtins.getEnv "NIXMGR_FLAKE");
  names = k: if builtins.hasAttr k f then builtins.attrNames f.${k} else [ ];
in
{
  nixos = names "nixosConfigurations";
  home = names "homeConfigurations";
  darwin = names "darwinConfigurations";
}
