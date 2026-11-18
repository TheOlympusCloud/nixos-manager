// Package mcpserver exposes the same core.Service the web UI uses as MCP tools.
package mcpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/TheOlympusCloud/nixos-manager/internal/core"
)

const outputTail = 20000

const instructions = `This server edits a git-backed Nix flake repository (NixOS, home-manager and other flake outputs).
Typical flow: list_files / read_file, write_file with the base_hash from read_file, stage, run_check, commit, push.
Commits and pushes are only accepted after run_check passed for exactly that content. Never store secrets in the repository.`

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

// ready waits for the job that computes a model or index, then asks again.
func ready[T any](ctx context.Context, f func() (T, *core.Job, error)) (T, error) {
	for {
		v, j, err := f()
		if err != nil || j == nil {
			return v, err
		}
		select {
		case <-j.Done():
		case <-ctx.Done():
			return v, ctx.Err()
		}
	}
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
	// Agents never get the force flag: only a person in the web UI may skip a check.
	tool(s, "commit", "Commit the staged changes. Refused unless run_check passed for exactly these staged changes.", func(ctx context.Context, in struct {
		Message string `json:"message"`
	}) (map[string]string, error) {
		sha, err := svc.Commit(ctx, in.Message, false)
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
	tool(s, "restore_file", "Put a file back the way it was at a commit (as an uncommitted change). Pass base_hash from read_file, or an empty base_hash when the file does not exist now; it is refused if the file changed since.", func(ctx context.Context, in struct {
		Path     string `json:"path"`
		Rev      string `json:"rev"`
		BaseHash string `json:"base_hash,omitempty" jsonschema:"hash from read_file; empty when the file does not exist now"`
	}) (done, error) {
		return done{true}, svc.Restore(ctx, in.Path, in.Rev, in.BaseHash)
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
	tool(s, "push", "Push the current branch to the remote. Refused until the commit being pushed has passed run_check (e.g. after a pull rebased local commits: run_check, then push).", func(ctx context.Context, _ struct{}) (map[string]string, error) {
		out, err := svc.Push(ctx, false)
		return map[string]string{"output": out}, err
	})
	tool(s, "pull", "Bring the current branch up to date with the remote (fast-forward, or rebase local commits; nothing changes on conflict).", func(ctx context.Context, _ struct{}) (map[string]string, error) {
		out, err := svc.Pull(ctx)
		return map[string]string{"output": out}, err
	})

	working := func(ctx context.Context) (*core.Model, error) {
		return ready(ctx, func() (*core.Model, *core.Job, error) { return svc.Jobs.Model(ctx, "working") })
	}
	type hostSummary struct {
		Name        string   `json:"name"`
		System      string   `json:"system"`
		Release     string   `json:"release"`
		ManagedFile *string  `json:"managedFile"`
		Packages    int      `json:"packages"`
		Features    []string `json:"features"`
		Users       []string `json:"users"`
		Error       string   `json:"error,omitempty"`
	}
	tool(s, "fleet_overview", "Summarise every machine of the flake as Nix evaluates the working copy: system, NixOS release, package count, enabled services/programs, users, evaluation errors, and the flake inputs.", func(ctx context.Context, _ struct{}) (map[string]any, error) {
		m, err := working(ctx)
		if err != nil {
			return nil, err
		}
		hosts := []hostSummary{}
		for _, h := range m.Hosts {
			sum := hostSummary{Name: h.Name, System: h.System, Release: h.Release, ManagedFile: h.ManagedFile, Packages: len(h.Packages), Features: []string{}, Users: []string{}, Error: h.Error}
			for _, f := range h.Features {
				sum.Features = append(sum.Features, f.Group+"."+f.Name)
			}
			for _, u := range h.Users {
				sum.Users = append(sum.Users, u.Name)
			}
			hosts = append(hosts, sum)
		}
		out := map[string]any{"hosts": hosts, "homes": m.Homes, "inputs": m.Inputs}
		if m.Error != "" {
			out["error"] = m.Error
		}
		return out, nil
	})
	tool(s, "fleet_insights", "Health and consistency findings for the fleet (evaluation errors, firewall, SSH hardening, garbage collection, outdated flake inputs, packages missing on one machine), each with a suggested fix.", func(ctx context.Context, _ struct{}) (map[string][]core.Insight, error) {
		m, err := working(ctx)
		if err != nil {
			return nil, err
		}
		return map[string][]core.Insight{"insights": core.Insights(m, time.Now())}, nil
	})
	tool(s, "compare_option", "The effective value of NixOS options on every machine (or the given ones), with the files that set each value. Names are full option names such as services.openssh.settings.PermitRootLogin; quote segments containing dots.", func(ctx context.Context, in struct {
		Names []string `json:"names"`
		Hosts []string `json:"hosts,omitempty"`
	}) (map[string][]core.OptionValues, error) {
		res, err := ready(ctx, func() ([]core.OptionValues, *core.Job, error) { return svc.Jobs.OptionValues(ctx, in.Names, in.Hosts) })
		return map[string][]core.OptionValues{"values": res}, err
	})
	tool(s, "create_machine", "Add a machine: hosts/<name>/ with default.nix, hardware-configuration.nix (the pasted nixos-generate-config output, or a placeholder) and a managed.json preset for the role. Says whether the flake picks it up by itself or which line to add.", func(ctx context.Context, in core.MachineSpec) (core.MachineCreated, error) {
		return svc.CreateMachine(in)
	})
	tool(s, "host_details", "Everything the fleet model knows about one machine: packages with the files that add them, enabled features, users and home-manager, ports, file systems, files, and whether it has a managed.json.", func(ctx context.Context, in struct {
		Host string `json:"host"`
	}) (core.Host, error) {
		m, err := working(ctx)
		if err != nil {
			return core.Host{}, err
		}
		for _, h := range m.Hosts {
			if h.Name == in.Host {
				return h, nil
			}
		}
		return core.Host{}, &core.Error{Kind: core.ErrNotFound, Msg: "no machine named " + in.Host}
	})
	tool(s, "model_diff", "What the uncommitted changes do to each machine: packages, services/programs, users and ports added or removed, compared with the last commit.", func(ctx context.Context, _ struct{}) (map[string]any, error) {
		head, err := ready(ctx, func() (*core.Model, *core.Job, error) { return svc.Jobs.Model(ctx, "head") })
		if err != nil {
			return nil, err
		}
		work, err := working(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"hosts": core.DiffModels(head, work)}, nil
	})
	tool(s, "search_packages", "Search the nixpkgs the flake locks. The first search builds the index, which can take minutes.", func(ctx context.Context, in struct {
		Query string `json:"query"`
		Limit int    `json:"limit,omitempty"`
	}) (map[string][]core.PackageResult, error) {
		res, err := ready(ctx, func() ([]core.PackageResult, *core.Job, error) {
			return svc.Jobs.SearchPackages(ctx, in.Query, in.Limit)
		})
		return map[string][]core.PackageResult{"results": res}, err
	})
	tool(s, "search_options", "Search the documented NixOS options of a machine (name, type, description, default, example). A prefix such as services.openssh lists that module's options, enable first.", func(ctx context.Context, in struct {
		Host  string `json:"host"`
		Query string `json:"query"`
		Limit int    `json:"limit,omitempty"`
	}) (map[string][]core.OptionDoc, error) {
		res, err := ready(ctx, func() ([]core.OptionDoc, *core.Job, error) {
			return svc.Jobs.SearchOptions(ctx, in.Host, in.Query, in.Limit)
		})
		return map[string][]core.OptionDoc{"results": res}, err
	})

	type hostPackages struct {
		Host     string   `json:"host" jsonschema:"machine name (nixosConfigurations attribute)"`
		Packages []string `json:"packages" jsonschema:"nixpkgs attribute names, e.g. htop or python3Packages.requests"`
	}
	type hostPath struct {
		Host string   `json:"host" jsonschema:"machine name (nixosConfigurations attribute)"`
		Path []string `json:"path" jsonschema:"option path as separate parts, e.g. [\"services\", \"openssh\", \"enable\"]"`
	}
	tool(s, "add_packages", "Install nixpkgs packages on a machine, through its managed.json (hosts/<host>/managed.json).", func(ctx context.Context, in hostPackages) (core.ManagedView, error) {
		return svc.AddPackages(in.Host, in.Packages)
	})
	tool(s, "remove_packages", "Remove packages from a machine's managed.json. Packages added in hand-written Nix files must be edited there.", func(ctx context.Context, in hostPackages) (core.ManagedView, error) {
		return svc.RemovePackages(in.Host, in.Packages)
	})
	tool(s, "set_option", "Set a NixOS option on a machine through its managed.json, e.g. path [\"services\",\"openssh\",\"enable\"] with value true. Use search_options to find options and their types.", func(ctx context.Context, in struct {
		Host  string   `json:"host"`
		Path  []string `json:"path"`
		Value any      `json:"value" jsonschema:"the JSON value: boolean, number, string, list or object"`
	}) (core.ManagedView, error) {
		return svc.SetOption(in.Host, in.Path, in.Value, nil)
	})
	tool(s, "unset_option", "Remove an option setting from a machine's managed.json.", func(ctx context.Context, in hostPath) (core.ManagedView, error) {
		return svc.UnsetOption(in.Host, in.Path, nil)
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
