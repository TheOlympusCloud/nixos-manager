#!/bin/sh
# Entrypoint of the Docker image. Started as root (the default), it gives the data
# folder and Nix's state to the nixos-manager user and runs the server as that user,
# as systemd's StateDirectory and User do for the NixOS module. Started as any other
# user (docker run --user), it runs the server as is.
set -eu

if [ "$(id -u)" != 0 ]; then
  exec /usr/local/bin/nixos-manager "$@"
fi

data=${NIXMGR_DATA:-/var/lib/nixos-manager}
mkdir -p "$data"
# Volumes of the earlier image, which ran as root, hold root-owned files, and so does
# Nix's state after a `docker exec` as root ran nix. (The home is left alone: it may hold
# a read-only ~/.ssh mount, and root's exec has HOME=/root.) find never follows links,
# and chown -h changes a link itself, never what it points to.
find "$data" /nix/var \( ! -user nixos-manager -o ! -group nixos-manager \) \
  -exec chown -h nixos-manager:nixos-manager {} +

export HOME=/home/nixos-manager USER=nixos-manager
# chroot to / with --userspec only switches to the user and its groups, then runs the command.
exec chroot --userspec=nixos-manager:nixos-manager / /usr/local/bin/nixos-manager "$@"
