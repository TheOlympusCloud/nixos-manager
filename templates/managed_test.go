package templates

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The settings managed.nix applies win value by value: hand-written users and SSH
// settings around them survive, and a list set here replaces the hand-written one.
func TestManagedSettingsKeepHandWrittenValues(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	out, err := exec.Command("nix", "--extra-experimental-features", "nix-command flakes",
		"eval", "--impure", "--json", "--file", "testdata/managed-merge.nix").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("nix eval: %v\n%s", err, ee.Stderr)
		}
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"isNormalUser":        true,
		"groups":              []any{"wheel"},
		"keys":                []any{"ssh-ed25519 AAAAmanaged managed"},
		"sshdUser":            true,
		"passwords":           false,
		"keyboardInteractive": false,
		"ports":               []any{float64(8080)},
		"hello":               true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

// modules/managed.json joins each machine's own managed.json, which wins where
// both set something, and a shared home reaches only machines that manage it.
func TestManagedSharedByEveryMachine(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	dir := t.TempDir()
	module, err := os.ReadFile("fleet/modules/managed.nix")
	if err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		"modules/managed.nix":          string(module),
		"modules/patches/nixmgr.patch": "",
		"modules/managed.json": `{
  "packages": ["hello", "jq"],
  "removedPackages": ["perl", "rsync"],
  "packageOverrides": {
    "hello": { "overrideAttrs": { "NIXMGR_FROM": "shared" }, "patches": [{ "file": "patches/nixmgr.patch" }] },
    "jq": { "overrideAttrs": { "NIXMGR_FROM": "shared" } }
  },
  "home": { "pablo": { "packages": ["cowsay"], "removedPackages": [] } },
  "settings": {
    "time": { "timeZone": "Europe/Madrid" },
    "networking": { "firewall": { "allowedTCPPorts": [80] } },
    "home-manager": { "users": { "pablo": { "home": { "sessionVariables": { "NIXMGR": "shared" } } } } }
  }
}`,
		// a adds perl back, takes jq out, builds its own jq, and manages pablo's home.
		"hosts/a/managed.json": `{
  "packages": ["perl"],
  "removedPackages": ["jq"],
  "packageOverrides": { "jq": { "overrideAttrs": { "NIXMGR_FROM": "a" } } },
  "home": { "pablo": { "packages": [], "removedPackages": [] } },
  "settings": {
    "networking": { "firewall": { "allowedTCPPorts": [22] } },
    "users": { "users": { "pablo": { "isNormalUser": true } } },
    "home-manager": { "users": { "pablo": { "home": { "stateVersion": "26.05" } } } }
  }
}`,
		"hosts/b/managed.json": `{ "packages": [], "settings": {} }`,
	} {
		p := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The fleet copy goes in the environment: nix eval --file does not call the file with --argstr.
	cmd := exec.Command("nix", "--extra-experimental-features", "nix-command flakes",
		"eval", "--impure", "--json", "--file", "testdata/managed-shared.nix")
	cmd.Env = append(os.Environ(), "NIXMGR_TEST_FLEET="+dir)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("nix eval: %v\n%s", err, ee.Stderr)
		}
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"a": map[string]any{
			"packages": []any{"hello", "perl"}, "home": []any{"cowsay"}, "sessionVariable": "shared",
			"helloFrom": "shared", "helloPatch": true, "jqFrom": "a",
			"timeZone": "Europe/Madrid", "ports": []any{float64(22)}, "version": float64(5),
			"helloFile": []any{"modules/managed.json"},
		},
		"b": map[string]any{
			"packages": []any{"hello", "jq"}, "home": nil, "sessionVariable": nil,
			"helloFrom": "shared", "helloPatch": true, "jqFrom": "shared",
			"timeZone": "Europe/Madrid", "ports": []any{float64(80)}, "version": float64(5),
			"helloFile": []any{"modules/managed.json"},
		},
	}
	for _, m := range []string{"a", "b"} {
		if !reflect.DeepEqual(got[m], want[m]) {
			t.Errorf("%s: got %v\nwant %v", m, got[m], want[m])
		}
	}
}

