# A standalone home-manager configuration whose modules load
# modules/managed-home.nix with homes/pablo@laptop/managed.json, a name a Nix
# path cannot hold (needs real Nix and network): what the file adds, takes out
# and sets over hand-written values, its app config, and custom.nix and files/
# next to it.
let
  nixpkgs = builtins.getFlake "github:NixOS/nixpkgs/nixos-26.05";
  home-manager = builtins.getFlake "github:nix-community/home-manager/release-26.05";
  pkgs = nixpkgs.legacyPackages.x86_64-linux;
  inherit (nixpkgs) lib;
  home = home-manager.lib.homeManagerConfiguration {
    inherit pkgs;
    modules = [
      (import ../fleet/modules/managed-home.nix (./homes + "/pablo@laptop/managed.json"))
      {
        home.username = "pablo";
        home.homeDirectory = "/home/pablo";
        home.stateVersion = "26.05";
        home.packages = [
          pkgs.hello
          pkgs.cowsay
        ];
        home.sessionVariables.NIXMGR = "hand";
      }
    ];
  };
  c = home.config;
  has = n: builtins.any (p: (p.pname or "") == n) c.home.packages;
in
{
  packages = builtins.filter has [
    "cowsay"
    "hello"
    "jq"
  ];
  sessionVariable = c.home.sessionVariables.NIXMGR;
  custom = c.home.sessionVariables.NIXMGR_CUSTOM or null;
  file = builtins.readFile c.home.file.".config/nixmgr/hello.txt".source;
  managedFile = lib.hasSuffix "/homes/pablo@laptop/managed.json" c.nixosManager.managedFile;
  packageNames = c.nixosManager.packageNames;
  # The app config, written at activation and at login with the same spec.
  configs =
    let
      run = c.home.activation.nixosManagerConfigs.data;
    in
    lib.hasInfix " configs /nix/store/" run
    && lib.toList c.systemd.user.services.nixos-manager-configs.Service.ExecStart == [ (lib.removePrefix "run " run) ];
  # The whole home evaluates.
  activation = lib.hasSuffix ".drv" home.activationPackage.drvPath;
}
