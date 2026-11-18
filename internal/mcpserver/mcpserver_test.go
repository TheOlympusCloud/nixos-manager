package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/TheOlympusCloud/nixos-manager/internal/core"
)

func TestMain(m *testing.M) {
	os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Unsetenv("GIT_CONFIG_COUNT")
	os.Exit(m.Run())
}

func session(t *testing.T) (*mcp.ClientSession, *core.Service) {
	t.Helper()
	repo, err := core.OpenRepo(filepath.Join(t.TempDir(), "repo"), "", "Test User", "test@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	svc := core.NewService(repo)
	nix := filepath.Join(t.TempDir(), "nix")
	os.WriteFile(nix, []byte("#!/bin/sh\necho \"nix $*\"\n"), 0o755)
	svc.Jobs.Nix = nix

	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	if _, err := New(svc, "test").Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, svc
}

// call runs a tool and decodes its structured result into out (if not nil).
func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any, out any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if out != nil && !res.IsError {
		b, _ := json.Marshal(res.StructuredContent)
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatalf("%s result: %v", name, err)
		}
	}
	return res
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestToolsList(t *testing.T) {
	cs, _ := session(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, tl := range res.Tools {
		have[tl.Name] = true
	}
	for _, want := range []string{"list_files", "read_file", "write_file", "create_folder", "move_path", "delete_path", "format_nix",
		"status", "diff", "stage", "unstage", "discard", "commit", "log", "show_commit", "read_file_at", "restore_file",
		"list_branches", "create_branch", "switch_branch", "push", "pull", "list_targets", "run_check", "flake_lock", "flake_update",
		"fleet_overview", "host_details", "model_diff", "add_packages", "remove_packages", "set_option", "unset_option",
		"search_packages", "search_options", "fleet_insights", "compare_option", "create_machine",
		"revert_commit", "merge_branch"} {
		if !have[want] {
			t.Errorf("missing tool %s", want)
		}
	}
}

// fleetSession is a session whose nix is scripts/dev/fake-nix serving the fixture fleet.
func fleetSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	cs, svc := session(t)
	nix, _ := filepath.Abs("../../scripts/dev/fake-nix")
	fixtures, _ := filepath.Abs("../core/testdata/fleet")
	t.Setenv("NIXMGR_FAKE_FIXTURES", fixtures)
	svc.Jobs.Nix = nix
	lock, _ := os.ReadFile("../core/testdata/fleet/flake.lock")
	if _, err := svc.Write("flake.lock", string(lock), ""); err != nil {
		t.Fatal(err)
	}
	return cs
}

func TestFleetTools(t *testing.T) {
	cs := fleetSession(t)
	var overview struct {
		Hosts []struct {
			Name     string
			Packages int
			Features []string
			Error    string
		}
		Inputs []core.Input
	}
	call(t, cs, "fleet_overview", nil, &overview)
	if len(overview.Hosts) != 4 || overview.Hosts[3].Name != "workstation" || overview.Hosts[3].Packages < 20 || overview.Hosts[1].Error == "" || len(overview.Inputs) != 3 {
		t.Fatalf("overview %+v", overview)
	}
	var h core.Host
	call(t, cs, "host_details", map[string]any{"host": "pi"}, &h)
	if h.System != "aarch64-linux" || len(h.Features) == 0 {
		t.Fatalf("host details %+v", h)
	}
	if res := call(t, cs, "host_details", map[string]any{"host": "nope"}, nil); !res.IsError {
		t.Fatal("unknown host accepted")
	}
	var diff struct{ Hosts []core.HostDiff }
	call(t, cs, "model_diff", nil, &diff)
	if len(diff.Hosts) != 4 || diff.Hosts[0].Status != "added" {
		t.Fatalf("diff %+v", diff)
	}
	var health struct{ Insights []core.Insight }
	call(t, cs, "fleet_insights", nil, &health)
	if len(health.Insights) == 0 || health.Insights[0].Severity != "error" {
		t.Fatalf("fleet insights %+v", health)
	}
	var cmp struct{ Values []core.OptionValues }
	call(t, cs, "compare_option", map[string]any{"names": []string{"services.openssh.settings.PermitRootLogin"}}, &cmp)
	if len(cmp.Values) != 1 || len(cmp.Values[0].Hosts) != 4 {
		t.Fatalf("compare option %+v", cmp)
	}
	var pk struct{ Results []core.PackageResult }
	call(t, cs, "search_packages", map[string]any{"query": "btop"}, &pk)
	if len(pk.Results) != 1 || pk.Results[0].Attr != "btop" {
		t.Fatalf("package search %+v", pk)
	}
	var op struct{ Results []core.OptionDoc }
	call(t, cs, "search_options", map[string]any{"host": "workstation", "query": "jellyfin"}, &op)
	if len(op.Results) != 2 || op.Results[0].Name != "services.jellyfin.enable" {
		t.Fatalf("option search %+v", op)
	}
}

