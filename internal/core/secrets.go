package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"
)

// Where the secrets live in the working copy. modules/secrets.nix of the
// starter fleet reads the same layout.
const (
	SopsConfig    = ".sops.yaml"          // who can open each file below; written here
	CommonSecrets = "secrets/common.json" // what every machine with a key gets
	hostSecrets   = "secrets/hosts"       // <name>.pub: a machine's SSH host key; <name>.json: what it alone gets
	ReportSecret  = "nixos-manager-report"
	CominSecret   = "comin-token"
	maxSecretSize = 64 << 10
)

// A secret's name is its file in /run/secrets. "sops" is the file's own
// metadata, and sops leaves a value whose name ends in _unencrypted in plain text.
var secretName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

// Secrets keeps the fleet's secrets for sops-nix in the working copy, as sops
// JSON files encrypted for this server's age key and the machines that get
// them. A machine's age key comes from its SSH host key (ssh-to-age), as
// sops-nix makes it on the machine. Values are in plain text only in this
// process and the pipes to sops: never in a file, a log line or a reply, except
// Reveal's.
//
// Which machines may decrypt is decided here, never by the working copy: only
// the keys in TrustFile are recipients, and only SetMachineKey and
// RemoveMachineKey change them. Anyone who can edit files could otherwise add
// a key of their own (secrets/hosts/<name>.pub) and read every secret.
type Secrets struct {
	Repo      *Repo
	KeyFile   string // this server's age identity, made on first use (mode 0600)
	TrustFile string // the approved machine keys, by machine (mode 0600)
	// The tools, from PATH unless set to a path.
	Sops, AgeKeygen, SSHToAge string

	mu        sync.Mutex // the key file
	keyBytes  []byte     // the key file recipient was read from
	recipient string
	ages      sync.Map // SSH key line -> its age recipient
}

func OpenSecrets(dataDir string, repo *Repo) *Secrets {
	return &Secrets{Repo: repo, KeyFile: filepath.Join(dataDir, "sops-age-key"), TrustFile: filepath.Join(dataDir, "sops-machines.json"),
		Sops: "sops", AgeKeygen: "age-keygen", SSHToAge: "ssh-to-age"}
}

// SecretsStatus is what Settings → Secrets shows; never a value.
type SecretsStatus struct {
	Missing   []string        `json:"missing"`   // tools not found on the server: nothing works without them
	Recipient string          `json:"recipient"` // this server's age key (its public half)
	KeyFile   string          `json:"keyFile"`   // its private half, to back up
	Machines  []SecretMachine `json:"machines"`  // machines with an approved key, by name
	// Proposed: keys in secrets/hosts/*.pub that are not approved here (added
	// outside Settings, by a pull or an edit); nothing is encrypted for them.
	Proposed []SecretMachine `json:"proposed"`
	Files    []SecretFile    `json:"files"` // common.json first
	// Stale: .sops.yaml or a file is not encrypted for exactly the keys it should be: Re-encrypt.
	Stale bool `json:"stale"`
}

type SecretMachine struct {
	Name        string `json:"name"`
	Key         string `json:"key"`         // its SSH host key
	Fingerprint string `json:"fingerprint"` // SHA256:…, as ssh-keygen -l prints it
	Recipient   string `json:"recipient"`   // the age key made of it
	Error       string `json:"error,omitempty"`
}

type SecretFile struct {
	Path   string   `json:"path"`
	Host   string   `json:"host"`   // "" for every machine with a key
	Names  []string `json:"names"`  // the secrets in it
	Stale  bool     `json:"stale"`  // encrypted for other keys than it should be
	Locked bool     `json:"locked"` // this server's key cannot open it
	Error  string   `json:"error,omitempty"`
}

func secretsFile(host string) string {
	if host == "" {
		return CommonSecrets
	}
	return hostSecrets + "/" + host + ".json"
}

// run runs a tool with this server's key for sops, stdin as given, and returns stdout.
func (s *Secrets) run(ctx context.Context, stdin []byte, tool string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Env = append(os.Environ(), "SOPS_AGE_KEY_FILE="+s.KeyFile)
	cmd.Stdin = bytes.NewReader(stdin)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		if len(msg) > 500 {
			msg = msg[:500] + "…"
		}
		return nil, fmt.Errorf("%s: %s", filepath.Base(tool), msg)
	}
	return out.Bytes(), nil
}

