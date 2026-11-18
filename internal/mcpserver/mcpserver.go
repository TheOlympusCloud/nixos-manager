// Package mcpserver exposes the same core.Service the web UI uses as MCP tools.
package mcpserver

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/TheOlympusCloud/nixos-manager/internal/core"
)

const outputTail = 20000

const instructions = `This server edits a git-backed Nix flake repository (NixOS, home-manager and other flake outputs).
Typical flow: list_files / read_file, write_file with the base_hash from read_file, stage, run_check, commit, push.
Commits are only accepted after run_check passed for exactly the staged changes. Never store secrets in the repository.`

type done struct {
	Done bool `json:"done"`
}

type pathIn struct {
	Path string `json:"path" jsonschema:"repository-relative path, e.g. hosts/web/default.nix"`
}

type pathsIn struct {
	Paths []string `json:"paths,omitempty" jsonschema:"repository-relative paths; empty means all changes"`
}

type jobOut struct {
	ID     string `json:"id"`
	State  string `json:"state" jsonschema:"queued, running, passed or failed"`
	Output string `json:"output" jsonschema:"the end of the command output"`
}

func tool[In, Out any](s *mcp.Server, name, desc string, fn func(context.Context, In) (Out, error)) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc}, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		out, err := fn(ctx, in)
		return nil, out, err
	})
}

// waitJob waits for a job to finish (or the call to be cancelled) and returns its output tail.
func waitJob(ctx context.Context, j *core.Job) jobOut {
	select {
	case <-j.Done():
	case <-ctx.Done():
	}
	info := j.Info(true)
	if len(info.Output) > outputTail {
		info.Output = "[…]\n" + info.Output[len(info.Output)-outputTail:]
	}
	return jobOut{ID: info.ID, State: info.State, Output: info.Output}
}

