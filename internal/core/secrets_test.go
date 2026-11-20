package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/TheOlympusCloud/nixos-manager/internal/fakesops"
)

// newSecrets is a Secrets on a fresh repository, with the fake tools (the real
// ones with NIXMGR_NIX_TESTS=1).
func newSecrets(t *testing.T) (*Secrets, fakesops.Paths) {
	t.Helper()
	tools := fakesops.Tools(t)
	s := OpenSecrets(t.TempDir(), newRepo(t))
	s.Sops, s.AgeKeygen, s.SSHToAge = tools.Sops, tools.AgeKeygen, tools.SSHToAge
	return s, tools
}

// asMachine is what a machine decrypts with: its own key, not the server's.
func asMachine(s *Secrets, identity string) *Secrets {
	return &Secrets{Repo: s.Repo, KeyFile: identity, Sops: s.Sops, AgeKeygen: s.AgeKeygen, SSHToAge: s.SSHToAge}
}

// canOpen says whether the identity decrypts p, and what name holds there.
func canOpen(t *testing.T, s *Secrets, p, name string) (string, bool) {
	t.Helper()
	b, err := s.Repo.root.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	vals, err := s.decrypt(context.Background(), p, b)
	if err != nil {
		return "", false
	}
	return vals[name], true
}

// noPlaintext fails when a value appears in any file under the folders.
func noPlaintext(t *testing.T, value string, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if b, err := os.ReadFile(p); err == nil && bytes.Contains(b, []byte(value)) {
				t.Errorf("%s holds the plain value", p)
			}
			return nil
		})
	}
}

func TestSecretsStatusWithoutTools(t *testing.T) {
	s := OpenSecrets(t.TempDir(), newRepo(t))
	s.Sops, s.AgeKeygen, s.SSHToAge = "nixmgr-no-sops", "nixmgr-no-age-keygen", "ssh-to-age-missing"
	st := must[SecretsStatus](t)(s.Status(context.Background()))
	if !slices.Equal(st.Missing, []string{"nixmgr-no-sops", "nixmgr-no-age-keygen", "ssh-to-age-missing"}) || st.Recipient != "" {
		t.Fatalf("%+v", st)
	}
	isErr(t, s.Set(context.Background(), "", "wifi", "x"), ErrInvalid)
	if _, err := os.Stat(s.KeyFile); err == nil {
		t.Fatal("a key was made without the tools")
	}
}

// The server's key is made once, readable by the service only, outside the
// working copy, and its public half is what the status shows.
func TestSecretsKeyIsMadeOnce(t *testing.T) {
	s, _ := newSecrets(t)
	ctx := context.Background()
	st := must[SecretsStatus](t)(s.Status(ctx))
	if !strings.HasPrefix(st.Recipient, "age1") || st.KeyFile != s.KeyFile || len(st.Missing) != 0 || st.Stale {
		t.Fatalf("%+v", st)
	}
	fi := must[os.FileInfo](t)(os.Stat(s.KeyFile))
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("key mode %v", fi.Mode())
	}
	if strings.HasPrefix(s.KeyFile, s.Repo.Dir) {
		t.Fatal("the key is in the working copy")
	}
	key := must[[]byte](t)(os.ReadFile(s.KeyFile))
	again := OpenSecrets(filepath.Dir(s.KeyFile), s.Repo)
	again.Sops, again.AgeKeygen, again.SSHToAge = s.Sops, s.AgeKeygen, s.SSHToAge
	if st2 := must[SecretsStatus](t)(again.Status(ctx)); st2.Recipient != st.Recipient {
		t.Fatalf("recipient changed: %s, %s", st.Recipient, st2.Recipient)
	}
	if !bytes.Equal(key, must[[]byte](t)(os.ReadFile(s.KeyFile))) {
		t.Fatal("the key was made again")
	}
	if tree := must[[]Entry](t)(s.Repo.Tree()); len(tree) != 0 {
		t.Fatalf("status wrote to the working copy: %v", tree)
	}
}

