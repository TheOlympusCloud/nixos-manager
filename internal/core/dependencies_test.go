package core

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// storeP is a store path with a hash made of c.
func storeP(c byte, name string) string {
	return "/nix/store/" + strings.Repeat(string(c), 32) + "-" + name
}

func TestSplitStoreName(t *testing.T) {
	for path, want := range map[string][3]string{
		storeP('a', "openssl-3.0.15-bin"):               {"openssl", "3.0.15", "bin"},
		storeP('a', "openssl-3.0.15"):                   {"openssl", "3.0.15", ""},
		storeP('a', "util-linux-2.41-bin"):              {"util-linux", "2.41", "bin"},
		storeP('a', "gcc-14.3.0-lib"):                   {"gcc", "14.3.0", "lib"},
		storeP('a', "linux-6.12.5-modules"):             {"linux", "6.12.5", "modules"},
		storeP('a', "perl5.40.0-Locale-gettext-1.07"):   {"perl5.40.0-Locale-gettext", "1.07", ""},
		storeP('a', "nixos-system-pi-26.05.20251120.a"): {"nixos-system-pi", "26.05.20251120.a", ""},
		storeP('a', "unit-sshd.service"):                {"unit-sshd.service", "", ""},
		storeP('a', "X-Restart-Triggers-sshd"):          {"X-Restart-Triggers-sshd", "", ""},
		storeP('a', "etc"):                              {"etc", "", ""},
		storeP('a', "python3-3.13.1-env"):               {"python3", "3.13.1-env", ""},
	} {
		n, v, o := splitStoreName(path)
		if got := [3]string{n, v, o}; got != want {
			t.Errorf("%s: %q, want %q", path, got, want)
		}
	}
}

// reported makes host known: closures are only taken from machines that report.
func reported(t *testing.T, d *Deployments, host, current string) {
	t.Helper()
	ok(t, d.Record(ReportIn{Host: host, Phase: "heartbeat", Current: current, SendsClosure: true}, time.Now()))
}

// smallClosure is system → bash, hello → glibc, and bash → glibc.
func smallClosure(system string) ClosureIn {
	return ClosureIn{Paths: []ClosurePath{
		{Path: system, NarSize: 10, Refs: []int{1, 2}},
		{Path: storeP('b', "bash-5.2p37"), NarSize: 200, Refs: []int{3}},
		{Path: storeP('c', "hello-2.12.2"), NarSize: 30, Refs: []int{3}},
		{Path: storeP('d', "glibc-2.40-66"), NarSize: 4000},
	}}
}

func TestRecordClosure(t *testing.T) {
	dir := t.TempDir()
	d := must[*Deployments](t)(OpenDeployments(dir, newRepo(t)))
	sys := storePath('a', "pi")
	now := time.Now()

	isErr(t, d.RecordClosure("pi", sys, smallClosure(sys), now), ErrInvalid) // pi never reported
	reported(t, d, "pi", sys)
	for name, c := range map[string]struct{ host, system string }{
		"host":   {"../pi", sys},
		"system": {"pi", "/etc/passwd"},
	} {
		isErr(t, d.RecordClosure(c.host, c.system, smallClosure(sys), now), ErrInvalid)
		if d.closures["pi"] != nil {
			t.Fatalf("%s: remembered %+v", name, d.closures["pi"])
		}
	}
	ok(t, d.RecordClosure("pi", sys, smallClosure(sys), now))
	fi, err := os.Stat(filepath.Join(dir, "closures", "pi.json"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("closures/pi.json: %v %v", fi, err)
	}
	again := must[*Deployments](t)(OpenDeployments(dir, d.repo))
	if c := again.closures["pi"]; c == nil || c.System != sys || len(c.Paths) != 4 || c.Paths[0].Refs[1] != 2 {
		t.Fatalf("after reopening: %+v", c)
	}

	// One bad entry refuses all of it, and the refusal is kept.
	bad := map[string]func(*ClosureIn){
		"index":      func(c *ClosureIn) { c.Paths[1].Refs = []int{4} },
		"negative":   func(c *ClosureIn) { c.Paths[1].Refs = []int{-1} },
		"path":       func(c *ClosureIn) { c.Paths[2].Path = "/nix/store/short-hello" },
		"size":       func(c *ClosureIn) { c.Paths[2].NarSize = -1 },
		"duplicate":  func(c *ClosureIn) { c.Paths[2].Path = c.Paths[1].Path },
		"no system":  func(c *ClosureIn) { c.Paths[0].Path = storeP('h', "other") },
		"empty":      func(c *ClosureIn) { c.Paths = nil },
		"long error": func(c *ClosureIn) { c.Paths = nil; c.Error = strings.Repeat("e", maxClosureError+1) },
	}
	for name, spoil := range bad {
		other := storePath('f', "pi-"+strings.ReplaceAll(name, " ", "-"))
		c := smallClosure(other)
		spoil(&c)
		if err := d.RecordClosure("pi", other, c, now); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
			continue
		}
		if r := d.closures["pi"]; r == nil || r.System != other || r.Refused == "" || r.Paths != nil {
			t.Errorf("%s: kept %+v", name, r)
		}
	}
	n := maxClosurePaths
	maxClosurePaths = 3
	defer func() { maxClosurePaths = n }()
	isErr(t, d.RecordClosure("pi", sys, smallClosure(sys), now), ErrInvalid)
	if r := d.closures["pi"]; !strings.Contains(r.Refused, "more than 3") {
		t.Fatalf("too many paths: %+v", r)
	}
}

