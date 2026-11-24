# A machine with sops-nix and home-manager whose managed.json (home-secrets/ok)
# links the secret netrc at ~/.netrc for pablo, and whose templates/ folder has
# ~/.config/gh/hosts.yml with the secret gh in it. missing links a secret the
# machine does not have, and unknown's template names one: both fail. The
# secrets are declared by hand here, as modules/secrets.nix declares them. Read
# by TestManagedHomeSecrets (needs real Nix and network).
let
  nixpkgs = builtins.getFlake "github:NixOS/nixpkgs/nixos-26.05";
  sops-nix = builtins.getFlake "github:Mic92/sops-nix";
  home-manager = builtins.getFlake "github:nix-community/home-manager/release-26.05";
  machine =
    managed:
    nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        (import ../fleet/modules/managed.nix managed)
        sops-nix.nixosModules.sops
        home-manager.nixosModules.home-manager
        {
          boot.loader.grub.enable = false;
          fileSystems."/" = {
            device = "nodev";
            fsType = "tmpfs";
          };
          system.stateVersion = "26.05";
          users.users.pablo.isNormalUser = true;
          sops.age.keyFile = "/var/lib/sops-nix/key.txt";
          sops.secrets = nixpkgs.lib.genAttrs [ "netrc" "gh" ] (_: {
            sopsFile = ./home-secrets/secrets.json;
            format = "json";
          });
        }
      ];
    };
  m = machine ./home-secrets/ok/managed.json;
  file = f: m.config.home-manager.users.pablo.home.file.${f}.source;
  template = m.config.sops.templates."home-pablo-.config-gh-hosts.yml";
in
{
  ok = {
    netrcOwner = m.config.sops.secrets.netrc.owner;
    # What the links in ~ point at.
    netrc = (file ".netrc").buildCommand;
    hosts = (file ".config/gh/hosts.yml").buildCommand;
    templateOwner = template.owner;
    templatePath = template.path;
    rendered = template.content == "github.com:\n    oauth_token: ${m.config.sops.placeholder.gh}\n    user: pablo\n";
    # Evaluates every assertion and sops-nix's manifest.
    system = builtins.isString m.config.system.build.toplevel.drvPath;
  };
  missing = (machine ./home-secrets/missing/managed.json).config.home-manager.users.pablo.home.file.".netrc".source.drvPath;
  unknown = (machine ./home-secrets/unknown/managed.json).config.sops.templates."home-pablo-.config-x.conf".content;
}