// A machine's key comes from its SSH host key, as pasted from the .pub file or
// from ssh-keyscan; .sops.yaml then names it, and other keys are refused.
func TestSecretsMachineKeys(t *testing.T) {
	s, _ := newSecrets(t)
	ctx := context.Background()
	pub, _ := fakesops.HostKey(t)
	m := must[SecretMachine](t)(s.SetMachineKey(ctx, "web", pub+" root@web\n"))
	if m.Name != "web" || m.Key != pub || !strings.HasPrefix(m.Recipient, "age1") || !strings.HasPrefix(m.Fingerprint, "SHA256:") {
		t.Fatalf("%+v", m)
	}
	if got, _, _ := s.Repo.Read("secrets/hosts/web.pub"); got != pub+"\n" {
		t.Fatalf("web.pub: %q", got)
	}
	pi, _ := fakesops.HostKey(t)
	f := strings.Fields(pi)
	must[SecretMachine](t)(s.SetMachineKey(ctx, "pi", "# pi:22 SSH-2.0-OpenSSH_9.9\npi "+f[0]+" "+f[1]+"\n"))
	st := must[SecretsStatus](t)(s.Status(ctx))
	if len(st.Machines) != 2 || st.Machines[0].Name != "pi" || st.Machines[1].Name != "web" || st.Stale {
		t.Fatalf("%+v", st)
	}
	conf, _, _ := s.Repo.Read(".sops.yaml")
	// "- <age key> # <machine>": the starter's modules/secrets.nix finds a machine's key by it.
	for _, want := range []string{"- " + st.Machines[0].Recipient + " # pi\n", "- " + st.Machines[1].Recipient + " # web\n", `'^secrets/hosts/web\.json$'`, `'^secrets/common\.json$'`} {
		if !strings.Contains(conf, want) {
			t.Fatalf(".sops.yaml lacks %s:\n%s", want, conf)
		}
	}
	// A clone with web's key is named in its own rule, so it finds its key too.
	must[SecretMachine](t)(s.SetMachineKey(ctx, "web2", pub))
	if conf, _, _ := s.Repo.Read(".sops.yaml"); !strings.Contains(conf, "- "+st.Machines[1].Recipient+" # web2\n") {
		t.Fatalf(".sops.yaml does not name web2:\n%s", conf)
	}
	isErr(t, must2(s.SetMachineKey(ctx, "web", "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7 root@web")), ErrInvalid)
	isErr(t, must2(s.SetMachineKey(ctx, "Web!", pub)), ErrInvalid)
	isErr(t, must2(s.SetMachineKey(ctx, "web", "ssh-ed25519 AAAAnotbase64")), ErrInvalid)
}

func must2[T any](_ T, err error) error { return err }

