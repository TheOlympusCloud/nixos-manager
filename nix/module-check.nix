# `nix flake check` runs this: it evaluates services.nixos-manager in the Proxmox LXC configuration the
# README gives and asserts the generated service. Evaluation only: the app itself is not built.
{
  nixpkgs,
  pkgs,
  module,
}:

let
  inherit (nixpkgs) lib;
  system = lib.nixosSystem {
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
    ];
  };
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
  };
  failed = lib.attrNames (lib.filterAttrs (_: ok: !ok) checks);
in
# The whole system must instantiate (module assertions, and the package's derivation too); only the
# plain path text is kept, so the check does not depend on building anything.
assert
  builtins.stringLength (builtins.unsafeDiscardStringContext config.system.build.toplevel.drvPath) > 0;
if failed != [ ] then
  throw "services.nixos-manager module check failed: ${lib.concatStringsSep ", " failed}"
else
  pkgs.runCommand "nixos-manager-module-check" { } "touch $out"
