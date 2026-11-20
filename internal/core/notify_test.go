package core

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNotifySettings(t *testing.T) {
	dir := t.TempDir()
	n := must[*Notifier](t)(OpenNotifier(dir, nil))
	t0 := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	email := func() *EmailChannel {
		return &EmailChannel{Host: "smtp.example.org", Port: 587, Security: "starttls", Username: "pablo", Password: "s3cret", From: "Fleet <fleet@example.org>", To: []string{"pablo@example.org", " "}}
	}
	for name, in := range map[string]NotifySettings{
		"console":       {ConsoleURL: "ftp://x"},
		"ntfy topic":    {Ntfy: &NtfyChannel{URL: "https://ntfy.sh/"}},
		"ntfy scheme":   {Ntfy: &NtfyChannel{URL: "file:///etc/passwd"}},
		"ntfy token":    {Ntfy: &NtfyChannel{URL: "https://ntfy.sh/fleet", Token: "a\nb"}},
		"webhook":       {Webhook: &WebhookChannel{URL: "not a url"}},
		"email host":    {Email: &EmailChannel{Host: "smtp.example.org:25", Port: 25, Security: "none", From: "a@b.c", To: []string{"d@e.f"}}},
		"email port":    {Email: &EmailChannel{Host: "smtp", Port: 0, Security: "none", From: "a@b.c", To: []string{"d@e.f"}}},
		"email sec":     {Email: &EmailChannel{Host: "smtp", Port: 25, Security: "ssl", From: "a@b.c", To: []string{"d@e.f"}}},
		"plain pass":    {Email: &EmailChannel{Host: "smtp", Port: 25, Security: "none", Username: "u", Password: "p", From: "a@b.c", To: []string{"d@e.f"}}},
		"from":          {Email: &EmailChannel{Host: "smtp", Port: 25, Security: "none", From: "nobody", To: []string{"d@e.f"}}},
		"no recipients": {Email: &EmailChannel{Host: "smtp", Port: 25, Security: "none", From: "a@b.c", To: []string{" "}}},
		"bad recipient": {Email: &EmailChannel{Host: "smtp", Port: 25, Security: "none", From: "a@b.c", To: []string{"d@e.f\r\nBcc: x@y.z"}}},
		"no password":   {Email: &EmailChannel{Host: "smtp", Port: 587, Security: "starttls", Username: "u", From: "a@b.c", To: []string{"d@e.f"}}},
	} {
		if _, err := n.Save(in, t0); !isKind(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}

	got := must[NotifySettings](t)(n.Save(NotifySettings{ConsoleURL: "http://manager.lan:8080/", Ntfy: &NtfyChannel{URL: "https://ntfy.sh/fleet", Token: "tk_1"},
		Webhook: &WebhookChannel{URL: "https://hooks.example/x"}, Email: email()}, t0))
	if got.ConsoleURL != "http://manager.lan:8080" || got.Ntfy.Token != "" || !got.Ntfy.HasToken || got.Email.Password != "" || !got.Email.HasPassword ||
		!slices.Equal(got.Email.To, []string{"pablo@example.org"}) || got.Email.From != `"Fleet" <fleet@example.org>` || !got.Ntfy.Since.Equal(t0) {
		t.Fatalf("saved: %+v %+v %+v", got, got.Ntfy, got.Email)
	}
	if fi, err := os.Stat(filepath.Join(dir, "notify.json")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("notify.json: %v %v", fi, err)
	}
	// Saved again as Settings sends it back: the secrets stay, and so does since.
	t1 := t0.Add(time.Hour)
	got = must[NotifySettings](t)(n.Save(got, t1))
	if n.settings.Ntfy.Token != "tk_1" || n.settings.Email.Password != "s3cret" || !n.settings.Ntfy.Since.Equal(t0) || !n.settings.Email.Since.Equal(t0) {
		t.Fatalf("kept: %+v %+v", n.settings.Ntfy, n.settings.Email)
	}
	// Another server never gets the saved secret; new recipients start from now.
	moved := got
	moved.Ntfy = &NtfyChannel{URL: "https://ntfy.evil.example/fleet", HasToken: true}
	e := email()
	e.Password, e.HasPassword, e.Host, e.To = "", true, "smtp.evil.example", []string{"other@example.org"}
	moved.Email = e
	if _, err := n.Save(moved, t1); !isKind(err, ErrInvalid) {
		t.Fatalf("the password went to another server: %v", err)
	}
	e.Host = "smtp.example.org"
	got = must[NotifySettings](t)(n.Save(moved, t1))
	if n.settings.Ntfy.Token != "" || !n.settings.Ntfy.Since.Equal(t1) || n.settings.Email.Password != "s3cret" || !n.settings.Email.Since.Equal(t1) {
		t.Fatalf("moved: %+v %+v", n.settings.Ntfy, n.settings.Email)
	}
	again := must[*Notifier](t)(OpenNotifier(dir, nil))
	if s, _ := again.Settings(); s.Email == nil || !s.Email.HasPassword || again.settings.Email.Password != "s3cret" {
		t.Fatalf("after reopening: %+v", s)
	}
	must[NotifySettings](t)(n.Save(NotifySettings{}, t1))
	if s, _ := n.Settings(); s.Ntfy != nil || s.Email != nil || s.Webhook != nil {
		t.Fatalf("cleared: %+v", s)
	}
	if _, err := n.Test(context.Background(), t1); !isKind(err, ErrInvalid) {
		t.Fatalf("a test without channels: %v", err)
	}
}

func isKind(err, kind error) bool { return err != nil && errors.Is(err, kind) }

// smtpMessage is what the fake SMTP server received.
type smtpMessage struct {
	auth, from string
	to         []string
	data       string
	tls        bool
}

// fakeSMTP speaks just enough SMTP to take one message per connection. With
// conf, it offers STARTTLS, or (implicit) speaks TLS from the start.
func fakeSMTP(t *testing.T, conf *tls.Config, implicit bool) (string, chan smtpMessage) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got := make(chan smtpMessage, 4)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				var m smtpMessage
				if implicit {
					conn, m.tls = tls.Server(conn, conf), true
				}
				tp := textproto.NewConn(conn)
				tp.PrintfLine("220 fake ESMTP")
				for {
					line, err := tp.ReadLine()
					if err != nil {
						return
					}
					cmd, arg, _ := strings.Cut(line, " ")
					switch strings.ToUpper(cmd) {
					case "EHLO":
						if conf != nil && !m.tls {
							tp.PrintfLine("250-fake")
							tp.PrintfLine("250-STARTTLS")
						} else {
							tp.PrintfLine("250-fake")
						}
						tp.PrintfLine("250 AUTH PLAIN")
					case "STARTTLS":
						tp.PrintfLine("220 go ahead")
						conn, m.tls = tls.Server(conn, conf), true
						tp = textproto.NewConn(conn)
					case "AUTH":
						m.auth = arg
						tp.PrintfLine("235 ok")
					case "MAIL":
						m.from = arg
						tp.PrintfLine("250 ok")
					case "RCPT":
						m.to = append(m.to, arg)
						tp.PrintfLine("250 ok")
					case "DATA":
						tp.PrintfLine("354 go")
						b, _ := tp.ReadDotBytes()
						m.data = string(b)
						tp.PrintfLine("250 queued")
					case "QUIT":
						tp.PrintfLine("221 bye")
						got <- m
						return
					default:
						tp.PrintfLine("502 what")
					}
				}
			}()
		}
	}()
	return ln.Addr().String(), got
}

