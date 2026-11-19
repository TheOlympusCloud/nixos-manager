package core

import (
	"cmp"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Forgejo is the connection to the server that hosts the fleet repository: the
// app's own SSH deploy key, the remote, connection tests and the push webhook.
type Forgejo struct {
	Repo      *Repo
	DataDir   string // holds the webhook secret; the deploy key lives in Repo.SSHDir
	EnvRemote string // NIXMGR_REMOTE, which sets origin again at every start

	mu      sync.Mutex
	last    *Delivery
	pending bool
}

// hookDelay gathers a burst of pushes into one fetch.
var hookDelay = 2 * time.Second

const (
	keyName     = "id_ed25519"
	testTimeout = 30 * time.Second
	unrelated   = "the working copy and the remote repository have separate histories (no commit in common), so they cannot be combined safely, and nothing was changed. To keep this working copy, point the remote at a new, empty repository and push. To use the remote's repository instead, move the working copy (NIXMGR_REPO) aside, restart nixos-manager and connect again"
)

// ---- SSH ---------------------------------------------------------------------

// shellQuote quotes s for the shell that runs GIT_SSH_COMMAND.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// sshCommand is git's ssh: the app's deploy key in dir when it exists, else ssh's
// own defaults (~/.ssh, the agent). Never interactive, and new hosts are recorded.
func sshCommand(dir string) string {
	cmd := "ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=20"
	if dir == "" {
		return cmd
	}
	key := filepath.Join(dir, keyName)
	if _, err := os.Stat(key); err != nil {
		return cmd
	}
	return cmd + " -i " + shellQuote(key) + " -o IdentitiesOnly=yes -o " + shellQuote("UserKnownHostsFile="+filepath.Join(dir, "known_hosts")) + " -o HashKnownHosts=no"
}

// generateKey writes a new ed25519 key pair: file (0600, OpenSSH format) and file.pub.
// It never replaces an existing key.
func generateKey(file, comment string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	block, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(pem.EncodeToMemory(block)); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(file)
		return err
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		return err
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sp))) + " " + comment + "\n"
	return os.WriteFile(file+".pub", []byte(line), 0o644)
}

// readPublicKey derives the public key from the private key file.
func readPublicKey(file string) (ssh.PublicKey, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(b)
	if err != nil {
		return nil, fmt.Errorf("reading the deploy key %s: %w", file, err)
	}
	return signer.PublicKey(), nil
}

// homeKey reports whether ~/.ssh holds a private key of its own (the mounted-key setup).
func homeKey() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	ms, _ := filepath.Glob(filepath.Join(home, ".ssh", "id_*"))
	for _, m := range ms {
		if !strings.HasSuffix(m, ".pub") {
			return true
		}
	}
	return false
}

// DeployKey is the public half of the app's SSH key; the private half never leaves the data dir.
type DeployKey struct {
	Public      string `json:"public"`
	Fingerprint string `json:"fingerprint"`
}

// SSHMode says which SSH identity git uses: "env" (GIT_SSH_COMMAND is set), "app"
// (the deploy key managed here), "home" (a key in ~/.ssh) or "none" yet.
func (f *Forgejo) SSHMode() string {
	switch {
	case os.Getenv("GIT_SSH_COMMAND") != "":
		return "env"
	case fileExists(filepath.Join(f.Repo.SSHDir, keyName)):
		return "app"
	case homeKey():
		return "home"
	}
	return "none"
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Key returns the deploy key, creating it first when create is set and there is none.
// It returns nil when there is no key and create is false.
func (f *Forgejo) Key(create bool) (*DeployKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	file := filepath.Join(f.Repo.SSHDir, keyName)
	if !fileExists(file) {
		if !create {
			return nil, nil
		}
		if err := f.outsideRepo(f.Repo.SSHDir); err != nil {
			return nil, err
		}
		host, _ := os.Hostname()
		if err := generateKey(file, "nixos-manager@"+host); err != nil {
			return nil, err
		}
	}
	pub, err := readPublicKey(file)
	if err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	return &DeployKey{Public: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))) + " nixos-manager@" + host, Fingerprint: ssh.FingerprintSHA256(pub)}, nil
}

