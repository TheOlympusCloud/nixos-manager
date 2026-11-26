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
#   It is copied to the Nix store, which every user can read: never a secret.
# - home/<user>/templates/<path>: a file with secrets, linked at ~/<path> for that
#   user. Each @secret:<name>@ in it is that secret of the machine, which
#   sops-nix fills in on the machine (sops.templates); the rest is plain text.
#
# managed.json's home.<user>.secrets links a secret of the machine as a whole
# file in ~ ({ ".netrc": "netrc" }): the secret becomes the user's, and
# ~/.netrc a link to it in /run/secrets. Neither way puts a secret in the store.
# A secret comes from modules/secrets.nix (Settings → Secrets) or Nix by hand.
#
# For apps that rewrite their own settings, where a link would break them:
# - home.<user>.configs.<path in ~> = { format, mode, settings, secrets, process,
#   lock }: modules/managed-configs.py writes the declared keys into that file at
#   each home-manager activation and login (mode "merge", the app's other keys
#   stay), or the whole file when there is none (mode "seed"). format is json,
#   toml, yaml, ini or properties. secrets has the shape of settings, with a
#   secret's name where its value goes: it is read on the machine, as the user,
#   from /run/secrets. process and lock keep it from writing while the app runs.
#   managed-configs.py says how lists merge.
# - home.<user>.keyring = [ { label, attributes, secret } ]: stored in the
#   user's keyring (Secret Service: GNOME Keyring) after the graphical session
#   starts, for apps that keep their password there.
# Each secret either uses becomes the user's, as a linked one does. What else
# the apps keep (logins in their own databases, rotating tokens) stays theirs.
#
# modules/managed.json, next to this module, has the same shape and is set for
# every machine that loads it; where both set something, the machine's own wins:
# - packages and removedPackages are joined. A name the machine removes takes a
#   shared package out of it; a shared removal never takes out what either file adds.
# - packageOverrides: the machine's entry for an attribute replaces the shared one.
#   A shared patch file is relative to modules/.
# - home.<user> applies where the machine's managed.json has home.<user> too (so
#   no machine gets a home for a user it lacks), joined as the system's packages;
#   in secrets and configs, the machine's entry at a path wins; in keyring, its
#   entry with the same attributes.
# - settings apply at priority 95, below the machine's (90) but still above
#   hand-written values; settings.home-manager.users.<user> applies as home does.
# The files next to it (custom.nix, packages/, home/) are not read.
#
# Fleet modules bundle packages and settings that machines turn on one by one:
# modules/fleet/<name>/managed.json, next to this module, is a NixOS module of
# its own, with an option fleet.modules.<name>.enable and the options it declares:
#   { "description": "Jellyfin with its firewall",
#     "options": { "port": { "type": "port", "default": 8096, "description": "…" } },
#     "packages": [ "ffmpeg" ],
#     "settings": { "services": { "jellyfin": { "enable": true } },
#                   "networking": { "firewall": { "allowedTCPPorts": [ "@option:port@" ] } } } }
# A type is bool, int, port, str, "listOf str" or anything; an option without a
# default must be set on each machine that turns the module on. "@option:<name>@"
# as a whole value is that option's value on the machine, inside a longer text
# its text. Where the module is on, its packages are installed and its settings
# apply as hand-written Nix does (lists and sets merge with the machine's), so a
# value set on the machine wins. A machine turns it on, and sets its options, as
# any option: fleet.modules.<name>.enable in its managed.json's settings, in
# modules/managed.json for every machine, or in Nix. custom.nix next to the
# module's managed.json is a NixOS module loaded with it, on every machine: put
# its config under lib.mkIf config.fleet.modules.<name>.enable.
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
  options,
  ...
}@args:
let
  read = f: if builtins.pathExists f then lib.importJSON f else { };
  data = read file;
  sharedFile = ./managed.json;
  shared = read sharedFile;
  dir = dirOf file;
  ifExists = p: lib.optional (builtins.pathExists p) p;
  # Each file under folder d, by its path relative to d; none without d.
  filesUnder =
    d:
    lib.optionalAttrs (builtins.pathExists d) (
      builtins.listToAttrs (
        map (f: lib.nameValuePair (lib.removePrefix "${toString d}/" (toString f)) f) (lib.filesystem.listFilesRecursive d)
      )
    );
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

  # Fleet modules: each folder in modules/fleet with a managed.json.
  fleetDir = ./fleet;
  fleet = lib.optionalAttrs (builtins.pathExists fleetDir) (
    lib.mapAttrs (n: _: lib.importJSON (fleetDir + "/${n}/managed.json")) (
      lib.filterAttrs (n: t: t == "directory" && builtins.pathExists (fleetDir + "/${n}/managed.json")) (builtins.readDir fleetDir)
    )
  );
  optionTypes = with lib.types; {
    inherit
      bool
      int
      port
      str
      anything
      ;
    "listOf str" = listOf str;
  };
  # The packages of the modules the machine turns on, which no removed name takes out either.
  fleetPackages = lib.concatLists (
    lib.mapAttrsToList (n: m: lib.optionals config.fleet.modules.${n}.enable (map package (m.packages or [ ]))) fleet
  );
  fleetModule =
    name: m:
    let
      mdir = fleetDir + "/${name}";
      what = "modules/fleet/${name}/managed.json";
      cfg = config.fleet.modules.${name};
      optionValue =
        o: if (m.options or { }) ? ${o} then cfg.${o} else throw "${what}: @option:${o}@ is not one of its options";
      asText = v: if builtins.isString v then v else builtins.toJSON v;
      # "@option:<o>@" as a whole value is that option's value; inside a longer text, its text.
      fill =
        v:
        if builtins.isAttrs v then
          lib.mapAttrs (_: fill) v
        else if builtins.isList v then
          map fill v
        else if builtins.isString v then
          let
            parts = builtins.split "@option:([A-Za-z_][A-Za-z0-9_-]*)@" v;
          in
          if builtins.length parts == 3 && builtins.head parts == "" && lib.last parts == "" then
            optionValue (builtins.head (builtins.elemAt parts 1))
          else
            lib.concatMapStrings (x: if builtins.isList x then asText (optionValue (builtins.head x)) else x) parts
        else
          v;
    in
    {
      # Its definitions are reported as coming from its managed.json.
      _file = mdir + "/managed.json";
      imports = ifExists (mdir + "/custom.nix");
      options.fleet.modules.${name} =
        lib.mapAttrs (
          o: d:
          lib.mkOption (
            {
              type = optionTypes.${d.type or "anything"} or (throw "${what}: the option ${o} has the unknown type ${d.type}");
              description = d.description or "";
            }
            // lib.optionalAttrs (d ? default) { inherit (d) default; }
          )
        ) (m.options or { })
        // {
          enable = lib.mkEnableOption (m.description or "the fleet module ${name}");
        };
      config = lib.mkIf cfg.enable (
        lib.mkMerge [
          (fill (m.settings or { }))
          { environment.systemPackages = map package (m.packages or [ ]); }
        ]
      );
    };
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
  # Secrets in homes, by user: home.<user>.secrets (path in ~ -> secret) and the
  # files in home/<user>/templates/ (path in ~ -> file).
  links = lib.mapAttrs (user: u: (sharedHomes.${user}.secrets or { }) // (u.secrets or { })) homes;
  templates = lib.mapAttrs (user: _: filesUnder (dir + "/home/${user}/templates")) homes;
  # A user's configs and keyring entries: the machine's own, then the shared ones it lacks.
  configsOf = user: u: (sharedHomes.${user}.configs or { }) // (u.configs or { });
  keyringOf =
    user: u:
    let
      own = u.keyring or [ ];
    in
    builtins.filter (e: !lib.any (o: o.attributes == e.attributes) own) (sharedHomes.${user}.keyring or [ ]) ++ own;
  leaves = t: lib.concatMap (v: if builtins.isAttrs v then leaves v else [ v ]) (builtins.attrValues t);
  # Every secret a user's home uses: linked whole, in a config or in the keyring.
  used = lib.mapAttrs (
    user: u:
    lib.unique (
      builtins.attrValues links.${user}
      ++ lib.concatMap (c: leaves (c.secrets or { })) (builtins.attrValues (configsOf user u))
      ++ map (e: e.secret) (keyringOf user u)
    )
  ) homes;
  # Writes configs and keyring entries on the machine (see managed-configs.py).
  writer =
    let
      python = pkgs.python3.withPackages (ps: [
        ps.tomlkit
        ps.ruamel-yaml
        ps.secretstorage
      ]);
    in
    pkgs.writeShellScript "nixos-manager-configs" ''exec ${python}/bin/python3 ${./managed-configs.py} "$@"'';
  # Rendered at /run/secrets/rendered/<name>.
  # shortcut: names can collide, so the check fails with a conflict: two paths of one
  # user that differ only in characters a store name cannot hold (/ included), or
  # user a's b-c and user a-b's c; add a hash of user and path if one ever does.
  templateName = user: p: lib.strings.sanitizeDerivationName "home-${user}-${p}";
  # @secret:<name>@ becomes that secret's placeholder, which sops-nix replaces on the machine.
  render =
    what: f:
    lib.concatMapStrings (
      x:
      if builtins.isList x then
        config.sops.placeholder.${builtins.head x}
          or (throw "${what}: @secret:${builtins.head x}@ is not one of this machine's secrets; add it in Settings → Secrets")
      else
        x
    ) (builtins.split "@secret:([A-Za-z0-9][A-Za-z0-9._-]*)@" (builtins.readFile f));
  # Whether a module other than this one declares the secret (modules/secrets.nix, or
  # Nix by hand). A linked secret gets its owner only then, so a missing one fails with
  # the message in homeModule instead of sops-nix's about its default file.
  # shortcut: a secret declared in this managed.json's own settings (sops.secrets)
  # shares this module's file, so it does not count; declare it in Nix instead.
  declared =
    name:
    lib.any (d: toString d.file != toString file && d.value ? ${name}) options.sops.secrets.definitionsWithLocations;
  sharedTop =
    builtins.removeAttrs sharedSettings [ "home-manager" ]
    // lib.optionalAttrs (hmRest != { }) { home-manager = hmRest; };
  homeModule =
    user: u: hm:
    let
      s = sharedHomes.${user} or { };
      gone = lib.unique ((s.removedPackages or [ ]) ++ (u.removedPackages or [ ]));
      kept = map package ((u.packages or [ ]) ++ sharedFor (u.removedPackages or [ ]) (s.packages or [ ]));
      link = hm.config.lib.file.mkOutOfStoreSymlink;
      secretPath =
        p: name:
        config.sops.secrets.${name}.path
          or (throw "managed.json links ~/${p} of ${user} to the secret ${name}, which this machine does not have; add it in Settings → Secrets, or unlink it");
      configs = configsOf user u;
      keyring = keyringOf user u;
      # Where the machine has the secret: the writer reads it there, so no value reaches the store.
      secretFile =
        what: name:
        config.sops.secrets.${name}.path
          or (throw "managed.json: ${what} of ${user} uses the secret ${name}, which this machine does not have; add it in Settings → Secrets");
      mapLeaves = f: lib.mapAttrs (_: v: if builtins.isAttrs v then mapLeaves f v else f v);
      configsSpec = pkgs.writeText "nixos-manager-configs.json" (
        builtins.toJSON {
          configs = lib.mapAttrs (
            p: c:
            builtins.removeAttrs c [ "secrets" ]
            // {
              mode = c.mode or "merge";
              secrets = mapLeaves (secretFile "~/${p}") (c.secrets or { });
            }
          ) configs;
        }
      );
      keyringSpec = pkgs.writeText "nixos-manager-keyring.json" (
        builtins.toJSON {
          entries = map (e: {
            inherit (e) label attributes;
            secret = secretFile "the keyring entry ${e.label}" e.secret;
          }) keyring;
        }
      );
      linkedTargets = map (f: f.target) (builtins.filter (f: f.enable) (builtins.attrValues hm.config.home.file));
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
          home.file = lib.mapAttrs (_: f: { source = f; }) (filesUnder (dir + "/home/${user}/files"));
        }
        # Separate definitions, so a path in ~ set twice fails as a conflict.
        { home.file = lib.mapAttrs (p: name: { source = link (secretPath p name); }) links.${user}; }
        { home.file = lib.mapAttrs (p: _: { source = link config.sops.templates.${templateName user p}.path; }) templates.${user}; }
        (values 95 (sharedHomeSettings.${user} or { }))
        (lib.mkIf (configs != { }) {
          assertions = [
            {
              assertion = lib.intersectLists (builtins.attrNames configs) linkedTargets == [ ];
              message = "managed.json: ${user}'s configs write ${lib.concatStringsSep ", " (lib.intersectLists (builtins.attrNames configs) linkedTargets)}, which home-manager also links (home.file, xdg.configFile or a module): keep one of the two";
            }
          ];
          # After the home's own files, and never holding them back: a file that cannot be written
          # is logged, and the login unit below fails, which systemctl --user --failed shows.
          home.activation.nixosManagerConfigs = hm.lib.hm.dag.entryAfter [ "linkGeneration" ] "run ${writer} configs ${configsSpec} || warnEcho 'nixos-manager: some app settings were not written (see above)'";
          # Again at login, before the apps start: a merge an app's process or lock held back.
          systemd.user.services.nixos-manager-configs = {
            Unit = {
              Description = "App settings set in managed.json";
              Before = [ "graphical-session-pre.target" ];
            };
            Service = {
              Type = "oneshot";
              ExecStart = "${writer} configs ${configsSpec}";
            };
            Install.WantedBy = [ "default.target" ];
          };
        })
        (lib.mkIf (keyring != [ ]) {
          # Once the session runs, the login keyring is unlocked.
          systemd.user.services.nixos-manager-keyring = {
            Unit = {
              Description = "Keyring entries set in managed.json";
              After = [ "graphical-session.target" ];
              PartOf = [ "graphical-session.target" ];
            };
            Service = {
              Type = "oneshot";
              ExecStart = "${writer} keyring ${keyringSpec}";
              TimeoutStartSec = "2min";
            };
            Install.WantedBy = [ "graphical-session.target" ];
          };
        })
      ];
    };
