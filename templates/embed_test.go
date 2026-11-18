package templates

import (
	"io/fs"
	"testing"
)

func TestFleetHasTheFlakeAtItsRoot(t *testing.T) {
	for _, p := range []string{"flake.nix", "hosts/example/default.nix", "modules/common.nix", "home/pablo.nix"} {
		if _, err := fs.Stat(Fleet, p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}
