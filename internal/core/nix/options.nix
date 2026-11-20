# nixmgr:options - the documented options of one NixOS machine (host), for search and forms.
{ flakeRef, host }:
let
  flake = builtins.getFlake flakeRef;
  machine = flake.nixosConfigurations.${host};
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
