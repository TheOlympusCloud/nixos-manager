# Another nixpkgs for TestManagedPackageFromAnotherInput: nixos-26.05, with
# hello and jq marked as coming from here, and with how it was imported.
args:
let
  pkgs = import (builtins.getFlake "github:NixOS/nixpkgs/nixos-26.05") args;
  mark =
    p:
    p.overrideAttrs {
      NIXMGR_INPUT = "other";
      NIXMGR_SYSTEM = args.localSystem.system;
      NIXMGR_UNFREE = pkgs.lib.boolToString (args.config.allowUnfree or false);
    };
in
pkgs
// {
  hello = mark pkgs.hello;
  jq = mark pkgs.jq;
}
