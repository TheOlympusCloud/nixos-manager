package mcpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/TheOlympusCloud/nixos-manager/internal/core"
)

func TestDeploymentsTool(t *testing.T) {
	cs, svc := session(t)
	var ov core.DeployOverview
	if res := call(t, cs, "deployments", nil, &ov); res.IsError || ov.Reporting || len(ov.Machines) != 0 {
		t.Fatalf("without reports: %s %+v", text(res), ov)
	}
	d, err := core.OpenDeployments(t.TempDir(), svc.Repo)
	if err != nil {
		t.Fatal(err)
	}
	svc.Deploys = d
	sha := strings.Repeat("c", 40)
	if err := d.Record(core.ReportIn{Host: "pi", Commit: sha, Status: "failed", Error: "switch-to-configuration failed"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if res := call(t, cs, "deployments", nil, &ov); res.IsError || !ov.Reporting || len(ov.Machines) != 1 {
		t.Fatalf("with a report: %s %+v", text(res), ov)
	}
	if m := ov.Machines[0]; m.Host != "pi" || m.State != "failed" || m.Failure == nil || m.Failure.Error != "switch-to-configuration failed" {
		t.Fatalf("pi: %+v", m)
	}
}

func TestDependencyTools(t *testing.T) {
	cs, svc := session(t)
	d, err := core.OpenDeployments(t.TempDir(), svc.Repo)
	if err != nil {
		t.Fatal(err)
	}
	svc.Deploys = d
	store := func(c byte, name string) string { return "/nix/store/" + strings.Repeat(string(c), 32) + "-" + name }
	pi, nas := store('a', "nixos-system-pi"), store('b', "nixos-system-nas")
	for host, sys := range map[string]string{"pi": pi, "nas": nas} {
		if err := d.Record(core.ReportIn{Host: host, Phase: "heartbeat", Current: sys, SendsClosure: true}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	var md struct {
		State   string
		Count   int
		Matches []core.DepEntry
		More    int
	}
	if res := call(t, cs, "machine_dependencies", map[string]any{"host": "pi"}, &md); res.IsError || md.State != "waiting" || md.Count != 0 {
		t.Fatalf("before the closure: %s %+v", text(res), md)
	}
	bash, glibc := store('c', "bash-5.2p37"), store('d', "glibc-2.40-66")
	if err := d.RecordClosure("pi", pi, core.ClosureIn{Paths: []core.ClosurePath{{Path: pi, Refs: []int{1}}, {Path: bash, Refs: []int{2}}, {Path: glibc}}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := d.RecordClosure("nas", nas, core.ClosureIn{Paths: []core.ClosurePath{{Path: nas, Refs: []int{1}}, {Path: store('f', "bash-5.3p3")}}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if res := call(t, cs, "machine_dependencies", map[string]any{"host": "pi", "name": "glibc"}, &md); res.IsError || md.State != "ready" || md.Count != 3 || len(md.Matches) != 1 ||
		strings.Join(md.Matches[0].Why, " ") != "nixos-system-pi bash-5.2p37 glibc-2.40-66" {
		t.Fatalf("glibc on pi: %s %+v", text(res), md)
	}
	if res := call(t, cs, "machine_dependencies", map[string]any{"host": "../pi"}, nil); !res.IsError {
		t.Fatal("an invalid host is an error")
	}
	var f struct {
		Hosts []core.FleetDepsHost
		Rows  []core.DepRow
		Total int
	}
	if res := call(t, cs, "fleet_dependencies", map[string]any{"differs": true}, &f); res.IsError || f.Total != 1 || len(f.Rows) != 1 || f.Rows[0].Name != "bash" || len(f.Hosts) != 2 {
		t.Fatalf("differing: %s %+v", text(res), f)
	}
	if res := call(t, cs, "fleet_dependencies", map[string]any{"name": "GLI"}, &f); res.IsError || f.Total != 1 || f.Rows[0].Name != "glibc" {
		t.Fatalf("glibc: %s %+v", text(res), f)
	}
}
