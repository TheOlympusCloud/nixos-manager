# Lets this machine download what nixos-manager's checks built (its binary
# cache) instead of building it itself: the Raspberry Pis no longer compile.
#
# On when fleet.binaryCache.url and publicKey are set (common.nix). The cache
# and its key come from services.nixos-manager.binaryCache on the server; the
# key is made there on the first start and is never in this repository.
{ config, lib, ... }:
let
  cfg = config.fleet.binaryCache;
in
{
  options.fleet.binaryCache = {
    url = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "http://nixos-manager.home.lan:5000";
      description = "The cache's address as this machine reaches it (LAN or Tailscale); null turns it off.";
    };
    publicKey = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "nixos-manager-1:AbCd…=";
      description = "The cache's public key: /var/lib/nixos-manager-cache/public-key on the server.";
    };
  };

  config = lib.mkIf (cfg.url != null) {
    assertions = [
      {
        assertion = cfg.publicKey != null;
        message = "fleet.binaryCache.publicKey must be set with fleet.binaryCache.url: without it Nix trusts nothing from the cache.";
      }
    ];
    # Next to cache.nixos.org and its key, which NixOS keeps in both lists.
    nix.settings = {
      substituters = [ cfg.url ];
      trusted-public-keys = lib.optional (cfg.publicKey != null) cfg.publicKey;
      # Away from home (a laptop), an unreachable cache must not stall every build.
      connect-timeout = lib.mkDefault 5;
      # A cache that fails half way through a download: build it here instead of failing the deployment.
      fallback = lib.mkDefault true;
    };
  };
}
