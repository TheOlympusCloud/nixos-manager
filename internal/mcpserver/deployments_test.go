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
