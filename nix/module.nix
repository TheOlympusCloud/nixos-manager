{
  config,
  lib,
  pkgs,
  ...
}:

let
  cfg = config.services.nixos-manager;
  inherit (lib) mkOption types;
in
{
  options.services.nixos-manager = {
    enable = lib.mkEnableOption "nixos-manager, the web console for a NixOS fleet kept in one flake repository";

    package = mkOption {
      type = types.package;
      description = "The nixos-manager package. The flake's `nixosModules.default` sets it to its own build.";
    };

    listen = mkOption {
      type = types.str;
      default = "0.0.0.0:8080";
      description = "Address and port the web UI and the MCP endpoint listen on.";
    };

    openFirewall = mkOption {
      type = types.bool;
      default = false;
      description = "Open the port of `listen` in the firewall.";
    };

    dataDir = mkOption {
      type = types.str;
      default = "/var/lib/nixos-manager";
      description = ''
        State folder: the working copy of the fleet repository, API tokens, and the service user's home,
        where git, SSH and Nix keep their configuration and caches. Must be under /var/lib.
      '';
    };

    author = {
      name = mkOption {
        type = types.str;
        example = "Pablo Saura";
        description = "Name every commit is made with.";
      };
      email = mkOption {
        type = types.str;
        example = "pablo@example.com";
        description = "Email every commit is made with.";
      };
    };

    passwordHashFile = mkOption {
      type = types.str;
      example = "/var/lib/secrets/nixos-manager-password";
      description = ''
        File holding the bcrypt hash of the login password (`nixos-manager hash-password` prints one).
        It is read by systemd and handed to the service as a credential, so it can stay readable by root only.
        A string, not a Nix path, so the hash never lands in the world-readable Nix store.
      '';
    };

    remote = mkOption {
      type = types.nullOr types.str;
      default = null;
      example = "ssh://git@forgejo.lan:2222/pablo/nixos-config.git";
      description = ''
        Git remote of the fleet repository (`NIXMGR_REMOTE`), cloned on first start and pushed to.
        It is set as `origin` at every start, over the one saved in Settings; leave it null to
        connect Forgejo in Settings instead. Without any remote the app starts a new repository
        from a starter flake.
      '';
    };

    environmentFile = mkOption {
      type = types.nullOr types.str;
      default = null;
      example = "/var/lib/secrets/nixos-manager.env";
      description = "Extra `KEY=value` environment for secrets that must not be in the Nix store.";
    };

    extraEnvironment = mkOption {
      type = types.attrsOf types.str;
      default = { };
      example = {
        NIXMGR_NIXD = "nixd --semantic-tokens=false";
      };
      description = "Extra environment variables for the service.";
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = lib.hasPrefix "/var/lib/" cfg.dataDir;
        message = "services.nixos-manager.dataDir must be under /var/lib (it is the service's StateDirectory).";
      }
    ];

    users.users.nixos-manager = {
      isSystemUser = true;
      group = "nixos-manager";
      home = cfg.dataDir;
      description = "nixos-manager";
    };
    users.groups.nixos-manager = { };

    networking.firewall.allowedTCPPorts = lib.mkIf cfg.openFirewall [
      (lib.toInt (lib.last (lib.splitString ":" cfg.listen)))
    ];

    systemd.services.nixos-manager = {
      description = "nixos-manager";
      wantedBy = [ "multi-user.target" ];
      # The first start clones the remote, and every evaluation may fetch flake inputs.
      wants = [ "network-online.target" ];
      after = [
        "network-online.target"
        "nix-daemon.socket"
      ];
      path = [
        config.nix.package
        pkgs.git
        pkgs.openssh
      ];
      environment = {
        NIXMGR_AUTHOR_NAME = cfg.author.name;
        NIXMGR_AUTHOR_EMAIL = cfg.author.email;
        NIXMGR_PASSWORD_HASH_FILE = "%d/password-hash";
        NIXMGR_DATA = cfg.dataDir;
        NIXMGR_LISTEN = cfg.listen;
        # Builds and store writes go through the daemon; the service user only evaluates and fetches.
        NIX_REMOTE = "daemon";
      }
      // lib.optionalAttrs (cfg.remote != null) { NIXMGR_REMOTE = cfg.remote; }
      // cfg.extraEnvironment;

      serviceConfig = {
        ExecStart = lib.getExe cfg.package;
        User = "nixos-manager";
        Group = "nixos-manager";
        StateDirectory = lib.removePrefix "/var/lib/" cfg.dataDir;
        StateDirectoryMode = "0700";
        WorkingDirectory = cfg.dataDir;
        LoadCredential = [ "password-hash:${cfg.passwordHashFile}" ];
        EnvironmentFile = lib.mkIf (cfg.environmentFile != null) cfg.environmentFile;
        Restart = "on-failure";
        RestartSec = 5;
        UMask = "0077";

        # Hardening. Nix needs: the daemon socket (connecting works on a read-only mount), the network
        # (fetching flake inputs, git and SSH to the remote) and a writable home ($HOME/.cache/nix).
        NoNewPrivileges = true;
        PrivateTmp = true;
        PrivateDevices = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectKernelLogs = true;
        ProtectControlGroups = true;
        ProtectClock = true;
        ProtectHostname = true;
        RestrictNamespaces = true;
        RestrictRealtime = true;
        RestrictSUIDSGID = true;
        LockPersonality = true;
        RestrictAddressFamilies = [
          "AF_UNIX"
          "AF_INET"
          "AF_INET6"
          "AF_NETLINK"
        ];
        CapabilityBoundingSet = "";
        SystemCallArchitectures = "native";
      };
    };
  };
}
