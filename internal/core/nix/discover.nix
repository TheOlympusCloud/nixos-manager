# nixmgr:discover - the configurations the flake defines, by kind.
# flakeRef is locked: these expressions are evaluated in pure mode, so fleet code cannot read the server's files.
{ flakeRef }:
let
  f = builtins.getFlake flakeRef;
  names = k: if builtins.hasAttr k f then builtins.attrNames f.${k} else [ ];
in
{
  nixos = names "nixosConfigurations";
  home = names "homeConfigurations";
  darwin = names "darwinConfigurations";
}
