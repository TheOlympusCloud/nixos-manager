# nixmgr:options - the documented options of one NixOS machine (NIXMGR_HOST), for search and forms.
let
  flake = builtins.getFlake (builtins.getEnv "NIXMGR_FLAKE");
  machine = flake.nixosConfigurations.${builtins.getEnv "NIXMGR_HOST"};
  lib = machine.pkgs.lib;
  try = v: default: let r = builtins.tryEval v; in if r.success then r.value else default;
  text = v: if v == null then null else if builtins.isString v then v else if builtins.isAttrs v then v.text or null else null;
in
map (o: {
  inherit (o) name;
  type = o.type or "";
  description = try (text (o.description or null)) null;
  default = try (text (o.default or null)) null;
  example = try (text (o.example or null)) null;
  readOnly = o.readOnly or false;
}) (builtins.filter (o: (o.visible or true) != false && !(o.internal or false)) (lib.optionAttrSetToDocList machine.options))
