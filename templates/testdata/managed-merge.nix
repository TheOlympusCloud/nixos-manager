# A machine with hand-written users and SSH settings plus click-edited values in
# managed.json: the clicked values must win without dropping the rest.
# Run by TestManagedSettingsKeepHandWrittenValues (needs real Nix and network).
let
  nixpkgs = builtins.getFlake "github:NixOS/nixpkgs/nixos-26.05";
  managed = builtins.toFile "managed.json" (
    builtins.toJSON {
      packages = [ "hello" ];
      settings = {
        users.users.pablo.openssh.authorizedKeys.keys = [ "ssh-ed25519 AAAAmanaged managed" ];
        services.openssh.settings.PasswordAuthentication = false;
        networking.firewall.allowedTCPPorts = [ 8080 ];
      };
    }
  );
  machine = nixpkgs.lib.nixosSystem {
    system = "x86_64-linux";
    modules = [
      (import ../fleet/modules/managed.nix managed)
      {
        boot.loader.grub.enable = false;
        fileSystems."/".device = "nodev";
        system.stateVersion = "26.05";
        users.users.pablo = {
          isNormalUser = true;
          extraGroups = [ "wheel" ];
          openssh.authorizedKeys.keys = [ "ssh-ed25519 AAAAhand hand" ];
        };
        services.openssh = {
          enable = true;
          settings = {
            PasswordAuthentication = true;
            KbdInteractiveAuthentication = false;
          };
        };
        networking.firewall.allowedTCPPorts = [ 22 ];
      }
    ];
  };
  c = machine.config;
in
{
  isNormalUser = c.users.users.pablo.isNormalUser;
  groups = c.users.users.pablo.extraGroups;
  keys = c.users.users.pablo.openssh.authorizedKeys.keys;
  sshdUser = c.users.users ? sshd;
  passwords = c.services.openssh.settings.PasswordAuthentication;
  keyboardInteractive = c.services.openssh.settings.KbdInteractiveAuthentication;
  ports = c.networking.firewall.allowedTCPPorts;
  hello = builtins.any (p: (p.pname or "") == "hello") c.environment.systemPackages;
}
