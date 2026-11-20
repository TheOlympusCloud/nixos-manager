package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// Login attempts are counted before the password is checked, so guesses sent
// in parallel cannot outrun the limits, and a refused attempt never waits in a
// queue. Successful logins do not count.
const (
	loginWindow    = 10 * time.Minute
	loginPerSource = 10 // attempts per window from one address, or one known browser
	loginGlobal    = 50 // attempts per window from all addresses together; known browsers are spared
	loginInFlight  = 4  // password checks running at once
	loginSlotWait  = 2 * time.Second
)

// LimitError refuses a login without checking the password.
type LimitError struct{ Wait time.Duration }

func (e *LimitError) Error() string {
	if s := int((e.Wait + time.Second - 1) / time.Second); s < 90 {
		return fmt.Sprintf("too many login attempts; try again in %d s", max(s, 1))
	}
	return fmt.Sprintf("too many login attempts; try again in %d min", int((e.Wait+time.Minute-1)/time.Minute))
}

// limiter keeps the start times of the attempts in the window, oldest first.
type limiter struct {
	mu      sync.Mutex
	now     func() time.Time
	sources map[string][]time.Time
	all     []time.Time // attempts of sources that are not known browsers
	swept   time.Time
}

// reserve counts an attempt and returns the func that takes it back, or nil
// and how long to wait.
func (l *limiter) reserve(source string, known bool) (func(), time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.swept) > loginWindow { // forget sources that stopped trying
		for k, ts := range l.sources {
			l.keep(k, l.recent(ts, now))
		}
		l.swept = now
	}
	mine := l.recent(l.sources[source], now)
	l.keep(source, mine)
	if len(mine) >= loginPerSource {
		return nil, mine[0].Add(loginWindow).Sub(now)
	}
	if !known {
		if l.all = l.recent(l.all, now); len(l.all) >= loginGlobal {
			return nil, l.all[0].Add(loginWindow).Sub(now)
		}
		l.all = append(l.all, now)
	}
	l.sources[source] = append(mine, now)
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.keep(source, remove(l.sources[source], now))
		if !known {
			l.all = remove(l.all, now)
		}
	}, 0
}

func (l *limiter) recent(ts []time.Time, now time.Time) []time.Time {
	i := 0
	for i < len(ts) && now.Sub(ts[i]) >= loginWindow {
		i++
	}
	return ts[i:]
}

// keep stores a source's attempts; a source without any is dropped, so
// refused attempts never grow the map.
func (l *limiter) keep(source string, ts []time.Time) {
	if len(ts) == 0 {
		delete(l.sources, source)
	} else {
		l.sources[source] = ts
	}
}

func remove(ts []time.Time, at time.Time) []time.Time {
	if i := slices.Index(ts, at); i >= 0 {
		return slices.Delete(ts, i, i+1)
	}
	return ts
}

// KnownBrowser returns the cookie value for a browser that has logged in: a
// random id signed with a key derived from the password hash, so it survives
// restarts and stops working when the password changes.
func (a *Auth) KnownBrowser() string {
	id := rand.Text()
	return id + "." + a.sign(id)
}

// CheckKnownBrowser returns the browser's id when KnownBrowser made the value.
func (a *Auth) CheckKnownBrowser(v string) (string, bool) {
	id, sig, _ := strings.Cut(v, ".")
	if id == "" || !hmac.Equal([]byte(sig), []byte(a.sign(id))) {
		return "", false
	}
	return id, true
}

func (a *Auth) sign(id string) string {
	key := sha256.Sum256(append([]byte("nixos-manager known browser\x00"), a.hash...))
	m := hmac.New(sha256.New, key[:])
	m.Write([]byte(id))
	return hex.EncodeToString(m.Sum(nil))
}
