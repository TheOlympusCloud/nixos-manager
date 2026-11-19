package templates

import (
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
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
