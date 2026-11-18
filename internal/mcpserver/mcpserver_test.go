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
		"list_branches", "create_branch", "switch_branch", "push", "pull", "list_targets", "run_check", "flake_lock", "flake_update"} {
		if !have[want] {
			t.Errorf("missing tool %s", want)
		}
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