// Values are encrypted before they reach the disk: for the server and every
// machine with a key (common) or that machine alone, and only those keys open them.
func TestSecretsSetRevealDelete(t *testing.T) {
	s, tools := newSecrets(t)
	ctx := context.Background()
	webPub, webPriv := fakesops.HostKey(t)
	piPub, piPriv := fakesops.HostKey(t)
	must[SecretMachine](t)(s.SetMachineKey(ctx, "web", webPub))
	must[SecretMachine](t)(s.SetMachineKey(ctx, "pi", piPub))
	const wifi, api = "correct horse battery staple", "multi\nline \"api\" key ✓"
	ok(t, s.Set(ctx, "", "wifi", wifi))
	ok(t, s.Set(ctx, "web", "api-key", api))
	noPlaintext(t, "horse battery", s.Repo.Dir, filepath.Dir(s.KeyFile))
	noPlaintext(t, "api\" key", s.Repo.Dir, filepath.Dir(s.KeyFile))

	if v := must[string](t)(s.Reveal(ctx, "", "wifi")); v != wifi {
		t.Fatalf("wifi %q", v)
	}
	if v := must[string](t)(s.Reveal(ctx, "web", "api-key")); v != api {
		t.Fatalf("api-key %q", v)
	}
	web := asMachine(s, fakesops.MachineIdentity(t, tools, webPriv))
	pi := asMachine(s, fakesops.MachineIdentity(t, tools, piPriv))
	if v, ok := canOpen(t, web, "secrets/common.json", "wifi"); !ok || v != wifi {
		t.Fatal("web cannot open common.json")
	}
	if v, ok := canOpen(t, pi, "secrets/common.json", "wifi"); !ok || v != wifi {
		t.Fatal("pi cannot open common.json")
	}
	if v, ok := canOpen(t, web, "secrets/hosts/web.json", "api-key"); !ok || v != api {
		t.Fatal("web cannot open its own file")
	}
	if _, ok := canOpen(t, pi, "secrets/hosts/web.json", "api-key"); ok {
		t.Fatal("pi opens web's file")
	}

	st := must[SecretsStatus](t)(s.Status(ctx))
	files := map[string][]string{}
	for _, f := range st.Files {
		files[f.Path] = f.Names
		if f.Stale || f.Locked || f.Error != "" {
			t.Fatalf("%+v", f)
		}
	}
	if !slices.Equal(files["secrets/common.json"], []string{"wifi"}) || !slices.Equal(files["secrets/hosts/web.json"], []string{"api-key"}) || len(files) != 2 {
		t.Fatalf("files %v", files)
	}

	ok(t, s.Set(ctx, "", "wifi", "new"))
	ok(t, s.Set(ctx, "", "token", "t"))
	if v := must[string](t)(s.Reveal(ctx, "", "wifi")); v != "new" {
		t.Fatalf("replaced wifi %q", v)
	}
	ok(t, s.Delete(ctx, "", "wifi"))
	isErr(t, must2(s.Reveal(ctx, "", "wifi")), ErrNotFound)
	if v := must[string](t)(s.Reveal(ctx, "", "token")); v != "t" {
		t.Fatalf("token %q", v)
	}
	ok(t, s.Delete(ctx, "web", "api-key"))
	if _, _, err := s.Repo.Read("secrets/hosts/web.json"); err == nil {
		t.Fatal("an empty file was kept")
	}
	isErr(t, s.Delete(ctx, "web", "api-key"), ErrNotFound)
	isErr(t, s.Delete(ctx, "", "nothing"), ErrNotFound)
}

func TestSecretsRefuse(t *testing.T) {
	s, _ := newSecrets(t)
	ctx := context.Background()
	pub, _ := fakesops.HostKey(t)
	must[SecretMachine](t)(s.SetMachineKey(ctx, "web", pub))
	for _, name := range []string{"", "sops", "a/b", "../x", "-x", "pass_unencrypted", strings.Repeat("a", 101), "with space"} {
		isErr(t, s.Set(ctx, "", name, "v"), ErrInvalid)
	}
	isErr(t, s.Set(ctx, "", "empty", ""), ErrInvalid)
	isErr(t, s.Set(ctx, "", "big", strings.Repeat("x", 64<<10+1)), ErrInvalid)
	isErr(t, s.Set(ctx, "", "bin", "\xff"), ErrInvalid)
	isErr(t, s.Set(ctx, "db", "x", "v"), ErrInvalid) // no key yet
	isErr(t, s.Set(ctx, "../web", "x", "v"), ErrInvalid)
	isErr(t, s.RemoveMachineKey(ctx, "db"), ErrNotFound)
	if tree := must[[]Entry](t)(s.Repo.Tree()); slices.ContainsFunc(tree, func(e Entry) bool { return strings.HasSuffix(e.Path, ".json") }) {
		t.Fatalf("a refused secret was written: %v", tree)
	}
}

