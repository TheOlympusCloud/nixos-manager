package templates

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func nixCmd(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "nix", append([]string{"--extra-experimental-features", "nix-command flakes"}, args...)...)
}

// The writer's own tests, with a Python that has its libraries: the one managed.nix
// gives it with real Nix, else NIXMGR_PYTHON or python3 when they are installed.
func TestManagedConfigsWriter(t *testing.T) {
	python := os.Getenv("NIXMGR_PYTHON")
	switch {
	case os.Getenv("NIXMGR_NIX_TESTS") != "":
		out, err := nixCmd(context.Background(), "build", "--impure", "--no-link", "--print-out-paths", "--expr",
			`(builtins.getFlake "github:NixOS/nixpkgs/nixos-26.05").legacyPackages.${builtins.currentSystem}.python3.withPackages (ps: [ ps.tomlkit ps.ruamel-yaml ps.secretstorage ])`).Output()
		if err != nil {
			t.Fatalf("nix build python: %v\n%s", err, stderr(err))
		}
		python = strings.TrimSpace(string(out)) + "/bin/python3"
	case python == "":
		python = "python3"
		if exec.Command(python, "-c", "import tomlkit, ruamel.yaml, secretstorage").Run() != nil {
			t.Skip("needs a Python with tomlkit, ruamel.yaml and secretstorage (NIXMGR_PYTHON), or real Nix (NIXMGR_NIX_TESTS=1)")
		}
	}
	out, err := exec.Command(python, "-B", "testdata/managed_configs_test.py", "-v").CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// managed.nix 6 makes the secrets a home's configs and keyring entries use the
// user's, and hands the writer the declared settings with each secret's path on
// the machine, never its value: at activation, again at login, and for the
// keyring once the graphical session runs. A secret the machine lacks, and a
// file home-manager also links, fail with a message that says so.
func TestManagedConfigs(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	eval := func(attr string) ([]byte, error) {
		out, err := nixCmd(context.Background(), "eval", "--impure", "--json", "--file", "testdata/managed-configs.nix", attr).Output()
		if err != nil {
			return stderr(err), err
		}
		return out, nil
	}
	out, err := eval("ok")
	if err != nil {
		t.Fatalf("nix eval: %v\n%s", err, out)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	want := map[string]any{
		"tokenOwner": "pablo", "pwOwner": "pablo", "version": float64(6), "system": true, "loginSameSpec": true,
		"loginWantedBy": []any{"default.target"}, "keyringWantedBy": []any{"graphical-session.target"},
		"configs": map[string]any{"configs": map[string]any{
			".config/supersonic/config.toml": map[string]any{
				"format": "toml", "mode": "merge", "process": "supersonic", "secrets": map[string]any{},
				"settings": map[string]any{"Application": map[string]any{"EnableSystemTray": true}, "Servers[ID=a1]": map[string]any{"Hostname": "https://music.example"}},
			},
			".config/app/settings.json": map[string]any{
				"format": "json", "mode": "seed", "settings": map[string]any{"a": float64(1)},
				"secrets": map[string]any{"auth": map[string]any{"token": "/run/secrets/token"}},
			},
		}},
		"keyring": map[string]any{"entries": []any{map[string]any{
			"label": "Password for 'a1' on 'supersonic'", "attributes": map[string]any{"service": "supersonic", "username": "a1"}, "secret": "/run/secrets/pw",
		}}},
	}
	for k, w := range want {
		if !reflect.DeepEqual(got[k], w) {
			t.Errorf("%s: got %#v\nwant %#v", k, got[k], w)
		}
	}
	for attr, msg := range map[string]string{
		"missing": "managed.json: ~/.config/a.json of pablo uses the secret nope, which this machine does not have",
		"overlap": "pablo's configs write .config/a.json, which home-manager also links",
	} {
		if out, err := eval(attr); err == nil || !strings.Contains(string(out), msg) {
			t.Errorf("%s: want a failure saying %q, got %v:\n%s", attr, msg, err, out)
		}
	}
}

// The VM test of testdata/managed-configs-vm.nix: what the writer does on a real
// machine, with sops-nix, home-manager and GNOME Keyring.
func TestManagedConfigsInVMWithRealNix(t *testing.T) {
	if os.Getenv("NIXMGR_VM_TESTS") == "" {
		t.Skip("needs real Nix, KVM and network access: set NIXMGR_VM_TESTS=1")
	}
	ctx := context.Background()
	if d, ok := t.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, d.Add(-time.Minute))
		defer cancel()
	}
	cmd := nixCmd(ctx, "build", "-L", "--impure", "--no-link", "--print-out-paths", "--file", "testdata/managed-configs-vm.nix")
	cmd.Stderr = os.Stderr // the VM's console and the test script's lines, as they run
	if err := cmd.Run(); err != nil {
		t.Fatalf("nix build: %v", err)
	}
}

func stderr(err error) []byte {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.Stderr
	}
	return nil
}
