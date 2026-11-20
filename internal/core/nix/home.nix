# nixmgr:home - packages and programs of one standalone home-manager configuration (host).
{ flakeRef, host }:
let
  flake = builtins.getFlake flakeRef;
  name = host;
  home = flake.homeConfigurations.${name};
  lib = if flake.inputs ? nixpkgs then flake.inputs.nixpkgs.lib else home.pkgs.lib;
  try = v: default: let r = builtins.tryEval v; in if r.success then r.value else default;
  pkgName = p: try (p.pname or (builtins.parseDrvName (p.name or "unknown")).name) "unknown";
in
{
  inherit name;
  packages = lib.unique (map pkgName (try home.config.home.packages [ ]));
  # Inside try as a whole: reading a removed option (programs.octant) throws.
  programs = builtins.attrNames (
    lib.filterAttrs (_: p: try (builtins.isAttrs p && (p.enable or false) == true) false) (try home.config.programs { })
  );
}