// Removing a machine deletes its own file and encrypts the shared one again
// without it, with a new data key: it cannot open what is added afterwards.
func TestSecretsRemoveMachine(t *testing.T) {
	s, tools := newSecrets(t)
	ctx := context.Background()
	webPub, _ := fakesops.HostKey(t)
	piPub, piPriv := fakesops.HostKey(t)
	must[SecretMachine](t)(s.SetMachineKey(ctx, "web", webPub))
	must[SecretMachine](t)(s.SetMachineKey(ctx, "pi", piPub))
	ok(t, s.Set(ctx, "", "wifi", "w"))
	ok(t, s.Set(ctx, "pi", "own", "o"))
	pi := asMachine(s, fakesops.MachineIdentity(t, tools, piPriv))
	if _, ok := canOpen(t, pi, "secrets/common.json", "wifi"); !ok {
		t.Fatal("pi cannot open common.json before")
	}
	ok(t, s.RemoveMachineKey(ctx, "pi"))
	for _, p := range []string{"secrets/hosts/pi.pub", "secrets/hosts/pi.json"} {
		if _, _, err := s.Repo.Read(p); err == nil {
			t.Fatalf("%s kept", p)
		}
	}
	if _, ok := canOpen(t, pi, "secrets/common.json", "wifi"); ok {
		t.Fatal("pi still opens common.json")
	}
	if v := must[string](t)(s.Reveal(ctx, "", "wifi")); v != "w" {
		t.Fatalf("wifi lost: %q", v)
	}
	st := must[SecretsStatus](t)(s.Status(ctx))
	if len(st.Machines) != 1 || st.Stale {
		t.Fatalf("%+v", st)
	}
	if conf, _, _ := s.Repo.Read(".sops.yaml"); strings.Contains(conf, "pi") {
		t.Fatalf(".sops.yaml still names pi:\n%s", conf)
	}
}

// A file encrypted for other keys than the approved ones (one restored by
// hand, a .sops.yaml or .pub edited) shows as stale until Re-encrypt; one this
// server cannot open is never overwritten.
func TestSecretsStaleAndLocked(t *testing.T) {
	s, _ := newSecrets(t)
	ctx := context.Background()
	ok(t, s.Set(ctx, "", "wifi", "w"))
	before, _, _ := s.Repo.Read("secrets/common.json") // for the server alone
	webPub, _ := fakesops.HostKey(t)
	must[SecretMachine](t)(s.SetMachineKey(ctx, "web", webPub))
	write(t, s.Repo, "secrets/common.json", before)
	write(t, s.Repo, ".sops.yaml", "creation_rules: []\n")
	ok(t, s.Repo.Delete("secrets/hosts/web.pub"))
	st := must[SecretsStatus](t)(s.Status(ctx))
	if !st.Stale || !st.Files[0].Stale {
		t.Fatalf("not stale: %+v", st)
	}
	ok(t, s.Reencrypt(ctx))
	if st := must[SecretsStatus](t)(s.Status(ctx)); st.Stale {
		t.Fatalf("still stale: %+v", st)
	}
	if got, _, _ := s.Repo.Read("secrets/hosts/web.pub"); got != webPub+"\n" {
		t.Fatalf("web.pub not written back: %q", got)
	}

	// Encrypted by another server's key only.
	other, _ := newSecrets(t)
	ok(t, other.Set(ctx, "", "x", "y"))
	b, _, _ := other.Repo.Read("secrets/common.json")
	write(t, s.Repo, "secrets/common.json", b)
	st = must[SecretsStatus](t)(s.Status(ctx))
	if !st.Files[0].Locked {
		t.Fatalf("not locked: %+v", st.Files)
	}
	isErr(t, s.Set(ctx, "", "wifi", "w2"), ErrConflict)
	isErr(t, s.Reencrypt(ctx), ErrConflict)
	isErr(t, must2(s.Reveal(ctx, "", "x")), ErrConflict)
	if got, _, _ := s.Repo.Read("secrets/common.json"); got != b {
		t.Fatal("a file this server cannot open was overwritten")
	}

	write(t, s.Repo, "secrets/common.json", "not json")
	st = must[SecretsStatus](t)(s.Status(ctx))
	if st.Files[0].Error == "" {
		t.Fatalf("no error for a broken file: %+v", st.Files)
	}
}

