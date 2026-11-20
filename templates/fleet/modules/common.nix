# Settings shared by every machine.
{ config, lib, ... }:
{
  imports = [
    ./report-deployments.nix
    ./binary-cache.nix
    ./secrets.nix
  ];

  nix.settings.experimental-features = [
    "nix-command"
    "flakes"
  ];

  # Each machine pulls this repository and switches to its own configuration.
  services.comin = {
    enable = true;
    remotes = [
      {
        name = "origin";
        url = "https://forgejo.example.lan/pablo/nixos-config.git"; # change to the Forgejo repository
        branches.main.name = "main";
        # A private repository needs a read-only Forgejo token on each machine: nixos-manager
        # keeps it as the secret comin-token (Settings → Secrets → Private repository).
        auth.access_token_path = lib.mkIf (config.sops.secrets ? comin-token) config.sops.secrets.comin-token.path;
      }
    ];
  };

  # After each deployment, comin tells nixos-manager what this machine now runs.
  # Set the address the machines reach nixos-manager at (LAN or Tailscale), and put
  # the report token on each machine: the Deployments view shows both.
  fleet.deployReport.url = null; # e.g. "http://nixos-manager.home.lan:8080"

  # Download what nixos-manager's checks built instead of building it on each
  # machine. Turn the cache on in the server's services.nixos-manager.binaryCache,
  # then set its address and the key from /var/lib/nixos-manager-cache/public-key there.
  fleet.binaryCache = {
    url = null; # e.g. "http://nixos-manager.home.lan:5000"
    publicKey = null; # e.g. "nixos-manager-1:AbCd…="
  };

  home-manager = {
    useGlobalPkgs = true;
    useUserPackages = true;
  };
}
