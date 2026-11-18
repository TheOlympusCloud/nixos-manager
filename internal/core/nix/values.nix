# nixmgr:values - the effective value of some options on one machine (NIXMGR_HOST),
# for the option paths in NIXMGR_PATHS (a JSON list of lists), keyed by their JSON.
let
  flake = builtins.getFlake (builtins.getEnv "NIXMGR_FLAKE");
  machine = flake.nixosConfigurations.${builtins.getEnv "NIXMGR_HOST"};
  inherit (machine) config options;
  lib = machine.pkgs.lib;
  paths = builtins.fromJSON (builtins.getEnv "NIXMGR_PATHS");

  root = "${toString flake.outPath}/";
  try = v: default: let r = builtins.tryEval v; in if r.success then r.value else default;
  rel =
    f:
    let
      s0 = toString f;
      s = if lib.hasPrefix "/" s0 && builtins.pathExists "${s0}/default.nix" then "${s0}/default.nix" else s0;
    in
    if lib.hasPrefix root s then lib.removePrefix root s else "nixos";
  # JSON-safe and bounded: packages by name, functions as a marker, nesting cut at four levels.
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
    else if t == "set" then
      lib.mapAttrs (_: show (depth - 1)) v
    else if t == "list" then
      map (show (depth - 1)) v
    else
      v;
  one =
    path:
    let
      opt = lib.attrByPath path null options;
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
