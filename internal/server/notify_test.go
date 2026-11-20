package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TheOlympusCloud/nixos-manager/internal/core"
)

// Notifications are set in Settings, by the person logged in only; secrets
// never come back.
func TestNotificationsOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	if code := e.do(t, "GET", "/api/notifications", nil, nil); code != 404 {
		t.Fatalf("while off: %d", code)
	}
	n, err := core.OpenNotifier(t.TempDir(), e.svc)
	if err != nil {
		t.Fatal(err)
	}
	e.s.Notify = n // read on every request

	got := make(chan string, 2)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got <- string(b)
	}))
	defer hook.Close()

	var res struct {
		Settings core.NotifySettings
		Status   []core.ChannelStatus
	}
	if code := e.do(t, "POST", "/api/notifications", map[string]any{"ntfy": map[string]any{"url": "https://ntfy.sh/"}}, nil); code != 400 {
		t.Fatalf("an invalid topic: %d", code)
	}
	if code := e.do(t, "POST", "/api/notifications/test", struct{}{}, nil); code != 400 {
		t.Fatalf("a test without channels: %d", code)
	}
	body := map[string]any{"webhook": map[string]any{"url": hook.URL}, "ntfy": map[string]any{"url": hook.URL + "/fleet", "token": "tk_secret"}}
	if code := e.do(t, "POST", "/api/notifications", body, &res); code != 200 || res.Settings.Ntfy == nil || res.Settings.Ntfy.Token != "" || !res.Settings.Ntfy.HasToken {
		t.Fatalf("save: %d %+v", code, res.Settings)
	}
	if code := e.do(t, "GET", "/api/notifications", nil, &res); code != 200 || res.Settings.Webhook.URL != hook.URL || res.Settings.Ntfy.Token != "" || len(res.Status) != 0 {
		t.Fatalf("get: %d %+v", code, res)
	}
	if code := e.do(t, "POST", "/api/notifications/test", struct{}{}, &res); code != 200 || len(res.Status) != 2 || !res.Status[0].OK || !res.Status[1].OK {
		t.Fatalf("test: %d %+v", code, res.Status)
	}
	ntfy, webhook := <-got, <-got // in that order
	var ev map[string]any
	if err := json.Unmarshal([]byte(webhook), &ev); err != nil || ev["kind"] != "test" || ntfy != "Notifications from nixos-manager reach you here." {
		t.Errorf("test messages: %q, %v %v", ntfy, err, ev)
	}

	tok := e.createToken(t, "full", "full", nil)
	for _, c := range []struct{ method, path string }{{"GET", "/api/notifications"}, {"POST", "/api/notifications"}, {"POST", "/api/notifications/test"}} {
		if code := e.bearer(t, tok.Token, c.method, c.path, struct{}{}, nil); code != 403 {
			t.Errorf("an API token on %s %s: %d", c.method, c.path, code)
		}
	}
}
