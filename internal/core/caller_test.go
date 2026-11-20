package core

import (
	"context"
	"strings"
	"testing"
)

func viaOf(t *testing.T, r *Repo) string {
	t.Helper()
	return strings.TrimSpace(gitOut(t, r.Dir, "log", "-1", "--format=%(trailers:key=Via,valueonly)"))
}

func TestCommitRecordsWhoMadeIt(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	web := WithCaller(ctx, Caller{Via: "web session 1a2b3c4d", Person: true})
	token := WithCaller(ctx, Caller{Via: "token ci (K3J9QW2M4X1A)"})

	write(t, s.Repo, "a.nix", "1\n")
	ok(t, s.Stage(ctx, nil))
	must[string](t)(s.Commit(web, "feat: a\n\nWhy it changed.", true))
	if via := viaOf(t, s.Repo); via != "web session 1a2b3c4d" {
		t.Fatalf("Via %q", via)
	}
	if msg := gitOut(t, s.Repo.Dir, "log", "-1", "--format=%B"); !strings.HasPrefix(msg, "feat: a\n\nWhy it changed.\n\nVia: web session 1a2b3c4d") {
		t.Fatalf("message %q", msg)
	}

	// The trailer joins the message's own trailers.
	write(t, s.Repo, "a.nix", "2\n")
	ok(t, s.Stage(ctx, nil))
	ok(t, passCheck(t, s))
	must[string](t)(s.Commit(token, "fix: b\n\nSigned-off-by: Someone <s@example.invalid>", false))
	if via := viaOf(t, s.Repo); via != "token ci (K3J9QW2M4X1A)" {
		t.Fatalf("Via %q", via)
	}
	if n := strings.TrimSpace(gitOut(t, s.Repo.Dir, "log", "-1", "--format=%(trailers:only,unfold)")); len(strings.Split(n, "\n")) != 2 {
		t.Fatalf("trailers %q", n)
	}

	// The server's own commits (no caller) carry no trailer.
	write(t, s.Repo, "a.nix", "3\n")
	ok(t, s.Stage(ctx, nil))
	must[string](t)(s.Commit(ctx, "chore: c", true))
	if via := viaOf(t, s.Repo); via != "" {
		t.Fatalf("Via %q without a caller", via)
	}
}

func TestCommitRefusesAForgedVia(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	token := WithCaller(ctx, Caller{Via: "token ci (K3J9QW2M4X1A)"})
	write(t, s.Repo, "a.nix", "1\n")
	ok(t, s.Stage(ctx, nil))
	ok(t, passCheck(t, s))
	head := s.Repo.hasHead(ctx)
	for _, msg := range []string{"feat: a\n\nVia: web session 00000000", "feat: a\n\nSigned-off-by: X <x@example.invalid>\nvia: web session 00000000"} {
		_, err := s.Commit(token, msg, false)
		isErr(t, err, ErrInvalid)
	}
	if s.Repo.hasHead(ctx) != head {
		t.Fatal("a refused message was committed")
	}
	// "Via:" in the body, not in the trailer block, is ordinary text.
	must[string](t)(s.Commit(token, "feat: a\n\nVia: the proxy\n\nMore text.", false))
	if via := viaOf(t, s.Repo); via != "token ci (K3J9QW2M4X1A)" {
		t.Fatalf("Via %q", via)
	}
}

func TestOnlyAPersonSkipsTheCheck(t *testing.T) {
	ctx := context.Background()
	r, _ := remotePair(t)
	s := NewService(r)
	s.Jobs.Nix = script(t, "nix", fakeNix)
	token := WithCaller(ctx, Caller{Via: "token ci (K3J9QW2M4X1A)"})
	web := WithCaller(ctx, Caller{Via: "web session 1a2b3c4d", Person: true})

	write(t, r, "b.nix", "b\n")
	ok(t, s.Stage(ctx, nil))
	_, err := s.Commit(token, "feat: b", true)
	isErr(t, err, ErrForbidden)
	must[string](t)(s.Commit(web, "feat: b", true))
	_, err = s.Push(token, true)
	isErr(t, err, ErrForbidden)
	_, err = s.Push(token, false)
	isErr(t, err, ErrNotChecked)
	must[string](t)(s.Push(web, true))
}

// passCheck runs a passing check of the staged changes.
func passCheck(t *testing.T, s *Service) error {
	t.Helper()
	j, err := s.Jobs.Submit(context.Background(), JobRequest{Kind: "check"})
	if err != nil {
		return err
	}
	if info := wait(t, j); info.State != "passed" {
		t.Fatalf("check %s:\n%s", info.State, info.Output)
	}
	return nil
}
