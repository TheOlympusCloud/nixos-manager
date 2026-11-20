package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func newAuth(t *testing.T, dir string) *Auth {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte("correct horse"), bcrypt.MinCost) // fast: tests log in hundreds of times
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(string(h), filepath.Join(dir, "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestNewRefusesEmptyOrBadHash(t *testing.T) {
	if _, err := New("", filepath.Join(t.TempDir(), "t.json")); err == nil {
		t.Fatal("started without a password hash")
	}
	if _, err := New("plaintext", filepath.Join(t.TempDir(), "t.json")); err == nil {
		t.Fatal("accepted a value that is not a bcrypt hash")
	}
}

func TestLoginSessions(t *testing.T) {
	a := newAuth(t, t.TempDir())
	if _, err := a.Login("wrong", "ip 192.0.2.1", false); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("wrong password: %v", err)
	}
	id, err := a.Login("correct horse", "ip 192.0.2.1", false)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Session(id) || a.Session("") || a.Session("forged") {
		t.Fatal("session check wrong")
	}
	a.Logout(id)
	if a.Session(id) {
		t.Fatal("session still valid after logout")
	}
}

func TestTokensCreateVerifyRevokePersist(t *testing.T) {
	dir := t.TempDir()
	a := newAuth(t, dir)
	if _, _, err := a.CreateToken("  "); err == nil {
		t.Fatal("created a token without a name")
	}
	info, secret, err := a.CreateToken("argus")
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "argus" || !strings.HasPrefix(secret, "nxm_") {
		t.Fatalf("token %+v %q", info, secret)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "tokens.json"))
	if strings.Contains(string(raw), secret) {
		t.Fatal("token secret stored in clear text")
	}
	if fi, _ := os.Stat(filepath.Join(dir, "tokens.json")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("tokens file mode %v", fi.Mode().Perm())
	}
	if got, ok := a.VerifyToken(secret); !ok || got.ID != info.ID {
		t.Fatal("valid token rejected")
	}
	if _, ok := a.VerifyToken(secret + "x"); ok {
		t.Fatal("wrong token accepted")
	}
	if l := a.Tokens(); len(l) != 1 || l[0].LastUsed.IsZero() {
		t.Fatalf("token list %+v", l)
	}

	b := newAuth(t, dir) // reload from disk
	if _, ok := b.VerifyToken(secret); !ok {
		t.Fatal("token lost after restart")
	}
	if err := b.RevokeToken(info.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.VerifyToken(secret); ok {
		t.Fatal("revoked token still accepted")
	}
	if err := b.RevokeToken(info.ID); !errors.Is(err, ErrNoToken) {
		t.Fatalf("revoking twice: %v", err)
	}
	if c := newAuth(t, dir); len(c.Tokens()) != 0 {
		t.Fatal("revocation not persisted")
	}
}
