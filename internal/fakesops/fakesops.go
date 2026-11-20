// Package fakesops plays sops, age-keygen and ssh-to-age in tests, as the test
// binary itself: a test package's TestMain calls Main first, and Tools puts
// scripts that run it on a path. Its "encryption" is base64 with the recipients
// written down, so a key that is not one of them cannot decrypt. With
// NIXMGR_NIX_TESTS=1 Tools returns the real tools instead (CI's Real Nix job).
package fakesops

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// Paths of the three tools a test gives core.Secrets.
type Paths struct{ Sops, AgeKeygen, SSHToAge string }

const toolEnv = "NIXMGR_FAKE_TOOL"

// Main runs the tool a script asked for and exits; it returns at once otherwise.
func Main() {
	tool := os.Getenv(toolEnv)
	if tool == "" {
		return
	}
	out, err := run(tool, os.Args[1:], os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", tool, err)
		os.Exit(1)
	}
	os.Stdout.Write(out)
	os.Exit(0)
}

// Tools returns the tools for a test: the real ones with NIXMGR_NIX_TESTS=1
// (the test fails when one is missing), else scripts that run the fakes.
func Tools(t testing.TB) Paths {
	t.Helper()
	if os.Getenv("NIXMGR_NIX_TESTS") != "" {
		for _, tool := range []string{"sops", "age-keygen", "ssh-to-age"} {
			if _, err := exec.LookPath(tool); err != nil {
				t.Fatalf("real Nix tests need %s on PATH (nix shell nixpkgs#sops nixpkgs#age nixpkgs#ssh-to-age)", tool)
			}
		}
		return Paths{"sops", "age-keygen", "ssh-to-age"}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script := func(tool string) string {
		p := filepath.Join(dir, tool)
		body := fmt.Sprintf("#!/bin/sh\n%s=%s exec '%s' \"$@\"\n", toolEnv, tool, strings.ReplaceAll(exe, "'", `'\''`))
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	return Paths{script("sops"), script("age-keygen"), script("ssh-to-age")}
}

// MachineIdentity writes the age identity a machine decrypts with, made from
// its SSH host key as sops-nix makes it (ssh-to-age -private-key), and returns
// its path. Real or fake as Tools.
func MachineIdentity(t testing.TB, tools Paths, priv ed25519.PrivateKey) string {
	t.Helper()
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(tools.SSHToAge, "-private-key")
	cmd.Stdin = bytes.NewReader(pem.EncodeToMemory(block))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ssh-to-age -private-key: %v", err)
	}
	p := filepath.Join(t.TempDir(), "machine-age-key")
	if err := os.WriteFile(p, out, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// HostKey makes an SSH ed25519 host key: its public line and private key.
func HostKey(t testing.TB) (string, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sp))), priv
}

// A fake identity is AGE-SECRET-KEY-1FAKE<ID>, its recipient age1fake<id>.
const identityPrefix = "AGE-SECRET-KEY-1FAKE"

func recipientOf(id string) string { return "age1fake" + strings.ToLower(id) }

func idOf(pub []byte) string {
	h := sha256.Sum256(pub)
	return strings.ToUpper(hex.EncodeToString(h[:8]))
}

func run(tool string, args []string, stdin io.Reader) ([]byte, error) {
	in, err := io.ReadAll(stdin)
	if err != nil {
		return nil, err
	}
	switch tool {
	case "age-keygen":
		if len(args) == 2 && args[0] == "-y" {
			b, err := os.ReadFile(args[1])
			if err != nil {
				return nil, err
			}
			for _, line := range strings.Split(string(b), "\n") {
				if id, ok := strings.CutPrefix(strings.TrimSpace(line), identityPrefix); ok {
					return []byte(recipientOf(id) + "\n"), nil
				}
			}
			return nil, fmt.Errorf("no identity in %s", args[1])
		}
		if len(args) != 0 {
			return nil, fmt.Errorf("unexpected arguments %q", args)
		}
		b := make([]byte, 8)
		rand.Read(b)
		id := strings.ToUpper(hex.EncodeToString(b))
		return fmt.Appendf(nil, "# created: 2026-11-20T12:00:00Z\n# public key: %s\n%s%s\n", recipientOf(id), identityPrefix, id), nil
	case "ssh-to-age":
		if slices.Equal(args, []string{"-private-key"}) {
			k, err := ssh.ParseRawPrivateKey(in)
			if err != nil {
				return nil, err
			}
			priv, ok := k.(*ed25519.PrivateKey)
			if !ok {
				return nil, fmt.Errorf("not an ed25519 key")
			}
			return []byte(identityPrefix + idOf(priv.Public().(ed25519.PublicKey)) + "\n"), nil
		}
		if len(args) != 0 {
			return nil, fmt.Errorf("unexpected arguments %q", args)
		}
		pub, _, _, _, err := ssh.ParseAuthorizedKey(in)
		if err != nil {
			return nil, err
		}
		if pub.Type() != ssh.KeyAlgoED25519 {
			return nil, nil // the real one skips other keys, printing nothing
		}
		return []byte(recipientOf(idOf(pub.(ssh.CryptoPublicKey).CryptoPublicKey().(ed25519.PublicKey))) + "\n"), nil
	case "sops":
		return sops(args, in)
	}
	return nil, fmt.Errorf("unknown tool")
}

