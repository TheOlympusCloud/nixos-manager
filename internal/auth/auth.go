// Package auth is the single-user login and the revocable API tokens for agents.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

const SessionTTL = 7 * 24 * time.Hour

var (
	ErrBadPassword = errors.New("wrong password")
	ErrNoToken     = errors.New("no such token")
)

// Scope is what a token may do; each scope includes the ones before it.
type Scope string

const (
	ScopeRead Scope = "read" // look at the repository, the fleet, checks and deployments
	ScopeEdit Scope = "edit" // also change files, check, commit, branch and pull; never push
	ScopeFull Scope = "full" // also push, which deploys
)

var scopeRank = map[Scope]int{ScopeRead: 1, ScopeEdit: 2, ScopeFull: 3}

// Allows says whether a token of scope s may do what needs scope need.
func (s Scope) Allows(need Scope) bool {
	return scopeRank[s] > 0 && scopeRank[need] > 0 && scopeRank[s] >= scopeRank[need]
}

// Token is an agent credential. Only the SHA-256 of the secret is kept.
type Token struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Hash     string    `json:"hash"`
	Scope    Scope     `json:"scope"`
	Created  time.Time `json:"created"`
	Expires  time.Time `json:"expires,omitzero"`
	LastUsed time.Time `json:"last_used,omitzero"`
}

// TokenInfo is a token without its hash, for listing.
type TokenInfo struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Scope    Scope     `json:"scope"`
	Created  time.Time `json:"created"`
	Expires  time.Time `json:"expires,omitzero"`
	LastUsed time.Time `json:"last_used,omitzero"`
}

// Via names the token in the Via trailer of the commits it makes.
func (t TokenInfo) Via() string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, t.Name)
	return fmt.Sprintf("token %s (%s)", name, t.ID)
}

type Auth struct {
	hash []byte
	path string

	mu       sync.Mutex
	sessions map[string]time.Time
	tokens   []Token

	limit    limiter
	slots    chan struct{} // password checks in flight
	slotWait time.Duration
	check    func(hash, password []byte) error
}

// HashPassword returns the bcrypt hash New expects.
func HashPassword(pw string) (string, error) {
	if pw == "" {
		return "", errors.New("empty password")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

// New needs the bcrypt hash of the login password; tokens persist in tokensPath.
func New(passwordHash, tokensPath string) (*Auth, error) {
	if _, err := bcrypt.Cost([]byte(passwordHash)); err != nil {
		return nil, fmt.Errorf("the password hash is missing or not a bcrypt hash (create one with `nixos-manager hash-password`): %w", err)
	}
	a := &Auth{hash: []byte(passwordHash), path: tokensPath, sessions: map[string]time.Time{},
		limit: limiter{now: time.Now, sources: map[string][]time.Time{}},
		slots: make(chan struct{}, loginInFlight), slotWait: loginSlotWait, check: bcrypt.CompareHashAndPassword}
	b, err := os.ReadFile(tokensPath)
	if errors.Is(err, os.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &a.tokens); err != nil {
		return nil, fmt.Errorf("reading %s: %w", tokensPath, err)
	}
	for i := range a.tokens {
		if a.tokens[i].Scope == "" { // made before scopes existed, when every token could do everything
			a.tokens[i].Scope = ScopeFull
		}
	}
	return a, nil
}

// Login returns a new session id for the right password. source names the
// client for the attempt limits (an address, or a browser that logged in
// before); known says it is such a browser, which the global limit spares.
// Over a limit it returns a *LimitError without checking the password.
func (a *Auth) Login(password, source string, known bool) (string, error) {
	undo, wait := a.limit.reserve(source, known)
	if undo == nil {
		return "", &LimitError{Wait: wait}
	}
	select {
	case a.slots <- struct{}{}:
	case <-time.After(a.slotWait):
		undo()
		return "", &LimitError{Wait: time.Second}
	}
	err := a.check(a.hash, []byte(password))
	<-a.slots
	if err != nil {
		return "", ErrBadPassword
	}
	undo()
	id := rand.Text() + rand.Text()
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for k, exp := range a.sessions {
		if now.After(exp) {
			delete(a.sessions, k)
		}
	}
	a.sessions[id] = now.Add(SessionTTL)
	return id, nil
}

// SessionLabel is a short public name for a session, for logs and commit
// trailers; the session id itself stays secret.
func SessionLabel(id string) string {
	h := sha256.Sum256([]byte(id))
	return hex.EncodeToString(h[:4])
}

func (a *Auth) Session(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	exp, ok := a.sessions[id]
	return ok && time.Now().Before(exp)
}

func (a *Auth) Logout(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, id)
}

func hashToken(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// CreateToken returns the new token and its secret, which is never shown
// again. A zero expires means the token does not expire.
func (a *Auth) CreateToken(name string, scope Scope, expires time.Time) (TokenInfo, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return TokenInfo{}, "", errors.New("a token needs a name of 1 to 64 characters, without line breaks or tabs")
	}
	if scopeRank[scope] == 0 {
		return TokenInfo{}, "", errors.New("a token's scope is read, edit or full")
	}
	if !expires.IsZero() && !expires.After(time.Now()) {
		return TokenInfo{}, "", errors.New("the expiry date must be in the future")
	}
	secret := "nxm_" + rand.Text() + rand.Text()
	t := Token{ID: rand.Text()[:12], Name: name, Hash: hashToken(secret), Scope: scope,
		Created: time.Now().UTC().Truncate(time.Second), Expires: expires.UTC().Truncate(time.Second)}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokens = append(a.tokens, t)
	if err := a.save(); err != nil {
		a.tokens = a.tokens[:len(a.tokens)-1]
		return TokenInfo{}, "", err
	}
	return info(t), secret, nil
}

func info(t Token) TokenInfo {
	return TokenInfo{ID: t.ID, Name: t.Name, Scope: t.Scope, Created: t.Created, Expires: t.Expires, LastUsed: t.LastUsed}
}

func (a *Auth) Tokens() []TokenInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]TokenInfo, 0, len(a.tokens))
	for _, t := range a.tokens {
		out = append(out, info(t))
	}
	return out
}

func (a *Auth) RevokeToken(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, t := range a.tokens {
		if t.ID == id {
			a.tokens = append(a.tokens[:i:i], a.tokens[i+1:]...)
			if err := a.save(); err != nil {
				a.tokens = append(a.tokens[:i:i], append([]Token{t}, a.tokens[i:]...)...)
				return err
			}
			return nil
		}
	}
	return ErrNoToken
}

// VerifyToken checks a bearer secret, refuses an expired token, and records
// when it was last used.
func (a *Auth) VerifyToken(secret string) (TokenInfo, bool) {
	h := hashToken(secret)
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.tokens {
		t := &a.tokens[i]
		if subtle.ConstantTimeCompare([]byte(t.Hash), []byte(h)) == 1 {
			if !t.Expires.IsZero() && !time.Now().Before(t.Expires) {
				return TokenInfo{}, false
			}
			if time.Since(t.LastUsed) > time.Minute {
				t.LastUsed = time.Now().UTC().Truncate(time.Second)
				a.save() // best effort: last_used is informational
			}
			return info(*t), true
		}
	}
	return TokenInfo{}, false
}

// save writes the token list atomically, readable by the service user only.
func (a *Auth) save() error {
	b, err := json.MarshalIndent(a.tokens, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(a.path), ".tokens-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), a.path)
}
