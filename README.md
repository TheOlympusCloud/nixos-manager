# nixos-manager

A self-hosted console for all the NixOS machines configured by one git flake
repository. It evaluates every machine with Nix and shows the fleet visually:
what each machine runs, where it comes from, and what a change will do to it.
An MCP server gives automation the same features.

- **Fleet graph**: machines, the services and programs they enable, the shared
  modules that configure them, their packages, users and flake inputs, as an
  interactive graph.
- **Health**: evaluation errors, a disabled firewall, SSH with passwords or
  root logins, no fail2ban or garbage collection, outdated flake inputs,
  unsupported NixOS releases, packages on every machine but one, failed or
  lagging deployments, and machines a push does not reach (comin off, or
  pulling another repository). Most findings have a **Fix** button that
  applies the setting to every affected machine.
- **Forgejo**: connect the fleet repository from Settings with a deploy key
  the app creates; a push webhook keeps the console in step with pushes made
  elsewhere, and commits and files link to their Forgejo pages.
- **Deployments**: every machine's comin reports what it deployed, so after a
  push you watch the rollout reach each machine, and see failures with
  comin's error.
- **Machine pages**: packages (with the file that adds each one), services and
  programs, users and SSH keys, ports and file systems, and **Essentials**:
  time zone, locale, keyboard, garbage collection, store optimisation, zram,
  firewall, SSH, fail2ban and Tailscale, each with its real value and where it
  is set.
- **Change things by clicking**: add or remove packages from a search of the
  nixpkgs the flake locks, add popular services (Jellyfin, Immich, Syncthing,
  Home Assistant, Docker…) from a catalog, add users and SSH keys, and set any
  option through forms built from the NixOS option documentation. These edits
  go into the machine's `hosts/<name>/managed.json`, which `modules/managed.nix`
  applies with priority over hand-written values (but not over `mkForce`);
  hand-written Nix is never rewritten.
- **Real values**: an option's effective value on each machine and the file
  that sets it, compared across the fleet, with "copy to" other machines.
- **New machines**: a wizard creates `hosts/<name>/` with a role preset
  (server, desktop or minimal) and the hardware configuration; the starter
  flake picks up every folder in `hosts/`.
- **Fleet matrices**: which package and which service runs on which machine.
- **Flake inputs**: each input's revision and age; update one or all and see
  what moved (with compare links) and which package versions change on which
  machine.
- **Changes in NixOS terms**: before committing, see per machine which
  packages, versions, services, users, ports and settings the working copy
  changes, and which inputs moved. History shows the same for every commit,
  and **Push** first shows it for what the push deploys, compared with what is
  pushed. A machine counts as changed whenever its system derivation
  (`system.build.toplevel`) does; a new kernel, kernel module, initrd or
  kernel parameter shows as "Reboot needed". A machine whose system does not
  evaluate, a failed assertion for instance, shows its error. Each changed
  machine also lists:
  - the systemd units a switch restarts, reloads, starts, stops, or leaves
    running the old version ("Not restarted", e.g. `restartIfChanged = false`),
    decided as `switch-to-configuration` decides for a unit that runs. Units
    are compared by their NixOS settings, so a change to a unit file that
    only a package ships is not listed;
  - the `/etc` files added, removed or changed;
  - when this server has built both systems (a check builds the machines it
    changes), every dependency whose version or size changes and the system's
    closure size before and after, from `nix store diff-closures`.

  An edit that touches none of these (a sysctl, say) shows as "Other
  configuration".
