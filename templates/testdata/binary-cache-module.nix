# A machine that uses nixos-manager's binary cache, one with no key, and one
# without the cache. Run by TestBinaryCacheModule (needs real Nix and network).
let
  nixpkgs = builtins.getFlake "github:NixOS/nixpkgs/nixos-26.05";
  inherit (nixpkgs) lib;
  machine =
    cache:
    nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        ../fleet/modules/binary-cache.nix
        {
          boot.loader.grub.enable = false;
          fileSystems."/" = {
            device = "nodev";
            fsType = "tmpfs";
          };
          system.stateVersion = "26.05";
          fleet.binaryCache = cache;
        }
      ];
    };
  on = machine {
    url = "http://manager.example.invalid:5000";
    publicKey = "nixos-manager-1:cBxBk6dHJiKPtMD5VsZ1v3IDHdM/kNKKoGMJAgGG2wY=";
  };
  s = on.config.nix.settings;
  off = (machine { }).config.nix.settings;
in
{
  # cache.nixos.org and its key stay, next to the fleet's cache.
  substituters = s.substituters == [
    "http://manager.example.invalid:5000"
    "https://cache.nixos.org/"
  ];
  key =
    lib.sort lib.lessThan s.trusted-public-keys == [
      "cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY="
      "nixos-manager-1:cBxBk6dHJiKPtMD5VsZ1v3IDHdM/kNKKoGMJAgGG2wY="
    ];
  timeout = s.connect-timeout == 5 && s.fallback;
  off = off.substituters == [ "https://cache.nixos.org/" ] && !(off ? connect-timeout);
  noKey = !(builtins.tryEval (machine { url = "http://manager.example.invalid:5000"; }).config.system.build.toplevel.drvPath).success;
  # Building it runs Nix's own check of the generated nix.conf.
  conf = on.config.environment.etc."nix/nix.conf".source;
}