// outsideRepo refuses to keep a secret inside the working copy, where it could be read, committed and pushed.
func (f *Forgejo) outsideRepo(p string) error {
	abs, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	if rel, err := filepath.Rel(f.Repo.Dir, abs); err == nil && filepath.IsLocal(rel) || abs == f.Repo.Dir {
		return errorf(ErrInvalid, "%s is inside the working copy; set NIXMGR_DATA and NIXMGR_REPO so that secrets stay outside it", p)
	}
	return nil
}

// HostKey is the SSH host key recorded on first contact with host:port, e.g.
// "ssh-ed25519 SHA256:…", or "" when none is recorded in the app's known_hosts.
func (f *Forgejo) HostKey(host, port string) string {
	if host == "" || f.Repo.SSHDir == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(f.Repo.SSHDir, "known_hosts"))
	if err != nil {
		return ""
	}
	if port == "" {
		port = "22"
	}
	want := knownhosts.Normalize(net.JoinHostPort(host, port))
	for len(b) > 0 {
		_, hosts, key, _, rest, err := ssh.ParseKnownHosts(b)
		if err != nil {
			return ""
		}
		for _, h := range hosts {
			if h == want {
				return key.Type() + " " + ssh.FingerprintSHA256(key)
			}
		}
		b = rest
	}
	return ""
}

// ForgejoState is everything Settings → Forgejo shows. Only the public key is in it.
type ForgejoState struct {
	Remote       string     `json:"remote"`     // origin, any password hidden
	EnvRemote    bool       `json:"env_remote"` // NIXMGR_REMOTE sets origin again at every start
	Kind         string     `json:"kind"`       // ssh, https, http, local, or "" when unset or unusual
	Web          string     `json:"web"`        // the repository's page
	WebBase      string     `json:"web_base"`   // the override of the web address, if any
	SSH          string     `json:"ssh"`        // see SSHMode
	Key          *DeployKey `json:"key"`
	HostKey      string     `json:"host_key"`
	KnownHosts   string     `json:"known_hosts"` // the app's known_hosts file, to remove a changed host key
	HookSecret   string     `json:"hook_secret"`
	LastDelivery *Delivery  `json:"last_delivery"`
}

// State gathers the Forgejo settings. Opening them is the deploy key's "first
// need": it is created here unless git already has another SSH setup or the
// remote does not use SSH.
func (f *Forgejo) State(ctx context.Context) (ForgejoState, error) {
	remote := f.Repo.originURL(ctx)
	u, _ := ParseRemote(remote)
	st := ForgejoState{Remote: redact(remote), EnvRemote: f.EnvRemote != "", Kind: u.Kind, Web: f.Repo.WebURL(ctx), WebBase: f.Repo.WebBase(ctx), SSH: f.SSHMode()}
	var err error
	if st.Key, err = f.Key(st.SSH == "none" && (remote == "" || u.Kind == "ssh")); err != nil {
		return st, err
	}
	st.SSH = f.SSHMode()
	if st.SSH == "app" {
		st.KnownHosts = filepath.Join(f.Repo.SSHDir, "known_hosts")
		if u.Kind == "ssh" {
			st.HostKey = f.HostKey(u.Host, u.Port)
		}
	} else if home, err := os.UserHomeDir(); err == nil && st.SSH == "home" {
		st.KnownHosts = filepath.Join(home, ".ssh", "known_hosts")
	}
	st.HookSecret, err = f.HookSecret()
	st.LastDelivery = f.LastDelivery()
	return st, err
}

// ---- remote URLs ----------------------------------------------------------------

// RemoteURL is a parsed git remote. Kind is "ssh", "https", "http" or "local".
type RemoteURL struct {
	Kind string
	Host string
	Port string
	Path string // owner/repo, without .git
}

var scpLike = regexp.MustCompile(`^(?:[A-Za-z0-9._-]+@)?([A-Za-z0-9][A-Za-z0-9.-]*):(.+)$`)

