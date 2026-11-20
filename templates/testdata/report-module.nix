# A machine with the deployment reporter on, and one with it off.
# Run by TestReportModuleWiresComin (needs real Nix and network).
let
  nixpkgs = builtins.getFlake "github:NixOS/nixpkgs/nixos-26.05";
  comin = builtins.getFlake "github:nlewo/comin";
  machine =
    url:
    nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        comin.nixosModules.comin
        ../fleet/modules/report-deployments.nix
        {
          boot.loader.grub.enable = false;
          fileSystems."/" = {
            device = "nodev";
            fsType = "tmpfs";
          };
          system.stateVersion = "26.05";
          networking.hostName = "pi";
          services.comin = {
            enable = true;
            remotes = [
              {
                name = "origin";
                url = "https://forgejo.example.invalid/fleet.git";
                branches.main.name = "main";
              }
            ];
          };
          fleet.deployReport.url = url;
        }
      ];
    };
  on = machine "http://manager.example.invalid:8080";
  off = machine null;
  c = on.config.services.comin;
in
{
  deploy = c.postDeploymentCommand != null;
  build = if on.options.services.comin ? postBuildCommand then c.postBuildCommand == c.postDeploymentCommand else "absent";
  off = off.config.services.comin.postDeploymentCommand == null;
  # Building this builds the script, which runs shellcheck on it.
  script = on.pkgs.runCommand "report-deployment-check" { } "test -x ${c.postDeploymentCommand} && touch $out";
}