func (s *Secrets) missing() []string {
	out := []string{}
	for _, t := range []string{s.Sops, s.AgeKeygen, s.SSHToAge} {
		if _, err := exec.LookPath(t); err != nil {
			out = append(out, filepath.Base(t))
		}
	}
	return out
}

// key returns this server's age recipient, making the key the first time.
func (s *Secrets) key(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.KeyFile)
	if errors.Is(err, fs.ErrNotExist) {
		if b, err = s.run(ctx, nil, s.AgeKeygen); err != nil {
			return "", err
		}
		if err = writeAtomic(s.KeyFile, b); err != nil {
			return "", err
		}
		slog.Info("made this server's age key for the fleet's secrets: back it up", "file", s.KeyFile)
	} else if err != nil {
		return "", err
	}
	if s.recipient != "" && bytes.Equal(b, s.keyBytes) {
		return s.recipient, nil
	}
	out, err := s.run(ctx, nil, s.AgeKeygen, "-y", s.KeyFile)
	if err != nil {
		return "", err
	}
	r := strings.TrimSpace(string(out))
	if !strings.HasPrefix(r, "age1") || strings.ContainsAny(r, " \n") {
		return "", fmt.Errorf("%s does not hold one age key", s.KeyFile)
	}
	s.keyBytes, s.recipient = b, r
	return r, nil
}

// parseHostKey finds the ed25519 key in what was pasted: the machine's
// ssh_host_ed25519_key.pub, or ssh-keyscan's "<host> ssh-ed25519 AAAA…".
func parseHostKey(text string) (ssh.PublicKey, error) {
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if i := slices.Index(f, ssh.KeyAlgoED25519); i >= 0 && i+1 < len(f) && !strings.HasPrefix(f[0], "#") {
			if pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(f[i] + " " + f[i+1])); err == nil {
				return pub, nil
			}
		}
	}
	return nil, errorf(ErrInvalid, "no SSH ed25519 host key in it: paste the machine's /etc/ssh/ssh_host_ed25519_key.pub")
}

// ageOf is the age recipient sops-nix decrypts with on the machine with this SSH key.
func (s *Secrets) ageOf(ctx context.Context, key string) (string, error) {
	if r, ok := s.ages.Load(key); ok {
		return r.(string), nil
	}
	out, err := s.run(ctx, []byte(key+"\n"), s.SSHToAge)
	if err != nil {
		return "", err
	}
	r := strings.TrimSpace(string(out))
	if !strings.HasPrefix(r, "age1") || strings.ContainsAny(r, " \n") {
		return "", fmt.Errorf("ssh-to-age made no age key of it")
	}
	s.ages.Store(key, r)
	return r, nil
}

func (s *Secrets) machine(ctx context.Context, name, text string) SecretMachine {
	m := SecretMachine{Name: name}
	pub, err := parseHostKey(text)
	if err != nil {
		m.Error = err.Error()
		return m
	}
	m.Key, m.Fingerprint = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))), ssh.FingerprintSHA256(pub)
	if m.Recipient, err = s.ageOf(ctx, m.Key); err != nil {
		m.Error = err.Error()
	}
	return m
}

// trusted reads the approved machine keys.
func (s *Secrets) trusted() (map[string]string, error) {
	keys := map[string]string{}
	b, err := os.ReadFile(s.TrustFile)
	if errors.Is(err, fs.ErrNotExist) {
		return keys, nil
	} else if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &keys); err != nil {
		return nil, fmt.Errorf("%s: %w", s.TrustFile, err)
	}
	return keys, nil
}

func (s *Secrets) saveTrusted(keys map[string]string) error {
	b, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.TrustFile, append(b, '\n'))
}

// machines are the machines with an approved key, by name.
func (s *Secrets) machines(ctx context.Context) ([]SecretMachine, error) {
	keys, err := s.trusted()
	if err != nil {
		return nil, err
	}
	out := []SecretMachine{}
	for _, name := range slices.Sorted(maps.Keys(keys)) {
		out = append(out, s.machine(ctx, name, keys[name]))
	}
	return out, nil
}

