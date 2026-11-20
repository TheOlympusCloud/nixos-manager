package core

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseDiffClosures(t *testing.T) {
	// As nix prints it: the size coloured, several versions of a name joined by commas.
	out := "firefox: 145.0 → 146.0, \x1b[31;1m+5434.1 KiB\x1b[0m\n" +
		"glibc: \x1b[32;1m-12.0 KiB\x1b[0m\n" +
		"hello: ∅ → 2.12.2, \x1b[31;1m+262.6 KiB\x1b[0m\n" +
		"nano: 8.6 → ∅, \x1b[32;1m-2861.5 KiB\x1b[0m\n" +
		"python3: 3.12.8, 3.13.1 → 3.12.9, 3.13.2\n" +
		"unit-foo: ∅ → ε\n" +
		// Nix 2.2x and later: no plus sign, and the unit Nix picks.
		"curl: 8.16.0 → 8.17.0, \x1b[31;1m273.1 KiB\x1b[0m\n" +
		"linux: 6.12.1 → 6.12.2, 1.5 MiB\n" +
		"python3.13: \x1b[32;1m-2.0 GiB\x1b[0m\n" +
		"\n" +
		"warning: something nix says\n"
	got := parseDiffClosures([]byte(out))
	want := []ClosureChange{
		{Name: "firefox", From: "145.0", To: "146.0", Delta: 5564518},
		{Name: "glibc", Delta: -12288},
		{Name: "hello", To: "2.12.2", Delta: 268902},
		{Name: "nano", From: "8.6", Delta: -2930176},
		{Name: "python3", From: "3.12.8, 3.13.1", To: "3.12.9, 3.13.2"},
		{Name: "unit-foo", To: "ε"},
		{Name: "curl", From: "8.16.0", To: "8.17.0", Delta: 279654},
		{Name: "linux", From: "6.12.1", To: "6.12.2", Delta: 1572864},
		{Name: "python3.13", Delta: -2147483648},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diff-closures\n got %+v\nwant %+v", got, want)
	}
	if got := parseDiffClosures(nil); got == nil || len(got) != 0 {
		t.Fatalf("nothing differs: %#v", got)
	}
}

func TestParseClosureSizes(t *testing.T) {
	out := "/nix/store/aaaa-nixos-system-web-26.05\t 1300000000\n/nix/store/bbbb-nixos-system-web-26.05   \t 1319000000\n\nnoise\n"
	got := parseClosureSizes([]byte(out))
	want := map[string]int64{"/nix/store/aaaa-nixos-system-web-26.05": 1300000000, "/nix/store/bbbb-nixos-system-web-26.05": 1319000000}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sizes %+v", got)
	}
}

const closureNix = `#!/bin/sh
echo "$*" >> "$NIXMGR_CALLS"
case "$*" in
  *"store diff-closures"*) printf 'hello: ∅ → 2.12.2, +262.6 KiB\n' ;;
  *"path-info --closure-size"*)
    shift 2 # the experimental features
    for p in "$@"; do case "$p" in /nix/*) printf '%s\t%s\n' "$p" "${#p}000" ;; esac; done ;;
  *) exit 1 ;;
esac
`

func TestAddClosuresOnlyForSystemsInTheStore(t *testing.T) {
	sys := func(c byte, host string) string {
		return "/nix/store/" + strings.Repeat(string(c), 32) + "-nixos-system-" + host
	}
	calls := filepath.Join(t.TempDir(), "calls")
	t.Setenv("NIXMGR_CALLS", calls)
	js := &Jobs{Nix: script(t, "nix", closureNix)}
	built := map[string]bool{sys('a', "web"): true, sys('b', "web"): true, sys('a', "db"): true, sys('a', "new"): true, sys('b', "new"): true}
	old := storeHas
	storeHas = func(p string) bool { return built[p] }
	t.Cleanup(func() { storeHas = old })
	h := func(name, path string) Host { return Host{Name: name, Detail: &HostDetail{SystemPath: path}} }
	head := &Model{Hosts: []Host{h("web", sys('a', "web")), h("db", sys('a', "db")), h("same", sys('a', "same"))}}
	work := &Model{Hosts: []Host{h("web", sys('b', "web")), h("db", sys('b', "db")), h("same", sys('a', "same")), h("new", sys('b', "new"))}}
	ds := []HostDiff{{Host: "db", Status: "changed"}, {Host: "new", Status: "added"}, {Host: "same", Status: "unchanged"}, {Host: "web", Status: "changed"}}

	js.AddClosures(context.Background(), head, work, ds)
	want := &ClosureDiff{SizeFrom: int64(len(sys('a', "web"))) * 1000, SizeTo: int64(len(sys('b', "web"))) * 1000,
		Changes: []ClosureChange{{Name: "hello", To: "2.12.2", Delta: 268902}}}
	if !reflect.DeepEqual(ds[3].Closure, want) {
		t.Fatalf("web\n got %+v\nwant %+v", ds[3].Closure, want)
	}
	if ds[0].Closure != nil || ds[1].Closure != nil || ds[2].Closure != nil {
		t.Fatalf("db is not built in its new version, new and same do not change: %+v", ds)
	}

	// The same pair again comes from the cache.
	again := []HostDiff{{Host: "web", Status: "changed"}}
	js.AddClosures(context.Background(), head, work, again)
	b, _ := os.ReadFile(calls)
	if n := strings.Count(string(b), "diff-closures"); n != 1 || !reflect.DeepEqual(again[0].Closure, want) {
		t.Fatalf("%d diff-closures runs, closure %+v", n, again[0].Closure)
	}

	// Not a store path (a broken evaluation, a fixture): never handed to nix.
	built["/tmp/x"] = true
	odd := &Model{Hosts: []Host{h("web", "/tmp/x")}}
	ds = []HostDiff{{Host: "web", Status: "changed"}}
	js.AddClosures(context.Background(), odd, work, ds)
	if ds[0].Closure != nil {
		t.Fatalf("closure from a path outside the store: %+v", ds[0].Closure)
	}

	// nix fails: the diff simply has no closure.
	js.Nix = script(t, "nix", "#!/bin/sh\nexit 1\n")
	built[sys('c', "web")] = true
	ds = []HostDiff{{Host: "web", Status: "changed"}}
	js.AddClosures(context.Background(), &Model{Hosts: []Host{h("web", sys('c', "web"))}}, work, ds)
	if ds[0].Closure != nil {
		t.Fatalf("closure although nix failed: %+v", ds[0].Closure)
	}
}
