package templates

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// stepScript is the run: block of the starter workflow's step called name.
func stepScript(t *testing.T, name string) string {
	t.Helper()
	b, err := fs.ReadFile(Fleet, ".forgejo/workflows/check.yml")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	indent := func(l string) int { return len(l) - len(strings.TrimLeft(l, " ")) }
	for i, l := range lines {
		if strings.TrimSpace(l) != "- name: "+name {
			continue
		}
		for j := i + 1; j < len(lines) && indent(lines[j]) > indent(l); j++ {
			if strings.TrimSpace(lines[j]) != "run: |" {
				continue
			}
			var body []string
			for _, s := range lines[j+1:] {
				if strings.TrimSpace(s) != "" && indent(s) <= indent(lines[j]) {
					break
				}
				body = append(body, strings.TrimPrefix(s, strings.Repeat(" ", indent(lines[j])+2)))
			}
			return strings.Join(body, "\n")
		}
	}
	t.Fatalf("the workflow has no step %q with a run block", name)
	return ""
}

// runStep runs a step's script as Forgejo's runner does with shell: bash.
func runStep(t *testing.T, dir, script string, env ...string) (string, error) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "step.sh")
	if err := os.WriteFile(f, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "--noprofile", "--norc", "-eo", "pipefail", f)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// starterRepo is the starter fleet committed in a new repository.
func starterRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	err := fs.WalkDir(Fleet, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(Fleet, p)
		if err == nil {
			os.MkdirAll(filepath.Join(dir, filepath.Dir(p)), 0o755)
			err = os.WriteFile(filepath.Join(dir, p), b, 0o644)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "starter")
	return dir
}

// The checkout step fetches exactly the pushed commit, with its whole history
// (Nix refuses a shallow repository), and the scripts pass ShellCheck.
func TestCheckWorkflowChecksOutThePushedCommit(t *testing.T) {
	for _, tool := range []string{"bash", "git", "base64"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("needs %s", tool)
		}
	}
	src := starterRepo(t)
	pushed := git(t, src, "rev-parse", "HEAD")
	os.WriteFile(filepath.Join(src, "later.txt"), []byte("later\n"), 0o644)
	git(t, src, "add", "-A")
	git(t, src, "commit", "-q", "-m", "later")

	work := checkout(t, src, pushed) // the commit the push left; the branch has moved on since
	if got := git(t, work, "rev-parse", "HEAD"); got != pushed {
		t.Fatalf("checked out %s, want %s", got, pushed)
	}
	if git(t, work, "rev-parse", "--is-shallow-repository") != "false" {
		t.Fatal("the checkout is shallow")
	}
	if _, err := os.Stat(filepath.Join(work, "flake.nix")); err != nil {
		t.Fatal("no flake.nix in the checkout")
	}

	if _, err := exec.LookPath("shellcheck"); err != nil {
		t.Skip("needs shellcheck")
	}
	for _, step := range []string{"Check out", "Check"} {
		cmd := exec.Command("shellcheck", "-s", "bash", "-")
		cmd.Stdin = strings.NewReader(stepScript(t, step))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("shellcheck %s: %v\n%s", step, err, out)
		}
	}
}

// checkout runs the workflow's checkout step for a commit of src in a new folder.
func checkout(t *testing.T, src, sha string) string {
	t.Helper()
	work := t.TempDir()
	out, err := runStep(t, work, stepScript(t, "Check out"), "GITHUB_SERVER_URL=file://"+filepath.Dir(src), "GITHUB_REPOSITORY="+filepath.Base(src),
		"GITHUB_SHA="+sha, "TOKEN=not-used-by-file-urls", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	if err != nil {
		t.Fatalf("checkout: %v\n%s", err, out)
	}
	return work
}

// The workflow's steps, checkout then check, pass on the locked starter fleet,
// building its machine, and fail when a machine's system does not build.
func TestCheckWorkflowWithRealNix(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	dir := starterRepo(t)
	nixConf := "NIX_CONFIG=experimental-features = nix-command flakes"
	if out, err := runStep(t, dir, "nix flake lock\n", nixConf); err != nil {
		t.Fatalf("lock: %v\n%s", err, out)
	}
	git(t, dir, "add", "flake.lock")
	git(t, dir, "commit", "-q", "-m", "lock")
	check := stepScript(t, "Check")
	out, err := runStep(t, checkout(t, dir, git(t, dir, "rev-parse", "HEAD")), check, nixConf)
	if err != nil {
		t.Fatalf("check of the starter fleet: %v\n%s", err, tail(out))
	}

	def := filepath.Join(dir, "hosts/example/default.nix")
	b, _ := os.ReadFile(def)
	broken := strings.Replace(strings.Replace(string(b), "{ ... }:", "{ config, ... }:", 1), "system.stateVersion",
		`environment.etc.nixmgr-broken.source = derivation { name = "nixmgr-broken"; system = config.nixpkgs.hostPlatform.system; builder = "/bin/sh"; args = [ "-c" "exit 1" ]; };`+"\n  system.stateVersion", 1)
	if broken == string(b) {
		t.Fatal("could not edit the example machine")
	}
	os.WriteFile(def, []byte(broken), 0o644)
	git(t, dir, "commit", "-q", "-am", "example: broken")
	if out, err := runStep(t, checkout(t, dir, git(t, dir, "rev-parse", "HEAD")), check, nixConf); err == nil || !strings.Contains(out, "nixmgr-broken") {
		t.Fatalf("a machine that does not build passed: %v\n%s", err, tail(out))
	}
}

func tail(s string) string {
	lines := strings.Split(s, "\n")
	return strings.Join(lines[max(0, len(lines)-60):], "\n")
}