// pubFile is where the working copy names a machine's key, for modules/secrets.nix.
func pubFile(host string) string { return hostSecrets + "/" + host + ".pub" }

// proposed reads the secrets/hosts/<name>.pub that do not hold the approved key.
func (s *Secrets) proposed(ctx context.Context, st secretsState) []SecretMachine {
	out := []SecretMachine{}
	matches, _ := fs.Glob(s.Repo.root.FS(), hostSecrets+"/*.pub")
	for _, p := range matches {
		name := strings.TrimSuffix(path.Base(p), ".pub")
		b, err := s.Repo.root.ReadFile(p)
		switch {
		case !machineName.MatchString(name):
			out = append(out, SecretMachine{Name: name, Error: "not a machine name"})
		case err != nil:
			out = append(out, SecretMachine{Name: name, Error: err.Error()})
		default:
			if m := s.machine(ctx, name, string(b)); m.Error != "" || !slices.ContainsFunc(st.machines, func(o SecretMachine) bool { return o.Name == name && o.Key == m.Key }) {
				out = append(out, m)
			}
		}
	}
	return out
}

// secretsState is what every change starts from.
type secretsState struct {
	admin    string
	machines []SecretMachine
}

func (s *Secrets) state(ctx context.Context) (secretsState, error) {
	if m := s.missing(); len(m) > 0 {
		return secretsState{}, errorf(ErrInvalid, "%s not found on the server: secrets need sops, age and ssh-to-age (the Nix package and the Docker image bring them)", list(m))
	}
	admin, err := s.key(ctx)
	if err != nil {
		return secretsState{}, err
	}
	ms, err := s.machines(ctx)
	return secretsState{admin, ms}, err
}

// usable refuses changes while an approved key cannot be used (ssh-to-age
// failed): encrypting without it would lock that machine out.
func (st secretsState) usable() error {
	for _, m := range st.machines {
		if m.Error != "" {
			return errorf(ErrInvalid, "the key of %s: %s; change or remove it first", m.Name, m.Error)
		}
	}
	return nil
}

func (st secretsState) has(host string) bool {
	return slices.ContainsFunc(st.machines, func(m SecretMachine) bool { return m.Name == host })
}

