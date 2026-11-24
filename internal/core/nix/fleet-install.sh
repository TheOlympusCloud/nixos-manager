# fleet-install: installs $host on this machine from the fleet repository, on the
# installer ISO nixos-manager builds (iso.nix sets the variables above this line).
#   --offline  install the commit on this ISO, without fetching the repository
#   --yes      ask nothing: erase, install, push and reboot (for tests)
# FLEET_INSTALL_ROOT is a folder standing in for / (tests).

offline='' yes=''
for arg in "$@"; do
  case $arg in
    --offline) offline=1 ;;
    --yes) yes=1 ;;
    *)
      echo "usage: fleet-install [--offline] [--yes]" >&2
      exit 2
      ;;
  esac
done
if [ "$(id -u)" != 0 ]; then
  exec sudo "$0" "$@"
fi

# ask <question>: true when answered y, or with --yes.
ask() {
  [ -n "$yes" ] && return 0
  local reply
  read -r -p "$1 [y/N] " reply
  case $reply in [yY]*) return 0 ;; *) return 1 ;; esac
}

# A Forgejo user and token asked for by git are kept in memory for the push at the end.
cred=(-c "credential.helper=cache --timeout=3600")
root=${FLEET_INSTALL_ROOT:-}
mnt=$root/mnt
work=$(mktemp -d)
src=$work/fleet

ssh-keygen -A > /dev/null # sshd made them at boot, unless it was stopped
echo "This installs $host${url:+ from $url ($branch)}."
echo
echo "The SSH host key $host keeps:"
ssh-keygen -lf "$root/etc/ssh/ssh_host_ed25519_key.pub"
echo "The addresses of this machine:"
ip -brief address show scope global || true
echo
if [ -z "$yes" ]; then
  cat << EOF
If $host gets secrets, approve this key first, so that it can read them once
installed: in nixos-manager, Machines, Installer ISO, enter one of the addresses
above, Fetch host key, compare the fingerprint, Use it for secrets, then commit
and push in Changes. No network? Connect with nmtui first.
EOF
  read -r -p "Press Enter to go on. " _
fi

if [ -z "$offline" ] && [ -n "$url" ]; then
  echo "Fetching $branch from $url…"
  if git "${cred[@]}" clone --quiet --branch "$branch" -- "$url" "$src"; then
    echo "Installing $(git -C "$src" log -1 --format='commit %h of %cs: %s')."
  else
    echo "Cannot fetch it. No network? Connect with nmtui. A private repository needs a Forgejo user and a token that can read it."
    ask "Install commit ${commit:0:7} from this ISO instead? Until comin fetches the repository, $host may not open its secrets." || exit 1
    offline=1
  fi
fi
if [ -n "$offline" ] || [ -z "$url" ]; then
  rm -rf "$src"
  cp -r "$baked" "$src"
  chmod -R u+w "$src"
  echo "Installing commit ${commit:0:7}, as this ISO has it."
fi

cfg=$src#nixosConfigurations.$host.config
disks=$(nix eval --raw "$cfg" --apply 'c: toString (map (d: d.device) (builtins.attrValues (c.disko.devices.disk or { })))')
lsblk -o NAME,SIZE,TYPE,MODEL,MOUNTPOINTS || true
if [ -n "$disks" ]; then
  echo "This erases $disks, partitions it as hosts/$host/disk-config.nix says, and installs $host."
elif mountpoint -q "$mnt"; then
  echo "$host has no disk layout: this installs it on what is mounted at /mnt."
else
  echo "$host has no disk layout: partition and format its disks, mount them at /mnt as its hardware configuration says (NixOS manual, Installation), then run fleet-install again."
  exit 1
fi
if [ -z "$yes" ]; then
  read -r -p "Type $host to go on: " typed
  if [ "$typed" != "$host" ]; then
    echo "Stopped: nothing was changed."
    exit 1
  fi
fi
if [ -n "$disks" ]; then
  disko=$(nix build --no-link --print-out-paths "$cfg.system.build.diskoScript")
  "$disko"
fi

# The wizard's placeholder becomes this machine's hardware configuration.
hw=$src/hosts/$host/hardware-configuration.nix
generated=''
if [ -f "$hw" ]; then
  case $(head -n 1 "$hw") in
    '# Placeholder'*)
      echo "Writing hosts/$host/hardware-configuration.nix for this machine."
      if [ -n "$disks" ]; then
        nixos-generate-config --root "$mnt" --no-filesystems --show-hardware-config > "$hw"
      else
        nixos-generate-config --root "$mnt" --show-hardware-config > "$hw"
      fi
      generated=1
      ;;
  esac
fi

# The key nixos-manager may have approved for its secrets.
mkdir -p "$mnt/etc/ssh"
for key in "$root"/etc/ssh/ssh_host_*_key; do
  install -m 600 "$key" "$mnt${key#"$root"}"
  install -m 644 "$key.pub" "$mnt${key#"$root"}.pub"
done

nixos-install --flake "$src#$host" --root "$mnt" --no-root-passwd --no-channel-copy

if [ -z "$yes" ]; then
  for user in $(nix eval --raw "$cfg" --apply 'c: toString (builtins.filter (n: (builtins.getAttr n c.users.users).isNormalUser) (builtins.attrNames c.users.users))'); do
    if ask "Set a password for $user?"; then
      nixos-enter --root "$mnt" -- passwd "$user" || true
    fi
  done
fi

if [ -n "$generated" ]; then
  mkdir -p "$mnt/etc/nixos"
  cp "$hw" "$mnt/etc/nixos/hardware-configuration.nix"
  pushed=''
  if [ -d "$src/.git" ]; then
    git -C "$src" -c user.name="$git_name" -c user.email="$git_email" commit --quiet -m "$host: hardware configuration from the installer" -- "hosts/$host/hardware-configuration.nix"
    if ask "Push it to $url, so that comin deploys $host with it?"; then
      git -C "$src" "${cred[@]}" push --quiet origin "HEAD:refs/heads/$branch" && pushed=1
    fi
  fi
  if [ -z "$pushed" ]; then
    cat << EOF
The repository still has the placeholder hardware-configuration.nix for $host,
and comin's first deployment would switch $host to a system without its own.
Put this one in hosts/$host/ before then: it is /etc/nixos/hardware-configuration.nix
on $host.
EOF
  fi
fi

echo
echo "$host is installed."
if [ -n "$url" ]; then
  echo "comin fetches $branch from $url and keeps $host in step; it shows in nixos-manager's Deployments within about 15 minutes of its first start."
fi
if ask "Reboot now? Take out the USB stick first."; then
  reboot
fi
