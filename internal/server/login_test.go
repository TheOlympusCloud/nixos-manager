package server

import (
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"strings"
	"testing"
)

// login posts a password with an optional X-Forwarded-For and returns the response.
func (e *env) loginAs(t *testing.T, c *http.Client, pw, forwardedFor string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/login", strings.NewReader(`{"password":"`+pw+`"}`))
	req.Header.Set("Content-Type", "application/json")
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func TestLoginLimitOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.login(t) // this browser is now known

	attacker := &http.Client{}
	for range 10 {
		if code := e.loginAs(t, attacker, "nope", "").StatusCode; code != 401 {
			t.Fatalf("wrong password: %d", code)
		}
	}
	resp := e.loginAs(t, attacker, "pw", "")
	if wait, _ := strconv.Atoi(resp.Header.Get("Retry-After")); resp.StatusCode != 429 || wait < 1 || wait > 600 {
		t.Fatalf("over the limit: %d, Retry-After %q", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	var er struct{ Error string }
	if code := e.do(t, "POST", "/api/login", map[string]string{"password": "nope"}, &er); code != 401 {
		t.Fatalf("a known browser shares the attacker's address and was refused: %d %q", code, er.Error)
	}
	if code := e.loginAs(t, &http.Client{}, "pw", "198.51.100.9").StatusCode; code != 204 {
		t.Fatalf("another client behind the local proxy: %d", code)
	}
}

func TestKnownBrowserCookie(t *testing.T) {
	e := newEnv(t)
	jar, _ := cookiejar.New(nil)
	resp := e.loginAs(t, &http.Client{Jar: jar}, "pw", "")
	var known *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == browserCookie {
			known = c
		}
	}
	if known == nil || !known.HttpOnly || known.SameSite != http.SameSiteStrictMode || known.Path != "/api/login" || known.MaxAge < 300*24*3600 {
		t.Fatalf("known-browser cookie %+v", known)
	}
	if _, ok := e.auth.CheckKnownBrowser(known.Value); !ok {
		t.Fatal("cookie value not accepted")
	}
	if resp := e.loginAs(t, &http.Client{}, "nope", ""); hasCookie(resp, browserCookie) {
		t.Fatal("a failed login marked the browser as known")
	}
}

func hasCookie(resp *http.Response, name string) bool {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return true
		}
	}
	return false
}

func TestClientIP(t *testing.T) {
	for _, c := range []struct{ remote, xff, want string }{
		{"192.0.2.1:5000", "", "192.0.2.1"},
		{"192.0.2.1:5000", "203.0.113.9", "192.0.2.1"}, // only a proxy on this machine is believed
		{"127.0.0.1:5000", "", "127.0.0.1"},
		{"127.0.0.1:5000", "203.0.113.9", "203.0.113.9"},
		{"127.0.0.1:5000", "10.0.0.1, 203.0.113.9", "203.0.113.9"}, // the proxy appends; earlier entries are the client's
		{"127.0.0.1:5000", "not-an-address", "127.0.0.1"},
		{"[::1]:5000", "2001:db8:1:2:3:4:5:6", "2001:db8:1:2::/64"},
		{"[2001:db8::1]:5000", "", "2001:db8::/64"},
		{"[::ffff:192.0.2.1]:5000", "", "192.0.2.1"},
		{"garbage", "", "garbage"},
	} {
		r, _ := http.NewRequest("POST", "/api/login", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := (&Server{}).clientIP(r); got != c.want {
			t.Errorf("%s with X-Forwarded-For %q: %q, want %q", c.remote, c.xff, got, c.want)
		}
	}
	r, _ := http.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = "127.0.0.1:1"
	r.Header.Add("X-Forwarded-For", "203.0.113.1")
	r.Header.Add("X-Forwarded-For", "203.0.113.2")
	if got := (&Server{}).clientIP(r); got != "203.0.113.2" {
		t.Errorf("two X-Forwarded-For headers: %q", got)
	}
}

// With Docker, a proxy on the host reaches the container from the bridge
// gateway: naming it as a trusted proxy makes its X-Forwarded-For count.
func TestClientIPBehindTrustedProxy(t *testing.T) {
	trusted, err := ParseTrustedProxies("172.18.0.1, fd00::/8")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{TrustedProxies: trusted}
	for _, c := range []struct{ remote, xff, want string }{
		{"172.18.0.1:5000", "100.64.0.5", "100.64.0.5"},
		{"172.18.0.1:5000", "", "172.18.0.1"},
		{"172.18.0.2:5000", "100.64.0.5", "172.18.0.2"}, // not the proxy: the header is the client's own
		{"[fd00::1]:5000", "100.64.0.5", "100.64.0.5"},
		{"127.0.0.1:5000", "100.64.0.5", "100.64.0.5"}, // loopback stays trusted
		{"192.0.2.1:5000", "100.64.0.5", "192.0.2.1"},
	} {
		r, _ := http.NewRequest("POST", "/api/login", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := s.clientIP(r); got != c.want {
			t.Errorf("%s with X-Forwarded-For %q: %q, want %q", c.remote, c.xff, got, c.want)
		}
	}
	if got := (&Server{}).clientIP(&http.Request{RemoteAddr: "172.18.0.1:5000", Header: http.Header{"X-Forwarded-For": {"100.64.0.5"}}}); got != "172.18.0.1" {
		t.Errorf("without the setting the gateway is the client: %q", got)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	got, err := ParseTrustedProxies(" 172.18.0.1,10.0.0.0/8  fd00::/8 ")
	if err != nil || len(got) != 3 || got[0].String() != "172.18.0.1/32" || got[1].String() != "10.0.0.0/8" || got[2].String() != "fd00::/8" {
		t.Fatalf("%v %v", got, err)
	}
	if got, err := ParseTrustedProxies(""); err != nil || got != nil {
		t.Fatalf("empty: %v %v", got, err)
	}
	for _, bad := range []string{"proxy.lan", "10.0.0.0/33", "10.0.0.1/8"} {
		if _, err := ParseTrustedProxies(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

// An anonymous login reads a few KiB at most, before any limit is checked.
func TestLoginBodyIsSmall(t *testing.T) {
	e := newEnv(t)
	if code := e.loginAs(t, &http.Client{}, strings.Repeat("a", maxLoginBody), "").StatusCode; code != 400 {
		t.Fatalf("a login body over %d bytes: %d", maxLoginBody, code)
	}
	if code := e.loginAs(t, &http.Client{}, "pw", "").StatusCode; code != 204 {
		t.Fatalf("a normal login: %d", code)
	}
}