// New builds the MCP server. Every tool calls the shared service.
func New(svc *core.Service, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "nixos-manager", Version: version}, &mcp.ServerOptions{Instructions: instructions})

	tool(s, "list_files", "List every file and folder in the working copy.", func(ctx context.Context, _ struct{}) (map[string][]core.Entry, error) {
		e, err := svc.Tree()
		return map[string][]core.Entry{"entries": e}, err
	})
	tool(s, "read_file", "Read a text file. Returns its content and the hash to pass to write_file.", func(ctx context.Context, in pathIn) (map[string]string, error) {
		c, h, err := svc.Read(in.Path)
		return map[string]string{"path": in.Path, "content": c, "hash": h}, err
	})
	tool(s, "write_file", "Create or replace a text file. Pass base_hash from read_file when replacing (the write is refused if the file changed since), or an empty base_hash to create a new file. Parent folders are created.",
		func(ctx context.Context, in struct {
			Path     string `json:"path"`
			Content  string `json:"content"`
			BaseHash string `json:"base_hash,omitempty" jsonschema:"hash from read_file; empty to create a new file"`
		}) (map[string]string, error) {
			h, err := svc.Write(in.Path, in.Content, in.BaseHash)
			return map[string]string{"hash": h}, err
		})
	tool(s, "create_folder", "Create a folder.", func(ctx context.Context, in pathIn) (done, error) {
		return done{true}, svc.Mkdir(in.Path)
	})
	tool(s, "move_path", "Rename or move a file or folder.", func(ctx context.Context, in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}) (done, error) {
		return done{true}, svc.Move(in.From, in.To)
	})
	tool(s, "delete_path", "Delete a file, or a folder with everything in it.", func(ctx context.Context, in pathIn) (done, error) {
		return done{true}, svc.Delete(in.Path)
	})
	tool(s, "format_nix", "Format Nix code with nixfmt and return the result (does not write any file).", func(ctx context.Context, in struct {
		Content string `json:"content"`
	}) (map[string]string, error) {
		out, err := svc.Jobs.Format(ctx, in.Content)
		return map[string]string{"content": out}, err
	})

	tool(s, "status", "Show the branch, ahead/behind counts and changed files (staged and unstaged), and whether the staged changes passed a check.", func(ctx context.Context, _ struct{}) (core.Status, error) {
		return svc.Status(ctx)
	})
	tool(s, "diff", "Show a unified diff of unstaged changes (default) or staged changes, optionally for one path.", func(ctx context.Context, in struct {
		Path   string `json:"path,omitempty"`
		Staged bool   `json:"staged,omitempty"`
	}) (map[string]string, error) {
		d, err := svc.Diff(ctx, in.Path, in.Staged)
		return map[string]string{"diff": d}, err
	})
	tool(s, "stage", "Stage changes (all when paths is empty) for the next commit.", func(ctx context.Context, in pathsIn) (done, error) {
		return done{true}, svc.Stage(ctx, in.Paths)
	})
	tool(s, "unstage", "Unstage changes (all when paths is empty); the files keep their content.", func(ctx context.Context, in pathsIn) (done, error) {
		return done{true}, svc.Unstage(ctx, in.Paths)
	})
	tool(s, "discard", "Throw away unstaged changes to the given paths; untracked files are deleted.", func(ctx context.Context, in struct {
		Paths []string `json:"paths"`
	}) (done, error) {
		return done{true}, svc.Discard(ctx, in.Paths)
	})
	tool(s, "commit", "Commit the staged changes. Refused unless run_check passed for exactly these staged changes; force skips that and should only be used when asked.", func(ctx context.Context, in struct {
		Message string `json:"message"`
		Force   bool   `json:"force,omitempty"`
	}) (map[string]string, error) {
		sha, err := svc.Commit(ctx, in.Message, in.Force)
		return map[string]string{"sha": sha}, err
	})
	tool(s, "log", "List commits newest first, for the repository or one file (following renames).", func(ctx context.Context, in struct {
		Path  string `json:"path,omitempty"`
		Rev   string `json:"rev,omitempty" jsonschema:"branch or commit to start from; default HEAD"`
		Limit int    `json:"limit,omitempty"`
	}) (map[string][]core.Commit, error) {
		c, err := svc.Log(ctx, in.Path, in.Rev, in.Limit)
		return map[string][]core.Commit{"commits": c}, err
	})
	tool(s, "show_commit", "Show a commit's message and full diff.", func(ctx context.Context, in struct {
		Rev string `json:"rev"`
	}) (core.CommitInfo, error) {
		return svc.CommitInfo(ctx, in.Rev)
	})
	tool(s, "read_file_at", "Read a file as it was at a commit.", func(ctx context.Context, in struct {
		Rev  string `json:"rev"`
		Path string `json:"path"`
	}) (map[string]string, error) {
		c, err := svc.FileAt(ctx, in.Rev, in.Path)
		return map[string]string{"content": c}, err
	})
	tool(s, "restore_file", "Put a file back the way it was at a commit (as an uncommitted change).", func(ctx context.Context, in struct {
		Path string `json:"path"`
		Rev  string `json:"rev"`
	}) (done, error) {
		return done{true}, svc.Restore(ctx, in.Path, in.Rev)
	})
	tool(s, "list_branches", "List local branches and the current one.", func(ctx context.Context, _ struct{}) (core.Branches, error) {
		return svc.Branches(ctx)
	})
	tool(s, "create_branch", "Create a branch (from HEAD, or from the given commit) and switch to it.", func(ctx context.Context, in struct {
		Name string `json:"name"`
		From string `json:"from,omitempty"`
	}) (done, error) {
		return done{true}, svc.CreateBranch(ctx, in.Name, in.From)
	})
	tool(s, "switch_branch", "Switch to another local branch; refused if local changes would be lost.", func(ctx context.Context, in struct {
		Name string `json:"name"`
	}) (done, error) {
		return done{true}, svc.Switch(ctx, in.Name)
	})
	tool(s, "push", "Push the current branch to the remote.", func(ctx context.Context, _ struct{}) (map[string]string, error) {
		out, err := svc.Push(ctx)
		return map[string]string{"output": out}, err
	})
	tool(s, "pull", "Bring the current branch up to date with the remote (fast-forward, or rebase local commits; nothing changes on conflict).", func(ctx context.Context, _ struct{}) (map[string]string, error) {
		out, err := svc.Pull(ctx)
		return map[string]string{"output": out}, err
	})

	tool(s, "list_targets", "List the NixOS, home-manager and nix-darwin configurations in the staged flake, with the attribute to pass to run_check.", func(ctx context.Context, _ struct{}) (map[string][]core.Target, error) {
		t, err := svc.Jobs.Targets(ctx)
		return map[string][]core.Target{"targets": t}, err
	})
	tool(s, "run_check", "Validate the staged changes: nix flake check, then a dry build of each target attribute. Waits for the result.", func(ctx context.Context, in struct {
		Targets []string `json:"targets,omitempty" jsonschema:"attributes from list_targets; empty runs only nix flake check"`
	}) (jobOut, error) {
		j, err := svc.Jobs.Submit(ctx, core.JobRequest{Kind: "check", Targets: in.Targets})
		if err != nil {
			return jobOut{}, err
		}
		return waitJob(ctx, j), nil
	})
	tool(s, "flake_lock", "Run nix flake lock in the working copy (after adding an input). Waits for the result.", func(ctx context.Context, _ struct{}) (jobOut, error) {
		j, err := svc.Jobs.Submit(ctx, core.JobRequest{Kind: "lock"})
		if err != nil {
			return jobOut{}, err
		}
		return waitJob(ctx, j), nil
	})
	tool(s, "flake_update", "Run nix flake update for the given inputs (all when empty) in the working copy. Waits for the result.", func(ctx context.Context, in struct {
		Inputs []string `json:"inputs,omitempty"`
	}) (jobOut, error) {
		j, err := svc.Jobs.Submit(ctx, core.JobRequest{Kind: "update", Inputs: in.Inputs})
		if err != nil {
			return jobOut{}, err
		}
		return waitJob(ctx, j), nil
	})
	return s
}

// Handler serves the server over Streamable HTTP. Callers must authenticate
// requests first; DNS-rebinding protection is off because a reverse proxy
// (e.g. tailscale serve) forwards from localhost and the bearer token already
// blocks browsers.
func Handler(s *mcp.Server) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{DisableLocalhostProtection: true})
}
