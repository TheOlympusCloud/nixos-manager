package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"golang.org/x/crypto/ssh"
)

func sign(secret, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return hex.EncodeToString(m.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	body := `{"ref":"refs/heads/main"}`
	good := sign("s3cret", body)
	for _, c := range []struct {
		secret, body, sig string
		want              bool
	}{
		{"s3cret", body, good, true},
		{"s3cret", body, "sha256=" + good, true},
		{"s3cret", body, strings.ToUpper(good), true},
		{"s3cret", body + " ", good, false},
		{"other", body, good, false},
		{"", body, sign("", body), false},
		{"s3cret", body, "", false},
		{"s3cret", body, "not-hex", false},
		{"s3cret", body, good[:20], false},
	} {
		if got := VerifySignature([]byte(c.secret), []byte(c.body), c.sig); got != c.want {
			t.Errorf("VerifySignature(%q, %q, %q) = %v", c.secret, c.body, c.sig, got)
		}
	}
}

func TestParseRemote(t *testing.T) {
	for in, want := range map[string]RemoteURL{
		"ssh://git@forgejo.lan:2222/pablo/nixos-config.git": {Kind: "ssh", Host: "forgejo.lan", Port: "2222", Path: "pablo/nixos-config"},
		"ssh://forgejo.lan/pablo/fleet":                     {Kind: "ssh", Host: "forgejo.lan", Path: "pablo/fleet"},
		"git@forgejo.lan:pablo/nixos-config.git":            {Kind: "ssh", Host: "forgejo.lan", Path: "pablo/nixos-config"},
		"forgejo.lan:pablo/fleet":                           {Kind: "ssh", Host: "forgejo.lan", Path: "pablo/fleet"},
		"https://forgejo.lan/pablo/nixos-config.git":        {Kind: "https", Host: "forgejo.lan", Path: "pablo/nixos-config"},
		"http://10.0.0.5:3000/pablo/fleet/":                 {Kind: "http", Host: "10.0.0.5", Port: "3000", Path: "pablo/fleet"},
		"/srv/git/fleet.git":                                {Kind: "local", Path: "/srv/git/fleet.git"},
		"file:///srv/git/fleet.git":                         {Kind: "local", Path: "/srv/git/fleet.git"},
	} {
		got, err := ParseRemote(in)
		if err != nil || got != want {
			t.Errorf("ParseRemote(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{
		"",
		"https://pablo:token@forgejo.lan/pablo/fleet.git", // credentials
		"https://pablo@forgejo.lan/pablo/fleet.git",
		"ssh://git:pw@forgejo.lan/pablo/fleet.git",
		"user:pass@forgejo.lan:pablo/fleet.git",
		"ext::sh -c touch% /tmp/pwned", // remote helpers run commands
		"ext::sh",
		"fd::3",
		"-oProxyCommand=x:y",
		"--upload-pack=x",
		"forgejo.lan:",
		"https://forgejo.lan/",
		"https://forgejo.lan/a?b=c",
		"ftp://forgejo.lan/a/b",
		"git@forgejo.lan:a b",
		"git@forgejo.lan:a\nb",
		"relative/path",
		"file://host/srv/x",
	} {
		if got, err := ParseRemote(in); err == nil {
			t.Errorf("ParseRemote(%q) accepted: %+v", in, got)
		} else {
			isErr(t, err, ErrInvalid)
		}
	}
	if _, err := ParseRemote("https://pablo:hunter2@forgejo.lan/p/f.git"); err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("credentials refused without echoing them: %v", err)
	}
}

func TestWebURL(t *testing.T) {
	for _, c := range []struct{ remote, base, want string }{
		{"ssh://git@forgejo.lan:2222/pablo/nixos-config.git", "", "https://forgejo.lan/pablo/nixos-config"},
		{"git@forgejo.lan:pablo/nixos-config.git", "", "https://forgejo.lan/pablo/nixos-config"},
		{"https://forgejo.lan:8443/pablo/fleet.git", "", "https://forgejo.lan:8443/pablo/fleet"},
		{"http://10.0.0.5:3000/pablo/fleet", "", "http://10.0.0.5:3000/pablo/fleet"},
		{"ssh://git@forgejo.lan:2222/pablo/fleet.git", "http://forgejo.lan:3000", "http://forgejo.lan:3000/pablo/fleet"},
		{"ssh://git@forgejo.lan:2222/pablo/fleet.git", "https://git.example.org/", "https://git.example.org/pablo/fleet"},
		{"/srv/git/fleet.git", "", ""},
		{"", "", ""},
		{"ext::sh", "", ""},
	} {
		if got := webURL(c.remote, c.base); got != c.want {
			t.Errorf("webURL(%q, %q) = %q, want %q", c.remote, c.base, got, c.want)
		}
	}
	for _, ok := range []string{"https://forgejo.lan", "http://10.0.0.5:3000", "https://host/git"} {
		if err := checkWebBase(ok); err != nil {
			t.Errorf("checkWebBase(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{"forgejo.lan", "javascript:alert(1)", "https://u:p@host", "ftp://host", "https://host/?x=1", "https://"} {
		if err := checkWebBase(bad); err == nil {
			t.Errorf("checkWebBase(%q) accepted", bad)
		}
	}

	r := newRepo(t)
	ctx := context.Background()
	ok(t, r.SetRemote(ctx, "ssh://git@forgejo.lan:2222/pablo/fleet.git"))
	if got := r.WebURL(ctx); got != "https://forgejo.lan/pablo/fleet" {
		t.Fatalf("derived %q", got)
	}
	gitOut(t, r.Dir, "config", "url.file:///srv/mirror/.insteadOf", "ssh://git@forgejo.lan:2222/")
	if got := r.WebURL(ctx); got != "https://forgejo.lan/pablo/fleet" {
		t.Fatalf("an insteadOf rewrite changed the web address: %q", got)
	}
	gitOut(t, r.Dir, "config", "--remove-section", "url.file:///srv/mirror/")
	ok(t, r.SetWebBase(ctx, "http://forgejo.lan:3000/"))
	if got := r.WebURL(ctx); got != "http://forgejo.lan:3000/pablo/fleet" {
		t.Fatalf("with override %q", got)
	}
	isErr(t, r.SetWebBase(ctx, "javascript:alert(1)"), ErrInvalid)
	ok(t, r.SetWebBase(ctx, ""))
	ok(t, r.SetWebBase(ctx, "")) // removing it twice is fine
	if got := r.WebURL(ctx); got != "https://forgejo.lan/pablo/fleet" {
		t.Fatalf("after removing the override %q", got)
	}
	isErr(t, r.SetRemote(ctx, "https://u:secret@forgejo.lan/p/f.git"), ErrInvalid)
	if got := r.Remote(ctx); got != "ssh://git@forgejo.lan:2222/pablo/fleet.git" {
		t.Fatalf("a refused remote changed origin: %q", got)
	}
}

// fakeSSH puts an ssh on PATH that records its arguments, one per line, and fails.
func fakeSSH(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "args")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done > " + shellQuote(log) + "\necho 'Permission denied (publickey).' >&2\nexit 255\n"
	ok(t, os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GIT_SSH_COMMAND", "")
	os.Unsetenv("GIT_SSH_COMMAND")
	return log
}

func TestSSHCommandUsesTheDeployKey(t *testing.T) {
	if sshCommand("") != sshCommand(t.TempDir()) {
		t.Fatal("a folder without a key changed the command")
	}
	ctx := context.Background()
	log := fakeSSH(t)
	// A path with a space and a quote must reach ssh as one argument.
	dir := filepath.Join(t.TempDir(), "data dir's", "ssh")
	r := must[*Repo](t)(OpenRepoSSH(filepath.Join(t.TempDir(), "repo"), "", testName, testEmail, dir))
	ok(t, r.SetRemote(ctx, "ssh://git@forgejo.lan:2222/pablo/fleet.git"))
	args := func() []string {
		os.Remove(log)
		r.git(ctx, "ls-remote", "origin")
		b, _ := os.ReadFile(log)
		return strings.Split(strings.TrimSpace(string(b)), "\n")
	}

	// No key yet: ssh's defaults, as before.
	got := args()
	if slices.Contains(got, "-i") || !slices.Contains(got, "BatchMode=yes") || !slices.Contains(got, "StrictHostKeyChecking=accept-new") {
		t.Fatalf("without a key: %q", got)
	}

	f := &Forgejo{Repo: r, DataDir: filepath.Dir(dir)}
	must[*DeployKey](t)(f.Key(true))
	got = args()
	key := filepath.Join(dir, "id_ed25519")
	i := slices.Index(got, "-i")
	if i < 0 || got[i+1] != key {
		t.Fatalf("key not passed: %q", got)
	}
	for _, want := range []string{"IdentitiesOnly=yes", "UserKnownHostsFile=" + filepath.Join(dir, "known_hosts"), "StrictHostKeyChecking=accept-new", "BatchMode=yes", "-p", "2222", "git@forgejo.lan"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if f.SSHMode() != "app" {
		t.Fatalf("mode %s", f.SSHMode())
	}

	// The user's own GIT_SSH_COMMAND wins.
	t.Setenv("GIT_SSH_COMMAND", "ssh -o User=mine")
	got = args()
	if slices.Contains(got, "-i") || !slices.Contains(got, "User=mine") || f.SSHMode() != "env" {
		t.Fatalf("GIT_SSH_COMMAND not respected: %q", got)
	}
}

func TestDeployKey(t *testing.T) {
	data := t.TempDir()
	r := must[*Repo](t)(OpenRepoSSH(filepath.Join(data, "repo"), "", testName, testEmail, filepath.Join(data, "ssh")))
	f := &Forgejo{Repo: r, DataDir: data}
	if k := must[*DeployKey](t)(f.Key(false)); k != nil {
		t.Fatal("Key(false) created a key")
	}
	k := must[*DeployKey](t)(f.Key(true))
	if !strings.HasPrefix(k.Public, "ssh-ed25519 AAAA") || !strings.HasPrefix(k.Fingerprint, "SHA256:") || strings.Contains(k.Public, "PRIVATE") {
		t.Fatalf("key %+v", k)
	}
	file := filepath.Join(data, "ssh", "id_ed25519")
	for p, mode := range map[string]os.FileMode{file: 0o600, filepath.Dir(file): 0o700 | os.ModeDir, file + ".pub": 0o644} {
		if fi, err := os.Stat(p); err != nil || fi.Mode() != mode {
			t.Errorf("%s: %v %v, want %v", p, fi.Mode(), err, mode)
		}
	}
	priv, err := os.ReadFile(file)
	ok(t, err)
	signer, err := ssh.ParsePrivateKey(priv)
	ok(t, err)
	if ssh.FingerprintSHA256(signer.PublicKey()) != k.Fingerprint {
		t.Fatal("the public key is not the private key's")
	}
	pubFile, _ := os.ReadFile(file + ".pub")
	if !strings.HasPrefix(string(pubFile), strings.Fields(k.Public)[0]+" "+strings.Fields(k.Public)[1]) {
		t.Fatalf("id_ed25519.pub = %q", pubFile)
	}
	// Never replaced.
	if err := generateKey(file, "x"); err == nil {
		t.Fatal("generateKey overwrote the key")
	}
	if k2 := must[*DeployKey](t)(f.Key(true)); k2.Fingerprint != k.Fingerprint {
		t.Fatal("the key changed")
	}

	// Secrets never go inside the working copy, where they could be committed.
	inside := must[*Repo](t)(OpenRepoSSH(t.TempDir(), "", testName, testEmail, ""))
	inside.SSHDir = filepath.Join(inside.Dir, "ssh")
	_, err = (&Forgejo{Repo: inside, DataDir: inside.Dir}).Key(true)
	isErr(t, err, ErrInvalid)
	_, err = (&Forgejo{Repo: inside, DataDir: inside.Dir}).HookSecret()
	isErr(t, err, ErrInvalid)
}

func TestHookSecretAndHostKey(t *testing.T) {
	data := t.TempDir()
	r := must[*Repo](t)(OpenRepoSSH(filepath.Join(data, "repo"), "", testName, testEmail, filepath.Join(data, "ssh")))
	f := &Forgejo{Repo: r, DataDir: data}
	s1 := must[string](t)(f.HookSecret())
	s2 := must[string](t)(f.HookSecret())
	if len(s1) < 40 || s1 != s2 {
		t.Fatalf("secret %q then %q", s1, s2)
	}
	if fi, _ := os.Stat(filepath.Join(data, "webhook-secret")); fi.Mode() != 0o600 {
		t.Fatalf("secret mode %v", fi.Mode())
	}

	k := must[*DeployKey](t)(f.Key(true)) // any public key will do as a host key
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(k.Public))
	ok(t, err)
	line := strings.Fields(k.Public)[0] + " " + strings.Fields(k.Public)[1]
	ok(t, os.WriteFile(filepath.Join(data, "ssh", "known_hosts"), []byte("# comment\nother.lan "+line+"\n[forgejo.lan]:2222 "+line+"\n"), 0o600))
	want := "ssh-ed25519 " + ssh.FingerprintSHA256(pub)
	if got := f.HostKey("forgejo.lan", "2222"); got != want {
		t.Fatalf("HostKey = %q, want %q", got, want)
	}
	if got := f.HostKey("forgejo.lan", ""); got != "" {
		t.Fatalf("port 22 matched the 2222 entry: %q", got)
	}
	if got := f.HostKey("other.lan", "22"); got != want {
		t.Fatalf("port 22 entry: %q", got)
	}
}

func TestClassify(t *testing.T) {
	for stderr, want := range map[string]string{
		"ssh: Could not resolve hostname forgejo.lan: Name or service not known\nfatal: Could not read from remote repository.": "unreachable",
		"ssh: connect to host forgejo.lan port 2222: Connection refused":                                                        "unreachable",
		"ssh: connect to host 10.0.0.9 port 22: Connection timed out":                                                           "unreachable",
		"git@forgejo.lan: Permission denied (publickey).\nfatal: Could not read from remote repository.":                        "auth",
		"fatal: could not read Username for 'https://forgejo.lan': terminal prompts disabled":                                   "auth",
		"remote: Repository not found.\nfatal: repository 'https://forgejo.lan/p/x.git/' not found":                             "not_found",
		"fatal: '/srv/nope.git' does not appear to be a git repository":                                                         "not_found",
		"@@@@@@@\n@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @\nHost key verification failed.":                   "host_key_changed",
		"fatal: something else entirely": "error",
	} {
		if got, msg := classify(stderr, "forgejo.lan"); got != want || msg == "" {
			t.Errorf("classify(%q) = %s, want %s", stderr, got, want)
		}
	}
}

// starter is a fresh working copy with the starter files written but not committed,
// connected to remote.
func starter(t *testing.T, remote string) (*Repo, *Forgejo) {
	t.Helper()
	data := t.TempDir()
	r := must[*Repo](t)(OpenRepoSSH(filepath.Join(data, "repo"), "", testName, testEmail, filepath.Join(data, "ssh")))
	if seeded := must[bool](t)(r.Seed(fstest.MapFS{"flake.nix": {Data: []byte("starter\n")}, "hosts/example/default.nix": {Data: []byte("{ }\n")}})); !seeded {
		t.Fatal("not seeded")
	}
	ok(t, r.SetRemote(context.Background(), remote))
	return r, &Forgejo{Repo: r, DataDir: data}
}

func TestReplaceStarterWithTheRemote(t *testing.T) {
	ctx := context.Background()
	fleet, _ := remotePair(t) // the remote has a.nix in one commit
	remote := gitOut(t, fleet.Dir, "remote", "get-url", "origin")

	r, f := starter(t, remote)
	write(t, r, "notes.txt", "my edit\n")
	ok(t, r.Stage(ctx, []string{"notes.txt"}))
	c := f.Test(ctx)
	if c.Result != "connected" || c.Next != "adopt" || !slices.Equal(c.Branches, []string{"main"}) {
		t.Fatalf("test before adopting: %+v", c)
	}
	msg := must[string](t)(r.AdoptRemote(ctx))
	if !strings.Contains(msg, "starter-backup") {
		t.Fatalf("message %q", msg)
	}
	if head, want := gitOut(t, r.Dir, "rev-parse", "HEAD"), gitOut(t, fleet.Dir, "rev-parse", "HEAD"); head != want {
		t.Fatalf("HEAD %s, want %s", head, want)
	}
	st := must[Status](t)(r.Status(ctx))
	if st.Upstream != "origin/main" || len(st.Files) != 0 {
		t.Fatalf("after adopting: %+v", st)
	}
	if _, err := os.Stat(filepath.Join(r.Dir, "hosts/example/default.nix")); err == nil {
		t.Fatal("a starter file stayed behind")
	}
	// The replaced files, the uncommitted edit too, are kept in git.
	for p, want := range map[string]string{"flake.nix": "starter", "notes.txt": "my edit"} {
		if got := gitOut(t, r.Dir, "show", "refs/nixmgr/starter-backup:"+p); got != want {
			t.Errorf("backup of %s = %q", p, got)
		}
	}
	if c := f.Test(ctx); c.Next != "" || c.Result != "connected" {
		t.Fatalf("after adopting: %+v", c)
	}

	// With commits of its own, the working copy is never replaced.
	_, err := r.AdoptRemote(ctx)
	isErr(t, err, ErrConflict)
}

func TestUnrelatedHistoriesRefused(t *testing.T) {
	ctx := context.Background()
	fleet, _ := remotePair(t)
	remote := gitOut(t, fleet.Dir, "remote", "get-url", "origin")
	r, f := starter(t, remote)
	mine := commitAll(t, r, "my own starter")

	c := f.Test(ctx)
	if c.Next != "unrelated" || !strings.Contains(c.Message, "separate histories") {
		t.Fatalf("test: %+v", c)
	}
	_, err := r.Pull(ctx)
	isErr(t, err, ErrConflict)
	_, err = r.AdoptRemote(ctx)
	isErr(t, err, ErrConflict)
	if head := gitOut(t, r.Dir, "rev-parse", "HEAD"); head != mine {
		t.Fatal("the local commit moved")
	}
	if c, _, _ := r.Read("flake.nix"); c != "starter\n" {
		t.Fatal("the working copy changed")
	}
	_, err = r.Push(ctx)
	isErr(t, err, ErrConflict)
	if msg := must[string](t)(r.FetchForward(ctx)); !strings.Contains(msg, "unrelated") {
		t.Fatalf("webhook fetch: %q", msg)
	}
}

func TestEmptyRemoteAndCloneFailure(t *testing.T) {
	ctx := context.Background()
	bare := filepath.Join(t.TempDir(), "empty.git")
	gitOut(t, t.TempDir(), "init", "-q", "--bare", "-b", "main", bare)
	r, f := starter(t, bare)
	if c := f.Test(ctx); c.Result != "empty" || c.Next != "push" || len(c.Branches) != 0 {
		t.Fatalf("empty remote: %+v", c)
	}
	_, err := r.AdoptRemote(ctx)
	isErr(t, err, ErrInvalid)

	// A clone that fails still starts, with origin set, so Settings can fix it.
	missing := filepath.Join(t.TempDir(), "missing.git")
	r2 := must[*Repo](t)(OpenRepo(filepath.Join(t.TempDir(), "repo"), missing, testName, testEmail))
	if got := r2.Remote(ctx); got != missing || r2.hasHead(ctx) {
		t.Fatalf("after a failed clone: remote %q", got)
	}
	f2 := &Forgejo{Repo: r2, DataDir: t.TempDir()}
	if c := f2.Test(ctx); c.Result != "not_found" {
		t.Fatalf("missing repository: %+v", c)
	}
	if c := (&Forgejo{Repo: newRepo(t)}).Test(ctx); c.Result != "no_remote" {
		t.Fatalf("no remote: %+v", c)
	}
}

func TestHook(t *testing.T) {
	old := hookDelay
	hookDelay = 20 * time.Millisecond
	t.Cleanup(func() { hookDelay = old })
	ctx := context.Background()
	r, other := remotePair(t)
	f := &Forgejo{Repo: r, DataDir: t.TempDir()}
	secret := must[string](t)(f.HookSecret())
	push := `{"ref":"refs/heads/main","after":"x"}`
	wait := func(want string) {
		t.Helper()
		for range 200 {
			if d := f.LastDelivery(); d != nil && strings.Contains(d.Result, want) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("last delivery %+v, want %q", f.LastDelivery(), want)
	}

	if code, _ := f.Hook("push", sign("wrong", push), []byte(push)); code != 401 || f.LastDelivery() != nil {
		t.Fatalf("bad signature: %d, and it must not replace the last delivery: %+v", code, f.LastDelivery())
	}
	if code, _ := f.Hook("push", "", []byte(push)); code != 401 {
		t.Fatalf("no signature: %d", code)
	}
	if code, _ := f.Hook("ping", sign(secret, "{}"), []byte("{}")); code != 200 || !f.LastDelivery().OK {
		t.Fatalf("ping: %d", code)
	}
	other2 := `{"ref":"refs/heads/testing-pi"}`
	if code, _ := f.Hook("push", sign(secret, other2), []byte(other2)); code != 200 || !strings.Contains(f.LastDelivery().Result, "testing-pi") {
		t.Fatalf("other branch: %d %+v", code, f.LastDelivery())
	}
	if code, _ := f.Hook("push", sign(secret, "nope"), []byte("nope")); code != 400 {
		t.Fatalf("not JSON: %d", code)
	}

	// Clean working copy: a burst of pushes ends fast-forwarded.
	otherCommit(t, other, "b.nix", "b\n")
	otherCommit(t, other, "c.nix", "c\n")
	for range 3 {
		if code, _ := f.Hook("push", sign(secret, push), []byte(push)); code != 202 {
			t.Fatalf("push: %d", code)
		}
	}
	wait("Fast-forwarded 2")
	if c, _, _ := r.Read("c.nix"); c != "c\n" {
		t.Fatal("not fast-forwarded")
	}

	// Uncommitted changes: only fetched, and Status shows the new commit.
	write(t, r, "a.nix", "local edit\n")
	otherCommit(t, other, "d.nix", "d\n")
	f.Hook("push", sign(secret, push), []byte(push))
	wait("uncommitted")
	st := must[Status](t)(r.Status(ctx))
	if st.Behind != 1 || st.Ahead != 0 {
		t.Fatalf("after fetch: %+v", st)
	}
	if c, _, _ := r.Read("a.nix"); c != "local edit\n" {
		t.Fatal("the local edit was touched")
	}

	// Local commits: only fetched.
	commitAll(t, r, "mine")
	otherCommit(t, other, "e.nix", "e\n")
	f.Hook("push", sign(secret, push), []byte(push))
	wait("its own commits")
	if st := must[Status](t)(r.Status(ctx)); st.Behind != 2 || st.Ahead != 1 {
		t.Fatalf("after fetch with local commits: %+v", st)
	}
}

func TestShellQuote(t *testing.T) {
	for _, s := range []string{"plain", "a b", "it's", `$(touch x)`, "'"} {
		out, err := exec.Command("sh", "-c", "printf %s "+shellQuote(s)).Output()
		if err != nil || string(out) != s {
			t.Errorf("shellQuote(%q) round trip = %q, %v", s, out, err)
		}
	}
}