// ParseRemote validates a remote the way the settings accept it: ssh://,
// git@host:path, https:// (or http://), and file:// or an absolute path for a
// repository on this server. Credentials in the URL are refused.
func ParseRemote(s string) (RemoteURL, error) {
	bad := func(why string) (RemoteURL, error) {
		return RemoteURL{}, errorf(ErrInvalid, "%s. Use ssh://git@host:2222/owner/repo.git, git@host:owner/repo.git or https://host/owner/repo.git", why)
	}
	if s == "" {
		return bad("enter the repository's URL")
	}
	if strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return bad("the URL contains spaces or control characters")
	}
	if strings.HasPrefix(s, "-") || strings.Contains(s, "::") {
		return bad("that is not a repository URL")
	}
	if strings.HasPrefix(s, "/") {
		return RemoteURL{Kind: "local", Path: s}, nil
	}
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return bad("the URL does not parse")
		}
		if _, has := u.User.Password(); has {
			return RemoteURL{}, errorf(ErrInvalid, "remove the password or token from the URL: it would be stored in plain text and shown in the logs. Use the deploy key over SSH, or a git credential helper for HTTPS (see the README)")
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return bad("the URL has a query or fragment")
		}
		switch u.Scheme {
		case "file":
			if u.Host != "" || u.Path == "" {
				return bad("a file:// URL needs an absolute path")
			}
			return RemoteURL{Kind: "local", Path: u.Path}, nil
		case "ssh", "https", "http":
		default:
			return bad("unsupported scheme " + u.Scheme)
		}
		if u.Scheme != "ssh" && u.User != nil {
			return RemoteURL{}, errorf(ErrInvalid, "remove the user name from the HTTPS URL; give the credentials to a git credential helper instead (see the README)")
		}
		p := repoPath(u.Path)
		if u.Hostname() == "" || p == "" {
			return bad("the URL needs a host and a repository path")
		}
		return RemoteURL{Kind: u.Scheme, Host: u.Hostname(), Port: u.Port(), Path: p}, nil
	}
	m := scpLike.FindStringSubmatch(s)
	if m == nil || strings.Contains(m[2], "@") || repoPath(m[2]) == "" {
		return bad("that is not a repository URL")
	}
	return RemoteURL{Kind: "ssh", Host: m[1], Path: repoPath(m[2])}, nil
}

func repoPath(p string) string {
	return strings.TrimSuffix(strings.Trim(p, "/"), ".git")
}