type meta struct {
	Age []struct {
		Recipient string `json:"recipient"`
		Enc       string `json:"enc"`
	} `json:"age"`
	Version string `json:"version"`
}

// sops takes what nixos-manager runs: --config /dev/null, then encrypt or
// decrypt with JSON on stdin.
func sops(args []string, in []byte) ([]byte, error) {
	var cmd, age string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "encrypt", "decrypt":
			cmd = a
		case "--age", "--config", "--input-type", "--output-type", "--filename-override":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("%s needs a value", a)
			}
			if a == "--age" {
				age = args[i+1]
			}
			if a == "--config" && args[i+1] != os.DevNull {
				return nil, fmt.Errorf("unexpected config %s", args[i+1])
			}
			i++
		default:
			return nil, fmt.Errorf("unexpected argument %q (files are passed on stdin)", a)
		}
	}
	var doc map[string]any
	if err := json.Unmarshal(in, &doc); err != nil {
		return nil, fmt.Errorf("Error unmarshalling input json: %v", err)
	}
	switch cmd {
	case "encrypt":
		if age == "" {
			return nil, fmt.Errorf("no keys")
		}
		if _, ok := doc["sops"]; ok {
			return nil, fmt.Errorf("already encrypted")
		}
		var m meta
		for _, r := range strings.Split(age, ",") {
			m.Age = append(m.Age, struct {
				Recipient string `json:"recipient"`
				Enc       string `json:"enc"`
			}{r, "fake"})
		}
		m.Version = "fake"
		for k, v := range doc {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("only text values in the fake")
			}
			if !strings.HasSuffix(k, "_unencrypted") { // as sops's default unencrypted_suffix
				doc[k] = "ENC[AES256_GCM,data:" + base64.StdEncoding.EncodeToString([]byte(s)) + ",iv:fake,tag:fake,type:str]"
			}
		}
		doc["sops"] = m
		return json.MarshalIndent(doc, "", "    ")
	case "decrypt":
		raw, err := json.Marshal(doc["sops"])
		if err != nil {
			return nil, err
		}
		var m meta
		if err := json.Unmarshal(raw, &m); err != nil || len(m.Age) == 0 {
			return nil, fmt.Errorf("sops metadata not found")
		}
		mine := map[string]bool{}
		if b, err := os.ReadFile(os.Getenv("SOPS_AGE_KEY_FILE")); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if id, ok := strings.CutPrefix(strings.TrimSpace(line), identityPrefix); ok {
					mine[recipientOf(id)] = true
				}
			}
		}
		if !slices.ContainsFunc(m.Age, func(a struct {
			Recipient string `json:"recipient"`
			Enc       string `json:"enc"`
		}) bool {
			return mine[a.Recipient]
		}) {
			return nil, fmt.Errorf("Failed to get the data key required to decrypt the SOPS file.")
		}
		delete(doc, "sops")
		for k, v := range doc {
			s, _ := v.(string)
			if data, ok := strings.CutPrefix(s, "ENC[AES256_GCM,data:"); ok {
				b, err := base64.StdEncoding.DecodeString(data[:strings.Index(data, ",")])
				if err != nil {
					return nil, err
				}
				doc[k] = string(b)
			}
		}
		return json.Marshal(doc)
	}
	return nil, fmt.Errorf("encrypt or decrypt")
}
