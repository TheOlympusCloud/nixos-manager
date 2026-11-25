# nixmgr:values - the effective value of some options on one machine (host), or
# on a standalone home-manager configuration (host "home:<name>"), for the
# option paths in pathsJSON (a JSON list of lists), keyed by their JSON.
{ flakeRef, host, pathsJSON }:
let
  flake = builtins.getFlake flakeRef;
  isHome = builtins.substring 0 5 host == "home:";
  machine =
    if isHome then
      flake.homeConfigurations.${builtins.substring 5 (builtins.stringLength host) host}
    else
      flake.nixosConfigurations.${host};
  inherit (machine) config options;
  lib = machine.pkgs.lib;
  paths = builtins.fromJSON pathsJSON;

  root = "${toString flake.outPath}/";
  try = v: default: let r = builtins.tryEval v; in if r.success then r.value else default;
  rel =
    f:
    let
      s0 = toString f;
      s = if lib.hasPrefix "/" s0 && builtins.pathExists "${s0}/default.nix" then "${s0}/default.nix" else s0;
    in
    if lib.hasPrefix root s then lib.removePrefix root s else "nixos";
  # JSON-safe and bounded: packages by name, functions as a marker, nesting cut at four
  # levels and big sets or lists (systemd.services, nixpkgs.pkgs) shown by their size.
  show =
    depth: v:
    let
      t = builtins.typeOf v;
    in
    if lib.isDerivation v then
      "«${v.name or "derivation"}»"
    else if t == "lambda" then
      "«function»"
    else if t == "path" then
      toString v
    else if depth <= 0 && (t == "set" || t == "list") then
      "«…»"
    else if t == "set" && builtins.length (builtins.attrNames v) > 200 then
      "«${toString (builtins.length (builtins.attrNames v))} attributes»"
    else if t == "list" && builtins.length v > 200 then
      "«${toString (builtins.length v)} items»"
    else if t == "set" then
      lib.mapAttrs (_: show (depth - 1)) v
    else if t == "list" then
      map (show (depth - 1)) v
    else
      v;
  # The option at path, through submodules' own options: users.users.pablo.shell,
  # and home-manager.users.pablo.olympus.x, which only pablo's own modules declare.
  optionAt =
    node: path:
    let
      h = builtins.head path;
      meta = node.valueMeta or { };
    in
    if path == [ ] then
      node
    else if lib.isOption node then
      if meta ? attrs then
        if meta.attrs ? ${h} && meta.attrs.${h} ? configuration then optionAt meta.attrs.${h}.configuration.options (builtins.tail path) else null
      else if meta ? configuration then
        optionAt meta.configuration.options path
      else
        null
    else if builtins.isAttrs node && node ? ${h} then
      optionAt node.${h} (builtins.tail path)
    else
      null;
  one =
    path:
    let
      opt = try (optionAt options path) null;
      found = opt != null && builtins.isAttrs opt && lib.isOption opt;
      forced =
        let
          r = builtins.tryEval (
            let
              v = show 4 (lib.attrByPath path null config);
            in
            builtins.deepSeq v v
          );
        in
        if r.success then r.value else "«evaluation error»";
    in
    if !found then
      { exists = false; }
    else
      {
        exists = true;
        value = forced;
        files = lib.unique (map (d: rel d.file) (try opt.definitionsWithLocations [ ]));
        type = try (opt.type.description or "") "";
      };
in
builtins.listToAttrs (
  map (p: {
    name = builtins.toJSON p;
    value = one p;
  }) paths
)
