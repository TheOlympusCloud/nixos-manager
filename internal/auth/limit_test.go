package auth

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Argus's case: many wrong passwords sent at once, with the right one among
// them. Only the source's budget is checked, the rest is refused at once
// instead of queueing, and the right password does not get in afterwards.
func TestLoginParallelGuessing(t *testing.T) {
	a := newAuth(t, t.TempDir())
	start := time.Now()
	var wg sync.WaitGroup
	var checked, limited, in atomic.Int32
	try := func(pw string) {
		defer wg.Done()
		_, err := a.Login(pw, "ip 192.0.2.7", false)
		var le *LimitError
		switch {
		case err == nil:
			checked.Add(1)
			in.Add(1)
		case errors.Is(err, ErrBadPassword):
			checked.Add(1)
		case errors.As(err, &le):
			limited.Add(1)
		default:
			t.Errorf("unexpected error %v", err)
		}
	}
	for i := range 31 {
		wg.Add(1)
		pw := fmt.Sprintf("guess %d", i)
		if i == 15 {
			pw = "correct horse"
		}
		go try(pw)
	}
	wg.Wait()
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("the attempts queued for %v", took)
	}
	// A successful login gives its attempt back, so the right password can make
	// room for one more guess: only the wrong ones are bounded.
	if wrong := checked.Load() - in.Load(); wrong > loginPerSource || checked.Load()+limited.Load() != 31 {
		t.Fatalf("%d wrong passwords checked (%d right), %d refused; want at most %d wrong checked", wrong, in.Load(), limited.Load(), loginPerSource)
	}
	if in.Load() == 0 {
		if _, err := a.Login("correct horse", "ip 192.0.2.7", false); !isLimit(err) {
			t.Fatalf("the right password got in after the budget was spent: %v", err)
		}
	}
}

func TestLoginAttackDoesNotLockOutOthers(t *testing.T) {
	a := newAuth(t, t.TempDir())
	for range loginPerSource + 5 {
		a.Login("guess", "ip 192.0.2.7", false)
	}
	start := time.Now()
	if _, err := a.Login("correct horse", "ip 198.51.100.1", false); err != nil {
		t.Fatalf("someone else could not log in: %v", err)
	}
	if took := time.Since(start); took > time.Second {
		t.Fatalf("someone else waited %v", took)
	}
}

