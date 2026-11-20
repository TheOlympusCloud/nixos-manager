# Gives this machine its secrets with sops-nix: each value in secrets/common.json
# and secrets/hosts/<this machine>.json becomes a file, /run/secrets/<name>,
# decrypted on the machine with its SSH host key. nixos-manager writes those files,
# encrypted for its own key and the machines', and .sops.yaml (Settings → Secrets);
# never put a secret in this repository unencrypted.
#
# A machine gets them once a person approved its SSH host key (Settings → Secrets
# → Machine keys): nixos-manager then names its age key in .sops.yaml. It declares
# only the files encrypted for that key, so a secret added for every machine never
# fails a deployment on one that cannot decrypt it, and neither does a key that is
# only in secrets/hosts/<name>.pub, waiting for approval.
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
  # This machine's age key: nixos-manager's line for it in .sops.yaml, "- age1… # <name>".
  sopsConfig = ../.sops.yaml;
  recipients = lib.concatMap (
    line:
    let
      m = builtins.match " *- (age1[0-9a-z]+) # (.*)" line;
    in
    lib.optional (m != null && builtins.elemAt m 1 == host) (builtins.head m)
  ) (lib.optionals (builtins.pathExists sopsConfig) (lib.splitString "\n" (builtins.readFile sopsConfig)));
  declare =
    file:
    let
      doc = lib.importJSON file;
    in
    lib.optionalAttrs
      (builtins.pathExists file && lib.any (a: builtins.elem (a.recipient or "") recipients) (doc.sops.age or [ ]))
      (
        lib.genAttrs (lib.remove "sops" (builtins.attrNames doc)) (_: {
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
    (lib.mkIf (secrets != { }) {
      sops.secrets = secrets;
      # The report token, when nixos-manager stored it (Settings → Secrets → Report tokens).
      fleet.deployReport.tokenFile = lib.mkIf (secrets ? nixos-manager-report) (
        lib.mkDefault config.sops.secrets.nixos-manager-report.path
      );
    })
  ];
}