// recipients of a file: this server, and every machine (host "") or that one.
func (st secretsState) recipients(host string) []string {
	out := []string{st.admin}
	for _, m := range st.machines {
		if (host == "" || m.Name == host) && m.Recipient != "" {
			out = append(out, m.Recipient)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// sopsYAML is .sops.yaml for the machines: the sops command line then
// encrypts each file for the keys the console does. The starter's
// modules/secrets.nix finds a machine's age key by its "- <key> # <name>" line.
func (st secretsState) sopsYAML() string {
	var b strings.Builder
	b.WriteString("# Written by nixos-manager (Settings → Secrets) when a machine's key is added or\n" +
		"# removed: who can open each file in secrets/. A machine's key is its SSH host key,\n" +
		"# in secrets/hosts/<name>.pub; edits here are replaced.\ncreation_rules:\n")
	rule := func(path, host string) {
		fmt.Fprintf(&b, "  - path_regex: '^%s$'\n    key_groups:\n      - age:\n", regexp.QuoteMeta(path))
		for _, r := range st.recipients(host) {
			who := "this nixos-manager server"
			if r != st.admin && host != "" { // the machine's own rule names it, even with a key another has too
				who = host
			} else if i := slices.IndexFunc(st.machines, func(m SecretMachine) bool { return m.Recipient == r }); i >= 0 && r != st.admin {
				who = st.machines[i].Name
			}
			fmt.Fprintf(&b, "          - %s # %s\n", r, who)
		}
	}
	rule(CommonSecrets, "")
	for _, m := range st.machines {
		if m.Error == "" {
			rule(secretsFile(m.Name), m.Name)
		}
	}
	return b.String()
}

// pubs adds each approved machine's .pub when the working copy lacks it or
// holds another key: modules/secrets.nix gives a machine its secrets by it.
func (s *Secrets) pubs(st secretsState, w writes) {
	for _, m := range st.machines {
		if _, ok := w[pubFile(m.Name)]; ok || m.Key == "" {
			continue
		}
		if cur, err := s.Repo.root.ReadFile(pubFile(m.Name)); err != nil || string(cur) != m.Key+"\n" {
			w[pubFile(m.Name)] = []byte(m.Key + "\n")
		}
	}
}

// sopsDoc is what a sops JSON file shows without decrypting it.
type sopsDoc struct {
	names      []string // sorted
	recipients []string // sorted
}

func parseSops(p string, b []byte) (sopsDoc, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(b, &doc); err != nil {
		return sopsDoc{}, errorf(ErrInvalid, "%s is not a sops JSON file", p)
	}
	var meta struct {
		Age []struct {
			Recipient string `json:"recipient"`
		} `json:"age"`
	}
	if raw, ok := doc["sops"]; !ok || json.Unmarshal(raw, &meta) != nil {
		return sopsDoc{}, errorf(ErrInvalid, "%s is not encrypted with sops", p)
	}
	d := sopsDoc{names: slices.Sorted(maps.Keys(doc)), recipients: []string{}}
	d.names = slices.DeleteFunc(d.names, func(n string) bool { return n == "sops" })
	for _, a := range meta.Age {
		d.recipients = append(d.recipients, a.Recipient)
	}
	slices.Sort(d.recipients)
	return d, nil
}

// decrypt opens a sops JSON file; every value must be text.
func (s *Secrets) decrypt(ctx context.Context, p string, b []byte) (map[string]string, error) {
	out, err := s.run(ctx, b, s.Sops, "--config", os.DevNull, "decrypt", "--input-type", "json", "--output-type", "json")
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", p, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", p, err)
	}
	vals := map[string]string{}
	for k, v := range doc {
		t, ok := v.(string)
		if !ok {
			return nil, errorf(ErrInvalid, "%s: %s is not text; edit this file with sops by hand", p, k)
		}
		vals[k] = t
	}
	return vals, nil
}

// encrypt makes a sops JSON file of vals for exactly these recipients (sorted),
// and checks that every value is encrypted before anything reaches the disk.
func (s *Secrets) encrypt(ctx context.Context, vals map[string]string, recipients []string) ([]byte, error) {
	plain, err := json.Marshal(vals)
	if err != nil {
		return nil, err
	}
	out, err := s.run(ctx, plain, s.Sops, "--config", os.DevNull, "encrypt", "--age", strings.Join(recipients, ","),
		"--input-type", "json", "--output-type", "json", "--filename-override", "secrets.json")
	if err != nil {
		return nil, err
	}
	d, err := parseSops("sops's output", out)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, err
	}
	for k := range vals {
		if v, _ := doc[k].(string); !strings.HasPrefix(v, "ENC[") {
			return nil, fmt.Errorf("sops did not encrypt %s", k)
		}
	}
	if len(d.names) != len(vals) || !slices.Equal(d.recipients, recipients) {
		return nil, fmt.Errorf("sops wrote other secrets or keys than asked")
	}
	if !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, '\n')
	}
	return out, nil
}