- **Checks**: on exactly the staged changes, with live output, `nix flake check`
  and then a real build of every machine whose system they change compared with
  what is pushed (by system derivation, as Changes compares), so a package or
  system that evaluates but does not build fails the check. With nothing pushed
  yet, every machine is built. The result says which machines were built, which
  failed, which need a reboot, and what Nix built and downloaded (from a dry run
  first). Configurations you tick are dry-built on top, which only evaluates
  them. A commit needs a passing check of the staged changes, and a push needs a
  passing check of the commit being pushed that compared it with what the push
  lands on now. In the web UI you can override both explicitly; MCP clients
  cannot. Passed checks are kept in `$NIXMGR_DATA/checks.json`, so a restart
  does not ask for them again; checks passed before checks built the machines
  only evaluated, and must run again. Checks run one at a time in their own
  queue, so a long check never holds up the fleet views, search, Lock or Update.
  The systems they build can be served to the machines as a [binary
  cache](#binary-cache).
- **Git**: stage, commit, branches, push and pull, revert a commit, merge a
  `testing-<host>` branch, repository and per-file history with diffs and
  restore. Every commit uses the configured person's name and email.
- **Code**: a Nix editor with `nixd` completion, diagnostics and option docs,
  and `nixfmt`, for the hand-written files.
- **Command palette** (Ctrl/Cmd+K): jump to any machine, package, service,
  option, file or action.
- Login for the UI, limited against password guessing; named, revocable API
  tokens for MCP clients and scripts, each read-only, edit without push, or
  full, with an optional expiry. Every commit names the session or token that
  made it.

The repository is a normal git repository whose remote is Forgejo. Each machine
runs [comin](https://github.com/nlewo/comin), pulls from Forgejo and switches
to `nixosConfigurations.<its hostname>`.

## Run it in a Proxmox LXC (recommended)

nixos-manager is happiest in its own NixOS container: Nix is right there, so it
evaluates, locks and checks the fleet for real, and the container can later
become one of the fleet's machines. The commands below run as root on the
Proxmox host, then in the container. The module builds nixos-manager with the
container's own nixpkgs, which needs Go 1.26: NixOS 26.05 or newer.

1. **Get the NixOS container template.** NixOS's Hydra builds it from
   `nixos/modules/virtualisation/proxmox-lxc.nix` for every 26.05 update
   (x86_64 only):

   ```sh
   wget -O /var/lib/vz/template/cache/nixos-26.05-proxmox-lxc.tar.xz \
     https://hydra.nixos.org/job/nixos/release-26.05/nixos.proxmoxLXC.x86_64-linux/latest/download-by-type/file/system-tarball
   ```

   That folder is the `local` storage's container templates; in the web UI the
   same is **local → CT Templates → Download from URL**.

2. **Create the container.** Put your SSH public key in a file first; Proxmox
   copies it to root in the container (a root password set from Proxmox does
   not work on NixOS).

   ```sh
   echo 'ssh-ed25519 AAAA... you@laptop' > /root/me.pub
   pct create 120 local:vztmpl/nixos-26.05-proxmox-lxc.tar.xz \
     --hostname nixos-manager \
     --ostype nixos --unprivileged 1 --features nesting=1 \
     --cores 4 --memory 4096 --swap 2048 \
     --rootfs local-lvm:30 \
     --net0 name=eth0,bridge=vmbr0,ip=dhcp \
     --ssh-public-keys /root/me.pub \
     --onboot 1 --start 1
   lxc-info -n 120 -iH    # the container's address
   ```

   - `120` is any free ID, and `local-lvm` the storage for its disk
     (`local-zfs` on a ZFS install).
   - `--ostype nixos` makes Proxmox hand the network to systemd-networkd, as the
     image expects. `--features nesting=1` is required: systemd's service
     sandboxing and Nix's build sandbox need it.
   - Size: every machine is a full NixOS evaluation, so memory goes first.
     4 GB RAM is a good start (8 GB past a dozen machines), and 30 GB of disk
     holds a few nixpkgs revisions and the machines' systems the checks build.
     More cores make checks faster. The fleet views evaluate two machines at once (about 1 GB
     each), and a check can run next to them, as can one evaluation of what a
     newly deployed machine should run (Deployments). On a tight container set
     `services.nixos-manager.extraEnvironment.NIXMGR_EVAL_JOBS = "1";`, or
     raise it when memory is to spare.

3. **Configure it.** `ssh root@<address>`, then write `/etc/nixos/flake.nix`:

   ```nix
   {
     inputs = {
       nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
       nixos-manager = {
         url = "github:TheOlympusCloud/nixos-manager";
         inputs.nixpkgs.follows = "nixpkgs";
       };
     };

     outputs =
       { nixpkgs, nixos-manager, ... }:
       {
         nixosConfigurations.nixos-manager = nixpkgs.lib.nixosSystem {
           modules = [
             nixos-manager.nixosModules.default
             ./configuration.nix
           ];
         };
       };
   }
   ```

   and `/etc/nixos/configuration.nix`:

   ```nix
   { config, modulesPath, ... }:
   {
     imports = [ (modulesPath + "/virtualisation/proxmox-lxc.nix") ];

     nixpkgs.hostPlatform = "x86_64-linux";
     nix.settings.experimental-features = [
       "nix-command"
       "flakes"
     ];
     # Each nixpkgs revision the fleet moves past stays in the store until collected.
     nix.gc = {
       automatic = true;
       dates = "weekly";
       options = "--delete-older-than 14d";
     };

     services.nixos-manager = {
       enable = true;
       openFirewall = true; # port 8080
       author = {
         name = "Your Name"; # every commit is made as this person
         email = "you@example.com";
       };
       passwordHashFile = "/var/lib/secrets/nixos-manager-password";
     };
     # For `nixos-manager hash-password`.
     environment.systemPackages = [ config.services.nixos-manager.package ];

     system.stateVersion = "26.05";
   }
   ```

   Switch to it. The first time builds nixos-manager (Go and the web UI), which
   takes a few minutes:

   ```sh
   nixos-rebuild switch --flake /etc/nixos#nixos-manager
   ```

   If this repository lives on your Forgejo instead of GitHub, use
   `url = "git+https://forgejo.lan/you/nixos-manager.git";` and add
   `programs.git.enable = true;`: Nix fetches git inputs with git, which the
   template lacks, so borrow it for the first switch:

   ```sh
   nix --extra-experimental-features 'nix-command flakes' shell nixpkgs#git \
     -c nixos-rebuild switch --flake /etc/nixos#nixos-manager
   ```

4. **Set the login password.** The service waits for this file:

   ```sh
   install -d -m 700 /var/lib/secrets
   read -rsp 'Password: ' pw; echo
   printf '%s\n' "$pw" | nixos-manager hash-password > /var/lib/secrets/nixos-manager-password
   unset pw
   systemctl restart nixos-manager
   ```

5. Open `http://<address>:8080`, log in, and connect Forgejo in **Settings**
   (see [Forgejo access](#forgejo-access)).

The service runs as the `nixos-manager` user, with its working copy, deploy
key, tokens and Nix caches in `/var/lib/nixos-manager`; `journalctl -u
nixos-manager` has its log. Other options: `listen` (default `0.0.0.0:8080`),
`remote` (sets `NIXMGR_REMOTE`; leave it out to connect Forgejo in Settings),
`environmentFile` (extra secrets as `KEY=value`), `extraEnvironment`, and
`binaryCache` (see [Binary cache](#binary-cache)).

### Make the container one of the fleet's machines

Once the fleet is on Forgejo and comin deploys it, the container can be managed
like every other machine:

1. In nixos-manager, **New machine**: name it `nixos-manager` (its host name),
   pick a role and the **Proxmox LXC** platform. The server role allows SSH keys
   only and no root login: add your user and key under **Users** first.
2. Add the app to the fleet's flake inputs and run **Lock**:

   ```nix
   nixos-manager = {
     url = "github:TheOlympusCloud/nixos-manager";
     inputs.nixpkgs.follows = "nixpkgs";
   };
   ```

3. Move the service settings into `hosts/nixos-manager/default.nix` (the
   starter flake passes `inputs` to every machine):

   ```nix
   { inputs, modulesPath, ... }:
   {
     imports = [
       (modulesPath + "/virtualisation/proxmox-lxc.nix")
       inputs.nixos-manager.nixosModules.default
     ];

     nixpkgs.hostPlatform = "x86_64-linux";
     proxmoxLXC.manageHostName = true;
     networking.hostName = "nixos-manager";

     services.nixos-manager = {
       enable = true;
       openFirewall = true;
       author = {
         name = "Your Name";
         email = "you@example.com";
       };
       passwordHashFile = "/var/lib/secrets/nixos-manager-password";
     };

     system.stateVersion = "26.05";
   }
   ```

4. Check, commit and push. Then switch the container once from its own working
   copy, as root in the container:

   ```sh
   nixos-rebuild switch --flake path:/var/lib/nixos-manager/repo#nixos-manager
   ```

   From then on comin (on in every machine through `modules/common.nix`) pulls
   from Forgejo and switches the container; `/etc/nixos` is no longer used.
   `path:` reads the files as they are, so do this with nothing uncommitted.
   Updating nixos-manager itself is then an update of its flake input.

## Run it with Docker Compose

The alternative, on any host with Docker. The image carries its own Nix.

> **Upgrading from an image that ran as root?** With Compose, nothing to do.
> With plain `docker run`, give the volume to the new user once first; the
> command is below.

1. Pick a login password and store its hash:

   ```sh
   mkdir -p secrets
   printf '%s\n' 'your password' | docker compose run --rm -T --build nixos-manager hash-password > secrets/password-hash
   ```

2. Edit `compose.yaml`: set `NIXMGR_AUTHOR_NAME` and `NIXMGR_AUTHOR_EMAIL` (the
   identity of every commit).
3. `docker compose up -d --build`, then open `http://<server>:8080`.

The server and its Nix run as the unprivileged `nixos-manager` user (uid and gid
10001), and nothing in its container runs as root. Before each start, Compose runs
`data-owner`, a one-shot container of its own that gives the data volume to that
user: volumes of earlier images, which ran as root, are root-owned. Without
Compose, a new volume needs nothing; give an earlier image's volume to the user
once with `docker run --rm --user root --entrypoint chown -v <volume>:/var/lib/nixos-manager
<image> -R -h nixos-manager:nixos-manager /var/lib/nixos-manager`.

Files you mount must be readable by uid 10001: the password hash, and a key in
`~/.ssh`. An SSH `config` there must also belong to root or to uid 10001, or ssh
refuses it. The user owns the container's Nix store, so it can replace any program
in it: never run commands in the server's container as root
(`docker compose exec` runs them as the user).

Either way, without a remote the app starts a new repository with a starter
flake (an example host, a home-manager user and comin). Run **Lock** once to
create `flake.lock`, then stage, check and commit. Connect Forgejo in
**Settings** when it is ready (below); **Push** publishes the branch.

### Forgejo access

Connect the repository in **Settings → Forgejo**; the steps are numbered there.

1. **Repository.** Paste its SSH URL (`ssh://git@forgejo.lan:2222/you/nixos-config.git`
   or `git@forgejo.lan:you/nixos-config.git`). It is stored as `origin` in the
   working copy's git config. `NIXMGR_REMOTE`, when set, still sets `origin` at
   every start and wins over the value saved in Settings. A URL with a password
   or token in it is refused.
2. **Deploy key.** The app creates an ed25519 key pair the first time Settings
   needs one: `$NIXMGR_DATA/ssh/id_ed25519` (mode 0600, never sent to the
   browser) and its `.pub`. Copy the public key into Forgejo: repository →
   **Settings → Deploy keys → Add deploy key**, and tick **Enable write access**.
   Git then connects with only this key, and records Forgejo's host key in
   `$NIXMGR_DATA/ssh/known_hosts` on first contact.
3. **Test connection** runs `git ls-remote` and says what is wrong in plain
   words: host unreachable, key refused, repository not found, read-only key,
   or a changed host key (with the `ssh-keygen -R` line that forgets the old one).
   If the Forgejo repository already has commits and this working copy has none
   (only the uncommitted starter flake), Settings offers to use the repository
   instead; the starter files are kept in git as `refs/nixmgr/starter-backup`.
   Local commits are never replaced: with two unrelated histories, Pull and the
   switch are refused. If the Forgejo repository is empty, **Push** fills it.
4. **Push webhook** (optional, recommended). Forgejo repository → **Settings →
   Webhooks → Add webhook → Forgejo**: the target URL and the secret shown in
   Settings, method POST, content type `application/json`, trigger on push
   events. On a push to the branch the console is on, nixos-manager fetches;
   when the working copy is clean and has no commits of its own it fast-forwards,
   otherwise the top bar shows "N new on Forgejo" and **Pull** brings them in.
   Calls are checked against the secret (HMAC-SHA256), and bodies over 1 MB are
   refused. The secret lives in `$NIXMGR_DATA/webhook-secret`.

If a clone at start fails (the key is not on Forgejo yet), the server still
starts with the starter flake and `origin` set, so you can finish the steps above.

Commits in History and files in Code link to Forgejo. The web address is derived
from the remote (`ssh://git@host:2222/o/r.git` → `https://host/o/r`); set
another host, port or http under **Web address** if Forgejo's web UI is elsewhere.

Other setups keep working:

- A key you mount at `~/.ssh` (`/home/nixos-manager/.ssh` in the container,
  see `compose.yaml`; it was `/root/.ssh` before the image ran as its own user)
  is used as before, until you choose **Create a deploy key** in
  Settings. A `GIT_SSH_COMMAND` in the environment always wins.
- HTTPS: put `https://<user>:<token>@forgejo.lan` in a git credentials file,
  mount it, and add `GIT_CONFIG_COUNT=1`, `GIT_CONFIG_KEY_0=credential.helper`,
  `GIT_CONFIG_VALUE_0=store --file=/run/secrets/nixmgr/git-credentials` to the
  environment. Use the plain `https://forgejo.lan/you/nixos-config.git` as the
  remote; never put the token in the URL.

### LAN and Tailscale

On the LAN, the container answers on `http://<address>:8080` (`openFirewall`
opens the port); with Docker, the port is published on the host. It is plain
HTTP: the password, the session cookie and the machines' deployment reports
(with their report token) cross the LAN unencrypted. Keep it on a trusted
network, use Tailscale as below, or put a TLS reverse proxy (Caddy or nginx with
a certificate) in front of it.

Tailscale encrypts the traffic, and `tailscale serve` adds HTTPS on the tailnet:

- **LXC.** Tailscale needs the TUN device: on the Proxmox host run
  `pct set 120 --dev0 /dev/net/tun` and restart the container. Add
  `services.tailscale.enable = true;` to its configuration and switch, then
  run `tailscale up` once and `tailscale serve --bg 8080`. It is reachable at
  `https://nixos-manager.<tailnet>.ts.net`.
- **Docker.** Run `tailscale serve --bg 8080` on the host, and set
  `NIXMGR_TRUSTED_PROXIES` in `compose.yaml` (see the login limits under
  Configuration).

If the LAN is not trusted, listen on loopback only and use it through
Tailscale: `listen = "127.0.0.1:8080";` without `openFirewall` in the
container, or `"127.0.0.1:8080:8080"` as the published port with Docker.

## Using it

1. Edit files; **Ctrl+S** saves, **Shift+Alt+F** formats.
2. In **Changes**, stage what you want to commit.
3. In **Checks**, run the check: it builds every machine the staged changes
   touch. Tick other configurations to dry-build them too.
4. Commit with a message, then **Push**. The push dialog shows what each
   machine gets compared with what is pushed; comin deploys it on each machine.

When **Pull** rebases your local commits onto new remote commits, the result
was never checked as a whole, so **Push** refuses it: run the checks again with
nothing staged or changed, then push.

A branch named `testing-<hostname>` is deployed by comin to that one machine
only, for trying a change before merging it into `main`.

Never store secrets in the repository; use sops-nix or agenix. Nix copies
what it evaluates into the store, which every user of the server can read.
**Lock** and **Update** give it only the tracked files (as they are in the
working copy) plus `flake.nix` and `flake.lock`, never `.git` or untracked or
ignored files, and then copy the new `flake.lock` back. If `flake.nix` or
`flake.lock` is saved while they run, they write nothing: run them again.

## Deployments

A push is a deploy: every machine's comin pulls the branch and switches to it.
With reporting on, each machine tells nixos-manager what comin did, and the
**Deployments** view shows the latest commits of the branch with the machines
that run each one, so after a push you watch them catch up. Each machine is
compared with the branch its own comin follows, read from
`services.comin.remotes` (the remote this console pushes to, else the first
one): its `branches.main.name`, or its `branches.testing.name`
(`testing-<hostname>` by default) when that branch is on top of the main one. Per machine it shows the
commit, when it was deployed, and comin's error when a build or switch failed;
the machine page has the same in its **Deployment** card, and the fleet graph
marks each machine deployed, behind, failed or silent. **Health** flags a failed
deployment, a machine behind for over an hour, and one that stopped reporting.

Every 15 minutes each machine also sends a heartbeat with the system it runs,
the one it booted and the one it boots next. A machine that has not reported for
`fleet.deployReport.silentAfter` hours (2 by default) is flagged as silent,
whatever its deployment state: an up-to-date machine that dies no longer stays
green. Set `silentAfter = null;` on a machine that is often off, such as a
laptop, so it is never flagged. No machine is called silent in the first 20
minutes after nixos-manager starts, while the machines report in again.

nixos-manager also evaluates what each machine's deployed commit builds,
the way comin does (the commit itself, `git+file://…?rev=…`, so a
configuration that records `self.rev` matches), one machine at a time in its
own job lane, and compares it with what the machine says it runs:

- **Changed by hand**: it runs a system that neither its commit builds nor it
  boots into from comin, after a `nixos-rebuild` on the machine or a rollback.
  comin replaces it with the next commit that changes the machine.
- **Switch unfinished**: comin installed the commit's system as the boot
  default but it does not run, or a switch failed partway and left the failed
  commit's system running.
- **Reboot pending**: the running kernel, its modules or the initrd are not the
  booted ones.

There is no verdict while comin switches to a new build, or before the commit
is evaluated; the result of each commit is kept until nixos-manager restarts,
and a failed evaluation is tried again after an hour.

To turn it on (the **Deployments** view walks you through it):

1. In `modules/common.nix`, set the address the machines reach nixos-manager
   at, then commit and push:

   ```nix
   fleet.deployReport.url = "http://nixos-manager.home.lan:8080";
   ```

   Over plain HTTP the token crosses the LAN readable; a Tailscale address or
   an `https://` one keeps it private.

2. Put each machine's own report token on it. The view shows the token and the
   command for each machine; the token travels on stdin:

   ```sh
   echo 'nxh_…' | ssh root@pi 'install -D -m 600 /dev/stdin /var/lib/nixos-manager-report/token'
   ```

   A token only reports for its own machine: nixos-manager refuses a report
   whose machine name is not the one the token was made for, so a machine (or
   whoever reads its token) cannot pass for another. The tokens are derived
   from a key in `$NIXMGR_DATA/report-key`, so nothing lists them.
   With sops-nix or agenix, point `fleet.deployReport.tokenFile` at the secret
   instead. Never commit a token. **New tokens for every machine** replaces the
   key; machines with an old token stop reporting until they get the new one.

3. The first report arrives with the next deployment after both are in place,
   and the first heartbeat a few minutes after the deployment that installs
   the timer.

**Upgrading from one fleet token.** Installs from before machine tokens kept a
single token for every machine in `$NIXMGR_DATA/deploy-token`. This release
still accepts it, and marks the machines that report with it in **Deployments**
and **Health**. Install each machine's own token in its token file (the same
file, step 2), wait for its next report, and when no machine is listed any
more, click **Stop accepting the shared token** in the setup, which deletes the
file. The next release no longer accepts it.

`modules/report-deployments.nix` hooks comin's `postDeploymentCommand` and, on
comin versions that have it, `postBuildCommand`, which also reports failed
builds and commits that leave a machine unchanged. Without it, a machine whose
configuration a commit does not change looks behind until a commit does change
it. A systemd timer, `nixos-manager-heartbeat.timer`, sends the heartbeat. The
hook posts with `curl` to `/api/deployments/report`, does nothing without the
token file, and never fails a deployment. Reports that cannot be delivered
(nixos-manager is down, say) wait in order in
`/var/lib/nixos-manager-report/pending.jsonl` (the newest 20), and the next
heartbeat or comin report sends them first. A report sent late is dated when
it arrives, so **Deployed** can be later than the switch itself.
nixos-manager keeps the last 20 reports per machine in
`$NIXMGR_DATA/deployments.json`, and what each machine's latest report or
heartbeat said in `$NIXMGR_DATA/heartbeats.json`. The reports of a machine that is no longer in the
flake are deleted once the working copy is exactly the tip of the branch comin
deploys, if the machine last reported a commit of that branch, deployed from
it. A machine deleted but not pushed, or one that comin deploys from another
branch, keeps them. **Forget** on a machine's row deletes its reports by hand; it comes
back if it reports again. A repository made from an older starter fleet has
no hook, or one without heartbeats (Health lists the machines that send none):
copy both `report-deployment*` files from `templates/fleet/modules/` into its
`modules/` and import `report-deployments.nix` from `common.nix`. Until then such
a machine is only flagged as silent when it is behind and has not reported for
7 days.

## Notifications

**Settings → Notifications** sends a message when:

- a machine fails to build or deploy a commit (once per machine and commit);
- a machine has been behind its branch for over an hour (and is not silent);
- a machine stops reporting (see `silentAfter` above);
- the commit the fleet's branch points at does not evaluate, as a whole or for
  a machine, so comin cannot deploy it (told only after a second evaluation
  fails too, so a network hiccup does not page you);
- a NixOS release the machines run reaches, or is a month from, its end of
  life, or a flake input follows a release branch whose support ended.

Channels, any or all of them:

- **ntfy**: the topic's address (`https://ntfy.sh/<topic>` or a server of your
  own) and, for a protected topic, an access token. On ntfy.sh anyone who
  knows the topic can read it, so pick a name nobody guesses.
- **Webhook**: a JSON `POST` with `key`, `kind` (`deploy-failed`,
  `deploy-stuck`, `host-silent`, `eval-failed`, `eol`, `test`), `severity`,
  `title`, `message`, `hosts`, `url` (a link into the console) and `at`.
- **Email** over SMTP, with STARTTLS (587), TLS (465) or, to a local relay
  only, none. A password is only ever sent encrypted.

Each event goes once to each channel while it lasts; when it ends it is
forgotten, so it is sent again only if it comes back. A channel only gets
events that start after it was set up or pointed somewhere new: what is
already wrong is in **Health**. A failed send is tried again a minute later,
then waiting twice as long each time, up to an hour; Settings shows how the
last send to each channel went, and **Send a test message** checks a channel
right away. Nothing is sent in the first 20 minutes after a start, while the
machines report in. The channels are in `$NIXMGR_DATA/notify.json`. The ntfy
token and the SMTP password are never shown again once saved, but the ntfy
topic and the webhook address are, to whoever is logged in: for ntfy.sh, Slack,
Discord or Home Assistant those addresses are secrets too. What was sent is in
`$NIXMGR_DATA/notify-state.json`, so a restart does not send it twice.

## Binary cache

A check builds the systems of the machines a change touches. With the binary
cache on, the server serves its Nix store to the machines, signed, so comin
downloads those systems instead of building them: a Raspberry Pi no longer
compiles anything the server already built. It needs the NixOS module (the
LXC setup above); with Docker, checks build as well, but nothing serves the
results.

1. On the server, add to its configuration and switch:

   ```nix
   services.nixos-manager.binaryCache = {
     enable = true;
     openFirewall = true; # port 5000
   };
   ```

   [harmonia](https://github.com/nix-community/harmonia) serves the store on
   port 5000 (`services.harmonia.cache.settings.bind` changes it). On the first
   start, `nixos-manager-cache-key.service` makes the signing key in
   `/var/lib/nixos-manager-cache`: `secret-key`, readable by root only, and
   `public-key`. It is never in the Nix store or the repository; delete both
   files and restart the service to make a new one, then give the machines
   the new public key.

2. In the fleet's `modules/common.nix`, set the address the machines reach the
   cache at and the public key (`cat /var/lib/nixos-manager-cache/public-key`
   on the server), then check, commit and push:

   ```nix
   fleet.binaryCache = {
     url = "http://nixos-manager.home.lan:5000";
     publicKey = "nixos-manager-1:…";
   };
   ```

   `modules/binary-cache.nix` adds the cache next to cache.nixos.org, with a
   5 s connection timeout (a laptop away from home does not stall) and
   fallback to building when a download fails. A repository made from an older
   starter fleet has no such module: copy `modules/binary-cache.nix` from
   `templates/fleet/` and import it from `common.nix`.

Anyone who can reach port 5000 can download any path of the server's store
whose name they know, so keep it on the LAN or the tailnet. Nix trusts only
what the key signed. Each machine's last system a check built has a GC root in
`$NIXMGR_DATA/gcroots`, so `nix.gc` keeps it until a later check replaces it;
the roots of machines removed from the flake go with the next check.

### Machines of another architecture

A check builds every changed machine on the server, so an `aarch64-linux`
machine (a Raspberry Pi) on an `x86_64-linux` server needs one of these, or
its builds fail and so does the check:

- **Emulation, server in a VM or on bare metal.** Add
  `boot.binfmt.emulatedSystems = [ "aarch64-linux" ];` and switch. Builds that
  are not in cache.nixos.org run emulated, which is slow.
- **Emulation, server in a Proxmox LXC.** A container cannot register
  emulators: install `qemu-user-static` and `binfmt-support` on the Proxmox
  host, check that `/proc/sys/fs/binfmt_misc/qemu-aarch64` lists the `F`
  flag (the container then uses the host's emulator), and add
  `nix.settings.extra-platforms = [ "aarch64-linux" ];` to the container.
- **A remote builder.** Any aarch64 machine with Nix (a Pi 4 or 5 will do)
  builds for the server over SSH: give root on the server an SSH key the
  builder accepts for a user listed in its `nix.settings.trusted-users`, then
  on the server:

  ```nix
  nix.distributedBuilds = true;
  nix.buildMachines = [
    {
      hostName = "pi5.home.lan";
      sshUser = "nixbuild";
      sshKey = "/root/.ssh/nixbuild";
      system = "aarch64-linux";
      maxJobs = 2;
    }
  ];
  ```

Either way, the built paths land in the server's store and the binary cache
serves them to the machines.

## MCP

Create a token on the **Tokens** page and point the client at `/mcp`:

```json
{
  "mcpServers": {
    "nixos-manager": {
      "type": "http",
      "url": "http://<server>:8080/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

Fleet tools: `fleet_overview`, `fleet_insights`, `host_details`, `model_diff`,
`compare_option`, `search_packages`, `search_options`, `add_packages`,
`remove_packages`, `set_option`, `unset_option`, `create_machine`. File and git
tools: `list_files`, `read_file`, `write_file`, `create_folder`, `move_path`,
`delete_path`, `format_nix`, `status`, `diff`, `stage`, `unstage`, `discard`,
`commit`, `log`, `show_commit`, `read_file_at`, `restore_file`,
`list_branches`, `create_branch`, `switch_branch`, `revert_commit`,
`merge_branch`, `push`, `pull`,
`list_targets`, `run_check`, `flake_lock`, `flake_update`. They call the same
code as the web UI, including the check rules for commit and push, which MCP
clients cannot override. Writes and restores need the `base_hash` from
`read_file`, so a client never overwrites an edit it has not seen.

### Token scopes

Each token gets a scope when it is created, and optionally an expiry date:

| Scope | What it can do |
|---|---|
| Read only | Read files, history, the fleet model, checks, jobs and deployments; it may also start a re-evaluation or search-index refresh, which uses CPU but changes nothing |
| Edit, no push | Also edit files, stage, run checks, commit, branch, revert, merge, pull, and lock or update inputs |
| Full | Also push, which deploys |

The MCP endpoint offers only the tools of the token's scope. The same token
works on the HTTP API, on the routes its scope covers:

```sh
curl -H "Authorization: Bearer <token>" http://<server>:8080/api/deployments
```

Managing tokens, Settings, the deployment report token and the editor's
language server need the web login. Only the person logged in to the web UI
can commit or push without a passing check. Tokens created before scopes
existed are full tokens, as before; an expired token stays listed, refused,
until you revoke it.

Every commit made through the console ends with a trailer that says how it
arrived: `Via: web session 3fa2c1d9` (the same label as the `login` line in the
service log) or `Via: token ci (K3J9QW2M4X1A)`, the token's name and id. A
commit message may not bring its own `Via` trailer.

## Configuration

| Variable | Meaning |
|---|---|
| `NIXMGR_AUTHOR_NAME`, `NIXMGR_AUTHOR_EMAIL` | Commit identity (required) |
| `NIXMGR_PASSWORD_HASH` or `NIXMGR_PASSWORD_HASH_FILE` | bcrypt hash of the login password (required) |
| `NIXMGR_DATA` | State folder: working copy, tokens, sessions, passed checks, GC roots of the systems checks built, and deployment reports (default `/var/lib/nixos-manager`) |
| `NIXMGR_REPO` | Working copy (default `$NIXMGR_DATA/repo`) |
| `NIXMGR_REMOTE` | Remote to clone from and push to |
| `NIXMGR_LISTEN` | Listen address (default `127.0.0.1:8080`; `0.0.0.0:8080` in the image) |
| `NIXMGR_TRUSTED_PROXIES` | Reverse proxies besides loopback, as addresses or CIDR ranges, whose `X-Forwarded-For` names the client for the login limits (e.g. the Docker gateway) |
| `NIXMGR_NIX`, `NIXMGR_NIXFMT`, `NIXMGR_NIXD` | Tool commands (default `nix`, `nixfmt`, `nixd`) |
| `NIXMGR_EVAL_JOBS` | Machines evaluated at once for the fleet views (default 2, or 1 on a single CPU). Each is a whole NixOS evaluation, about 1 GB of memory, and a check can run at the same time. **Evaluate again** after a failure re-evaluates only the machines that failed |

Sessions last 7 days and survive a restart: `$NIXMGR_DATA/sessions.json` keeps
a keyed hash of each one, never the session itself. Changing the password ends
every session.

Logins are limited before the password is checked: 10 attempts per 10
minutes from one address, and 50 per 10 minutes from all addresses together.
More get `429 Too Many Requests` with `Retry-After`; successful logins do not
count. A browser that has logged in before keeps a cookie that gives it its own
limit of 10, outside the shared one, so someone guessing from elsewhere cannot
lock you out of a browser you already use. Behind a proxy on the same machine
(`tailscale serve`, Caddy, nginx) the address is the last one the proxy added
to `X-Forwarded-For`. With Docker, a proxy on the host reaches the container
from the Docker gateway rather than loopback: set `NIXMGR_TRUSTED_PROXIES` to
that address (the `remote=` of the `login` lines in the log), or every proxied
login shares one address's limit. List only proxies there, since anything in
that list can claim any client address. The counts are kept in memory.

Logins are limited before the password is checked: 10 attempts per 10
minutes from one address, and 50 per 10 minutes from all addresses together.
More get `429 Too Many Requests` with `Retry-After`; successful logins do not
count. A browser that has logged in before keeps a cookie that gives it its own
limit of 10, outside the shared one, so someone guessing from elsewhere cannot
lock you out of a browser you already use. Behind a proxy on the same machine
(`tailscale serve`, Caddy, nginx) the address is the last one the proxy added
to `X-Forwarded-For`. With Docker, a proxy on the host reaches the container
from the Docker gateway rather than loopback: set `NIXMGR_TRUSTED_PROXIES` to
that address (the `remote=` of the `login` lines in the log), or every proxied
login shares one address's limit. List only proxies there, since anything in
that list can claim any client address. The counts are kept in memory.

## Visual editing on an existing repository

The starter flake loads each machine's `managed.json`. In a repository you
bring yourself, add this line to a machine's `modules` list to turn visual
editing on for it (the machine page shows it too):

```nix
(import ./modules/managed.nix ./hosts/<name>/managed.json)
```

and copy `modules/managed.nix` from `templates/fleet/modules/`.

## Development

```sh
cd web && npm ci && npm test && npm run check && npm run build && cd ..
go test -race ./...
go build ./cmd/nixos-manager
```

CI (`.github/workflows/ci.yml`) runs all of this on every branch push and
nightly on `main`: Go, web, the stubbed end-to-end run and ShellCheck, the tests
and the end-to-end run below with real Nix, and `scripts/docker-smoke.sh`, which
builds the Docker image and checks that it runs as its own user (an earlier
image's volume included) with real Nix inside. When a Nix or Docker step fails,
the end of its output is repeated as an annotation on the run's page.

`scripts/e2e.sh` runs the real server against a fresh flake and a bare git
remote: login, lock, check, commit gate, commit, push, edit, history and
restore. It uses `nix` from `PATH`; `E2E_STUB_NIX=1` stubs Nix out.

`NIXMGR_NIX_TESTS=1 go test -timeout 30m ./templates ./nix ./internal/core`
runs the checks that need real Nix (network needed): values set by clicking
win without dropping the hand-written users and SSH settings around them, the
deployment reporter wires into comin, the binary cache module adds the
fleet's cache next to cache.nixos.org, new LXC and VM machines evaluate, a
sysctl shows as a change and kernel parameters as a reboot while a container
never needs one, a check builds the one machine an edit changes and fails on a
system that does not build, a standalone home-manager configuration shows its
packages and programs, and the flake builds the package and passes
`nix flake check`, which also makes and keeps a binary cache key with the
module's script.

`scripts/dev/fake-nix` stands in for Nix on machines without it: it answers
the app's evaluations with a fixture fleet of four machines
(`internal/core/testdata/fleet/`). Run the server with
`NIXMGR_NIX=$PWD/scripts/dev/fake-nix` (and `NIXMGR_FAKE_DELAY=1` to see the
progress states) to work on the UI.

`npm run dev` in `web/` serves the UI with hot reload and proxies `/api` to a
backend on `127.0.0.1:8080`.
