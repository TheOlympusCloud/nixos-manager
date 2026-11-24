package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/TheOlympusCloud/nixos-manager/internal/core"
)

// An ISO is built from what is pushed, listed with its SHA-256, and downloaded
// whole or in part; API tokens cannot reach any of it.
func TestISOsOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	nix := filepath.Join(t.TempDir(), "nix")
	os.WriteFile(nix, []byte(`#!/bin/sh
case "$*" in *" hash path "*) echo sha256-fake; exit ;; esac
prev=
for a in "$@"; do [ "$prev" = --out-link ] && out=$a; prev=$a; done
store=$(mktemp -d "$(dirname "$0")/store-XXXXXX")
mkdir "$store/iso" && printf 'an installer' > "$store/iso/nixos.iso" && ln -s "$store" "$out"
`), 0o755)
	e.svc.Jobs.Nix = nix
	ctx := context.Background()
	for p, c := range map[string]string{"hosts/pc/default.nix": "{ }\n", "hosts/pc/disk-config.nix": "{ }\n"} {
		if _, err := e.svc.Write(p, c, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.svc.Stage(ctx, nil); err != nil {
		t.Fatal(err)
	}
	sha, err := e.svc.Commit(ctx, "pc", true)
	if err != nil {
		t.Fatal(err)
	}
	var er struct{ Error string }
	if code := e.do(t, "POST", "/api/isos", map[string]string{"host": "pc"}, &er); code != 400 {
		t.Fatalf("an ISO of what is not pushed: %d %+v", code, er)
	}
	bare := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", "-b", "main", bare).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if out, err := exec.Command("git", "-C", e.svc.Repo.Dir, "remote", "add", "origin", bare).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if _, err := e.svc.Push(ctx, true); err != nil {
		t.Fatal(err)
	}

	var iso core.ISO
	if code := e.do(t, "POST", "/api/isos", map[string]string{"host": "pc"}, &iso); code != 200 || iso.Commit != sha || iso.Job == "" {
		t.Fatalf("build: %d %+v", code, iso)
	}
	var list struct{ ISOs []core.ISO }
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		e.do(t, "GET", "/api/isos", nil, &list)
		if len(list.ISOs) == 1 && list.ISOs[0].State == "ready" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never ready: %+v", list)
		}
	}
	sum := sha256.Sum256([]byte("an installer"))
	if got := list.ISOs[0]; got.ID != iso.ID || got.SHA256 != hex.EncodeToString(sum[:]) || got.Size != 12 {
		t.Fatalf("listed %+v", got)
	}

	get := func(rng string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest("GET", e.srv.URL+"/api/isos/"+iso.ID+"/download", nil)
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		resp, err := e.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	resp, body := get("")
	if resp.StatusCode != 200 || body != "an installer" || resp.Header.Get("Content-Disposition") != `attachment; filename="nixos-pc-`+sha[:7]+`.iso"` ||
		resp.Header.Get("X-Checksum-Sha256") != list.ISOs[0].SHA256 {
		t.Fatalf("download: %d %q %v", resp.StatusCode, body, resp.Header)
	}
	if resp, body := get("bytes=3-"); resp.StatusCode != 206 || body != "installer" {
		t.Fatalf("resumed download: %d %q", resp.StatusCode, body)
	}
	if code := e.do(t, "GET", "/api/isos/pc-0000000-1/download", nil, nil); code != 404 {
		t.Fatalf("unknown ISO: %d", code)
	}

	full := e.createToken(t, "full", "full", nil)
	for _, c := range []struct{ method, path string }{{"GET", "/api/isos"}, {"POST", "/api/isos"}, {"GET", "/api/isos/" + iso.ID + "/download"}} {
		var body any
		if c.method == "POST" {
			body = map[string]string{"host": "pc"}
		}
		if code := e.bearer(t, full.Token, c.method, c.path, body, nil); code != 403 {
			t.Errorf("%s %s with a full token: %d", c.method, c.path, code)
		}
	}
}
