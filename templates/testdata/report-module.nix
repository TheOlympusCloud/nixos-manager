# A machine with the deployment reporter on, a laptop that is never flagged as
# silent, and a machine with reporting off.
# Run by TestReportModuleWiresComin (needs real Nix and network).
let
  nixpkgs = builtins.getFlake "github:NixOS/nixpkgs/nixos-26.05";
  comin = builtins.getFlake "github:nlewo/comin";
  inherit (nixpkgs) lib;
  machine =
    url: extra:
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
        extra
      ];
    };
  on = machine "http://manager.example.invalid:8080" { };
  laptop = machine "http://manager.example.invalid:8080" { fleet.deployReport.silentAfter = null; };
  off = machine null { };
  c = on.config.services.comin;
  beat = on.config.systemd.services.nixos-manager-heartbeat;
in
{
  deploy = c.postDeploymentCommand != null;
  build = if on.options.services.comin ? postBuildCommand then c.postBuildCommand == c.postDeploymentCommand else "absent";
  off = off.config.services.comin.postDeploymentCommand == null && !(off.config.systemd.services ? nixos-manager-heartbeat);
  heartbeat = beat.serviceConfig.ExecStart == "${c.postDeploymentCommand} heartbeat"
    && on.config.systemd.timers.nixos-manager-heartbeat.timerConfig.OnUnitActiveSec == "15min"
    && builtins.elem "timers.target" on.config.systemd.timers.nixos-manager-heartbeat.wantedBy;
  # How many hours each machine may stay silent, as the script sends it.
  silentAfter = [
    (lib.hasInfix "silent_after=2\n" (builtins.readFile c.postDeploymentCommand))
    (lib.hasInfix "silent_after=0\n" (builtins.readFile laptop.config.services.comin.postDeploymentCommand))
  ];
  # Building this builds the script, which runs shellcheck on it.
  script = on.pkgs.runCommand "report-deployment-check" { } "test -x ${c.postDeploymentCommand} && touch $out";
}
