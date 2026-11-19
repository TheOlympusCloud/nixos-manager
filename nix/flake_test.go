// Package nix checks the repository's flake with real Nix. Like ./templates, it only runs when asked:
//
//	NIXMGR_NIX_TESTS=1 go test -timeout 30m ./nix
//
// A git flake only sees tracked files, so commit (or git add) flake.nix and nix/ first.
package nix

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func nix(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("nix", append([]string{"--extra-experimental-features", "nix-command flakes"}, args...)...)
	cmd.Dir = ".."
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = ee.Stderr
		}
		t.Fatalf("nix %s: %v\n%s", strings.Join(args, " "), err, stderr)
	}
	return strings.TrimSpace(string(out))
}

// The module check (the README's LXC configuration, evaluated), then a real build of the package:
// the build is what proves vendorHash and the npm lock import right.
func TestFlake(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	nix(t, "flake", "check", "-L", ".")
	out := nix(t, "build", "-L", "--no-link", "--print-out-paths", ".#default")
	bin := out + "/bin/nixos-manager"
	if v, err := exec.Command(bin, "version").Output(); err != nil || strings.TrimSpace(string(v)) == "" {
		t.Fatalf("%s version: %q %v", bin, v, err)
	}
	wrapper, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"-git-", "-openssh-", "-nixd-", "-nixfmt-"} {
		if !strings.Contains(string(wrapper), tool) {
			t.Errorf("the wrapper does not put %s on PATH:\n%s", strings.Trim(tool, "-"), wrapper)
		}
	}
}