// rewrite opens p (when it exists) with this server's key, lets edit change
// its values, and encrypts them for recipients. nil: no value left.
func (s *Secrets) rewrite(ctx context.Context, st secretsState, p string, recipients []string, edit func(map[string]string) error) ([]byte, error) {
	vals := map[string]string{}
	b, err := s.Repo.root.ReadFile(p)
	if err == nil {
		d, err := parseSops(p, b)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(d.recipients, st.admin) {
			return nil, errorf(ErrConflict, "this server's key cannot open %s: it was encrypted without it. Put the key it was made with in %s, or delete the file", p, s.KeyFile)
		}
		if vals, err = s.decrypt(ctx, p, b); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err := edit(vals); err != nil {
		return nil, err
	}
	if len(vals) == 0 {
		return nil, nil
	}
	return s.encrypt(ctx, vals, recipients)
}

// files lists the secrets files in the working copy, common.json first.
func (s *Secrets) files() []string {
	out := []string{}
	if _, err := s.Repo.root.Stat(CommonSecrets); err == nil {
		out = append(out, CommonSecrets)
	}
	matches, _ := fs.Glob(s.Repo.root.FS(), hostSecrets+"/*.json")
	return append(out, matches...)
}

func hostOf(p string) string {
	if p == CommonSecrets {
		return ""
	}
	h, _ := strings.CutSuffix(strings.TrimPrefix(p, hostSecrets+"/"), ".json")
	return h
}

// writes are the files a change replaces (nil deletes), all prepared before
// any is written.
type writes map[string][]byte

// reconcile adds .sops.yaml, the approved keys' .pub files, and every file
// whose recipients are not the ones it should have, encrypted again (which
// also gives it a new data key). A machine's file without an approved key is
// left alone.
func (s *Secrets) reconcile(ctx context.Context, st secretsState, w writes) error {
	s.config(st, w)
	s.pubs(st, w)
	for _, p := range s.files() {
		host := hostOf(p)
		if _, ok := w[p]; ok || (host != "" && !st.has(host)) {
			continue
		}
		b, err := s.Repo.root.ReadFile(p)
		if err != nil {
			return err
		}
		d, err := parseSops(p, b)
		if err != nil {
			return err
		}
		want := st.recipients(host)
		if slices.Equal(d.recipients, want) {
			continue
		}
		if w[p], err = s.rewrite(ctx, st, p, want, func(map[string]string) error { return nil }); err != nil {
			return err
		}
	}
	return nil
}

// config adds .sops.yaml when it is not what the machines' keys make.
func (s *Secrets) config(st secretsState, w writes) {
	want := st.sopsYAML()
	if cur, err := s.Repo.root.ReadFile(SopsConfig); err != nil || string(cur) != want {
		w[SopsConfig] = []byte(want)
	}
}

// apply writes the prepared files. Needs Repo.mu.
func (s *Secrets) apply(w writes) error {
	for _, p := range slices.Sorted(maps.Keys(w)) {
		if w[p] == nil {
			if err := s.Repo.root.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		} else if err := s.Repo.writeFile(p, w[p]); err != nil {
			return err
		}
	}
	return nil
}

// Status lists the machines' keys and the secrets' names, making this server's
// key the first time; it decrypts nothing.
func (s *Secrets) Status(ctx context.Context) (SecretsStatus, error) {
	out := SecretsStatus{Missing: s.missing(), KeyFile: s.KeyFile, Machines: []SecretMachine{}, Files: []SecretFile{}}
	if len(out.Missing) > 0 {
		return out, nil
	}
	st, err := s.state(ctx)
	if err != nil {
		return out, err
	}
	out.Recipient, out.Machines, out.Proposed = st.admin, st.machines, s.proposed(ctx, st)
	for _, p := range s.files() {
		f := SecretFile{Path: p, Host: hostOf(p), Names: []string{}}
		b, err := s.Repo.root.ReadFile(p)
		if err == nil {
			var d sopsDoc
			if d, err = parseSops(p, b); err == nil {
				f.Names, f.Locked = d.names, !slices.Contains(d.recipients, st.admin)
				f.Stale = (f.Host == "" || st.has(f.Host)) && !slices.Equal(d.recipients, st.recipients(f.Host))
			}
		}
		if err != nil {
			f.Error = err.Error()
		}
		out.Stale = out.Stale || f.Stale
		out.Files = append(out.Files, f)
	}
	if len(out.Files) > 0 || len(out.Machines) > 0 {
		w := writes{}
		s.config(st, w)
		s.pubs(st, w)
		out.Stale = out.Stale || len(w) > 0
	}
	return out, nil
}

// SetMachineKey approves a machine's SSH host key, new or replacing its last
// one, so it gets common.json and its own file, which are encrypted again for
// it. Only a person or a full token may: the key decides who reads the secrets.
func (s *Secrets) SetMachineKey(ctx context.Context, host, key string) (SecretMachine, error) {
	if !machineName.MatchString(host) {
		return SecretMachine{}, errorf(ErrInvalid, "invalid machine name %q", host)
	}
	if _, err := parseHostKey(key); err != nil {
		return SecretMachine{}, err
	}
	s.Repo.mu.Lock()
	defer s.Repo.mu.Unlock()
	st, err := s.state(ctx)
	if err != nil {
		return SecretMachine{}, err
	}
	m := s.machine(ctx, host, key)
	if m.Error != "" {
		return m, errorf(ErrInvalid, "%s", m.Error)
	}
	st.machines = slices.DeleteFunc(st.machines, func(o SecretMachine) bool { return o.Name == host })
	if err := st.usable(); err != nil {
		return m, err
	}
	st.machines = append(st.machines, m)
	slices.SortFunc(st.machines, func(a, b SecretMachine) int { return strings.Compare(a.Name, b.Name) })
	w := writes{}
	if err := s.reconcile(ctx, st, w); err != nil {
		return m, err
	}
	if err := s.setTrusted(host, m.Key); err != nil {
		return m, err
	}
	return m, s.apply(w)
}

// setTrusted approves host's key ("" removes it).
func (s *Secrets) setTrusted(host, key string) error {
	keys, err := s.trusted()
	if err != nil {
		return err
	}
	if key == "" {
		delete(keys, host)
	} else {
		keys[host] = key
	}
	return s.saveTrusted(keys)
}

// RemoveMachineKey removes a machine's key and its own secrets; common.json
// is encrypted again without it, with a new data key.
func (s *Secrets) RemoveMachineKey(ctx context.Context, host string) error {
	s.Repo.mu.Lock()
	defer s.Repo.mu.Unlock()
	st, err := s.state(ctx)
	if err != nil {
		return err
	}
	if !st.has(host) {
		return errorf(ErrNotFound, "%s has no key here", host)
	}
	st.machines = slices.DeleteFunc(st.machines, func(m SecretMachine) bool { return m.Name == host })
	if err := st.usable(); err != nil {
		return err
	}
	w := writes{pubFile(host): nil, secretsFile(host): nil}
	if err := s.reconcile(ctx, st, w); err != nil {
		return err
	}
	if err := s.setTrusted(host, ""); err != nil {
		return err
	}
	return s.apply(w)
}

func checkSecret(host, name string) error {
	if host != "" && !machineName.MatchString(host) {
		return errorf(ErrInvalid, "invalid machine name %q", host)
	}
	if !secretName.MatchString(name) || name == "sops" || strings.HasSuffix(name, "_unencrypted") {
		return errorf(ErrInvalid, "a secret's name is up to 100 letters, digits, '.', '_' and '-', starting with a letter or digit (not \"sops\", not ending in _unencrypted)")
	}
	return nil
}

// update changes the values of some machines' files ("" for common.json),
// encrypted for their recipients, and keeps .sops.yaml current.
func (s *Secrets) update(ctx context.Context, hosts []string, edit func(host string, vals map[string]string) error) error {
	s.Repo.mu.Lock()
	defer s.Repo.mu.Unlock()
	st, err := s.state(ctx)
	if err != nil {
		return err
	}
	if err := st.usable(); err != nil {
		return err
	}
	w := writes{}
	for _, h := range hosts {
		if h != "" && !st.has(h) {
			return errorf(ErrInvalid, "%s has no key yet: add its SSH host key first", h)
		}
		p := secretsFile(h)
		if w[p], err = s.rewrite(ctx, st, p, st.recipients(h), func(v map[string]string) error { return edit(h, v) }); err != nil {
			return err
		}
	}
	s.config(st, w)
	return s.apply(w)
}

// Set adds or replaces a secret: for every machine with a key (host "") or one.
func (s *Secrets) Set(ctx context.Context, host, name, value string) error {
	if err := checkSecret(host, name); err != nil {
		return err
	}
	if value == "" || len(value) > maxSecretSize || !utf8.ValidString(value) {
		return errorf(ErrInvalid, "a secret's value is text, from 1 byte to 64 KiB")
	}
	return s.update(ctx, []string{host}, func(_ string, v map[string]string) error {
		v[name] = value
		return nil
	})
}

// Delete removes a secret; a file left empty is deleted.
func (s *Secrets) Delete(ctx context.Context, host, name string) error {
	if err := checkSecret(host, name); err != nil {
		return err
	}
	return s.update(ctx, []string{host}, func(_ string, v map[string]string) error {
		if _, ok := v[name]; !ok {
			return errorf(ErrNotFound, "no secret %s in %s", name, secretsFile(host))
		}
		delete(v, name)
		return nil
	})
}

// Reveal decrypts one secret's value, for a person who asked to see it.
func (s *Secrets) Reveal(ctx context.Context, host, name string) (string, error) {
	if err := checkSecret(host, name); err != nil {
		return "", err
	}
	st, err := s.state(ctx)
	if err != nil {
		return "", err
	}
	p := secretsFile(host)
	b, err := s.Repo.root.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return "", errorf(ErrNotFound, "no secret %s in %s", name, p)
	} else if err != nil {
		return "", err
	}
	d, err := parseSops(p, b)
	if err != nil {
		return "", err
	}
	if !slices.Contains(d.names, name) {
		return "", errorf(ErrNotFound, "no secret %s in %s", name, p)
	}
	if !slices.Contains(d.recipients, st.admin) {
		return "", errorf(ErrConflict, "this server's key cannot open %s: it was encrypted without it", p)
	}
	vals, err := s.decrypt(ctx, p, b)
	if err != nil {
		return "", err
	}
	return vals[name], nil
}

