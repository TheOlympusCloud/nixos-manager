package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/TheOlympusCloud/nixos-manager/internal/auth"
	"github.com/TheOlympusCloud/nixos-manager/internal/core"
)

func forgejoEnv(t *testing.T) (*env, *core.Forgejo) {
	t.Helper()
	data := t.TempDir()
	repo, err := core.OpenRepoSSH(filepath.Join(data, "repo"), "", "Test User", "test@example.invalid", filepath.Join(data, "ssh"))
	if err != nil {
		t.Fatal(err)
	}
	svc := core.NewService(repo)
	hash, _ := auth.HashPassword("pw")
	a, err := auth.New(hash, filepath.Join(data, "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	f := &core.Forgejo{Repo: repo, DataDir: data}
	s := &Server{Svc: svc, Auth: a, Forgejo: f, UI: fstest.MapFS{"index.html": {Data: []byte("app")}}}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &env{srv: srv, client: &http.Client{Jar: jar}, auth: a, svc: svc}, f
}

func hook(t *testing.T, e *env, event, sig string, body []byte) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/hooks/forgejo", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitea-Event", event)
	if sig != "" {
		req.Header.Set("X-Forgejo-Signature", sig)
	}
	resp, err := http.DefaultClient.Do(req) // no session cookie
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestForgejoWebhook(t *testing.T) {
	e, f := forgejoEnv(t)
	secret, err := f.HookSecret()
	if err != nil {
		t.Fatal(err)
	}
	sign := func(b []byte) string {
		m := hmac.New(sha256.New, []byte(secret))
		m.Write(b)
		return hex.EncodeToString(m.Sum(nil))
	}
	ping := []byte(`{"zen":"x"}`)
	if code, _ := hook(t, e, "ping", sign(ping), ping); code != 200 {
		t.Fatalf("signed ping without a session: %d", code)
	}
	if code, _ := hook(t, e, "ping", "", ping); code != 401 {
		t.Fatalf("unsigned: %d", code)
	}
	if code, _ := hook(t, e, "ping", sign([]byte("other")), ping); code != 401 {
		t.Fatalf("wrong signature: %d", code)
	}
	big := append([]byte(`{"ref":"`), bytes.Repeat([]byte("a"), 1<<20)...)
	if code, _ := hook(t, e, "push", sign(big), big); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("1 MB body: %d", code)
	}
}

func TestForgejoSettingsNeedLoginAndKeepThePrivateKey(t *testing.T) {
	home := t.TempDir() // no ~/.ssh keys: the panel creates the deploy key
	t.Setenv("HOME", home)
	os.Unsetenv("GIT_SSH_COMMAND")
	e, f := forgejoEnv(t)
	if code := e.do(t, "GET", "/api/forgejo", nil, nil); code != 401 {
		t.Fatalf("without login: %d", code)
	}
	if code := e.do(t, "POST", "/api/forgejo/remote", map[string]string{"url": "/tmp/x"}, nil); code != 401 {
		t.Fatalf("set remote without login: %d", code)
	}
	e.login(t)
	var st core.ForgejoState
	if code := e.do(t, "GET", "/api/forgejo", nil, &st); code != 200 || st.Key == nil || st.SSH != "app" || st.HookSecret == "" {
		t.Fatalf("state %d %+v", code, st)
	}
	resp, err := e.client.Get(e.srv.URL + "/api/forgejo")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	priv, _ := os.ReadFile(filepath.Join(f.Repo.SSHDir, "id_ed25519"))
	if strings.Contains(string(b), "PRIVATE KEY") || len(priv) == 0 || bytes.Contains(b, priv[40:80]) {
		t.Fatal("the private key reached the browser")
	}

	var er struct{ Error string }
	if code := e.do(t, "POST", "/api/forgejo/remote", map[string]string{"url": "https://me:tok@forgejo.lan/p/f.git"}, &er); code != 400 || strings.Contains(er.Error, "tok@") {
		t.Fatalf("credentials in the URL: %d %q", code, er.Error)
	}
	if code := e.do(t, "POST", "/api/forgejo/remote", map[string]string{"url": "ssh://git@forgejo.lan:2222/pablo/fleet.git"}, &st); code != 200 || st.Web != "https://forgejo.lan/pablo/fleet" || st.Kind != "ssh" {
		t.Fatalf("set remote: %d %+v", code, st)
	}
	if code := e.do(t, "POST", "/api/forgejo/web", map[string]string{"url": "http://forgejo.lan:3000"}, &st); code != 200 || st.Web != "http://forgejo.lan:3000/pablo/fleet" {
		t.Fatalf("web override: %d %+v", code, st)
	}
	var info map[string]string
	e.do(t, "GET", "/api/info", nil, &info)
	if info["web"] != "http://forgejo.lan:3000/pablo/fleet" {
		t.Fatalf("info %+v", info)
	}
}

// The commit status token is saved through the web login only, after Forgejo
// accepted it, and never sent back.
func TestForgejoStatusToken(t *testing.T) {
	forgejo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/pablo/fleet" || r.Header.Get("Authorization") != "token s3cret-token" {
			w.WriteHeader(401)
			return
		}
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(forgejo.Close)
	e, _ := forgejoEnv(t)
	in := map[string]string{"token": "s3cret-token", "console": "http://console.lan:8080"}
	if code := e.do(t, "POST", "/api/forgejo/status", in, nil); code != 401 {
		t.Fatalf("without login: %d", code)
	}
	e.login(t)
	e.do(t, "POST", "/api/forgejo/remote", map[string]string{"url": "ssh://git@forgejo.lan:2222/pablo/fleet.git"}, nil)
	e.do(t, "POST", "/api/forgejo/web", map[string]string{"url": forgejo.URL}, nil)

	var er struct{ Error string }
	if code := e.do(t, "POST", "/api/forgejo/status", map[string]string{"token": "wrong"}, &er); code != 400 || !strings.Contains(er.Error, "refused the token") {
		t.Fatalf("wrong token: %d %q", code, er.Error)
	}
	resp, err := e.client.Post(e.srv.URL+"/api/forgejo/status", "application/json", strings.NewReader(`{"token":"s3cret-token","console":"http://console.lan:8080"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), `"status_token":true`) || strings.Contains(string(b), "s3cret-token") {
		t.Fatalf("save: %d %s", resp.StatusCode, b)
	}
	var st core.ForgejoState
	if code := e.do(t, "GET", "/api/forgejo", nil, &st); code != 200 || !st.StatusToken || st.StatusConsole != "http://console.lan:8080" || st.StatusAPI != forgejo.URL+"/api/v1/repos/pablo/fleet" {
		t.Fatalf("state %d %+v", code, st)
	}
	if code := e.do(t, "POST", "/api/forgejo/status", map[string]string{"token": ""}, &st); code != 200 || st.StatusToken {
		t.Fatalf("remove: %d %+v", code, st)
	}
}