// webURL derives the repository's web page from its remote: SSH remotes become
// https://host/owner/repo (the SSH port is not the web port); HTTP(S) remotes
// keep their scheme and port. base, when set, replaces scheme, host and port.
func webURL(remote, base string) string {
	u, err := ParseRemote(remote)
	if err != nil || u.Kind == "local" {
		return ""
	}
	if base != "" {
		return strings.TrimRight(base, "/") + "/" + u.Path
	}
	host := u.Host
	if u.Kind != "ssh" && u.Port != "" {
		host = net.JoinHostPort(u.Host, u.Port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	scheme := "https"
	if u.Kind == "http" {
		scheme = "http"
	}
	return scheme + "://" + host + "/" + u.Path
}

// checkWebBase validates the web address override: http(s), a host, nothing secret.
func checkWebBase(s string) error {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errorf(ErrInvalid, "enter the address you open Forgejo at, like https://forgejo.lan or http://192.168.1.10:3000")
	}
	return nil
}

// originURL is origin's URL as configured (get-url would apply insteadOf rewrites).
func (r *Repo) originURL(ctx context.Context) string {
	out, _ := r.git(ctx, "config", "--get", "remote.origin.url")
	return strings.TrimSpace(out)
}

// WebURL is the repository's page on Forgejo, or "" when the remote has none.
func (r *Repo) WebURL(ctx context.Context) string {
	base, _ := r.git(ctx, "config", "--get", "nixmgr.weburl")
	return webURL(r.originURL(ctx), strings.TrimSpace(base))
}

// WebBase is the web address override ("" when derived from the remote).
func (r *Repo) WebBase(ctx context.Context) string {
	base, _ := r.git(ctx, "config", "--get", "nixmgr.weburl")
	return strings.TrimSpace(base)
}

// SetWebBase stores the web address override in the repository's git config; "" removes it.
func (r *Repo) SetWebBase(ctx context.Context, base string) error {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	r.mu.Lock()
	defer r.mu.Unlock()
	if base == "" {
		r.git(ctx, "config", "--unset", "nixmgr.weburl") // exit 5 when it was not set
		return nil
	}
	if err := checkWebBase(base); err != nil {
		return err
	}
	_, err := r.git(ctx, "config", "nixmgr.weburl", base)
	return err
}

// SetRemote makes url origin, in the repository's own git config. NIXMGR_REMOTE,
// when set, replaces it again at the next start.
func (r *Repo) SetRemote(ctx context.Context, remote string) error {
	remote = strings.TrimSpace(remote)
	if _, err := ParseRemote(remote); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.git(ctx, "remote", "get-url", "origin"); err != nil {
		_, err = r.git(ctx, "remote", "add", "origin", remote)
		return err
	}
	_, err := r.git(ctx, "remote", "set-url", "origin", remote)
	return err
}

// ---- connection test -----------------------------------------------------------

// Check is the result of testing the connection, in plain words.
type Check struct {
	// connected, empty, readonly, unreachable, auth, not_found, host_key_changed, no_remote or error
	Result   string   `json:"result"`
	Message  string   `json:"message"`
	Detail   string   `json:"detail,omitempty"` // git's own words
	Branches []string `json:"branches"`
	HostKey  string   `json:"host_key,omitempty"`
	// What the working copy should do next: "push" (the remote lacks the branch),
	// "adopt" (no local commits; take the remote's), "unrelated" (refused), or "".
	Next string `json:"next,omitempty"`
}

// classify turns git's error output into a result and a plain explanation.
func classify(stderr, host string) (string, string) {
	s := strings.ToLower(stderr)
	has := func(words ...string) bool {
		return slices.ContainsFunc(words, func(w string) bool { return strings.Contains(s, w) })
	}
	switch {
	case has("remote host identification has changed", "host key verification failed"):
		return "host_key_changed", fmt.Sprintf("%s's SSH host key is not the one recorded on first contact. If Forgejo was reinstalled or moved, remove the old key (below) and test again; if not, someone may be intercepting the connection.", host)
	case has("could not resolve", "name or service not known", "temporary failure in name resolution"):
		return "unreachable", fmt.Sprintf("The name %s does not resolve from this server. Check the address, or use the IP address.", host)
	case has("connection refused"):
		return "unreachable", fmt.Sprintf("%s refused the connection: nothing listens on that port. Check the port (Forgejo's SSH is often 2222) and that Forgejo runs.", host)
	case has("timed out", "no route to host", "network is unreachable", "connection reset"):
		return "unreachable", fmt.Sprintf("%s does not answer from this server. Check the address, the firewall, and that Forgejo runs.", host)
	case has("permission denied", "authentication failed", "could not read username", "could not read password", "terminal prompts disabled", "unauthorized", "invalid credentials", "returned error: 401", "returned error: 403"):
		return "auth", "Forgejo refused this server's key. Add the deploy key below to the repository (Settings → Deploy keys), with write access."
	case has("not found", "does not exist", "does not appear to be a git repository", "no such file or directory"):
		return "not_found", "The server answered, but there is no repository at that path. Check the owner and the repository name, or create the repository first."
	}
	return "error", "git could not reach the repository; its explanation is below."
}

// Test checks the remote with git ls-remote and says what the working copy can do next.
// It fetches when the local branch has commits, so ahead and behind are current.
func (f *Forgejo) Test(ctx context.Context) (c Check) {
	r := f.Repo
	c.Branches = []string{}
	remote := r.originURL(ctx)
	if remote == "" {
		c.Result, c.Message = "no_remote", "No remote is set yet. Enter the repository's URL above."
		return c
	}
	u, _ := ParseRemote(remote) // NIXMGR_REMOTE may not parse; the test still runs
	if u.Kind == "ssh" && f.SSHMode() == "app" {
		defer func() { c.HostKey = f.HostKey(u.Host, u.Port) }()
	}
	tctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	out, errOut, err := r.gitRun(tctx, nil, nil, "ls-remote", "--heads", "origin")
	if err != nil {
		if tctx.Err() != nil {
			errOut += "\nconnection timed out"
		}
		c.Result, c.Message = classify(errOut, cmp.Or(u.Host, "the server"))
		if c.Result == "auth" && (u.Kind == "https" || u.Kind == "http") {
			c.Message = "Forgejo refused the credentials. Check the user and token that git's credential helper gives for this server (see the README)."
		}
		c.Detail = strings.ReplaceAll(strings.TrimSpace(errOut), remote, redact(remote))
		return c
	}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if _, ref, ok := strings.Cut(l, "\t"); ok {
			c.Branches = append(c.Branches, strings.TrimPrefix(ref, "refs/heads/"))
		}
	}
	// Writing needs more than reading: ask the write side for its refs, which a read-only key may not do.
	if u.Kind == "ssh" || u.Kind == "local" {
		if _, errOut, err := r.gitRun(tctx, nil, nil, "ls-remote", "--upload-pack=git-receive-pack", "origin"); err != nil {
			c.Result, c.Detail = "readonly", strings.TrimSpace(errOut)
			c.Message = "Connected, but this server may only read: Forgejo refused it write access. Edit the deploy key (repository → Settings → Deploy keys) and tick “Enable write access”."
			return c
		}
	}
	c.Result = "connected"
	c.Message, c.Next = f.relation(ctx, c.Branches)
	if len(c.Branches) == 0 {
		c.Result = "empty"
	}
	return c
}

