# nixmgr:home - packages, programs and files of one standalone home-manager configuration (host).
{ flakeRef, host }:
let
  flake = builtins.getFlake flakeRef;
  name = host;
  home = flake.homeConfigurations.${name};
  lib = if flake.inputs ? nixpkgs then flake.inputs.nixpkgs.lib else home.pkgs.lib;
  root = "${toString flake.outPath}/";
  try = v: default: let r = builtins.tryEval v; in if r.success then r.value else default;
  pkgName = p: try (p.pname or (builtins.parseDrvName (p.name or "unknown")).name) "unknown";
  managed = try (home.config.nixosManager.managedFile or null) null;
  files =
    let
      ts = lib.unique (map (f: f.target) (builtins.attrValues (lib.filterAttrs (_: f: f.enable or true) home.config.home.file)));
    in
    try (builtins.deepSeq ts ts) [ ];
in
{
  inherit name files;
  packages = lib.unique (map pkgName (try home.config.home.packages [ ]));
  # Inside try as a whole: reading a removed option (programs.octant) throws.
  programs = builtins.attrNames (
    lib.filterAttrs (_: p: try (builtins.isAttrs p && (p.enable or false) == true) false) (try home.config.programs { })
  );
  # The managed.json modules/managed-home.nix applies, relative to the flake; null
  # without it, or for one outside the flake, which cannot be edited here.
  managedFile = if managed != null && lib.hasPrefix root managed then lib.removePrefix root managed else null;
  # attribute -> package name for managed.json packages (one bad entry does not hide the others)
  managedPackages = lib.mapAttrs (_: n: try n "") (try (home.config.nixosManager.packageNames or { }) { });
}
