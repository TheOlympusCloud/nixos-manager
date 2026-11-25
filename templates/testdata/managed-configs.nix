# A machine with sops-nix and home-manager whose managed.json (configs/ok) gives
# pablo app configs and a keyring entry (managed.nix 6), with a secret in each.
# missing uses a secret the machine does not have; overlap writes a file
# home-manager also links. The secrets are declared by hand, as
# modules/secrets.nix declares them. Read by TestManagedConfigs (needs real Nix
# and network; reading the writer's specs builds them, two small text files).
let
  nixpkgs = builtins.getFlake "github:NixOS/nixpkgs/nixos-26.05";
  sops-nix = builtins.getFlake "github:Mic92/sops-nix";
  home-manager = builtins.getFlake "github:nix-community/home-manager/release-26.05";
  inherit (nixpkgs) lib;
  machine =
    managed: extra:
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
          sops.secrets = lib.genAttrs [ "token" "pw" ] (_: {
            sopsFile = ./home-secrets/secrets.json;
            format = "json";
          });
        }
        extra
      ];
    };
  m = machine ./configs/ok/managed.json { };
  hm = m.config.home-manager.users.pablo;
  # home-manager keeps a unit's ExecStart as a list.
  execStart = unit: lib.head (lib.toList unit.Service.ExecStart);
  # The spec is the word after the writer's command: the file it reads.
  specOf =
    command:
    let
      w = lib.splitString " " command;
    in
    builtins.elemAt w (1 + lib.lists.findFirstIndex (x: x == "configs" || x == "keyring") null w);
  spec = command: builtins.fromJSON (builtins.readFile (specOf command));
  activation = hm.home.activation.nixosManagerConfigs;
  login = hm.systemd.user.services.nixos-manager-configs;
  keyring = hm.systemd.user.services.nixos-manager-keyring;
in
{
  ok = {
    tokenOwner = m.config.sops.secrets.token.owner;
    pwOwner = m.config.sops.secrets.pw.owner;
    version = m.config.nixosManager.version;
    configs = spec activation.data;
    activationAfter = activation.after;
    loginSameSpec = specOf (execStart login) == specOf activation.data;
    loginWantedBy = login.Install.WantedBy;
    keyring = spec (execStart keyring);
    keyringWantedBy = keyring.Install.WantedBy;
    # Evaluates every assertion and sops-nix's manifest.
    system = builtins.isString m.config.system.build.toplevel.drvPath;
  };
  missing = (machine ./configs/missing/managed.json { }).config.home-manager.users.pablo.home.activation.nixosManagerConfigs.data;
  overlap =
    (machine ./configs/overlap/managed.json {
      home-manager.users.pablo.xdg.configFile."a.json".text = "{}";
    }).config.system.build.toplevel.drvPath;
}
