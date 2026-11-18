# Applies a machine's managed.json, the file nixos-manager edits when you
# change packages and options by clicking. Hand-written settings stay in the
# other files; this module only reads the JSON.
#
# Use it per machine: (import ./modules/managed.nix ./hosts/<name>/managed.json)
file:
{ lib, pkgs, ... }:
let
  data = if builtins.pathExists file then lib.importJSON file else { };
  package = name: lib.getAttrFromPath (lib.splitString "." name) pkgs;
in
{
  # Definitions below are reported as coming from the JSON file.
  _file = file;

  options.nixosManager.managedFile = lib.mkOption {
    type = lib.types.str;
    default = toString file;
    internal = true;
    description = "The managed.json that nixos-manager edits for this machine.";
  };

  config = lib.mkMerge [
    (data.settings or { })
    { environment.systemPackages = map package (data.packages or [ ]); }
  ];
}