// A package whose override has "from" comes from that flake input, imported for
// the machine's platform with its nixpkgs.config; the override's other changes
// apply on top. Without the input, the build fails and says what to do.
func TestManagedPackageFromAnotherInput(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	// The JSON on success, Nix's messages on failure: another nix process fetching the same
	// input makes this one say so on stderr, which must not reach the JSON.
	eval := func(attr string) ([]byte, error) {
		out, err := exec.Command("nix", "--extra-experimental-features", "nix-command flakes",
			"eval", "--impure", "--json", "--file", "testdata/managed-from-input.nix", attr).Output()
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.Stderr, err
		}
		return out, err
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
		"installed": []any{"other"},
		"hello":     []any{"other", "x86_64-linux", "true"},
		"jq":        []any{"other", "managed"},
		"rsync":     []any{nil, "managed"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
	for attr, msg := range map[string]string{
		"missing":  "takes packages from the flake input other, which flake.nix does not have: add it to its inputs, then Lock",
		"noInputs": "takes packages from the flake input other, but this machine's nixosSystem gets no specialArgs.inputs",
	} {
		if out, err := eval(attr); err == nil || !strings.Contains(string(out), msg) {
			t.Errorf("%s: want a failure saying %q, got %v:\n%s", attr, msg, err, out)
		}
	}
}

// A secret linked into a home becomes its user's and ~/.netrc a link to it in
// /run/secrets; a template in home/<user>/templates/ is rendered by sops-nix,
// for its user, with the secret's placeholder where @secret:<name>@ was, and
// linked into ~. A secret the machine lacks fails, saying so.
func TestManagedHomeSecrets(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	// The JSON on success, Nix's messages (fetching inputs, the error) on failure.
	eval := func(attr string) ([]byte, error) {
		out, err := exec.Command("nix", "--extra-experimental-features", "nix-command flakes",
			"eval", "--impure", "--json", "--file", "testdata/managed-home-secrets.nix", attr).Output()
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.Stderr, err
		}
		return out, err
	}
	out, err := eval("ok")
	if err != nil {
		t.Fatalf("nix eval: %v\n%s", err, out)
	}
	var got struct {
		NetrcOwner, Netrc, Hosts, TemplateOwner, TemplatePath string
		Rendered, System                                      bool
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	rendered := "/run/secrets/rendered/home-pablo-.config-gh-hosts.yml"
	if got.NetrcOwner != "pablo" || got.TemplateOwner != "pablo" || got.TemplatePath != rendered || !got.Rendered || !got.System ||
		!strings.Contains(got.Netrc, "/run/secrets/netrc") || !strings.Contains(got.Hosts, rendered) {
		t.Errorf("got %+v", got)
	}
	for attr, msg := range map[string]string{
		"missing": "managed.json links ~/.netrc of pablo to the secret nope, which this machine does not have",
		"unknown": "home/pablo/templates/.config/x.conf: @secret:nope@ is not one of this machine's secrets",
	} {
		if out, err := eval(attr); err == nil || !strings.Contains(string(out), msg) {
			t.Errorf("%s: want a failure saying %q, got %v:\n%s", attr, msg, err, out)
		}
	}
}

// A standalone home applies its own managed.json through modules/managed-home.nix:
// packages it adds, names it takes out of what hand-written Nix adds, its settings
// over hand-written values, and custom.nix and files/ next to it.
func TestManagedStandaloneHome(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	out, err := exec.Command("nix", "--extra-experimental-features", "nix-command flakes",
		"eval", "--impure", "--json", "--file", "testdata/managed-standalone-home.nix").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("nix eval: %v\n%s", err, ee.Stderr)
		}
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"packages":        []any{"hello", "jq"},
		"sessionVariable": "managed",
		"custom":          "custom",
		"file":            "linked from files/\n",
		"managedFile":     true,
		"packageNames":    map[string]any{"jq": "jq"},
		"activation":      true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}
