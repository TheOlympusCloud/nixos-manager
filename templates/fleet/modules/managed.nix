# Applies a machine's managed.json, the file nixos-manager edits when you
# change packages and options by clicking, and the files next to it that
# nixos-manager creates for you. Hand-written settings stay in the other files;
# this module only reads them.
#
# Use it per machine: (import ./modules/managed.nix ./hosts/<name>/managed.json)
#
# Next to managed.json, each optional:
# - custom.nix: a NixOS module of your own for this machine.
# - packages/<attribute>.nix: `{ pkgs, prev, lib, ... }: pkg: <package>`, which
#   changes that package on this machine; pkg already has managed.json's changes.
# - home/<user>/custom.nix: a home-manager module for one user of managed.json's "home".
# - home/<user>/files/<path>: a file linked at ~/<path> for that user (home.file).
file:
{ lib, pkgs, ... }:
let
  data = if builtins.pathExists file then lib.importJSON file else { };
  dir = dirOf file;
  ifExists = p: lib.optional (builtins.pathExists p) p;
  attrPath = lib.splitString ".";
  # throw (not abort): a misspelled name fails this machine's build with a clear
  # message, and the fleet view still shows the rest of the machine.
  package = name: lib.attrByPath (attrPath name) (throw "managed.json: package ${name} is not in nixpkgs") pkgs;
  pname = p: p.pname or (builtins.parseDrvName p.name).name;
  # Removed packages go by name, whoever adds them (NixOS, an input or hand-written Nix),
  # except those added here: replacing nodejs_20 with nodejs_24 keeps nodejs_24.
  without = names: keep: builtins.filter (p: !(builtins.elem (pname p) names) || builtins.elem p keep);
  # Priority on each value, never on a whole attribute set: users.users or
  # services.openssh.settings set here must still merge with hand-written entries.
  # Lists are values, so a list set here replaces the hand-written one.
  values = v: if builtins.isAttrs v then lib.mapAttrs (_: values) v else lib.mkOverride 90 v;

  removed = data.removedPackages or [ ];
  overrides = data.packageOverrides or { };
  packageDir = dir + "/packages";
  packageFiles = lib.optionalAttrs (builtins.pathExists packageDir) (
    lib.mapAttrs' (n: _: lib.nameValuePair (lib.removeSuffix ".nix" n) (packageDir + "/${n}")) (
      lib.filterAttrs (n: t: t == "regular" && lib.hasSuffix ".nix" n) (builtins.readDir packageDir)
    )
  );
  changed = lib.unique (builtins.attrNames overrides ++ builtins.attrNames packageFiles);
  # An empty hash is lib.fakeHash: the build fails and prints the right one.
  hashOf = x: if (x.hash or "") == "" then lib.fakeHash else x.hash;
  source =
    final: s:
    if s ? github then
      final.fetchFromGitHub {
        owner = builtins.head (lib.splitString "/" s.github);
        repo = lib.last (lib.splitString "/" s.github);
        inherit (s) rev;
        hash = hashOf s;
      }
    else
      final.fetchurl {
        inherit (s) url;
        hash = hashOf s;
      };
  patch =
    final: p:
    if p ? file then
      dir + "/${p.file}"
    else
      final.fetchpatch {
        inherit (p) url;
        hash = hashOf p;
      };
  customise =
    final: prev: attr: pkg:
    let
      o = overrides.${attr} or { };
      patches = o.patches or [ ];
      attrs =
        (o.overrideAttrs or { })
        // lib.optionalAttrs (o ? version) { inherit (o) version; }
        // lib.optionalAttrs (o ? src) { src = source final o.src; };
      overridden = if o ? override then pkg.override o.override else pkg;
      built =
        if attrs == { } && patches == [ ] then
          overridden
        else
          overridden.overrideAttrs (
            old: attrs // lib.optionalAttrs (patches != [ ]) { patches = (old.patches or [ ]) ++ map (patch final) patches; }
          );
    in
    if packageFiles ? ${attr} then
      import packageFiles.${attr} {
        pkgs = final;
        inherit prev lib;
      } built
    else
      built;
  # python3Packages.requests replaces requests inside python3Packages, keeping the rest.
  # shortcut: a nested attribute is changed where it is read directly, not inside
  # python3.withPackages or another package set's own fixed point; use a raw Nix file for those.
  put =
    acc: prevSet: path: value:
    let
      h = builtins.head path;
    in
    if builtins.length path == 1 then
      acc // { ${h} = value; }
    else
      acc // { ${h} = put (acc.${h} or prevSet.${h}) prevSet.${h} (builtins.tail path) value; };
  overlay =
    final: prev:
    builtins.foldl' (
      acc: attr: put acc prev (attrPath attr) (customise final prev attr (lib.getAttrFromPath (attrPath attr) prev))
    ) { } changed;

  homes = data.home or { };
  homeModule =
    user: u:
    let
      gone = u.removedPackages or [ ];
      files = dir + "/home/${user}/files";
    in
    {
      _file = file;
      imports = ifExists (dir + "/home/${user}/custom.nix");
      options = lib.optionalAttrs (gone != [ ]) {
        home.packages = lib.mkOption { apply = without gone (map package (u.packages or [ ])); };
      };
      config = {
        home.packages = map package (u.packages or [ ]);
        home.file = lib.optionalAttrs (builtins.pathExists files) (
          builtins.listToAttrs (
            map (f: lib.nameValuePair (lib.removePrefix "${toString files}/" (toString f)) { source = f; }) (
              lib.filesystem.listFilesRecursive files
            )
          )
        );
      };
    };
in
{
  # Definitions below are reported as coming from the JSON file.
  _file = file;
  imports = ifExists (dir + "/custom.nix");

  options = {
    nixosManager.managedFile = lib.mkOption {
      type = lib.types.str;
      default = toString file;
      internal = true;
      description = "The managed.json that nixos-manager edits for this machine.";
    };
    nixosManager.version = lib.mkOption {
      type = lib.types.int;
      default = 2;
      internal = true;
      description = "What this module can apply: 2 adds removed packages, package overrides, home-manager and the files next to managed.json.";
    };
    nixosManager.packageNames = lib.mkOption {
      type = lib.types.attrsOf lib.types.str;
      default = lib.listToAttrs (
        map (name: {
          inherit name;
          value = pname (package name);
        }) (lib.unique (data.packages or [ ] ++ changed ++ lib.concatMap (u: u.packages or [ ]) (builtins.attrValues homes)))
      );
      internal = true;
      description = "The package name of each attribute managed.json names, so nixos-manager can match them.";
    };
  }
  # A second declaration may add the apply function NixOS's own lacks.
  // lib.optionalAttrs (removed != [ ]) {
    environment.systemPackages = lib.mkOption { apply = without removed (map package (data.packages or [ ])); };
  };

  config = lib.mkMerge [
    # A value set by clicking is the machine's final value: it wins over hand-written
    # definitions of the same option (priority 90 beats the default 100), but not
    # over lib.mkForce. Lists are therefore written whole, not merged.
    (values (data.settings or { }))
    { environment.systemPackages = map package (data.packages or [ ]); }
    # Changed packages are changed for everything on the machine that uses them,
    # home-manager included when it shares the system's pkgs (useGlobalPkgs).
    { nixpkgs.overlays = lib.optional (changed != [ ]) overlay; }
    (lib.optionalAttrs (homes != { }) { home-manager.users = lib.mapAttrs homeModule homes; })
  ];
}
