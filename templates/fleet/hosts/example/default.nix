{ ... }:
{
  imports = [ ./hardware-configuration.nix ];

  networking.hostName = "example";

  users.users.pablo = {
    isNormalUser = true;
    extraGroups = [ "wheel" ];
  };
  home-manager.users.pablo = import ../../home/pablo.nix;

  system.stateVersion = "26.05";
}
