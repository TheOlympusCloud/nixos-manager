# `nix flake check` runs this: it evaluates services.nixos-manager in the Proxmox LXC configuration the
# README gives and asserts the generated service, then the same with the binary cache on, whose key
# script it runs, and with checks that do not build. The app itself is not built.
{
  nixpkgs,
  pkgs,
  module,
}:

let
  inherit (nixpkgs) lib;
  machine =
    extra:
    lib.nixosSystem {
      modules = [
        module
        # README.md's /etc/nixos/configuration.nix; keep the two alike.
        (
          { config, modulesPath, ... }:
          {
            imports = [ (modulesPath + "/virtualisation/proxmox-lxc.nix") ];

            nixpkgs.hostPlatform = pkgs.stdenv.hostPlatform.system;
            nix.settings.experimental-features = [
              "nix-command"
              "flakes"
            ];
            nix.gc = {
              automatic = true;
              dates = "weekly";
              options = "--delete-older-than 14d";
            };

            services.nixos-manager = {
              enable = true;
              openFirewall = true;
              author = {
                name = "Your Name";
                email = "you@example.com";
              };
              passwordHashFile = "/var/lib/secrets/nixos-manager-password";
            };
            environment.systemPackages = [ config.services.nixos-manager.package ];

            system.stateVersion = "26.05";
          }
        )
        { services.nixos-manager.remote = "ssh://git@forgejo.lan:2222/test/fleet.git"; }
        extra
      ];
    };
  system = machine { };
  cached = machine {
    services.nixos-manager.binaryCache = {
      enable = true;
      openFirewall = true;
    };
  };
  evalOnly = machine { services.nixos-manager.checks.build = false; };
  inherit (system) config;
  service = config.systemd.services.nixos-manager;
  path = service.environment.PATH;
  # hasInfix matches with a regex, and a regex may not refer to a store path: compare the plain text.
  onPath = p: lib.hasInfix (builtins.unsafeDiscardStringContext "${lib.getBin p}/bin") path;
  checks = {
    "ExecStart runs the package" =
      service.serviceConfig.ExecStart == lib.getExe config.services.nixos-manager.package;
    "runs as its own user" = service.serviceConfig.User == "nixos-manager";
    "the user's home is the state folder" =
      config.users.users.nixos-manager.home == "/var/lib/nixos-manager";
    "state folder" = service.serviceConfig.StateDirectory == "nixos-manager";
    "nix on PATH" = onPath config.nix.package;
    "git on PATH" = onPath pkgs.git;
    "password hash as a credential" =
      service.serviceConfig.LoadCredential == [
        "password-hash:/var/lib/secrets/nixos-manager-password"
      ]
      && service.environment.NIXMGR_PASSWORD_HASH_FILE == "%d/password-hash";
    "remote" =
      service.environment.NIXMGR_REMOTE == "ssh://git@forgejo.lan:2222/test/fleet.git";
    "listens on every address" = service.environment.NIXMGR_LISTEN == "0.0.0.0:8080";
    "firewall port" = lib.elem 8080 config.networking.firewall.allowedTCPPorts;
    "no binary cache unless asked" =
      !config.services.harmonia.cache.enable && !(config.systemd.services ? nixos-manager-cache-key);
    "binary cache served by harmonia" = cached.config.services.harmonia.cache.enable;
    "it signs with the key made on first start" =
      cached.config.services.harmonia.cache.signKeyPaths == [ "/var/lib/nixos-manager-cache/secret-key" ];
    "harmonia waits for the key" =
      lib.elem "nixos-manager-cache-key.service" cached.config.systemd.services.harmonia.requires
      && lib.elem "nixos-manager-cache-key.service" cached.config.systemd.services.harmonia.after;
    "cache port" = lib.elem 5000 cached.config.networking.firewall.allowedTCPPorts;
    "checks build by default" = !(service.environment ? NIXMGR_CHECK_BUILD);
    "checks.build = false reaches the service" =
      evalOnly.config.systemd.services.nixos-manager.environment.NIXMGR_CHECK_BUILD == "0";
  };
  makeKey = cached.config.systemd.services.nixos-manager-cache-key.serviceConfig.ExecStart;
  failed = lib.attrNames (lib.filterAttrs (_: ok: !ok) checks);
in
# The whole system must instantiate (module assertions, and the package's derivation too); only the
# plain path text is kept, so the check does not depend on building anything.
assert
  builtins.stringLength (builtins.unsafeDiscardStringContext config.system.build.toplevel.drvPath) > 0;
assert
  builtins.stringLength (builtins.unsafeDiscardStringContext cached.config.system.build.toplevel.drvPath)
  > 0;
if failed != [ ] then
  throw "services.nixos-manager module check failed: ${lib.concatStringsSep ", " failed}"
else
  # The key script as systemd runs it: the first start makes the key, a restart keeps it, and a lost
  # public key is derived again from the secret one.
  pkgs.runCommand "nixos-manager-module-check" { } ''
    export HOME=$TMPDIR STATE_DIRECTORY=$TMPDIR/cache
    mkdir -m 755 "$STATE_DIRECTORY"
    ${makeKey}
    test "$(stat -c %a "$STATE_DIRECTORY/secret-key")" = 600
    test "$(stat -c %a "$STATE_DIRECTORY/public-key")" = 644
    grep -Eq '^nixos-manager-1:[A-Za-z0-9+/]{43}=$' "$STATE_DIRECTORY/public-key"
    cp "$STATE_DIRECTORY/public-key" "$STATE_DIRECTORY/secret-key" .
    ${makeKey}
    cmp public-key "$STATE_DIRECTORY/public-key"
    cmp secret-key "$STATE_DIRECTORY/secret-key"
    rm "$STATE_DIRECTORY/public-key"
    ${makeKey}
    cmp public-key "$STATE_DIRECTORY/public-key"
    touch $out
  ''