// relation compares the working copy with the remote's branches.
func (f *Forgejo) relation(ctx context.Context, branches []string) (string, string) {
	r := f.Repo
	r.mu.Lock()
	defer r.mu.Unlock()
	branch, _ := r.git(ctx, "symbolic-ref", "--short", "-q", "HEAD")
	branch = strings.TrimSpace(branch)
	head := r.hasHead(ctx)
	switch {
	case len(branches) == 0 && head:
		return "Connected. The repository is empty: Push sends this fleet there.", "push"
	case len(branches) == 0:
		return "Connected. The repository is empty: commit the starter fleet in Changes, then Push sends it there.", "push"
	case !head:
		return fmt.Sprintf("Connected. The repository already has a history (%s), and this working copy has no commits of its own yet. Use the repository's files instead of the starter fleet?", strings.Join(branches, ", ")), "adopt"
	}
	nctx, cancel := context.WithTimeout(ctx, netTimeout)
	defer cancel()
	if _, err := r.git(nctx, "fetch", "-q", "origin"); err != nil {
		return "Connected, but fetching failed: " + err.Error(), ""
	}
	up := "refs/remotes/origin/" + branch
	if _, err := r.git(ctx, "rev-parse", "--verify", "-q", up); err != nil {
		return fmt.Sprintf("Connected. %s is not on the remote yet: Push creates it there.", branch), "push"
	}
	if !r.related(ctx, up) {
		return "Connected, but " + unrelated + ".", "unrelated"
	}
	return "Connected. Pull and Push work.", ""
}

// related reports whether HEAD and rev share history. Needs r.mu.
func (r *Repo) related(ctx context.Context, rev string) bool {
	_, err := r.git(ctx, "merge-base", "HEAD", rev)
	return err == nil
}

// AdoptRemote replaces a working copy that has no commits yet (the starter fleet,
// committed or not) with the remote's branch. Nothing committed can be lost: it
// refuses when there are local commits. The files it replaces stay in git as
// refs/nixmgr/starter-backup.
func (r *Repo) AdoptRemote(ctx context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hasHead(ctx) {
		return "", errorf(ErrConflict, "this working copy already has commits; use Pull to bring in the remote's")
	}
	if _, err := r.git(ctx, "remote", "get-url", "origin"); err != nil {
		return "", errorf(ErrInvalid, "no remote is configured")
	}
	nctx, cancel := context.WithTimeout(ctx, netTimeout)
	defer cancel()
	if _, err := r.git(nctx, "fetch", "-q", "origin"); err != nil {
		return "", err
	}
	cur, _ := r.git(ctx, "symbolic-ref", "--short", "-q", "HEAD")
	refs, err := r.git(ctx, "for-each-ref", "--format=%(refname:lstrip=3)", "refs/remotes/origin/")
	if err != nil {
		return "", err
	}
	var remote []string
	for _, b := range strings.Fields(refs) {
		if b != "HEAD" {
			remote = append(remote, b)
		}
	}
	branch := ""
	for _, want := range []string{strings.TrimSpace(cur), "main", "master"} {
		if want != "" && slices.Contains(remote, want) {
			branch = want
			break
		}
	}
	if branch == "" && len(remote) == 1 {
		branch = remote[0]
	}
	if branch == "" {
		if len(remote) == 0 {
			return "", errorf(ErrInvalid, "the remote repository is empty; push this fleet there instead")
		}
		return "", errorf(ErrInvalid, "the remote has several branches (%s) and none is main", strings.Join(remote, ", "))
	}
	backup, err := r.backupWorkingTree(ctx)
	if err != nil {
		return "", err
	}
	if _, err := r.git(ctx, "checkout", "-q", "-f", "-B", branch, "--track", "origin/"+branch); err != nil {
		return "", err
	}
	if _, err := r.git(ctx, "clean", "-f", "-d", "-q"); err != nil { // starter files the remote does not have
		return "", err
	}
	n, _ := r.git(ctx, "rev-list", "--count", "HEAD")
	msg := fmt.Sprintf("The working copy is now %s from the remote (%s commits).", branch, strings.TrimSpace(n))
	if backup {
		msg += " The files it replaced are kept in git as refs/nixmgr/starter-backup."
	}
	return msg, nil
}

