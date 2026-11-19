# Tells nixos-manager what comin did on this machine, so its Deployments view
# shows which commit every machine runs and why a deployment failed.
#
# On when fleet.deployReport.url is set (common.nix). Each machine also needs
# the fleet report token from the Deployments view in a file, never in this
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
    ];
    text = ''
      url=${lib.escapeShellArg cfg.url}
      token_file=${lib.escapeShellArg cfg.tokenFile}
      host=${lib.escapeShellArg config.services.comin.hostname}
    ''
    + builtins.readFile ./report-deployment.sh;
  };
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
      description = "File holding the fleet report token. Put it there over SSH, or point this at a sops-nix or agenix secret.";
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
  };
}
