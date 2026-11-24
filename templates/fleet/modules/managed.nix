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
#
# modules/managed.json, next to this module, has the same shape and is set for
# every machine that loads it; where both set something, the machine's own wins:
# - packages and removedPackages are joined. A name the machine removes takes a
#   shared package out of it; a shared removal never takes out what either file adds.
# - packageOverrides: the machine's entry for an attribute replaces the shared one.
#   A shared patch file is relative to modules/.
# - home.<user> applies where the machine's managed.json has home.<user> too (so
#   no machine gets a home for a user it lacks), joined as the system's packages.
# - settings apply at priority 95, below the machine's (90) but still above
#   hand-written values; settings.home-manager.users.<user> applies as home does.
# The files next to it (custom.nix, packages/, home/) are not read.
#
# A packageOverrides entry with "from": "<input>" takes the package from that
# flake input, another nixpkgs (nixpkgs-unstable, say), instead of the machine's;
# its other changes apply on top. The input comes from nixosSystem's
# specialArgs.inputs, as in the starter fleet's flake.nix.
file:
{
  lib,
  pkgs,
  config,
  ...
}@args:
let
  read = f: if builtins.pathExists f then lib.importJSON f else { };
  data = read file;
  sharedFile = ./managed.json;
  shared = read sharedFile;
  dir = dirOf file;
  ifExists = p: lib.optional (builtins.pathExists p) p;
  attrPath = lib.splitString ".";
  # throw (not abort): a misspelled name fails this machine's build with a clear
  # message, and the fleet view still shows the rest of the machine.
  package = name: lib.attrByPath (attrPath name) (throw "managed.json: package ${name} is not in nixpkgs") pkgs;
  pname = p: p.pname or (builtins.parseDrvName p.name).name;
  # null for a name not in nixpkgs, which fails where it is installed instead.
  nameOf =
    a:
    let
      r = builtins.tryEval (pname (package a));
    in
    if r.success then r.value else null;
  # Shared attributes, less those whose package the machine's removed names take
  # out: these are still defined, so the fleet view lists them as removed.
  sharedFor = removedHere: builtins.filter (a: !builtins.elem (nameOf a) removedHere);
  # Removed packages go by name, whoever adds them (NixOS, an input or hand-written Nix),
  # except those added here: replacing nodejs_20 with nodejs_24 keeps nodejs_24.
  without = names: keep: builtins.filter (p: !(builtins.elem (pname p) names) || builtins.elem p keep);
  # Priority on each value, never on a whole attribute set: users.users or
  # services.openssh.settings set here must still merge with hand-written entries.
  # Lists are values, so a list set here replaces the hand-written one.
  values = priority: v: if builtins.isAttrs v then lib.mapAttrs (_: values priority) v else lib.mkOverride priority v;

  ownPackages = data.packages or [ ];
  removed = lib.unique ((shared.removedPackages or [ ]) ++ (data.removedPackages or [ ]));
  # What no removed name takes out.
  added = map package (ownPackages ++ sharedFor (data.removedPackages or [ ]) (shared.packages or [ ]));
  # attribute -> { o = override; base = the folder its patch files are in }
  overrides =
    lib.mapAttrs (_: o: {
      inherit o;
      base = dirOf sharedFile;
    }) (shared.packageOverrides or { })
    // lib.mapAttrs (_: o: {
      inherit o;
      base = dir;
    }) (data.packageOverrides or { });
  packageDir = dir + "/packages";
  packageFiles = lib.optionalAttrs (builtins.pathExists packageDir) (
    lib.mapAttrs' (n: _: lib.nameValuePair (lib.removeSuffix ".nix" n) (packageDir + "/${n}")) (
      lib.filterAttrs (n: t: t == "regular" && lib.hasSuffix ".nix" n) (builtins.readDir packageDir)
    )
  );
  changed = lib.unique (builtins.attrNames overrides ++ builtins.attrNames packageFiles);
  # The flake inputs that changed packages come from.
  froms = lib.unique (lib.concatMap (x: lib.optional (x.o ? from) x.o.from) (builtins.attrValues overrides));
  inputOf =
    from:
    if !(args ? inputs) then
      throw "managed.json takes packages from the flake input ${from}, but this machine's nixosSystem gets no specialArgs.inputs"
    else
      args.inputs.${from}
        or (throw "managed.json takes packages from the flake input ${from}, which flake.nix does not have: add it to its inputs, then Lock");
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
    final: base: p:
    if p ? file then
      base + "/${p.file}"
    else
      final.fetchpatch {
        inherit (p) url;
        hash = hashOf p;
      };
  customise =
    final: prev: attr: pkg:
    let
      inherit
        (overrides.${attr} or {
          o = { };
          base = dir;
        })
        o
        base
        ;
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
            old: attrs // lib.optionalAttrs (patches != [ ]) { patches = (old.patches or [ ]) ++ map (patch final base) patches; }
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
    let
      # Each such nixpkgs once per machine, for its platform and with its
      # nixpkgs.config (allowUnfree…), but not its overlays.
      # shortcut: a machine that sets nixpkgs.pkgs has an empty nixpkgs.config, so its
      # allowUnfree does not reach the input; take the config from prev if one needs it.
      # shortcut: built natively for the machine's platform even where the machine
      # is cross-compiled; pass buildPlatform as well if a fleet cross-compiles.
      other = lib.genAttrs froms (
        from:
        import (inputOf from) {
          localSystem = { inherit (prev.stdenv.hostPlatform) system; };
          config = config.nixpkgs.config;
        }
      );
      # The package before its changes: from the machine's nixpkgs or the input's.
      base =
        attr:
        let
          from = overrides.${attr}.o.from or null;
        in
        if from == null then
          lib.getAttrFromPath (attrPath attr) prev
        else
          lib.attrByPath (attrPath attr) (throw "managed.json: package ${attr} is not in the flake input ${from}") other.${from};
    in
    builtins.foldl' (acc: attr: put acc prev (attrPath attr) (customise final prev attr (base attr))) { } changed;

  homes = data.home or { };
  sharedHomes = shared.home or { };
  sharedSettings = shared.settings or { };
  # Shared home-manager options go into the homes below, never onto a machine
  # without the user; the rest of home-manager's settings apply as any setting.
  sharedHomeSettings = sharedSettings.home-manager.users or { };
  hmRest = builtins.removeAttrs (sharedSettings.home-manager or { }) [ "users" ];
  sharedTop =
    builtins.removeAttrs sharedSettings [ "home-manager" ]
    // lib.optionalAttrs (hmRest != { }) { home-manager = hmRest; };
  homeModule =
    user: u:
    let
      s = sharedHomes.${user} or { };
      gone = lib.unique ((s.removedPackages or [ ]) ++ (u.removedPackages or [ ]));
      kept = map package ((u.packages or [ ]) ++ sharedFor (u.removedPackages or [ ]) (s.packages or [ ]));
      files = dir + "/home/${user}/files";
    in
    {
      _file = file;
      imports = ifExists (dir + "/home/${user}/custom.nix");
      options = lib.optionalAttrs (gone != [ ]) {
        home.packages = lib.mkOption { apply = without gone kept; };
      };
      config = lib.mkMerge [
        {
          home.packages = map package ((u.packages or [ ]) ++ (s.packages or [ ]));
          home.file = lib.optionalAttrs (builtins.pathExists files) (
            builtins.listToAttrs (
              map (f: lib.nameValuePair (lib.removePrefix "${toString files}/" (toString f)) { source = f; }) (
                lib.filesystem.listFilesRecursive files
              )
            )
          );
        }
        (values 95 (sharedHomeSettings.${user} or { }))
      ];
    };
