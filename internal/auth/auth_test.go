package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if _, _, err := a.CreateToken("  ", ScopeFull, time.Time{}); err == nil {
		t.Fatal("created a token without a name")
	}
	info, secret, err := a.CreateToken("argus", ScopeFull, time.Time{})
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

func TestTokenScopesAndExpiry(t *testing.T) {
	a := newAuth(t, t.TempDir())
	for _, bad := range []struct {
		name    string
		scope   Scope
		expires time.Time
	}{
		{"ci", "", time.Time{}},
		{"ci", "admin", time.Time{}},
		{"ci", ScopeRead, time.Now().Add(-time.Minute)},
		{"line\nbreak", ScopeRead, time.Time{}},
		{"tab\there", ScopeRead, time.Time{}},
	} {
		if _, _, err := a.CreateToken(bad.name, bad.scope, bad.expires); err == nil {
			t.Errorf("created %+v", bad)
		}
	}
	ro, roSecret, err := a.CreateToken("status page", ScopeRead, time.Time{})
	if err != nil || ro.Scope != ScopeRead || !ro.Expires.IsZero() {
		t.Fatalf("read-only token %+v %v", ro, err)
	}
	exp := time.Now().Add(time.Hour)
	ed, edSecret, err := a.CreateToken("agent", ScopeEdit, exp)
	if err != nil || ed.Scope != ScopeEdit || !ed.Expires.Equal(exp.UTC().Truncate(time.Second)) {
		t.Fatalf("expiring token %+v %v", ed, err)
	}
	if got, ok := a.VerifyToken(roSecret); !ok || got.Scope != ScopeRead {
		t.Fatalf("read-only token verified as %+v %v", got, ok)
	}
	if _, ok := a.VerifyToken(edSecret); !ok {
		t.Fatal("token refused before it expires")
	}
	a.tokens[1].Expires = time.Now().Add(-time.Second)
	if _, ok := a.VerifyToken(edSecret); ok {
		t.Fatal("expired token accepted")
	}
	if l := a.Tokens(); len(l) != 2 || l[1].Expires.IsZero() {
		t.Fatalf("an expired token should stay listed until revoked: %+v", l)
	}
}

func TestScopeAllows(t *testing.T) {
	for _, c := range []struct {
		have, need Scope
		ok         bool
	}{
		{ScopeRead, ScopeRead, true}, {ScopeRead, ScopeEdit, false}, {ScopeRead, ScopeFull, false},
		{ScopeEdit, ScopeRead, true}, {ScopeEdit, ScopeEdit, true}, {ScopeEdit, ScopeFull, false},
		{ScopeFull, ScopeFull, true}, {"", ScopeRead, false}, {"admin", ScopeRead, false}, {ScopeFull, "", false},
	} {
		if got := c.have.Allows(c.need); got != c.ok {
			t.Errorf("%q allows %q: %v", c.have, c.need, got)
		}
	}
}

// tokens.json from before scopes keeps working, with full access as before.
func TestTokensWithoutScopeLoadAsFull(t *testing.T) {
	dir := t.TempDir()
	secret := "nxm_old"
	old := `[{"id":"OLDTOKEN0001","name":"old\nname","hash":"` + hashToken(secret) + `","created":"2026-01-01T00:00:00Z"}]`
	if err := os.WriteFile(filepath.Join(dir, "tokens.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	a := newAuth(t, dir)
	got, ok := a.VerifyToken(secret)
	if !ok || got.Scope != ScopeFull || !got.Expires.IsZero() {
		t.Fatalf("old token: %+v %v", got, ok)
	}
	if via := got.Via(); via != "token old name (OLDTOKEN0001)" {
		t.Fatalf("via %q", via)
	}
	if _, _, err := a.CreateToken("new", ScopeRead, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "tokens.json")); !strings.Contains(string(raw), `"scope": "full"`) {
		t.Fatalf("migrated scope not saved: %s", raw)
	}
}
