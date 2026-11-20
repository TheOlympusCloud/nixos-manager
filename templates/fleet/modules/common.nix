# Settings shared by every machine.
{ ... }:
{
  imports = [
    ./report-deployments.nix
    ./binary-cache.nix
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
        # For a private repository, put a read-only Forgejo token on the machine (sops-nix or agenix):
        # auth.access_token_path = "/run/secrets/comin-token";
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