// Only keys approved through SetMachineKey are recipients. A key someone
// writes into the working copy (an edit, a pull) is only proposed: neither
// Re-encrypt nor another machine's key change encrypts for it, and a .pub
// changed for an approved machine does not change whom its secrets are for.
func TestSecretsOnlyApprovedKeysAreRecipients(t *testing.T) {
	s, tools := newSecrets(t)
	ctx := context.Background()
	webPub, webPriv := fakesops.HostKey(t)
	must[SecretMachine](t)(s.SetMachineKey(ctx, "web", webPub))
	ok(t, s.Set(ctx, "", "wifi", "w"))
	ok(t, s.Set(ctx, "web", "api", "a"))

	evilPub, evilPriv := fakesops.HostKey(t)
	evil := asMachine(s, fakesops.MachineIdentity(t, tools, evilPriv))
	write(t, s.Repo, "secrets/hosts/evil.pub", evilPub+"\n")
	write(t, s.Repo, "secrets/hosts/web.pub", evilPub+"\n")
	write(t, s.Repo, "secrets/hosts/bad.pub", "nonsense\n")
	st := must[SecretsStatus](t)(s.Status(ctx))
	names := []string{}
	for _, m := range st.Proposed {
		names = append(names, m.Name)
	}
	if len(st.Machines) != 1 || st.Machines[0].Key != webPub || !slices.Equal(names, []string{"bad", "evil", "web"}) || st.Proposed[0].Error == "" || !st.Stale {
		t.Fatalf("%+v", st)
	}
	ok(t, s.Reencrypt(ctx))
	piPub, _ := fakesops.HostKey(t)
	must[SecretMachine](t)(s.SetMachineKey(ctx, "pi", piPub))
	ok(t, s.Set(ctx, "", "token", "t"))
	for _, p := range []string{"secrets/common.json", "secrets/hosts/web.json"} {
		if _, opened := canOpen(t, evil, p, "wifi"); opened {
			t.Fatalf("a key written into the working copy opens %s", p)
		}
	}
	web := asMachine(s, fakesops.MachineIdentity(t, tools, webPriv))
	if v, opened := canOpen(t, web, "secrets/hosts/web.json", "api"); !opened || v != "a" {
		t.Fatal("web lost its own secrets")
	}
	if got, _, _ := s.Repo.Read("secrets/hosts/web.pub"); got != webPub+"\n" {
		t.Fatalf("web.pub not written back: %q", got)
	}
	if conf, _, _ := s.Repo.Read(".sops.yaml"); strings.Contains(conf, "evil") {
		t.Fatalf(".sops.yaml names an unapproved key:\n%s", conf)
	}

	// Approving it (a person, in Settings) makes it a recipient; the approval is kept outside the working copy.
	must[SecretMachine](t)(s.SetMachineKey(ctx, "evil", evilPub))
	if _, opened := canOpen(t, evil, "secrets/common.json", "wifi"); !opened {
		t.Fatal("an approved key cannot open common.json")
	}
	fi := must[os.FileInfo](t)(os.Stat(s.TrustFile))
	if fi.Mode().Perm() != 0o600 || strings.HasPrefix(s.TrustFile, s.Repo.Dir) {
		t.Fatalf("trust file %s mode %v", s.TrustFile, fi.Mode())
	}
	again := OpenSecrets(filepath.Dir(s.TrustFile), s.Repo)
	again.Sops, again.AgeKeygen, again.SSHToAge = s.Sops, s.AgeKeygen, s.SSHToAge
	if st := must[SecretsStatus](t)(again.Status(ctx)); len(st.Machines) != 3 {
		t.Fatalf("approvals not kept: %+v", st.Machines)
	}
}

