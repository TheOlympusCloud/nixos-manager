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
  unsupported NixOS releases, and packages on every machine but one. Most
  findings have a **Fix** button that applies the setting to every affected
  machine.
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
  changes, and which inputs moved. History shows the same for every commit.
- **Checks**: `nix flake check` plus a dry build of the chosen configurations,
  run on exactly the staged changes, with live output. A commit needs a passing
  check of the staged changes, and a push needs a passing check of the commit
  being pushed. In the web UI you can override both explicitly; MCP clients
  cannot.
- **Git**: stage, commit, branches, push and pull, revert a commit, merge a
  `testing-<host>` branch, repository and per-file history with diffs and
  restore. Every commit uses the configured person's name and email.
- **Code**: a Nix editor with `nixd` completion, diagnostics and option docs,
  and `nixfmt`, for the hand-written files.
- **Command palette** (Ctrl/Cmd+K): jump to any machine, package, service,
  option, file or action.
- Login for the UI; named, revocable API tokens for MCP clients.

The repository is a normal git repository whose remote is Forgejo. Each machine
runs [comin](https://github.com/nlewo/comin), pulls from Forgejo and switches
to `nixosConfigurations.<its hostname>`.

## Run it with Docker Compose

1. Pick a login password and store its hash:

   ```sh
   mkdir -p secrets
   printf '%s\n' 'your password' | docker compose run --rm -T --build nixos-manager hash-password > secrets/password-hash
   ```

2. Edit `compose.yaml`: set `NIXMGR_AUTHOR_NAME` and `NIXMGR_AUTHOR_EMAIL` (the
   identity of every commit) and, once the Forgejo repository exists,
   `NIXMGR_REMOTE`.
3. `docker compose up -d --build`, then open `http://<server>:8080`.

Without `NIXMGR_REMOTE` the app starts a new repository with a starter flake
(an example host, a home-manager user and comin). Run **Lock** once to create
`flake.lock`, then stage, check and commit. To connect Forgejo later, set
`NIXMGR_REMOTE` and restart: it is added as `origin`, and **Push** publishes the
branch.

### Forgejo access

- SSH (recommended): create a deploy key with write access on the Forgejo
  repository and mount the private key and a `known_hosts` file at `/root/.ssh`
  (see the commented line in `compose.yaml`).
- HTTPS: put `https://<user>:<token>@forgejo.lan` in a git credentials file,
  mount it, and add `GIT_CONFIG_COUNT=1`, `GIT_CONFIG_KEY_0=credential.helper`,
  `GIT_CONFIG_VALUE_0=store --file=/run/secrets/nixmgr/git-credentials` to the
  environment. Do not put the token in `NIXMGR_REMOTE`.

### LAN and Tailscale

The port is published on the host, so it is reachable on the LAN and on the
host's Tailscale address. On the LAN it is plain HTTP: the password and the
session cookie cross the LAN unencrypted. Tailscale traffic is encrypted, and
`tailscale serve --bg 8080` on the host adds HTTPS on the tailnet. If the LAN is
not trusted, publish the port on loopback only (`"127.0.0.1:8080:8080"`) and use
it through Tailscale.

## Using it

1. Edit files; **Ctrl+S** saves, **Shift+Alt+F** formats.
2. In **Changes**, stage what you want to commit.
3. In **Checks**, pick the configurations to dry-build and run the check.
4. Commit with a message, then **Push**. comin deploys on each machine.

When **Pull** rebases your local commits onto new remote commits, the result
was never checked as a whole, so **Push** refuses it: run the checks again with
nothing staged or changed, then push.

A branch named `testing-<hostname>` is deployed by comin to that one machine
only, for trying a change before merging it into `main`.

Never store secrets in the repository; use sops-nix or agenix.

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

## Configuration

| Variable | Meaning |
|---|---|
| `NIXMGR_AUTHOR_NAME`, `NIXMGR_AUTHOR_EMAIL` | Commit identity (required) |
| `NIXMGR_PASSWORD_HASH` or `NIXMGR_PASSWORD_HASH_FILE` | bcrypt hash of the login password (required) |
| `NIXMGR_DATA` | State folder: working copy and tokens (default `/var/lib/nixos-manager`) |
| `NIXMGR_REPO` | Working copy (default `$NIXMGR_DATA/repo`) |
| `NIXMGR_REMOTE` | Remote to clone from and push to |
| `NIXMGR_LISTEN` | Listen address (default `127.0.0.1:8080`; `0.0.0.0:8080` in the image) |
| `NIXMGR_NIX`, `NIXMGR_NIXFMT`, `NIXMGR_NIXD` | Tool commands (default `nix`, `nixfmt`, `nixd`) |

Sessions last 7 days and are kept in memory, so a restart logs you out.

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

`scripts/e2e.sh` runs the real server against a fresh flake and a bare git
remote: login, lock, check, commit gate, commit, push, edit, history and
restore. It uses `nix` from `PATH`; `E2E_STUB_NIX=1` stubs Nix out.

`scripts/dev/fake-nix` stands in for Nix on machines without it: it answers
the app's evaluations with a fixture fleet of four machines
(`internal/core/testdata/fleet/`). Run the server with
`NIXMGR_NIX=$PWD/scripts/dev/fake-nix` (and `NIXMGR_FAKE_DELAY=1` to see the
progress states) to work on the UI.

`npm run dev` in `web/` serves the UI with hot reload and proxies `/api` to a
backend on `127.0.0.1:8080`.