func TestManagedTools(t *testing.T) {
	cs, svc := session(t)
	var v core.ManagedView
	call(t, cs, "add_packages", map[string]any{"host": "web", "packages": []string{"htop", "git"}}, &v)
	call(t, cs, "set_option", map[string]any{"host": "web", "path": []string{"services", "openssh", "enable"}, "value": true}, &v)
	call(t, cs, "remove_packages", map[string]any{"host": "web", "packages": []string{"htop"}}, &v)
	call(t, cs, "unset_option", map[string]any{"host": "web", "path": []string{"services", "openssh", "enable"}}, &v)
	call(t, cs, "set_option", map[string]any{"host": "web", "path": []string{"time", "timeZone"}, "value": "Europe/Madrid"}, &v)
	got, err := svc.ManagedView("web")
	if err != nil || len(got.Packages) != 1 || got.Packages[0] != "git" || got.Settings["time"] == nil {
		t.Fatalf("managed file after MCP edits %+v %v", got, err)
	}
	if res := call(t, cs, "add_packages", map[string]any{"host": "web", "packages": []string{"$(evil)"}}, nil); !res.IsError {
		t.Fatal("invalid package accepted over MCP")
	}
}

func TestEditCheckCommitFlow(t *testing.T) {
	cs, svc := session(t)
	var w struct{ Hash string }
	call(t, cs, "write_file", map[string]any{"path": "hosts/web/default.nix", "content": "{ }\n"}, &w)
	if res := call(t, cs, "write_file", map[string]any{"path": "hosts/web/default.nix", "content": "x", "base_hash": "stale"}, nil); !res.IsError || !strings.Contains(text(res), "changed since") {
		t.Fatalf("stale write not refused: %s", text(res))
	}
	var r struct{ Content, Hash string }
	call(t, cs, "read_file", map[string]any{"path": "hosts/web/default.nix"}, &r)
	if r.Content != "{ }\n" || r.Hash != w.Hash {
		t.Fatalf("read %+v", r)
	}
	if res := call(t, cs, "read_file", map[string]any{"path": "../etc/passwd"}, nil); !res.IsError {
		t.Fatal("path escape not refused")
	}

	call(t, cs, "stage", map[string]any{}, nil)
	if res := call(t, cs, "commit", map[string]any{"message": "feat: add web"}, nil); !res.IsError || !strings.Contains(text(res), "run the checks first") {
		t.Fatalf("unchecked commit not refused: %s", text(res))
	}
	if res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "commit", Arguments: map[string]any{"message": "x", "force": true}}); err == nil && !res.IsError {
		t.Fatal("an agent bypassed the check with force")
	}
	var job struct{ State, Output string }
	call(t, cs, "run_check", map[string]any{"targets": []string{"nixosConfigurations.web.config.system.build.toplevel"}}, &job)
	if job.State != "passed" || !strings.Contains(job.Output, "flake check") {
		t.Fatalf("check %+v", job)
	}
	var st core.Status
	call(t, cs, "status", nil, &st)
	if !st.StagedChecked {
		t.Fatal("status does not show the passed check")
	}
	var c struct{ SHA string }
	call(t, cs, "commit", map[string]any{"message": "feat: add web"}, &c)

	// The UI's service sees the same commit, made as the configured human.
	log, err := svc.Log(context.Background(), "hosts/web/default.nix", "", 5)
	if err != nil || len(log) != 1 || log[0].SHA != c.SHA || log[0].Author != "Test User" {
		t.Fatalf("log %+v %v", log, err)
	}
	var info core.CommitInfo
	call(t, cs, "show_commit", map[string]any{"rev": c.SHA}, &info)
	if !strings.Contains(info.Diff, "hosts/web/default.nix") {
		t.Fatalf("show_commit %+v", info)
	}

	// restore_file refuses to overwrite an edit it has not seen.
	call(t, cs, "write_file", map[string]any{"path": "hosts/web/default.nix", "content": "{ edited = true; }\n", "base_hash": r.Hash}, nil)
	if res := call(t, cs, "restore_file", map[string]any{"path": "hosts/web/default.nix", "rev": c.SHA, "base_hash": r.Hash}, nil); !res.IsError {
		t.Fatal("restore over an unseen edit was accepted")
	}
	call(t, cs, "read_file", map[string]any{"path": "hosts/web/default.nix"}, &r)
	if res := call(t, cs, "restore_file", map[string]any{"path": "hosts/web/default.nix", "rev": c.SHA, "base_hash": r.Hash}, nil); res.IsError {
		t.Fatalf("restore with the current hash: %s", text(res))
	}
}
