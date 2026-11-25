# Tells nixos-manager what comin did on this machine, so its Deployments view
# shows which commit every machine runs and why a deployment failed. A timer
# also reports every 15 minutes that the machine is up and which system it
# runs, so nixos-manager notices a machine that stops, or that runs something
# else than its commit builds. When nixos-manager asks in its reply, a report
# also sends the closure of the system the machine runs (every store path, its
# size and what it references), for its Dependencies views.
#
# On when fleet.deployReport.url is set (common.nix). Each machine also needs
# its own report token from the Deployments view in a file, never in this
# repository: without the file it reports nothing. It never fails a deployment.
{
  config,
  lib,
  options,
  pkgs,
  ...
}:
let
  cfg = config.fleet.deployReport;
  report = pkgs.writeShellApplication {
    name = "report-deployment";
    runtimeInputs = [
      pkgs.coreutils
      pkgs.curl
      pkgs.jq
      pkgs.util-linux # flock
      config.nix.package # reads the closure through this machine's own Nix daemon
    ];
    text = ''
      url=${lib.escapeShellArg cfg.url}
      token_file=${lib.escapeShellArg cfg.tokenFile}
      host=${lib.escapeShellArg config.services.comin.hostname}
      state_dir=${stateDir}
      silent_after=${toString (if cfg.silentAfter == null then 0 else cfg.silentAfter)}
      root=
    ''
    + builtins.readFile ./report-deployment.sh;
  };
  # Holds the default token file and a report that could not be delivered yet.
  stateDir = "/var/lib/nixos-manager-report";
in
{
  options.fleet.deployReport = {
    url = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "http://nixos-manager.home.lan:8080";
      description = "nixos-manager's address as this machine reaches it (LAN or Tailscale); null turns reporting off.";
    };
    tokenFile = lib.mkOption {
      # A string, not a path: a path would copy the token into the world-readable Nix store.
      type = lib.types.str;
      default = "/var/lib/nixos-manager-report/token";
      example = lib.literalExpression "config.sops.secrets.nixos-manager-report.path";
      description = "File holding this machine's report token. Put it there over SSH, or point this at a sops-nix or agenix secret.";
    };
    silentAfter = lib.mkOption {
      type = lib.types.nullOr (lib.types.ints.between 1 8784);
      default = 2;
      example = null;
      description = "Hours without a report after which nixos-manager flags this machine as silent. null for a machine that is often off, like a laptop: it is never flagged.";
    };
  };

  config = lib.mkIf (cfg.url != null) {
    services.comin = {
      postDeploymentCommand = lib.getExe report;
    }
    # Newer comin also runs a command after every build: failed builds, and new
    # commits that leave this machine unchanged (so nothing is deployed), get reported too.
    // lib.optionalAttrs (options.services.comin ? postBuildCommand) {
      postBuildCommand = lib.getExe report;
    };

    systemd.services.nixos-manager-heartbeat = {
      description = "Tell nixos-manager this machine is up and what it runs";
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${lib.getExe report} heartbeat";
        StateDirectory = "nixos-manager-report";
        StateDirectoryMode = "0700";
        ProtectSystem = "strict";
        PrivateTmp = true;
        NoNewPrivileges = true;
      };
    };
    systemd.timers.nixos-manager-heartbeat = {
      wantedBy = [ "timers.target" ];
      timerConfig = {
        OnBootSec = "2min";
        OnUnitActiveSec = "15min";
        RandomizedDelaySec = "1min";
      };
    };
  };
}
