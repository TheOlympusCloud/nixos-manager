# Placeholder: replace with the output of `nixos-generate-config --show-hardware-config`
# on the real machine.
{ lib, ... }:
{
  nixpkgs.hostPlatform = lib.mkDefault "x86_64-linux";
  boot.loader.grub.device = "nodev";
  fileSystems."/" = {
    device = "/dev/disk/by-label/nixos";
    fsType = "ext4";
  };
}
