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

	"golang.org/x/crypto/bcrypt"
)

const SessionTTL = 7 * 24 * time.Hour

var (
	ErrBadPassword = errors.New("wrong password")
	ErrNoToken     = errors.New("no such token")
)

// Token is an agent credential. Only the SHA-256 of the secret is kept.
type Token struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Hash     string    `json:"hash"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used,omitzero"`
}

// TokenInfo is a token without its hash, for listing.
type TokenInfo struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used,omitzero"`
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

// CreateToken returns the new token and its secret, which is never shown again.
func (a *Auth) CreateToken(name string) (TokenInfo, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return TokenInfo{}, "", errors.New("a token needs a name of 1 to 64 characters")
	}
	secret := "nxm_" + rand.Text() + rand.Text()
	t := Token{ID: rand.Text()[:12], Name: name, Hash: hashToken(secret), Created: time.Now().UTC().Truncate(time.Second)}
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
	return TokenInfo{ID: t.ID, Name: t.Name, Created: t.Created, LastUsed: t.LastUsed}
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

// VerifyToken checks a bearer secret and records when it was last used.
func (a *Auth) VerifyToken(secret string) (TokenInfo, bool) {
	h := hashToken(secret)
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.tokens {
		t := &a.tokens[i]
		if subtle.ConstantTimeCompare([]byte(t.Hash), []byte(h)) == 1 {
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