func TestWantsClosure(t *testing.T) {
	d := must[*Deployments](t)(OpenDeployments(t.TempDir(), newRepo(t)))
	sys, next := storePath('a', "pi"), storePath('b', "pi")
	if d.WantsClosure("pi", sys) {
		t.Fatal("wants the closure of a machine that never reported")
	}
	reported(t, d, "pi", sys)
	if !d.WantsClosure("pi", sys) || d.WantsClosure("pi", "") {
		t.Fatal("nothing kept: wants it, and only for a system")
	}
	ok(t, d.RecordClosure("pi", sys, smallClosure(sys), time.Now()))
	if d.WantsClosure("pi", sys) || !d.WantsClosure("pi", next) {
		t.Fatal("kept: wants only another system's")
	}
	ok(t, d.RefuseClosure("pi", next, "larger than 64 MiB", time.Now()))
	if d.WantsClosure("pi", next) || !d.WantsClosure("pi", sys) {
		t.Fatal("refused: not again until the system changes")
	}
	// The machine could not read it: a passing error, asked again.
	ok(t, d.RecordClosure("pi", sys, ClosureIn{Error: "cannot connect to the daemon"}, time.Now()))
	if !d.WantsClosure("pi", sys) {
		t.Fatal("an error record must be asked again")
	}
	if r := d.closures["pi"]; r.Error != "cannot connect to the daemon" || r.Paths != nil {
		t.Fatalf("error record: %+v", r)
	}
	var nilD *Deployments
	if nilD.WantsClosure("pi", sys) {
		t.Fatal("nil wants")
	}
}

func TestForgetDropsTheClosure(t *testing.T) {
	dir := t.TempDir()
	d := must[*Deployments](t)(OpenDeployments(dir, newRepo(t)))
	sys := storePath('a', "pi")
	reported(t, d, "pi", sys)
	ok(t, d.RecordClosure("pi", sys, smallClosure(sys), time.Now()))
	ok(t, d.Forget("pi"))
	if _, err := os.Stat(filepath.Join(dir, "closures", "pi.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closures/pi.json after forgetting pi: %v", err)
	}
	if d.closures["pi"] != nil {
		t.Fatal("still in memory")
	}
}

func TestCorruptClosureFileIsSkipped(t *testing.T) {
	dir := t.TempDir()
	ok(t, os.MkdirAll(filepath.Join(dir, "closures"), 0o700))
	ok(t, os.WriteFile(filepath.Join(dir, "closures", "pi.json"), []byte("{not json"), 0o600))
	ok(t, os.WriteFile(filepath.Join(dir, "closures", "..json"), []byte("{}"), 0o600))
	d := must[*Deployments](t)(OpenDeployments(dir, newRepo(t)))
	if len(d.closures) != 0 {
		t.Fatalf("closures: %v", d.closures)
	}
}

func TestMachineDependenciesStates(t *testing.T) {
	d := must[*Deployments](t)(OpenDeployments(t.TempDir(), newRepo(t)))
	sys := storePath('a', "pi")
	state := func(host string) MachineDeps {
		t.Helper()
		return must[MachineDeps](t)(d.MachineDependencies(host))
	}
	if s := state("pi"); s.State != "none" || s.Nodes == nil {
		t.Fatalf("never reported: %+v", s)
	}
	ok(t, d.Record(ReportIn{Host: "pi", Phase: "heartbeat", Current: sys}, time.Now()))
	if s := state("pi"); s.State != "old" || s.Current != sys {
		t.Fatalf("older report module: %+v", s)
	}
	reported(t, d, "pi", sys)
	if s := state("pi"); s.State != "waiting" {
		t.Fatalf("asked: %+v", s)
	}
	ok(t, d.RecordClosure("pi", sys, ClosureIn{Error: "nix: cannot connect"}, time.Now()))
	if s := state("pi"); s.State != "error" || s.Message != "nix: cannot connect" {
		t.Fatalf("error: %+v", s)
	}
	ok(t, d.RefuseClosure("pi", sys, "larger than 64 MiB", time.Now()))
	if s := state("pi"); s.State != "refused" || s.Message != "larger than 64 MiB" {
		t.Fatalf("refused: %+v", s)
	}
	ok(t, d.RecordClosure("pi", sys, smallClosure(sys), time.Now()))
	s := state("pi")
	if s.State != "ready" || s.System != sys || s.Size != 4240 || len(s.Nodes) != 4 || s.Nodes[s.Root].Path != sys {
		t.Fatalf("ready: %+v", s)
	}
	if n := s.Nodes[3]; n.Name != "glibc" || n.Version != "2.40-66" || n.NarSize != 4000 || n.Refs == nil {
		t.Fatalf("glibc: %+v", n)
	}
	// glibc is reached through bash, the first entry the system references.
	if s.Nodes[3].Via != 1 || s.Nodes[1].Via != s.Root || s.Nodes[s.Root].Via != -1 {
		t.Fatalf("via: %+v", s.Nodes)
	}

	// The shared fleet token never gets a tree.
	ok(t, os.WriteFile(d.tokenPath, []byte("nxs_shared\n"), 0o600))
	d2 := must[*Deployments](t)(OpenDeployments(filepath.Dir(d.tokenPath), d.repo))
	ok(t, d2.Record(ReportIn{Host: "nas", Phase: "heartbeat", Current: sys, SendsClosure: true, SharedToken: true}, time.Now()))
	if s := must[MachineDeps](t)(d2.MachineDependencies("nas")); s.State != "shared" {
		t.Fatalf("shared token: %+v", s)
	}
	isErr(t, must2(d.MachineDependencies("../pi")), ErrInvalid)
	var nilD *Deployments
	isErr(t, must2(nilD.MachineDependencies("pi")), ErrNotFound)
}