// Reencrypt writes .sops.yaml and encrypts every file again whose keys are
// not the machines' (one changed outside the console).
func (s *Secrets) Reencrypt(ctx context.Context) error {
	s.Repo.mu.Lock()
	defer s.Repo.mu.Unlock()
	st, err := s.state(ctx)
	if err != nil {
		return err
	}
	if err := st.usable(); err != nil {
		return err
	}
	w := writes{}
	if err := s.reconcile(ctx, st, w); err != nil {
		return err
	}
	return s.apply(w)
}

// StoreReportTokens puts each machine's own report token in its secrets, for
// every machine with a key; modules/secrets.nix points the report hook at it.
func (s *Service) StoreReportTokens(ctx context.Context) ([]string, error) {
	if s.Deploys == nil || s.Secrets == nil {
		return nil, errorf(ErrInvalid, "deployment reports or secrets are off on this server")
	}
	s.Secrets.Repo.mu.Lock()
	ms, err := s.Secrets.machines(ctx)
	s.Secrets.Repo.mu.Unlock()
	if err != nil {
		return nil, err
	}
	hosts := []string{}
	for _, m := range ms {
		hosts = append(hosts, m.Name)
	}
	if len(hosts) == 0 {
		return nil, errorf(ErrInvalid, "no machine has a key yet: add their SSH host keys first")
	}
	return hosts, s.Secrets.update(ctx, hosts, s.reportToken)
}