// backupWorkingTree records every file of the working copy (untracked ones too) as
// a commit under refs/nixmgr/starter-backup, through a private index. Needs r.mu.
func (r *Repo) backupWorkingTree(ctx context.Context) (bool, error) {
	idx := filepath.Join(r.Dir, ".git", "nixmgr-backup.index")
	defer os.Remove(idx)
	env := []string{"GIT_INDEX_FILE=" + idx}
	if _, _, err := r.gitRun(ctx, env, nil, "add", "-A", "--", "."); err != nil {
		return false, err
	}
	tree, _, err := r.gitRun(ctx, env, nil, "write-tree")
	if err != nil {
		return false, err
	}
	tree = strings.TrimSpace(tree)
	if tree == "4b825dc642cb6eb9a060e54bf8d69288fbee4904" { // the empty tree: nothing to keep
		return false, nil
	}
	sha, _, err := r.gitRun(ctx, nil, []byte("Working copy before it was replaced by the remote repository\n"), "commit-tree", tree)
	if err != nil {
		return false, err
	}
	_, err = r.git(ctx, "update-ref", "refs/nixmgr/starter-backup", strings.TrimSpace(sha))
	return err == nil, err
}

// ---- webhook --------------------------------------------------------------------

// Delivery is the last webhook call, as Settings shows it.
type Delivery struct {
	Time   time.Time `json:"time"`
	Event  string    `json:"event"`
	OK     bool      `json:"ok"`
	Result string    `json:"result"`
}

// VerifySignature checks a hex HMAC-SHA256 of body (as Forgejo and Gitea send it,
// optionally prefixed "sha256=") in constant time.
func VerifySignature(secret, body []byte, sig string) bool {
	got, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(sig), "sha256="))
	if err != nil || len(secret) == 0 {
		return false
	}
	m := hmac.New(sha256.New, secret)
	m.Write(body)
	return hmac.Equal(m.Sum(nil), got)
}

// HookSecret returns the webhook secret, creating it (0600, in the data dir) on first use.
func (f *Forgejo) HookSecret() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	file := filepath.Join(f.DataDir, "webhook-secret")
	if b, err := os.ReadFile(file); err == nil && strings.TrimSpace(string(b)) != "" {
		return strings.TrimSpace(string(b)), nil
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := f.outsideRepo(f.DataDir); err != nil {
		return "", err
	}
	secret := rand.Text() + rand.Text()
	if err := os.MkdirAll(f.DataDir, 0o700); err != nil {
		return "", err
	}
	return secret, os.WriteFile(file, []byte(secret+"\n"), 0o600)
}

// LastDelivery is the last webhook call, or nil.
func (f *Forgejo) LastDelivery() *Delivery {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.last == nil {
		return nil
	}
	d := *f.last
	return &d
}

func (f *Forgejo) record(event string, ok bool, result string) {
	f.mu.Lock()
	f.last = &Delivery{Time: time.Now().UTC(), Event: event, OK: ok, Result: result}
	f.mu.Unlock()
}

