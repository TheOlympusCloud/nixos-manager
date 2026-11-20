{
  config,
  lib,
  pkgs,
  ...
}:

let
  cfg = config.services.nixos-manager;
  inherit (lib) mkOption types;
  cache = cfg.binaryCache;
  cacheDir = "/var/lib/nixos-manager-cache";
  # The cache's signing key, made once: never in the Nix store or a repository.
  cacheKey = pkgs.writeShellApplication {
    name = "nixos-manager-cache-key";
    runtimeInputs = [
      config.nix.package
      pkgs.coreutils
    ];
    text = ''
      umask 077
      cd "$STATE_DIRECTORY"
      if [ ! -s secret-key ]; then
        nix --extra-experimental-features nix-command key generate-secret --key-name nixos-manager-1 > secret-key.new
        mv secret-key.new secret-key
        rm -f public-key
      fi
      if [ ! -s public-key ]; then
        nix --extra-experimental-features nix-command key convert-secret-to-public < secret-key > public-key.new
        chmod 644 public-key.new
        mv public-key.new public-key
      fi
    '';
  };
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

    binaryCache = {
      enable = lib.mkEnableOption ''
        a binary cache of this server's Nix store, served by harmonia, so the machines download the
        systems the checks built instead of building them. It signs with a key made on the first start in
        `${cacheDir}`: `secret-key` stays there, readable by root only, and `public-key` is what the
        machines trust'';
      openFirewall = mkOption {
        type = types.bool;
        default = false;
        description = "Open the cache's port (harmonia's `bind`, 5000 by default) in the firewall.";
      };
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

    networking.firewall.allowedTCPPorts =
      lib.optional cfg.openFirewall (lib.toInt (lib.last (lib.splitString ":" cfg.listen)))
      ++ lib.optional (cache.enable && cache.openFirewall) (
        lib.toInt (lib.last (lib.splitString ":" config.services.harmonia.cache.settings.bind))
      );

    services.harmonia.cache = lib.mkIf cache.enable {
      enable = true;
      signKeyPaths = [ "${cacheDir}/secret-key" ];
    };
    systemd.services.harmonia = lib.mkIf cache.enable {
      requires = [ "nixos-manager-cache-key.service" ];
      after = [ "nixos-manager-cache-key.service" ];
    };
    systemd.services.nixos-manager-cache-key = lib.mkIf cache.enable {
      description = "nixos-manager binary cache signing key";
      wantedBy = [ "multi-user.target" ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        ExecStart = lib.getExe cacheKey;
        StateDirectory = lib.removePrefix "/var/lib/" cacheDir;
        StateDirectoryMode = "0755"; # for the public key; the script keeps the secret one 0600
      };
    };

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
        # Builds (a check builds the machines a change touches) and store writes go through the daemon.
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
