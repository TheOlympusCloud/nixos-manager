package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const testName, testEmail = "Test User", "test@example.invalid"

func TestMain(m *testing.M) {
	// Keep the developer's own git config and hooks out of the test repos.
	os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Unsetenv("GIT_CONFIG_COUNT")
	os.Exit(m.Run())
}

func newRepo(t *testing.T) *Repo {
	t.Helper()
	r, err := OpenRepo(filepath.Join(t.TempDir(), "repo"), "", testName, testEmail)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func must[T any](t *testing.T) func(T, error) T {
	return func(v T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

func ok(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func isErr(t *testing.T, err, kind error) {
	t.Helper()
	if !errors.Is(err, kind) {
		t.Fatalf("got error %v, want %v", err, kind)
	}
}

func write(t *testing.T, r *Repo, p, content string) {
	t.Helper()
	cur := ""
	if _, h, err := r.Read(p); err == nil {
		cur = h
	}
	must[string](t)(r.Write(p, content, cur))
}

func commitAll(t *testing.T, r *Repo, msg string) string {
	t.Helper()
	ctx := context.Background()
	ok(t, r.Stage(ctx, nil))
	return must[string](t)(r.Commit(ctx, msg))
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestPathValidation(t *testing.T) {
	r := newRepo(t)
	for _, p := range []string{"", ".", "/etc/passwd", "../x", "a/../../x", ".git/config", ".git"} {
		_, _, err := r.Read(p)
		isErr(t, err, ErrInvalid)
		_, err = r.Write(p, "x", "")
		isErr(t, err, ErrInvalid)
		isErr(t, r.Delete(p), ErrInvalid)
	}
	// A symlink pointing outside the repo must not be followed.
	ok(t, os.Symlink("/etc", filepath.Join(r.Dir, "escape")))
	_, _, err := r.Read("escape/hostname")
	if err == nil {
		t.Fatal("read through an escaping symlink succeeded")
	}
}

func TestWriteReadConflicts(t *testing.T) {
	r := newRepo(t)
	h := must[string](t)(r.Write("hosts/web/default.nix", "{ }\n", ""))
	content, h2, err := r.Read("hosts/web/default.nix")
	ok(t, err)
	if content != "{ }\n" || h2 != h {
		t.Fatalf("read back %q %s, want %s", content, h2, h)
	}
	_, err = r.Write("hosts/web/default.nix", "x", "")
	isErr(t, err, ErrExists)
	_, err = r.Write("hosts/web/default.nix", "x", "stale")
	isErr(t, err, ErrConflict)
	h3 := must[string](t)(r.Write("hosts/web/default.nix", "{ a = 1; }\n", h))
	if h3 == h {
		t.Fatal("hash did not change")
	}
	_, err = r.Write("gone.nix", "x", h)
	isErr(t, err, ErrConflict)
	_, _, err = r.Read("missing.nix")
	isErr(t, err, ErrNotFound)
	ok(t, os.WriteFile(filepath.Join(r.Dir, "blob.bin"), []byte{0xff, 0xfe, 0}, 0o644))
	_, _, err = r.Read("blob.bin")
	isErr(t, err, ErrInvalid)
	_, _, err = r.Read("hosts")
	isErr(t, err, ErrInvalid)
	_, err = r.Write("hosts", "x", "")
	isErr(t, err, ErrInvalid)
}

func TestTreeMkdirMoveDelete(t *testing.T) {
	r := newRepo(t)
	write(t, r, "flake.nix", "{ }")
	write(t, r, "hosts/web/default.nix", "{ }")
	ok(t, r.Mkdir("modules"))
	isErr(t, r.Mkdir("modules"), ErrExists)
	got := must[[]Entry](t)(r.Tree())
	want := []Entry{{"flake.nix", false}, {"hosts", true}, {"hosts/web", true}, {"hosts/web/default.nix", false}, {"modules", true}}
	if len(got) != len(want) {
		t.Fatalf("tree %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tree %v, want %v", got, want)
		}
	}
	isErr(t, r.Move("hosts/web", "flake.nix"), ErrExists)
	isErr(t, r.Move("nope", "x"), ErrNotFound)
	ok(t, r.Move("hosts/web", "machines/web"))
	if _, _, err := r.Read("machines/web/default.nix"); err != nil {
		t.Fatal(err)
	}
	ok(t, r.Delete("machines"))
	isErr(t, r.Delete("machines"), ErrNotFound)
}

func TestStatusStageCommitOnFreshRepo(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	write(t, r, "flake.nix", "{ }\n")
	st := must[Status](t)(r.Status(ctx))
	if st.Branch != "main" || len(st.Files) != 1 || st.Files[0].Unstaged != "?" {
		t.Fatalf("status %+v", st)
	}
	_, err := r.Commit(ctx, "nothing staged")
	isErr(t, err, ErrNothingStaged)
	ok(t, r.Stage(ctx, []string{"flake.nix"}))
	st = must[Status](t)(r.Status(ctx))
	if st.Files[0].Staged != "A" {
		t.Fatalf("status %+v", st)
	}
	ok(t, r.Unstage(ctx, nil))
	st = must[Status](t)(r.Status(ctx))
	if st.Files[0].Staged != "" || st.Files[0].Unstaged != "?" {
		t.Fatalf("after unstage %+v", st)
	}
	_, err = r.Commit(ctx, "  ")
	isErr(t, err, ErrInvalid)
	sha := commitAll(t, r, "feat: first")
	if got := gitOut(t, r.Dir, "log", "-1", "--format=%an <%ae>|%cn <%ce>|%s", sha); got != "Test User <test@example.invalid>|Test User <test@example.invalid>|feat: first" {
		t.Fatalf("commit identity/message: %s", got)
	}
	st = must[Status](t)(r.Status(ctx))
	if len(st.Files) != 0 {
		t.Fatalf("clean status expected, got %+v", st)
	}
}

func TestStagedTreeChangesWithIndexOnly(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	write(t, r, "a.nix", "1")
	commitAll(t, r, "init")
	t0 := must[Status](t)(r.Status(ctx)).StagedTree
	write(t, r, "a.nix", "2")
	if t1 := must[Status](t)(r.Status(ctx)).StagedTree; t1 != t0 {
		t.Fatal("unstaged edit changed the staged tree")
	}
	ok(t, r.Stage(ctx, nil))
	if t2 := must[Status](t)(r.Status(ctx)).StagedTree; t2 == t0 {
		t.Fatal("staging did not change the staged tree")
	}
}

func TestDiffIncludesUntrackedAndStaged(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	write(t, r, "a.nix", "one\n")
	commitAll(t, r, "init")
	write(t, r, "a.nix", "two\n")
	write(t, r, "new.nix", "fresh\n")
	d := must[string](t)(r.Diff(ctx, "", false))
	for _, want := range []string{"-one", "+two", "+fresh", "new.nix"} {
		if !strings.Contains(d, want) {
			t.Fatalf("unstaged diff lacks %q:\n%s", want, d)
		}
	}
	d = must[string](t)(r.Diff(ctx, "new.nix", false))
	if !strings.Contains(d, "+fresh") || strings.Contains(d, "+two") {
		t.Fatalf("path diff wrong:\n%s", d)
	}
	ok(t, r.Stage(ctx, []string{"a.nix"}))
	d = must[string](t)(r.Diff(ctx, "", true))
	if !strings.Contains(d, "+two") || strings.Contains(d, "fresh") {
		t.Fatalf("staged diff wrong:\n%s", d)
	}
}

func TestDiscard(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	write(t, r, "a.nix", "one\n")
	commitAll(t, r, "init")
	write(t, r, "a.nix", "two\n")
	write(t, r, "junk.nix", "x\n")
	isErr(t, r.Discard(ctx, nil), ErrInvalid)
	ok(t, r.Discard(ctx, []string{"a.nix", "junk.nix"}))
	if c, _, _ := r.Read("a.nix"); c != "one\n" {
		t.Fatalf("a.nix = %q", c)
	}
	if _, _, err := r.Read("junk.nix"); !errors.Is(err, ErrNotFound) {
		t.Fatal("untracked file survived discard")
	}
}

func TestHistoryAndRestore(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	if log := must[[]Commit](t)(r.Log(ctx, "", "", 10)); len(log) != 0 {
		t.Fatalf("log on empty repo: %v", log)
	}
	const v1 = "{\n  networking.hostName = \"a\";\n  time.timeZone = \"Europe/Madrid\";\n  services.openssh.enable = true;\n}\n"
	write(t, r, "hosts/a.nix", v1)
	c1 := commitAll(t, r, "feat: add a")
	write(t, r, "other.nix", "x\n")
	commitAll(t, r, "chore: other")
	ok(t, r.Move("hosts/a.nix", "hosts/b.nix"))
	write(t, r, "hosts/b.nix", v1+"# renamed\n")
	commitAll(t, r, "refactor: rename a to b")

	log := must[[]Commit](t)(r.Log(ctx, "", "", 10))
	if len(log) != 3 || log[0].Subject != "refactor: rename a to b" || log[2].SHA != c1 || log[0].Author != testName {
		t.Fatalf("repo log %+v", log)
	}
	flog := must[[]Commit](t)(r.Log(ctx, "hosts/b.nix", "", 10))
	if len(flog) != 2 || flog[1].SHA != c1 {
		t.Fatalf("file log should follow the rename: %+v", flog)
	}
	info := must[CommitInfo](t)(r.CommitInfo(ctx, c1))
	if info.Message != "feat: add a\n" || !strings.Contains(info.Diff, "+  time.timeZone") {
		t.Fatalf("commit info %+v", info)
	}
	old := must[string](t)(r.FileAt(ctx, c1, "hosts/a.nix"))
	if old != v1 {
		t.Fatalf("file at c1 = %q", old)
	}
	_, err := r.FileAt(ctx, c1, "hosts/b.nix")
	isErr(t, err, ErrNotFound)
	_, err = r.FileAt(ctx, "--output=/tmp/x", "a")
	isErr(t, err, ErrInvalid)
	_, err = r.FileAt(ctx, "deadbeef", "a")
	isErr(t, err, ErrNotFound)

	// hosts/a.nix is gone from disk (renamed), so restoring it means creating it.
	isErr(t, r.Restore(ctx, "hosts/a.nix", c1, "stale"), ErrConflict)
	ok(t, r.Restore(ctx, "hosts/a.nix", c1, ""))
	if c, _, _ := r.Read("hosts/a.nix"); c != v1 {
		t.Fatalf("restored content %q", c)
	}
	isErr(t, r.Restore(ctx, "hosts/a.nix", c1, ""), ErrExists)

	// An edit someone else saved since we read the file must not be overwritten.
	_, seen, _ := r.Read("hosts/b.nix")
	write(t, r, "hosts/b.nix", "edited by someone else\n")
	isErr(t, r.Restore(ctx, "hosts/b.nix", log[0].SHA, seen), ErrConflict)
	if c, _, _ := r.Read("hosts/b.nix"); c != "edited by someone else\n" {
		t.Fatalf("restore overwrote an unseen edit: %q", c)
	}
	_, cur, _ := r.Read("hosts/b.nix")
	_, err = r.FileAt(ctx, c1, "hosts/b.nix")
	isErr(t, err, ErrNotFound)
	isErr(t, r.Restore(ctx, "hosts/b.nix", c1, cur), ErrNotFound)
}

func TestPathsAreLiteralNotGlobs(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	for _, f := range []string{"a.nix", "b.nix", "hosts/x.nix"} {
		write(t, r, f, "committed\n")
	}
	commitAll(t, r, "init")
	for _, f := range []string{"a.nix", "b.nix", "hosts/x.nix"} {
		write(t, r, f, "unstaged edit\n")
	}
	for _, glob := range []string{"*.nix", "hosts/*", ":(glob)**/*.nix", ":(exclude)a.nix"} {
		if err := r.Discard(ctx, []string{glob}); err == nil {
			t.Errorf("discard %q matched nothing but reported success", glob)
		}
		_ = r.Stage(ctx, []string{glob})
		if d := must[string](t)(r.Diff(ctx, glob, false)); d != "" {
			t.Errorf("diff %q expanded the pattern:\n%s", glob, d)
		}
	}
	for _, f := range []string{"a.nix", "b.nix", "hosts/x.nix"} {
		if c, _, _ := r.Read(f); c != "unstaged edit\n" {
			t.Fatalf("%s lost its unstaged edit: %q", f, c)
		}
	}
	if st := must[Status](t)(r.Status(ctx)); len(st.Files) != 3 || st.Files[0].Staged != "" {
		t.Fatalf("a pattern staged files: %+v", st.Files)
	}
	// The empty list still means everything.
	ok(t, r.Stage(ctx, nil))
	if st := must[Status](t)(r.Status(ctx)); st.Files[0].Staged != "M" {
		t.Fatalf("stage all after literal pathspecs: %+v", st.Files)
	}
	ok(t, r.Unstage(ctx, nil))
}

func TestBranches(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	write(t, r, "a.nix", "1\n")
	commitAll(t, r, "init")
	ok(t, r.CreateBranch(ctx, "testing-web", ""))
	isErr(t, r.CreateBranch(ctx, "-bad", ""), ErrInvalid)
	isErr(t, r.CreateBranch(ctx, "a..b", ""), ErrInvalid)
	b := must[Branches](t)(r.Branches(ctx))
	if b.Current != "testing-web" || len(b.Branches) != 2 {
		t.Fatalf("branches %+v", b)
	}
	write(t, r, "a.nix", "2\n")
	commitAll(t, r, "change on testing")
	write(t, r, "a.nix", "dirty\n")
	isErr(t, r.Switch(ctx, "main"), ErrConflict)
	ok(t, r.Discard(ctx, []string{"a.nix"}))
	ok(t, r.Switch(ctx, "main"))
	if c, _, _ := r.Read("a.nix"); c != "1\n" {
		t.Fatalf("main content %q", c)
	}
	isErr(t, r.Switch(ctx, "nope"), ErrConflict)
}

// remotePair returns a bare remote, a repo cloned from it through OpenRepo, and
// a second plain clone standing in for another machine pushing to the same remote.
func remotePair(t *testing.T) (*Repo, string) {
	t.Helper()
	ctx := context.Background()
	base := t.TempDir()
	bare := filepath.Join(base, "remote.git")
	gitOut(t, base, "init", "-q", "--bare", "-b", "main", bare)
	seed := must[*Repo](t)(OpenRepo(filepath.Join(base, "seed"), bare, testName, testEmail))
	write(t, seed, "a.nix", "1\n")
	commitAll(t, seed, "init")
	must[string](t)(seed.Push(ctx))
	r := must[*Repo](t)(OpenRepo(filepath.Join(base, "app"), bare, testName, testEmail))
	other := filepath.Join(base, "other")
	gitOut(t, base, "clone", "-q", bare, other)
	gitOut(t, other, "config", "user.name", "Other")
	gitOut(t, other, "config", "user.email", "other@example.invalid")
	return r, other
}

func otherCommit(t *testing.T, other, file, content string) {
	t.Helper()
	ok(t, os.WriteFile(filepath.Join(other, file), []byte(content), 0o644))
	gitOut(t, other, "add", "-A")
	gitOut(t, other, "commit", "-q", "-m", "other: "+file)
	gitOut(t, other, "push", "-q")
}

func TestPushPull(t *testing.T) {
	ctx := context.Background()
	r, other := remotePair(t)
	if st := must[Status](t)(r.Status(ctx)); st.Upstream != "origin/main" {
		t.Fatalf("clone has no upstream: %+v", st)
	}

	// Fast-forward.
	otherCommit(t, other, "b.nix", "b\n")
	must[string](t)(r.Pull(ctx))
	if c, _, _ := r.Read("b.nix"); c != "b\n" {
		t.Fatal("fast-forward pull did not bring b.nix")
	}

	// Push, and a push rejected because the remote moved on.
	write(t, r, "c.nix", "c\n")
	commitAll(t, r, "add c")
	must[string](t)(r.Push(ctx))
	gitOut(t, other, "pull", "-q")
	otherCommit(t, other, "d.nix", "d\n")
	write(t, r, "e.nix", "e\n")
	commitAll(t, r, "add e")
	_, err := r.Push(ctx)
	isErr(t, err, ErrConflict)

	// Diverged with a dirty tree: refused, nothing touched.
	write(t, r, "a.nix", "local edit\n")
	_, err = r.Pull(ctx)
	isErr(t, err, ErrConflict)
	ok(t, r.Discard(ctx, []string{"a.nix"}))

	// Diverged, no overlap: rebased onto the remote.
	must[string](t)(r.Pull(ctx))
	st := must[Status](t)(r.Status(ctx))
	if st.Ahead != 1 || st.Behind != 0 {
		t.Fatalf("after rebase pull %+v", st)
	}
	must[string](t)(r.Push(ctx))

	// Diverged with a real conflict: aborted, local commit kept.
	gitOut(t, other, "pull", "-q")
	otherCommit(t, other, "a.nix", "theirs\n")
	write(t, r, "a.nix", "ours\n")
	mine := commitAll(t, r, "ours")
	_, err = r.Pull(ctx)
	isErr(t, err, ErrConflict)
	if head := gitOut(t, r.Dir, "rev-parse", "HEAD"); head != mine {
		t.Fatal("conflicting pull did not restore the local commit")
	}
	if st := must[Status](t)(r.Status(ctx)); len(st.Files) != 0 {
		t.Fatalf("conflicting pull left changes: %+v", st)
	}
}

func TestPushWithoutRemote(t *testing.T) {
	r := newRepo(t)
	write(t, r, "a.nix", "1\n")
	commitAll(t, r, "init")
	_, err := r.Push(context.Background())
	isErr(t, err, ErrInvalid)
}

func TestOpenAddsRemoteToExistingRepo(t *testing.T) {
	r := newRepo(t)
	bare := filepath.Join(t.TempDir(), "remote.git")
	gitOut(t, t.TempDir(), "init", "-q", "--bare", bare)
	r2 := must[*Repo](t)(OpenRepo(r.Dir, bare, testName, testEmail))
	if got := gitOut(t, r2.Dir, "remote", "get-url", "origin"); got != bare || r2.Remote(context.Background()) != bare {
		t.Fatalf("origin = %q", got)
	}
	other := filepath.Join(t.TempDir(), "moved.git")
	r3 := must[*Repo](t)(OpenRepo(r.Dir, other, testName, testEmail))
	if got := gitOut(t, r3.Dir, "remote", "get-url", "origin"); got != other {
		t.Fatalf("changed remote not applied: %q", got)
	}
	gitOut(t, r2.Dir, "remote", "set-url", "origin", "https://pablo:secret-token@forgejo.lan/pablo/nixos-config.git")
	if got := r2.Remote(context.Background()); strings.Contains(got, "secret-token") {
		t.Fatalf("remote shown with its password: %s", got)
	}
}

func TestSeedOnlyIntoEmptyRepo(t *testing.T) {
	r := newRepo(t)
	tmpl := fstest.MapFS{"flake.nix": {Data: []byte("{ }\n")}, "hosts/a/default.nix": {Data: []byte("{ }\n")}}
	if seeded := must[bool](t)(r.Seed(tmpl)); !seeded {
		t.Fatal("empty repo not seeded")
	}
	if c, _, _ := r.Read("hosts/a/default.nix"); c != "{ }\n" {
		t.Fatalf("seeded content %q", c)
	}
	if seeded := must[bool](t)(r.Seed(tmpl)); seeded {
		t.Fatal("seeded a repo that already has files")
	}
}

// Reverting stages the inverse changes; the user checks and commits them like any change.
func TestRevertStagesTheInverse(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	write(t, r, "a.nix", "one\n")
	commitAll(t, r, "first")
	write(t, r, "a.nix", "two\n")
	bad := commitAll(t, r, "second")
	ok(t, r.Revert(ctx, bad))
	if c, _, _ := r.Read("a.nix"); c != "one\n" {
		t.Fatalf("after revert a.nix = %q", c)
	}
	st := must[Status](t)(r.Status(ctx))
	if len(st.Files) != 1 || st.Files[0].Staged != "M" {
		t.Fatalf("revert should be staged, not committed: %+v", st.Files)
	}
	if head := gitOut(t, r.Dir, "rev-parse", "HEAD"); head != bad {
		t.Fatal("revert made a commit")
	}
	isErr(t, r.Revert(ctx, bad), ErrConflict) // local changes in the way
	_, err := r.FileAt(ctx, "nope", "a.nix")
	isErr(t, err, ErrNotFound)
}

func TestMergeStagesTheBranch(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	write(t, r, "a.nix", "base\n")
	commitAll(t, r, "base")
	ok(t, r.CreateBranch(ctx, "testing-pi", ""))
	write(t, r, "pi.nix", "tried on pi\n")
	commitAll(t, r, "try on pi")
	ok(t, r.Switch(ctx, "main"))
	write(t, r, "main.nix", "meanwhile\n")
	commitAll(t, r, "meanwhile on main")
	head := gitOut(t, r.Dir, "rev-parse", "HEAD")
	msg := must[string](t)(r.Merge(ctx, "testing-pi"))
	if c, _, _ := r.Read("pi.nix"); c != "tried on pi\n" || gitOut(t, r.Dir, "rev-parse", "HEAD") != head {
		t.Fatal("merge must stage the branch without committing")
	}
	if !strings.Contains(msg, "testing-pi") {
		t.Fatalf("suggested message %q", msg)
	}
	ok(t, r.Stage(ctx, nil))
	sha := must[string](t)(r.Commit(ctx, msg))
	if parents := gitOut(t, r.Dir, "rev-list", "--parents", "-n1", sha); len(strings.Fields(parents)) != 2 {
		t.Fatalf("a squashed merge is an ordinary commit: %s", parents)
	}
	if _, err := os.Stat(filepath.Join(r.Dir, ".git", "MERGE_HEAD")); err == nil {
		t.Fatal("git was left in the middle of a merge")
	}

	// A conflicting merge is undone and reported.
	ok(t, r.CreateBranch(ctx, "clash", ""))
	write(t, r, "a.nix", "theirs\n")
	commitAll(t, r, "theirs")
	ok(t, r.Switch(ctx, "main"))
	write(t, r, "a.nix", "ours\n")
	commitAll(t, r, "ours")
	_, err := r.Merge(ctx, "clash")
	isErr(t, err, ErrConflict)
	if st := must[Status](t)(r.Status(ctx)); len(st.Files) != 0 {
		t.Fatalf("a failed merge left changes: %+v", st.Files)
	}
	_, err = r.Merge(ctx, "-x")
	isErr(t, err, ErrInvalid)
}