func (s *Service) reportToken(host string, v map[string]string) error {
	t, err := s.Deploys.HostToken(host)
	v[ReportSecret] = t
	return err
}

// RotateReportTokens makes a new report token for every machine and stores the
// new ones again in the secrets of the machines that read theirs there (their
// own file has it): those would stop reporting with nothing said. It returns
// them; their secrets then wait to be committed and pushed.
func (s *Service) RotateReportTokens(ctx context.Context) ([]string, error) {
	if s.Deploys == nil {
		return nil, errorf(ErrInvalid, "deployment reports are off on this server")
	}
	if err := s.Deploys.RotateKey(); err != nil {
		return nil, err
	}
	hosts := []string{}
	if s.Secrets == nil || len(s.Secrets.missing()) > 0 {
		return hosts, nil
	}
	s.Secrets.Repo.mu.Lock()
	ms, err := s.Secrets.machines(ctx)
	for _, m := range ms {
		p := secretsFile(m.Name)
		if b, err := s.Secrets.Repo.root.ReadFile(p); err == nil {
			if d, err := parseSops(p, b); err == nil && slices.Contains(d.names, ReportSecret) {
				hosts = append(hosts, m.Name)
			}
		}
	}
	s.Secrets.Repo.mu.Unlock()
	if err == nil && len(hosts) > 0 {
		err = s.Secrets.update(ctx, hosts, s.reportToken)
	}
	if err != nil {
		return nil, fmt.Errorf("the new report tokens were made, but storing them again in the secrets failed (store them under Settings → Secrets → Report tokens): %w", err)
	}
	return hosts, nil
}