in
{
  # Definitions below are reported as coming from the JSON file.
  _file = file;
  imports = ifExists (dir + "/custom.nix") ++ lib.mapAttrsToList fleetModule fleet ++ [
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
      default = 7;
      internal = true;
      description = "What this module can apply: 2 adds removed packages, package overrides, home-manager and the files next to managed.json; 3 adds modules/managed.json; 4 adds packages from another flake input (from); 5 adds secrets in homes (home.<user>.secrets and home/<user>/templates/); 6 adds app configs and keyring entries in homes (home.<user>.configs and keyring); 7 adds fleet modules (modules/fleet/<name>/managed.json).";
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
    environment.systemPackages = lib.mkOption { apply = without removed (added ++ fleetPackages); };
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
    # A secret a home uses is its user's; two users cannot both own one.
    (lib.optionalAttrs (lib.any (l: l != [ ]) (builtins.attrValues used)) {
      sops.secrets = lib.mkMerge (
        lib.mapAttrsToList (user: names: lib.genAttrs names (name: lib.mkIf (declared name) { owner = user; })) used
      );
    })
    (lib.optionalAttrs (lib.any (t: t != { }) (builtins.attrValues templates)) {
      sops.templates = lib.mkMerge (
        lib.mapAttrsToList (
          user:
          lib.mapAttrs' (
            p: f:
            lib.nameValuePair (templateName user p) {
              content = render "home/${user}/templates/${p}" f;
              owner = user;
            }
          )
        ) templates
      );
    })
  ];
}