func TestNotifyChannels(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	ev := Event{Key: "deploy-failed:pi:" + sha('a'), Kind: "deploy-failed", Severity: "error", Hosts: []string{"pi"}, Link: "#/deployments",
		Title: "pi failed to deploy aaaaaaa — ünïcode", Message: "error: boom\nIt still runs bbbbbbb."}
	n := must[*Notifier](t)(OpenNotifier(t.TempDir(), nil))

	type req struct {
		header http.Header
		body   string
	}
	reqs := make(chan req, 4)
	code := http.StatusOK
	var mu sync.Mutex
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		reqs <- req{r.Header.Clone(), string(b)}
		mu.Lock()
		defer mu.Unlock()
		w.WriteHeader(code)
		w.Write([]byte("nope"))
	}))
	defer srv.Close()
	n.Client = srv.Client()
	n.RootCAs = srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs

	s := NotifySettings{ConsoleURL: "http://manager.lan:8080", Ntfy: &NtfyChannel{URL: srv.URL + "/fleet", Token: "tk_1"}, Webhook: &WebhookChannel{URL: srv.URL + "/hook"}}
	ok(t, n.send(ctx, s, "ntfy", ev, now))
	r := <-reqs
	title, _ := new(mime.WordDecoder).DecodeHeader(r.header.Get("Title"))
	if title != ev.Title || r.header.Get("Priority") != "high" || r.header.Get("Tags") != "rotating_light" || r.header.Get("Click") != "http://manager.lan:8080/#/deployments" ||
		r.header.Get("Authorization") != "Bearer tk_1" || r.body != ev.Message {
		t.Errorf("ntfy: %q %v %q", title, r.header, r.body)
	}
	ok(t, n.send(ctx, s, "webhook", ev, now))
	r = <-reqs
	var hook map[string]any
	if err := json.Unmarshal([]byte(r.body), &hook); err != nil || r.header.Get("Content-Type") != "application/json" || hook["key"] != ev.Key || hook["title"] != ev.Title ||
		hook["url"] != "http://manager.lan:8080/#/deployments" || hook["at"] != "2026-11-20T08:00:00Z" || hook["severity"] != "error" {
		t.Errorf("webhook: %v %s", err, r.body)
	}
	mu.Lock()
	code = http.StatusInternalServerError
	mu.Unlock()
	if err := n.send(ctx, s, "webhook", ev, now); err == nil || !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "nope") {
		t.Errorf("a failed webhook: %v", err)
	}
	<-reqs

	conf := srv.TLS
	for _, c := range []struct {
		security string
		conf     *tls.Config
		implicit bool
		auth     bool
	}{{"none", nil, false, false}, {"starttls", conf, false, true}, {"tls", conf, true, true}} {
		addr, got := fakeSMTP(t, c.conf, c.implicit)
		host, port, _ := net.SplitHostPort(addr)
		e := &EmailChannel{Host: host, Port: atoi(port), Security: c.security, From: "Fleet <fleet@example.org>", To: []string{"pablo@example.org", "ops@example.org"}}
		if c.auth {
			e.Username, e.Password = "pablo", "s3cret"
		}
		ok(t, n.send(ctx, NotifySettings{ConsoleURL: "http://manager.lan:8080", Email: e}, "email", ev, now))
		m := <-got
		if m.tls != (c.security != "none") || m.from != "FROM:<fleet@example.org>" || !slices.Equal(m.to, []string{"TO:<pablo@example.org>", "TO:<ops@example.org>"}) {
			t.Errorf("%s: %+v", c.security, m)
		}
		if want := "PLAIN " + base64.StdEncoding.EncodeToString([]byte("\x00pablo\x00s3cret")); c.auth && m.auth != want {
			t.Errorf("%s: auth %q", c.security, m.auth)
		}
		msg, err := (&textproto.Reader{R: bufio.NewReader(strings.NewReader(m.data))}).ReadMIMEHeader()
		if err != nil {
			t.Fatalf("%s: %v\n%s", c.security, err, m.data)
		}
		subject, _ := new(mime.WordDecoder).DecodeHeader(msg.Get("Subject"))
		if subject != "[nixos-manager] "+ev.Title || msg.Get("To") != "pablo@example.org, ops@example.org" || msg.Get("Content-Type") != "text/plain; charset=utf-8" ||
			!strings.Contains(m.data, "error: boom\nIt still runs bbbbbbb.\n\nhttp://manager.lan:8080/#/deployments") { // the reader undoes CRLF
			t.Errorf("%s: message\n%s", c.security, m.data)
		}
	}
	// STARTTLS required but not offered: nothing is sent in the clear.
	addr, _ := fakeSMTP(t, nil, false)
	host, port, _ := net.SplitHostPort(addr)
	if err := n.send(ctx, NotifySettings{Email: &EmailChannel{Host: host, Port: atoi(port), Security: "starttls", From: "a@b.c", To: []string{"d@e.f"}}}, "email", ev, now); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("no STARTTLS: %v", err)
	}
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// Each event goes once to each channel while it lasts, also across a restart;
// over, it is forgotten and sent again when it comes back. A channel only gets
// events that start after it was set up; a failed send is tried again after a
// backoff. Nothing is sent just after a start.
func TestNotifierSendsEachEventOnce(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	var mu sync.Mutex
	got := map[string][]string{} // path -> event keys
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e Event
		b, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/hook" {
			json.Unmarshal(b, &e)
		} else {
			e.Key = string(b)
		}
		mu.Lock()
		defer mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		got[r.URL.Path] = append(got[r.URL.Path], e.Key)
	}))
	defer srv.Close()
	sent := func(path string) []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(got[path])
	}
	var events []Event
	looked := map[string]bool{"deploy": true, "eval": true}
	ev := func(key string) Event {
		return Event{Key: key, Kind: "x", Severity: "warning", Title: key, Message: key, Hosts: []string{}}
	}
	open := func(started time.Time) *Notifier {
		n := must[*Notifier](t)(OpenNotifier(dir, nil))
		n.started = started
		n.events = func(context.Context, time.Time) ([]Event, map[string]bool) { return events, looked }
		return n
	}
	t0 := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	n := open(t0.Add(-time.Hour))
	must[NotifySettings](t)(n.Save(NotifySettings{Webhook: &WebhookChannel{URL: srv.URL + "/hook"}}, t0))

	events = []Event{ev("deploy-failed:pi:a")}
	n.Tick(ctx, t0.Add(time.Minute))
	n.Tick(ctx, t0.Add(2*time.Minute))
	open(t0.Add(-time.Hour)).Tick(ctx, t0.Add(3*time.Minute)) // restarted
	if s := sent("/hook"); !slices.Equal(s, []string{"deploy-failed:pi:a"}) {
		t.Fatalf("sent %v", s)
	}
	if fi, err := os.Stat(filepath.Join(dir, "notify-state.json")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("state: %v %v", fi, err)
	}

	// Just after a start nothing is looked at, nor forgotten.
	events = nil
	open(t0.Add(3*time.Minute)).Tick(ctx, t0.Add(4*time.Minute))
	events = []Event{ev("deploy-failed:pi:a")}
	n = open(t0.Add(-time.Hour))
	n.Tick(ctx, t0.Add(5*time.Minute))
	if s := sent("/hook"); len(s) != 1 {
		t.Fatalf("sent again after the warm-up: %v", s)
	}

	// Over, then back: sent again. An event whose source could not be looked at is not over.
	events = []Event{ev("eval-failed:pi")}
	n.Tick(ctx, t0.Add(6*time.Minute))
	looked = map[string]bool{"deploy": true}
	events = []Event{ev("deploy-failed:pi:a")}
	n.Tick(ctx, t0.Add(7*time.Minute))
	looked = map[string]bool{"deploy": true, "eval": true}
	events = []Event{ev("deploy-failed:pi:a"), ev("eval-failed:pi")}
	n.Tick(ctx, t0.Add(8*time.Minute))
	if s := sent("/hook"); !slices.Equal(s, []string{"deploy-failed:pi:a", "eval-failed:pi", "deploy-failed:pi:a"}) {
		t.Fatalf("sent %v", s)
	}

	// ntfy, set up now, gets what starts from now on, not what was already true.
	must[NotifySettings](t)(n.Save(NotifySettings{Webhook: &WebhookChannel{URL: srv.URL + "/hook"}, Ntfy: &NtfyChannel{URL: srv.URL + "/topic"}}, t0.Add(9*time.Minute)))
	events = append(events, ev("host-silent:nas"))
	n.Tick(ctx, t0.Add(10*time.Minute))
	if s := sent("/topic"); !slices.Equal(s, []string{"host-silent:nas"}) {
		t.Fatalf("ntfy got %v", s)
	}

	// A failing channel waits a minute, then two, and gets the event once it works.
	mu.Lock()
	fail = true
	mu.Unlock()
	events = append(events, ev("deploy-stuck:pc"))
	n.Tick(ctx, t0.Add(11*time.Minute))
	if _, st := n.Settings(); len(st) != 2 || st[1].Channel != "webhook" || st[1].OK || !strings.Contains(st[1].Error, "502") {
		t.Fatalf("status %+v", st)
	}
	n.Tick(ctx, t0.Add(11*time.Minute+30*time.Second)) // backing off: not tried
	n.Tick(ctx, t0.Add(12*time.Minute+time.Second))    // tried, fails again: two minutes now
	mu.Lock()
	fail = false
	mu.Unlock()
	n.Tick(ctx, t0.Add(13*time.Minute+30*time.Second))
	if s := sent("/hook"); slices.Contains(s, "deploy-stuck:pc") {
		t.Fatalf("tried while backing off: %v", s)
	}
	n.Tick(ctx, t0.Add(14*time.Minute+30*time.Second))
	n.Tick(ctx, t0.Add(15*time.Minute))
	if s := sent("/hook"); s[len(s)-1] != "deploy-stuck:pc" || slices.Index(s, "deploy-stuck:pc") != len(s)-1 {
		t.Fatalf("after the backoff: %v", s)
	}
	if s := sent("/topic"); !slices.Equal(s, []string{"host-silent:nas", "deploy-stuck:pc"}) {
		t.Fatalf("ntfy got %v", s)
	}
	if _, st := n.Settings(); !st[0].OK || !st[1].OK {
		t.Fatalf("status %+v", st)
	}

	res := must[[]ChannelStatus](t)(n.Test(ctx, t0.Add(16*time.Minute)))
	if len(res) != 2 || !res[0].OK || !res[1].OK || !slices.Contains(sent("/topic"), "Notifications from nixos-manager reach you here.") {
		t.Fatalf("test: %+v %v", res, sent("/topic"))
	}
}

