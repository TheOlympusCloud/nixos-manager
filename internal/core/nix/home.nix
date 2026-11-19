# nixmgr:home - packages and programs of one standalone home-manager configuration (NIXMGR_HOST).
let
  flake = builtins.getFlake (builtins.getEnv "NIXMGR_FLAKE");
  name = builtins.getEnv "NIXMGR_HOST";
  home = flake.homeConfigurations.${name};
  lib = if flake.inputs ? nixpkgs then flake.inputs.nixpkgs.lib else home.pkgs.lib;
  try = v: default: let r = builtins.tryEval v; in if r.success then r.value else default;
  pkgName = p: try (p.pname or (builtins.parseDrvName (p.name or "unknown")).name) "unknown";
in
{
  inherit name;
  packages = lib.unique (map pkgName (try home.config.home.packages [ ]));
  programs = builtins.attrNames (
    lib.filterAttrs (_: p: builtins.isAttrs p && try (p.enable or false) false == true) (try home.config.programs { })
  );
}
