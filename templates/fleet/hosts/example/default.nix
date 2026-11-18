{ ... }:
{
  imports = [ ./hardware-configuration.nix ];

  networking.hostName = "example";

  users.users.pablo = {
    isNormalUser = true;
    extraGroups = [ "wheel" ];
  };
  home-manager.users.pablo = import ../../home/pablo.nix;

  services.openssh.enable = true;

  system.stateVersion = "26.05";
}