func TestLoginGlobalLimitSparesKnownBrowsers(t *testing.T) {
	a := newAuth(t, t.TempDir())
	for i := range loginGlobal {
		if _, err := a.Login("guess", fmt.Sprintf("ip 203.0.113.%d", i), false); !errors.Is(err, ErrBadPassword) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	_, err := a.Login("correct horse", "ip 198.51.100.1", false)
	var le *LimitError
	if !errors.As(err, &le) || le.Wait <= 0 || le.Wait > loginWindow {
		t.Fatalf("a new source after the global budget: %v", err)
	}
	id, ok := a.CheckKnownBrowser(a.KnownBrowser())
	if !ok {
		t.Fatal("own known-browser value rejected")
	}
	if _, err := a.Login("correct horse", "browser "+id, true); err != nil {
		t.Fatalf("a known browser was locked out: %v", err)
	}
}

func TestLoginWindowAndSuccessDoNotCount(t *testing.T) {
	a := newAuth(t, t.TempDir())
	now := time.Now()
	a.limit.now = func() time.Time { return now }
	for i := range 2 * loginPerSource {
		if _, err := a.Login("correct horse", "ip 192.0.2.7", false); err != nil {
			t.Fatalf("successful login %d counted: %v", i, err)
		}
	}
	for range loginPerSource {
		a.Login("guess", "ip 192.0.2.7", false)
	}
	_, err := a.Login("correct horse", "ip 192.0.2.7", false)
	var le *LimitError
	if !errors.As(err, &le) || le.Wait != loginWindow {
		t.Fatalf("after %d failures: %v", loginPerSource, err)
	}
	now = now.Add(loginWindow / 2)
	if _, err := a.Login("correct horse", "ip 192.0.2.7", false); !errors.As(err, &le) || le.Wait != loginWindow/2 {
		t.Fatalf("half a window later: %v", err)
	}
	now = now.Add(loginWindow/2 + time.Second)
	if _, err := a.Login("correct horse", "ip 192.0.2.7", false); err != nil {
		t.Fatalf("after the window: %v", err)
	}
	if n := len(a.limit.sources); n != 0 {
		t.Fatalf("%d sources kept after they stopped counting", n)
	}
}

func TestLoginRefusedSourcesAreNotKept(t *testing.T) {
	a := newAuth(t, t.TempDir())
	for i := range loginGlobal {
		a.Login("guess", fmt.Sprintf("ip 203.0.113.%d", i), false)
	}
	for i := range 1000 { // a distributed attack once the global budget is spent
		a.Login("guess", fmt.Sprintf("ip 10.0.%d.%d", i/250, i%250), false)
	}
	if n := len(a.limit.sources); n != loginGlobal {
		t.Fatalf("%d sources kept, want %d", n, loginGlobal)
	}
}

func TestLoginCapsPasswordChecksInFlight(t *testing.T) {
	a := newAuth(t, t.TempDir())
	var running, most atomic.Int32
	slow := a.check
	a.check = func(h, pw []byte) error {
		n := running.Add(1)
		for m := most.Load(); n > m && !most.CompareAndSwap(m, n); m = most.Load() {
		}
		time.Sleep(100 * time.Millisecond)
		running.Add(-1)
		return slow(h, pw)
	}
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Go(func() {
			if _, err := a.Login("guess", fmt.Sprintf("ip 192.0.2.%d", i), false); !errors.Is(err, ErrBadPassword) {
				t.Errorf("login %d: %v", i, err)
			}
		})
	}
	wg.Wait()
	if most.Load() != loginInFlight {
		t.Fatalf("%d password checks ran at once, want %d", most.Load(), loginInFlight)
	}

	// Slots that stay busy past the wait refuse the login, which then does not count.
	block := make(chan struct{})
	a.check = func(h, pw []byte) error { <-block; return ErrBadPassword }
	a.slotWait = 50 * time.Millisecond
	for i := range loginInFlight {
		wg.Go(func() { a.Login("guess", fmt.Sprintf("ip 198.51.100.%d", i), false) })
	}
	for len(a.slots) < loginInFlight {
		time.Sleep(time.Millisecond)
	}
	_, err := a.Login("correct horse", "ip 192.0.2.200", false)
	var le *LimitError
	if !errors.As(err, &le) || le.Wait != time.Second {
		t.Fatalf("with every slot busy: %v", err)
	}
	if n := len(a.limit.sources["ip 192.0.2.200"]); n != 0 {
		t.Fatalf("a login refused for lack of a slot still counts (%d)", n)
	}
	close(block)
	wg.Wait()
}

func TestKnownBrowser(t *testing.T) {
	a := newAuth(t, t.TempDir())
	v := a.KnownBrowser()
	id, ok := a.CheckKnownBrowser(v)
	if !ok || id == "" {
		t.Fatalf("own value %q rejected", v)
	}
	if v2 := a.KnownBrowser(); v2 == v {
		t.Fatal("every browser gets the same value")
	}
	for _, bad := range []string{"", "x", id, id + ".", id + ".00", "." + v, v + "0"} {
		if _, ok := a.CheckKnownBrowser(bad); ok {
			t.Errorf("accepted %q", bad)
		}
	}
	if _, ok := newAuth(t, t.TempDir()).CheckKnownBrowser(v); ok {
		t.Fatal("another password hash accepted the value")
	}
}

func TestLimitErrorMessage(t *testing.T) {
	for wait, want := range map[time.Duration]string{
		300 * time.Millisecond:      "too many login attempts; try again in 1 s",
		42 * time.Second:            "too many login attempts; try again in 42 s",
		7*time.Minute + time.Second: "too many login attempts; try again in 8 min",
	} {
		if got := (&LimitError{Wait: wait}).Error(); got != want {
			t.Errorf("%v: %q, want %q", wait, got, want)
		}
	}
}

func isLimit(err error) bool {
	var le *LimitError
	return errors.As(err, &le)
}
