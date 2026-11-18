# nixos-manager

A self-hosted web editor for one git repository of Nix flake configurations
(NixOS hosts, home-manager users, any other flake outputs), with an MCP server
so automation can use the same features.

- Editor with Nix highlighting, `nixd` completion, diagnostics and option docs,
  and `nixfmt` formatting.
- File tree: create, rename, move and delete files and folders.
- Changes: stage, unstage, discard, commit; branches; push and pull.
- History: repository and per-file log, diffs, restore a file from any commit.
- Checks: `nix flake check` plus a dry build of the chosen configurations, run on
  exactly the staged changes, with live output. A commit needs a passing check
  of the staged changes, and a push needs a passing check of the commit being
  pushed. In the web UI you can override both explicitly; MCP clients cannot.
- `nix flake lock` and `nix flake update` for adding or updating inputs.
- Login for the UI; named, revocable API tokens for MCP clients.
- Every commit uses the configured person's name and email.

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
host's Tailscale address. For HTTPS on the tailnet, run
`tailscale serve --bg 8080` on the host.

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

Tools: `list_files`, `read_file`, `write_file`, `create_folder`, `move_path`,
`delete_path`, `format_nix`, `status`, `diff`, `stage`, `unstage`, `discard`,
`commit`, `log`, `show_commit`, `read_file_at`, `restore_file`,
`list_branches`, `create_branch`, `switch_branch`, `push`, `pull`,
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

## Development

```sh
cd web && npm ci && npm test && npm run check && npm run build && cd ..
go test -race ./...
go build ./cmd/nixos-manager
```

`scripts/e2e.sh` runs the real server against a fresh flake and a bare git
remote: login, lock, check, commit gate, commit, push, edit, history and
restore. It uses `nix` from `PATH`; `E2E_STUB_NIX=1` stubs Nix out.

`npm run dev` in `web/` serves the UI with hot reload and proxies `/api` to a
backend on `127.0.0.1:8080`.