// Hook handles a webhook call whose body was already size-capped. It answers the
// HTTP status and a short result. Only a correctly signed push to the current
// branch does anything: a fetch shortly after, and a fast-forward when safe.
func (f *Forgejo) Hook(event, sig string, body []byte) (int, string) {
	secret, err := f.HookSecret()
	if err != nil {
		return 500, "no webhook secret"
	}
	if !VerifySignature([]byte(secret), body, sig) {
		// Logged, not shown as the last delivery: anyone who reaches the port could overwrite it.
		slog.Warn("webhook refused: the signature does not match the secret in Settings", "event", event)
		return 401, "bad signature"
	}
	switch event {
	case "ping":
		f.record(event, true, "Ping received.")
		return 200, "pong"
	case "push":
	default:
		f.record(event, true, "Ignored: only push events are used.")
		return 200, "ignored"
	}
	var p struct {
		Ref string `json:"ref"`
	}
	if err := json.Unmarshal(body, &p); err != nil || p.Ref == "" {
		f.record(event, false, "Refused: the body is not a push event.")
		return 400, "not a push event"
	}
	cur, _ := f.Repo.git(context.Background(), "symbolic-ref", "--short", "-q", "HEAD")
	if branch := strings.TrimSpace(cur); p.Ref != "refs/heads/"+branch {
		f.record(event, true, fmt.Sprintf("Ignored: a push to %s; this console is on %s.", strings.TrimPrefix(p.Ref, "refs/heads/"), branch))
		return 200, "ignored"
	}
	f.record(event, true, fmt.Sprintf("Push to %s received; fetching.", strings.TrimPrefix(p.Ref, "refs/heads/")))
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.pending {
		f.pending = true
		time.AfterFunc(hookDelay, f.sync)
	}
	return 202, "fetching"
}

func (f *Forgejo) sync() {
	f.mu.Lock()
	f.pending = false // a push arriving from now on gets its own fetch
	f.mu.Unlock()
	msg, err := f.Repo.FetchForward(context.Background())
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.last != nil {
		f.last.OK = err == nil
		if err != nil {
			f.last.Result = "Fetch failed: " + err.Error()
		} else {
			f.last.Result = msg
		}
	}
}

// FetchForward fetches origin and fast-forwards the current branch when that
// cannot lose anything: no uncommitted changes to tracked files and no local
// commits. Otherwise the new commits wait for Pull.
func (r *Repo) FetchForward(ctx context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	branch, err := r.currentBranch(ctx)
	if err != nil {
		return "", err
	}
	nctx, cancel := context.WithTimeout(ctx, netTimeout)
	defer cancel()
	if _, err := r.git(nctx, "fetch", "-q", "origin"); err != nil {
		return "", err
	}
	up := "refs/remotes/origin/" + branch
	if _, err := r.git(ctx, "rev-parse", "--verify", "-q", up); err != nil {
		return fmt.Sprintf("Fetched; %s is not on the remote.", branch), nil
	}
	if !r.hasHead(ctx) {
		return "Fetched; this working copy has no commits yet (see Settings).", nil
	}
	if !r.related(ctx, up) {
		return "Fetched; the remote's history is unrelated to this working copy (see Settings).", nil
	}
	counts, err := r.git(ctx, "rev-list", "--left-right", "--count", "HEAD..."+up)
	if err != nil {
		return "", err
	}
	var ahead, behind int
	fmt.Sscanf(counts, "%d %d", &ahead, &behind)
	switch {
	case behind == 0:
		return "Fetched; already up to date.", nil
	case ahead > 0:
		return fmt.Sprintf("Fetched %d new commit(s); this console has its own commits too, so Pull combines them.", behind), nil
	}
	if r.clean(ctx) != nil {
		return fmt.Sprintf("Fetched %d new commit(s); not merged by itself because the working copy has uncommitted changes. Pull brings them in.", behind), nil
	}
	if _, err := r.git(ctx, "merge", "-q", "--ff-only", up); err != nil {
		return fmt.Sprintf("Fetched %d new commit(s); could not fast-forward, use Pull.", behind), nil
	}
	return fmt.Sprintf("Fast-forwarded %d commit(s).", behind), nil
}
