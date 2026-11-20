# Gives this machine its secrets with sops-nix: each value in secrets/common.json
# and secrets/hosts/<this machine>.json becomes a file, /run/secrets/<name>,
# decrypted on the machine with its SSH host key. nixos-manager writes those files,
# encrypted for its own key and the machines', and .sops.yaml (Settings → Secrets);
# never put a secret in this repository unencrypted.
#
# A machine gets them once its SSH host key is in secrets/hosts/<name>.pub
# (Settings → Secrets → Machine keys). Until then it declares none, so a secret
# added for every machine never fails a deployment on one that cannot decrypt it.
# Owner, mode or neededForUsers of a secret are set as usual, next to it:
#   sops.secrets.wifi.owner = "wpa_supplicant";
{
  config,
  lib,
  inputs,
  ...
}:
let
  host = config.networking.hostName;
  hostKey = ../secrets/hosts + "/${host}.pub";
  declare =
    file:
    lib.optionalAttrs (builtins.pathExists file) (
      lib.genAttrs (lib.remove "sops" (builtins.attrNames (lib.importJSON file))) (_: {
        sopsFile = lib.mkDefault file;
        format = lib.mkDefault "json";
      })
    );
  # A machine's own value wins over the one every machine gets.
  secrets = declare ../secrets/common.json // declare (../secrets/hosts + "/${host}.json");
in
{
  imports = [ inputs.sops-nix.nixosModules.sops ];

  config = lib.mkMerge [
    {
      # The key sops-nix decrypts with, made even without an SSH server: its public
      # half, /etc/ssh/ssh_host_ed25519_key.pub, is what nixos-manager asks for.
      services.openssh.generateHostKeys = lib.mkDefault true;
    }
    (lib.mkIf (builtins.pathExists hostKey) {
      sops.secrets = secrets;
      # The report token, when nixos-manager stored it (Settings → Secrets → Report tokens).
      fleet.deployReport.tokenFile = lib.mkIf (secrets ? nixos-manager-report) (
        lib.mkDefault config.sops.secrets.nixos-manager-report.path
      );
    })
  ];
}