func TestFleetDependencies(t *testing.T) {
	d := must[*Deployments](t)(OpenDeployments(t.TempDir(), newRepo(t)))
	pi, nas := storePath('a', "pi"), storePath('b', "nas-26.05.20251120.ab12cd3")
	reported(t, d, "pi", pi)
	reported(t, d, "nas", nas)
	ok(t, d.RecordClosure("pi", pi, smallClosure(pi), time.Now()))
	ok(t, d.RecordClosure("nas", nas, ClosureIn{Paths: []ClosurePath{
		{Path: nas, Refs: []int{1, 2, 3}},
		{Path: storeP('j', "bash-5.3p3")},
		{Path: storeP('f', "bash-5.2p37")},
		{Path: storeP('d', "glibc-2.40-66")},
		{Path: storeP('g', "unit-sshd.service")},
	}}, time.Now()))

	f := d.FleetDependencies(fleet("laptop", "nas", "pi"))
	hosts := []string{}
	for _, h := range f.Hosts {
		hosts = append(hosts, h.Host+":"+h.State)
	}
	if !slices.Equal(hosts, []string{"laptop:none", "nas:ready", "pi:ready"}) {
		t.Fatalf("hosts: %v", hosts)
	}
	rows := map[string]DepRow{}
	for _, r := range f.Rows {
		rows[r.Name] = r
	}
	if _, ok := rows["unit-sshd.service"]; ok {
		t.Fatal("unversioned entries are left out")
	}
	if _, ok := rows["nixos-system-nas"]; ok {
		t.Fatal("each machine's own system is left out")
	}
	if r := rows["bash"]; !r.Differs || !slices.Equal(r.Versions[1], []string{"5.2p37", "5.3p3"}) || !slices.Equal(r.Versions[2], []string{"5.2p37"}) || len(r.Versions[0]) != 0 {
		t.Fatalf("bash: %+v", r)
	}
	if r := rows["glibc"]; r.Differs {
		t.Fatalf("glibc is the same everywhere it is: %+v", r)
	}
	if r := rows["hello"]; r.Differs || len(r.Versions[1]) != 0 {
		t.Fatalf("hello is on pi only: %+v", r)
	}
	if !slices.IsSortedFunc(f.Rows, func(a, b DepRow) int { return strings.Compare(a.Name, b.Name) }) {
		t.Fatal("rows are sorted by name")
	}
	if n := len(d.FleetDependencies(nil).Hosts); n != 2 {
		t.Fatalf("without a model, the machines that report: %d", n)
	}
}

func TestLookupDependencies(t *testing.T) {
	d := must[*Deployments](t)(OpenDeployments(t.TempDir(), newRepo(t)))
	sys := storePath('a', "pi")
	reported(t, d, "pi", sys)
	ok(t, d.RecordClosure("pi", sys, smallClosure(sys), time.Now()))
	md := must[MachineDeps](t)(d.MachineDependencies("pi"))
	got, more := md.Lookup("GLIBC", 25)
	if len(got) != 1 || more != 0 {
		t.Fatalf("glibc: %+v %d", got, more)
	}
	g := got[0]
	if g.Name != "glibc" || g.Version != "2.40-66" || g.Size != 4000 || len(g.Uses) != 0 ||
		!slices.Equal(g.UsedBy, []string{"bash-5.2p37", "hello-2.12.2"}) || !slices.Equal(g.Why, []string{"nixos-system-pi", "bash-5.2p37", "glibc-2.40-66"}) {
		t.Fatalf("glibc: %+v", g)
	}
	// Exact names first, then by name; at most limit, and how many more.
	all, more := md.Lookup("", 2)
	if len(all) != 2 || more != 2 || all[0].Name != "bash" {
		t.Fatalf("everything, 2 at most: %+v %d", all, more)
	}
	if got, _ := md.Lookup("hello", 5); len(got) != 1 || !slices.Equal(got[0].Uses, []string{"glibc-2.40-66"}) {
		t.Fatalf("hello: %+v", got)
	}
}
