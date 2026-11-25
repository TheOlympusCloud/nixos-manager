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
  docs = lib.optionAttrSetToDocList machine.options;
  # home-manager's NixOS module leaves its options out of these docs (visible =
  # "shallow"), and the type's would lack what a user's own modules declare
  # (olympus.supersonic.enable): each user's home options are listed from their
  # evaluated home, by name, home-manager.users.pablo.…, less any the docs have.
  known = builtins.listToAttrs (map (o: lib.nameValuePair o.name true) docs);
  ownHomeOptions = lib.concatLists (
    lib.mapAttrsToList (
      user: meta:
      let
        prefix = "${lib.showOption [ "home-manager" "users" user ]}.";
      in
      builtins.filter (o: !(known ? "home-manager.users.<name>.${lib.removePrefix prefix o.name}")) (
        try (lib.optionAttrSetToDocList meta.configuration.options) [ ]
      )
    ) (if isHome then { } else machine.options.home-manager.users.valueMeta.attrs or { })
  );
in
map (o: {
  inherit (o) name;
  type = o.type or "";
  description = try (text (o.description or null)) null;
  default = try (text (o.default or null)) null;
  example = try (text (o.example or null)) null;
  readOnly = o.readOnly or false;
}) (builtins.filter (o: (o.visible or true) != false && !(o.internal or false)) (docs ++ ownHomeOptions))
