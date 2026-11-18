# home-manager configuration for pablo, shared by every machine that imports it.
{ pkgs, ... }:
{
  home.packages = [ pkgs.htop ];
  programs.git.enable = true;

  home.stateVersion = "26.05";
}