// The events of a fleet: a failed and a silent machine, a machine of the
// deployed branch that does not evaluate (told only once it failed twice),
// and a release past its end of life.
func TestFleetEvents(t *testing.T) {
	ctx := context.Background()
	s := fleetService(t, "testdata/fleet")
	c := commitAll(t, s.Repo, "fleet")
	bare := filepath.Join(t.TempDir(), "remote.git")
	gitOut(t, s.Repo.Dir, "init", "-q", "--bare", "-b", "main", bare)
	gitOut(t, s.Repo.Dir, "remote", "add", "origin", bare)
	must[string](t)(s.Push(ctx, true))
	s.Deploys = must[*Deployments](t)(OpenDeployments(t.TempDir(), s.Repo))
	s.Deploys.started = time.Time{}
	now := time.Now()
	ok(t, s.Deploys.Record(ReportIn{Host: "pi", Commit: c, Status: "failed", Error: "error: unit failed\nmore"}, now.Add(-time.Minute)))
	ok(t, s.Deploys.Record(ReportIn{Host: "homeserver", Commit: c, Status: "done"}, now.Add(-3*time.Hour)))
	ok(t, s.Deploys.Record(ReportIn{Host: "homeserver", Phase: "heartbeat", SilentAfter: 2}, now.Add(-3*time.Hour)))
	ok(t, s.Deploys.Record(ReportIn{Host: "gone", Commit: c, Status: "failed"}, now)) // not in the fleet
	n := must[*Notifier](t)(OpenNotifier(t.TempDir(), s))

	keys := func(es []Event) (out []string) {
		for _, e := range es {
			out = append(out, e.Key)
		}
		slices.Sort(out)
		return out
	}
	if es, looked := n.fleetEvents(ctx, now); len(es) != 0 || len(looked) != 0 {
		t.Fatalf("while the fleet evaluates: %v %v", es, looked)
	}
	readyModel(t, s, "working")
	es, looked := n.fleetEvents(ctx, now)
	if !looked["deploy"] || looked["eval"] || !slices.Equal(keys(es), []string{"deploy-failed:pi:" + c, "host-silent:homeserver"}) {
		t.Fatalf("first look: %v %v", keys(es), looked)
	}
	if f := es[slices.IndexFunc(es, func(e Event) bool { return e.Kind == "deploy-failed" })]; f.Severity != "error" || f.Title != "pi failed to deploy "+c[:7] || f.Message != "error: unit failed" {
		t.Errorf("failure: %+v", f)
	}
	// oldbox does not evaluate: evaluated again before it is told.
	for range 200 {
		if es, looked = n.fleetEvents(ctx, now); looked["eval"] {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !slices.Contains(keys(es), "eval-failed:oldbox") {
		t.Fatalf("after the second evaluation: %v %v", keys(es), looked)
	}
	if e := es[slices.IndexFunc(es, func(e Event) bool { return e.Kind == "eval-failed" })]; e.Title != "oldbox does not evaluate on main" || !strings.Contains(e.Message, "infinite recursion") {
		t.Errorf("eval: %+v", e)
	}
	// NixOS 26.05 ended with 2026.
	es, _ = n.fleetEvents(ctx, time.Date(2027, 1, 5, 0, 0, 0, 0, time.UTC))
	if i := slices.IndexFunc(es, func(e Event) bool { return e.Kind == "eol" }); i < 0 || es[i].Key != "eol:release-eol:NixOS 26.05 no longer gets security updates" || es[i].Severity != "warning" {
		t.Errorf("end of life: %v", keys(es))
	}
}