// The setup step stores each machine's own report token as its secret.
func TestStoreReportTokens(t *testing.T) {
	sec, _ := newSecrets(t)
	svc := NewService(sec.Repo)
	svc.Secrets = sec
	ctx := context.Background()
	isErr(t, must2(svc.StoreReportTokens(ctx)), ErrInvalid) // reports are off
	var err error
	if svc.Deploys, err = OpenDeployments(t.TempDir(), svc.Repo); err != nil {
		t.Fatal(err)
	}
	isErr(t, must2(svc.StoreReportTokens(ctx)), ErrInvalid) // no machine has a key
	webPub, _ := fakesops.HostKey(t)
	piPub, _ := fakesops.HostKey(t)
	must[SecretMachine](t)(sec.SetMachineKey(ctx, "web", webPub))
	must[SecretMachine](t)(sec.SetMachineKey(ctx, "pi", piPub))
	ok(t, sec.Set(ctx, "web", "other", "kept"))
	hosts := must[[]string](t)(svc.StoreReportTokens(ctx))
	if !slices.Equal(hosts, []string{"pi", "web"}) {
		t.Fatalf("hosts %v", hosts)
	}
	for _, h := range hosts {
		want := must[string](t)(svc.Deploys.HostToken(h))
		if got := must[string](t)(sec.Reveal(ctx, h, ReportSecret)); got != want {
			t.Fatalf("%s: %q, want %q", h, got, want)
		}
		noPlaintext(t, want, sec.Repo.Dir)
	}
	if v := must[string](t)(sec.Reveal(ctx, "web", "other")); v != "kept" {
		t.Fatal("another secret was lost")
	}

	// New tokens are stored again for the machines that read theirs from the
	// secrets, or they would stop reporting with nothing said; pi no longer does.
	ok(t, sec.Delete(ctx, "pi", ReportSecret))
	old := must[string](t)(svc.Deploys.HostToken("web"))
	if hosts := must[[]string](t)(svc.RotateReportTokens(ctx)); !slices.Equal(hosts, []string{"web"}) {
		t.Fatalf("stored again for %v", hosts)
	}
	want := must[string](t)(svc.Deploys.HostToken("web"))
	if got := must[string](t)(sec.Reveal(ctx, "web", ReportSecret)); want == old || got != want {
		t.Fatalf("web keeps %q; its token is %q (was %q)", got, want, old)
	}
	if _, err := sec.Reveal(ctx, "pi", ReportSecret); err == nil {
		t.Fatal("pi got a report token it did not keep in its secrets")
	}
}

// The sops JSON that reaches the disk names only the given keys, and its
// recipients are the ones asked for.
func TestSecretsFileFormat(t *testing.T) {
	s, _ := newSecrets(t)
	ctx := context.Background()
	ok(t, s.Set(ctx, "", "wifi", "w"))
	b, _, _ := s.Repo.Read("secrets/common.json")
	var doc map[string]json.RawMessage
	ok(t, json.Unmarshal([]byte(b), &doc))
	if len(doc) != 2 || doc["wifi"] == nil || doc["sops"] == nil || !strings.HasPrefix(string(doc["wifi"]), `"ENC[AES256_GCM,`) {
		t.Fatalf("%s", b)
	}
	if !strings.HasSuffix(b, "\n") {
		t.Fatal("no final newline")
	}
}

// A value sops leaves in plain text (its unencrypted_suffix) never reaches the disk.
func TestSecretsEncryptRefusesPlainValues(t *testing.T) {
	s, _ := newSecrets(t)
	ctx := context.Background()
	admin := must[string](t)(s.key(ctx))
	if _, err := s.encrypt(ctx, map[string]string{"pass_unencrypted": "plain"}, []string{admin}); err == nil || !strings.Contains(err.Error(), "did not encrypt") {
		t.Fatalf("got %v", err)
	}
}
