# Settings shared by every machine.
{ ... }:
{
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

  home-manager = {
    useGlobalPkgs = true;
    useUserPackages = true;
  };
}
