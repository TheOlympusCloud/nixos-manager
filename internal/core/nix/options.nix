# nixmgr:options - the documented options of one NixOS machine (host), or of a
# standalone home-manager configuration (host "home:<name>"), for search and forms.
{ flakeRef, host }:
let
  flake = builtins.getFlake flakeRef;
  isHome = builtins.substring 0 5 host == "home:";
  machine =
    if isHome then
      flake.homeConfigurations.${builtins.substring 5 (builtins.stringLength host) host}
    else
      flake.nixosConfigurations.${host};
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
