# Applies a standalone home's managed.json, the file nixos-manager edits when you
# change its packages and options by clicking. A standalone home is a
# home-manager configuration of its own (homeConfigurations.<name>), for a
# machine that is not NixOS or not managed here. Hand-written settings stay in
# the other files; this module only reads them.
#
# Use it in the home's modules (homeManagerConfiguration's modules):
#   (import ./modules/managed-home.nix ./homes/<name>/managed.json)
# A name a Nix path cannot hold, such as pablo@laptop, goes in a string:
#   (import ./modules/managed-home.nix (./homes + "/pablo@laptop/managed.json"))
#
# managed.json has packages, removedPackages and settings (home-manager options),
# applied as a machine's managed.json applies them to the system. Next to it,
# each optional:
# - custom.nix: a home-manager module of your own for this home.
# - files/<path>: a file linked at ~/<path> (home.file). It is copied to the Nix
#   store, which every user can read: never a secret.
#
# managed.json's configs.<path in ~> are app settings written into files the
# apps also write, as managed.nix's home.<user>.configs, but without secrets: a
# standalone home gets none of a machine's (modules/managed-configs.py says how).
#
# modules/managed.json, set for every NixOS machine, does not apply here.
file:
{
  lib,
  pkgs,
  config,
  ...
}:
let
  data = if builtins.pathExists file then lib.importJSON file else { };
  dir = dirOf file;
  files = dir + "/files";
  # throw (not abort): a misspelled name fails this home's build with a clear message.
  package = name: lib.attrByPath (lib.splitString "." name) (throw "managed.json: package ${name} is not in nixpkgs") pkgs;
  pname = p: p.pname or (builtins.parseDrvName p.name).name;
  # Priority on each value, never on a whole attribute set, as in managed.nix.
  values = priority: v: if builtins.isAttrs v then lib.mapAttrs (_: values priority) v else lib.mkOverride priority v;
  configs = lib.mapAttrs (
    p: c:
    if (c.secrets or { }) != { } then
      throw "managed.json: configs.${p} has secrets, which a standalone home cannot get; put them in a NixOS machine's home"
    else
      c // { mode = c.mode or "merge"; }
  ) (data.configs or { });
  # As in managed.nix.
  writer =
    let
      python = pkgs.python3.withPackages (ps: [
        ps.tomlkit
        ps.ruamel-yaml
        ps.secretstorage
      ]);
    in
    pkgs.writeShellScript "nixos-manager-configs" ''exec ${python}/bin/python3 ${./managed-configs.py} "$@"'';
  spec = pkgs.writeText "nixos-manager-configs.json" (builtins.toJSON { inherit configs; });
  linkedTargets = map (f: f.target) (builtins.filter (f: f.enable) (builtins.attrValues config.home.file));
  own = data.packages or [ ];
  removed = data.removedPackages or [ ];
  added = map package own;
in
{
  # Definitions below are reported as coming from the JSON file.
  _file = file;
  imports = lib.optional (builtins.pathExists (dir + "/custom.nix")) (dir + "/custom.nix");

  options = {
    nixosManager.managedFile = lib.mkOption {
      type = lib.types.str;
      default = toString file;
      internal = true;
      description = "The managed.json that nixos-manager edits for this home.";
    };
    nixosManager.packageNames = lib.mkOption {
      type = lib.types.attrsOf lib.types.str;
      default = lib.genAttrs own (name: pname (package name));
      internal = true;
      description = "The package name of each attribute managed.json names, so nixos-manager can match them.";
    };
  }
  # Removed packages go by name, whoever adds them, except those added here.
  // lib.optionalAttrs (removed != [ ]) {
    home.packages = lib.mkOption {
      apply = builtins.filter (p: !(builtins.elem (pname p) removed) || builtins.elem p added);
    };
  };

  config = lib.mkMerge [
    # A value set by clicking wins over hand-written definitions (priority 90 beats
    # the default 100), but not over lib.mkForce. Lists are written whole.
    (values 90 (data.settings or { }))
    {
      home.packages = added;
      home.file = lib.optionalAttrs (builtins.pathExists files) (
        builtins.listToAttrs (
          map (f: lib.nameValuePair (lib.removePrefix "${toString files}/" (toString f)) { source = f; }) (
            lib.filesystem.listFilesRecursive files
          )
        )
      );
    }
    (lib.mkIf (configs != { }) {
      assertions = [
        {
          assertion = lib.intersectLists (builtins.attrNames configs) linkedTargets == [ ];
          message = "managed.json: configs write ${lib.concatStringsSep ", " (lib.intersectLists (builtins.attrNames configs) linkedTargets)}, which home-manager also links (home.file, xdg.configFile or a module): keep one of the two";
        }
      ];
      home.activation.nixosManagerConfigs = lib.hm.dag.entryAfter [ "writeBoundary" ] "run ${writer} configs ${spec}";
      systemd.user.services.nixos-manager-configs = {
        Unit = {
          Description = "App settings set in managed.json";
          Before = [ "graphical-session-pre.target" ];
        };
        Service = {
          Type = "oneshot";
          ExecStart = "${writer} configs ${spec}";
        };
        Install.WantedBy = [ "default.target" ];
      };
    })
  ];
}
