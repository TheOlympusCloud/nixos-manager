# nixmgr:iso - the installer ISO of one machine (host): the NixOS minimal installer
# from the machine's own nixpkgs, with fleet-install, which installs the machine from
# the repository. The commit's tree is on it too, for an install without the network
# to Forgejo. Nothing secret is: no age key, no token, and the SSH host key the
# machine keeps is made when the ISO boots, in memory.
# flakeRef is locked: evaluated in pure mode, like every expression here.
{
  flakeRef,
  host,
  commit,
  installer,
  gitName,
  gitEmail,
}:
let
  fleet = builtins.getFlake flakeRef;
  target =
    (fleet.nixosConfigurations or { }).${host}
      or (throw "${host} is not in nixosConfigurations at commit ${commit}: create it, commit and push, then build the ISO again.");
  cfg = target.config;
  inherit (target.pkgs) lib;
  nixpkgs = target.pkgs.path;
  # Where the machine's comin pulls from: fleet-install fetches the same.
  remote = lib.findFirst (_: true) null (
    lib.optionals (cfg.services.comin.enable or false) (cfg.services.comin.remotes or [ ])
  );
  fleetInstall = target.pkgs.writeShellApplication {
    name = "fleet-install";
    runtimeInputs = [ target.pkgs.git ];
    text = ''
      host=${lib.escapeShellArg host}
      commit=${lib.escapeShellArg commit}
      url=${lib.escapeShellArg (if remote == null then "" else remote.url)}
      branch=${lib.escapeShellArg (if remote == null then "" else remote.branches.main.name)}
      baked=${fleet.outPath}
      git_name=${lib.escapeShellArg gitName}
      git_email=${lib.escapeShellArg gitEmail}
    ''
    + installer;
  };
  iso = import (nixpkgs + "/nixos/lib/eval-config.nix") {
    inherit (target.pkgs.stdenv.hostPlatform) system;
    modules = [
      (nixpkgs + "/nixos/modules/installer/cd-dvd/installation-cd-minimal.nix")
      {
        environment.systemPackages = [ fleetInstall ];
        # The caches the machine uses itself (with nixos-manager's binary cache, what
        # its checks built), so that the install downloads instead of building.
        nix.settings = {
          experimental-features = [
            "nix-command"
            "flakes"
          ];
          substituters = cfg.nix.settings.substituters or [ ];
          trusted-public-keys = cfg.nix.settings.trusted-public-keys or [ ];
        };
        services.getty.helpLine = ''

          This installer is for ${host}. To install it, erasing its disk: sudo fleet-install
        '';
      }
    ];
  };
in
if cfg.boot.isContainer then
  throw "${host} is a container: it is not installed from an ISO."
else
  # Fails here, not halfway through an install, when the machine does not evaluate.
  # fleetInstall alone, and extendModules for an ISO the VM test can drive, are for the tests.
  builtins.seq cfg.system.build.toplevel.drvPath iso.config.system.build.isoImage
  // {
    inherit fleetInstall;
    inherit (iso) extendModules;
  }
