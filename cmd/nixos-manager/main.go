// Command nixos-manager serves the web editor and MCP server for a Nix flake repository.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/TheOlympusCloud/nixos-manager/internal/auth"
	"github.com/TheOlympusCloud/nixos-manager/internal/core"
	"github.com/TheOlympusCloud/nixos-manager/internal/mcpserver"
	"github.com/TheOlympusCloud/nixos-manager/internal/server"
	"github.com/TheOlympusCloud/nixos-manager/templates"
	"github.com/TheOlympusCloud/nixos-manager/web"
)

var version = "dev"

const usage = `nixos-manager: web editor and MCP server for a Nix flake repository.

Usage:
  nixos-manager                 run the server (configured by environment variables)
  nixos-manager hash-password   read a password on stdin and print its hash for NIXMGR_PASSWORD_HASH
  nixos-manager version

Environment:
  NIXMGR_AUTHOR_NAME, NIXMGR_AUTHOR_EMAIL   commit identity (required)
  NIXMGR_PASSWORD_HASH or _FILE             bcrypt hash of the login password (required)
  NIXMGR_DATA      state folder (default /var/lib/nixos-manager)
  NIXMGR_REPO      working copy (default $NIXMGR_DATA/repo)
  NIXMGR_REMOTE    git remote to clone from and push to (optional; also settable in Settings)
  NIXMGR_LISTEN    address to listen on (default 127.0.0.1:8080)
  NIXMGR_NIX, NIXMGR_NIXFMT, NIXMGR_NIXD   tool commands (default nix, nixfmt, nixd)
  NIXMGR_EVAL_JOBS machines evaluated at once (default 2, or 1 on a single CPU; about 1 GB each)
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hash-password":
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			h, err := auth.HashPassword(strings.TrimRight(line, "\r\n"))
			if err != nil {
				fail(err)
			}
			fmt.Println(h)
			return
		case "version":
			fmt.Println(version)
			return
		default:
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
	}
	if err := run(); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "nixos-manager:", err)
	os.Exit(1)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func run() error {
	name, email := os.Getenv("NIXMGR_AUTHOR_NAME"), os.Getenv("NIXMGR_AUTHOR_EMAIL")
	if name == "" || email == "" {
		return errors.New("set NIXMGR_AUTHOR_NAME and NIXMGR_AUTHOR_EMAIL: every commit is made as this person")
	}
	hash := os.Getenv("NIXMGR_PASSWORD_HASH")
	if f := os.Getenv("NIXMGR_PASSWORD_HASH_FILE"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		hash = strings.TrimSpace(string(b))
	}
	proxies, err := server.ParseTrustedProxies(os.Getenv("NIXMGR_TRUSTED_PROXIES"))
	if err != nil {
		return fmt.Errorf("NIXMGR_TRUSTED_PROXIES: %w", err)
	}
	evalJobs := 0 // the default
	if v := os.Getenv("NIXMGR_EVAL_JOBS"); v != "" {
		if evalJobs, err = strconv.Atoi(v); err != nil || evalJobs < 1 {
			return fmt.Errorf("NIXMGR_EVAL_JOBS: want how many machines to evaluate at once, 1 or more, not %q", v)
		}
	}
	data := env("NIXMGR_DATA", "/var/lib/nixos-manager")
	if err := os.MkdirAll(data, 0o700); err != nil {
		return err
	}
	a, err := auth.New(hash, filepath.Join(data, "tokens.json"))
	if err != nil {
		return err
	}
	if err := a.PersistSessions(filepath.Join(data, "sessions.json")); err != nil {
		return err
	}
	repo, err := core.OpenRepoSSH(env("NIXMGR_REPO", filepath.Join(data, "repo")), os.Getenv("NIXMGR_REMOTE"), name, email, filepath.Join(data, "ssh"))
	if err != nil {
		return fmt.Errorf("opening the repository: %w", err)
	}
	if seeded, err := repo.Seed(templates.Fleet); err != nil {
		return err
	} else if seeded {
		slog.Info("empty repository: wrote the starter flake (not committed yet)", "dir", repo.Dir)
	}
	svc := core.NewService(repo)
	svc.Jobs.Nix, svc.Jobs.Nixfmt = env("NIXMGR_NIX", "nix"), env("NIXMGR_NIXFMT", "nixfmt")
	if evalJobs > 0 {
		svc.Jobs.EvalJobs = evalJobs
	}
	if err := svc.Jobs.PersistChecks(filepath.Join(data, "checks.json")); err != nil {
		return err
	}
	svc.Jobs.GCRoots = filepath.Join(data, "gcroots") // each machine's last system a check built
	if svc.Deploys, err = core.OpenDeployments(data, repo); err != nil {
		return err
	}
	svc.Deploys.Systems = svc.Jobs             // what each machine should run, to compare with what it says it runs
	svc.Secrets = core.OpenSecrets(data, repo) // sops, age-keygen and ssh-to-age from PATH
	notifier, err := core.OpenNotifier(data, svc)
	if err != nil {
		return err
	}

	forgejo := &core.Forgejo{Repo: repo, DataDir: data, EnvRemote: os.Getenv("NIXMGR_REMOTE"), Jobs: svc.Jobs}
	if err := forgejo.Resume(); err != nil { // checks the pushed commits that waited when it stopped
		return err
	}

	s := &server.Server{
		Svc: svc, Auth: a, UI: web.UI,
		MCP:  mcpserver.Handler(svc, version),
		Nixd: strings.Fields(env("NIXMGR_NIXD", "nixd")),

		Forgejo:        forgejo,
		Notify:         notifier,
		TrustedProxies: proxies,
	}
	// No write timeout: job streams, the editor socket and MCP calls are long-lived.
	hs := &http.Server{Addr: env("NIXMGR_LISTEN", "127.0.0.1:8080"), Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go notifier.Run(ctx)
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hs.Shutdown(sctx)
	}()
	slog.Info("nixos-manager listening", "addr", hs.Addr, "repo", repo.Dir, "version", version)
	if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