in
{
  # Definitions below are reported as coming from the JSON file.
  _file = file;
  imports = ifExists (dir + "/custom.nix") ++ [
    {
      # Reported as coming from modules/managed.json.
      _file = sharedFile;
      config = lib.mkMerge [
        (values 95 sharedTop)
        { environment.systemPackages = map package (shared.packages or [ ]); }
      ];
    }
  ];

  options = {
    nixosManager.managedFile = lib.mkOption {
      type = lib.types.str;
      default = toString file;
      internal = true;
      description = "The managed.json that nixos-manager edits for this machine.";
    };
    nixosManager.version = lib.mkOption {
      type = lib.types.int;
      default = 4;
      internal = true;
      description = "What this module can apply: 2 adds removed packages, package overrides, home-manager and the files next to managed.json; 3 adds modules/managed.json; 4 adds packages from another flake input (from).";
    };
    nixosManager.packageNames = lib.mkOption {
      type = lib.types.attrsOf lib.types.str;
      default = lib.listToAttrs (
        map (name: {
          inherit name;
          value = pname (package name);
        }) (
          lib.unique (
            ownPackages
            ++ (shared.packages or [ ])
            ++ changed
            ++ lib.concatMap (u: u.packages or [ ]) (builtins.attrValues homes ++ builtins.attrValues sharedHomes)
          )
        )
      );
      internal = true;
      description = "The package name of each attribute managed.json names, so nixos-manager can match them.";
    };
  }
  # A second declaration may add the apply function NixOS's own lacks.
  // lib.optionalAttrs (removed != [ ]) {
    environment.systemPackages = lib.mkOption { apply = without removed added; };
  };

  config = lib.mkMerge [
    # A value set by clicking is the machine's final value: it wins over hand-written
    # definitions of the same option (priority 90 beats the default 100), but not
    # over lib.mkForce. Lists are therefore written whole, not merged.
    (values 90 (data.settings or { }))
    { environment.systemPackages = map package ownPackages; }
    # Changed packages are changed for everything on the machine that uses them,
    # home-manager included when it shares the system's pkgs (useGlobalPkgs).
    { nixpkgs.overlays = lib.optional (changed != [ ]) overlay; }
    (lib.optionalAttrs (homes != { }) { home-manager.users = lib.mapAttrs homeModule homes; })
  ];
}
