package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TheOlympusCloud/nixos-manager/internal/core"
)

// The update runs from the API, is merged with one call, and each route takes
// the scope it says; the schedule is the web login's alone.
func TestUpdatesOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	if code := e.do(t, "GET", "/api/updates", nil, nil); code != 404 {
		t.Fatalf("while off: %d", code)
	}
	ctx := context.Background()
	dir := e.svc.Repo.Dir
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	// nix flake update moves the lock; the flake has no machines, so the check builds nothing.
	nix := filepath.Join(t.TempDir(), "nix")
	os.WriteFile(nix, []byte("#!/bin/sh\ncase \"$*\" in\n  *\" hash path \"*) echo sha256-fake; exit ;;\n  *\"eval --json\"*) echo '{\"nixos\":[],\"home\":[],\"darwin\":[]}'; exit ;;\n  *\"flake update\"*) d=${*##*path:}; echo '{\"version\":7,\"new\":true}' > \"$d/flake.lock\" ;;\nesac\necho \"nix $*\"\n"), 0o755)
	e.svc.Jobs.Nix = nix
	bare := filepath.Join(t.TempDir(), "remote.git")
	git("init", "-q", "--bare", "-b", "main", bare)
	git("remote", "add", "origin", bare)
	e.svc.Write("flake.nix", "{ }\n", "")
	e.svc.Write("flake.lock", "{\"version\":7}\n", "")
	e.svc.Stage(ctx, nil)
	if _, err := e.svc.Commit(ctx, "flake", true); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Push(ctx, true); err != nil {
		t.Fatal(err)
	}
	u, err := core.OpenUpdates(t.TempDir(), e.svc)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Updates = u

	var st core.UpdateStatus
	if code := e.do(t, "POST", "/api/updates/schedule", map[string]any{"enabled": true, "day": 9, "hour": 4}, nil); code != 400 {
		t.Fatalf("an invalid day: %d", code)
	}
	if code := e.do(t, "POST", "/api/updates/schedule", map[string]any{"enabled": true, "day": 1, "hour": 4}, &st); code != 200 || !st.Schedule.Enabled || st.Next.Weekday() != time.Monday || st.Next.Hour() != 4 {
		t.Fatalf("schedule: %d %+v", code, st)
	}
	if code := e.do(t, "POST", "/api/updates/run", struct{}{}, &st); code != 200 || st.Update == nil || st.Update.State != "updating" {
		t.Fatalf("run: %d %+v", code, st)
	}
	deadline := time.Now().Add(20 * time.Second)
	for e.do(t, "GET", "/api/updates", nil, &st); st.Update.State != "ready"; e.do(t, "GET", "/api/updates", nil, &st) {
		if st.Update.State == "error" || time.Now().After(deadline) {
			t.Fatalf("update %+v", st.Update)
		}
		time.Sleep(20 * time.Millisecond)
	}
	commit := st.Update.Commit

	read, edit, full := e.createToken(t, "r", "read", nil), e.createToken(t, "e", "edit", nil), e.createToken(t, "f", "full", nil)
	for _, c := range []struct {
		tok          string
		method, path string
		want         int
	}{
		{read.Token, "GET", "/api/updates", 200},
		{read.Token, "POST", "/api/updates/discard", 403},
		{edit.Token, "POST", "/api/updates/run", 403},
		{edit.Token, "POST", "/api/updates/merge", 403},
		{full.Token, "POST", "/api/updates/merge", 409}, // allowed; no such update
		{full.Token, "POST", "/api/updates/schedule", 403},
	} {
		if code := e.bearer(t, c.tok, c.method, c.path, map[string]string{"commit": "0000000000000000000000000000000000000000"}, nil); code != c.want {
			t.Errorf("%s %s: %d, want %d", c.method, c.path, code, c.want)
		}
	}

	var res struct{ Message string }
	if code := e.do(t, "POST", "/api/updates/merge", map[string]string{"commit": commit}, &res); code != 200 || !strings.Contains(res.Message, "Merged the update into main") {
		t.Fatalf("merge: %d %+v", code, res)
	}
	if got := git("ls-remote", "origin", "refs/heads/main"); !strings.HasPrefix(got, commit) {
		t.Fatalf("main on the remote: %s", got)
	}
	if code := e.bearer(t, edit.Token, "POST", "/api/updates/discard", struct{}{}, nil); code != 409 { // allowed; merged
		t.Fatalf("discard after the merge: %d", code)
	}
}
