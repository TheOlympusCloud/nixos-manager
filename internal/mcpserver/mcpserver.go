// Package mcpserver exposes the same core.Service the web UI uses as MCP tools.
package mcpserver

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/TheOlympusCloud/nixos-manager/internal/auth"
	"github.com/TheOlympusCloud/nixos-manager/internal/core"
)

const outputTail = 20000

const instructions = `This server edits a git-backed Nix flake repository (NixOS, home-manager and other flake outputs).
Typical flow: list_files / read_file, write_file with the base_hash from read_file, stage, run_check, commit, push.
Commits and pushes are only accepted after run_check passed for exactly that content; it builds every machine the change touches, or, when the server sets checks.build = false, only evaluates and dry-builds them (summary.evaluatedOnly) and each machine builds itself when comin deploys. Never write a secret in a file: set_secret stores it encrypted for the machines (sops-nix).`

type done struct {
	Done bool `json:"done"`
}

type pathIn struct {
	Path string `json:"path" jsonschema:"repository-relative path, e.g. hosts/web/default.nix"`
}

type pathsIn struct {
	Paths []string `json:"paths,omitempty" jsonschema:"repository-relative paths; empty means all changes"`
}

// copyOut wraps copy_to_machines' results: a tool's output is an object.
type copyOut struct {
	Results []core.CopyResult `json:"results"`
}

type jobOut struct {
	ID      string             `json:"id"`
	State   string             `json:"state" jsonschema:"queued, running, passed or failed"`
	Summary *core.CheckSummary `json:"summary,omitempty" jsonschema:"for a check: the machines it compared, built, only evaluated (evaluatedOnly: checks.build = false) or failed, and what Nix built and downloaded"`
	Output  string             `json:"output" jsonschema:"the end of the command output"`
}

// toolScope is the token scope each tool needs. A tool missing here needs full.
var toolScope = map[string]auth.Scope{
	"list_files": auth.ScopeRead, "read_file": auth.ScopeRead, "format_nix": auth.ScopeRead, "status": auth.ScopeRead,
	"diff": auth.ScopeRead, "log": auth.ScopeRead, "show_commit": auth.ScopeRead, "read_file_at": auth.ScopeRead,
	"list_branches": auth.ScopeRead, "fleet_overview": auth.ScopeRead, "fleet_insights": auth.ScopeRead,
	"deployments": auth.ScopeRead, "compare_option": auth.ScopeRead, "host_details": auth.ScopeRead,
	"model_diff": auth.ScopeRead, "search_packages": auth.ScopeRead, "search_options": auth.ScopeRead,
	"list_targets": auth.ScopeRead, "list_secrets": auth.ScopeRead, "machine_dependencies": auth.ScopeRead, "fleet_dependencies": auth.ScopeRead,

	"write_file": auth.ScopeEdit, "create_folder": auth.ScopeEdit, "move_path": auth.ScopeEdit, "delete_path": auth.ScopeEdit,
	"stage": auth.ScopeEdit, "unstage": auth.ScopeEdit, "discard": auth.ScopeEdit, "commit": auth.ScopeEdit,
	"restore_file": auth.ScopeEdit, "create_branch": auth.ScopeEdit, "switch_branch": auth.ScopeEdit,
	"revert_commit": auth.ScopeEdit, "merge_branch": auth.ScopeEdit, "pull": auth.ScopeEdit,
	"create_machine": auth.ScopeEdit, "add_packages": auth.ScopeEdit, "remove_packages": auth.ScopeEdit,
	"set_option": auth.ScopeEdit, "unset_option": auth.ScopeEdit, "run_check": auth.ScopeEdit,
	"edit_packages": auth.ScopeEdit, "set_package_override": auth.ScopeEdit, "set_home_manager": auth.ScopeEdit, "link_home_secret": auth.ScopeEdit,
	"set_home_config": auth.ScopeEdit, "set_home_config_key": auth.ScopeEdit, "remove_home_config_key": auth.ScopeEdit,
	"set_keyring_entry": auth.ScopeEdit, "remove_keyring_entry": auth.ScopeEdit,
	"copy_to_machines":      auth.ScopeEdit,
	"update_managed_module": auth.ScopeEdit,
	"flake_lock":            auth.ScopeEdit, "flake_update": auth.ScopeEdit, "set_secret": auth.ScopeEdit, "delete_secret": auth.ScopeEdit,

	"push": auth.ScopeFull, "try_on_machine": auth.ScopeFull, "end_trial": auth.ScopeFull,
}

// server is an MCP server with the tools one token scope allows.
type server struct {
	*mcp.Server
	scope auth.Scope
}

// tool adds a tool when the scope allows it. Every call names the token, from
// the request's token info, as the caller.
func tool[In, Out any](s server, name, desc string, fn func(context.Context, In) (Out, error)) {
	need, ok := toolScope[name]
	if !ok {
		need = auth.ScopeFull
	}
	if !s.scope.Allows(need) {
		return
	}
	mcp.AddTool(s.Server, &mcp.Tool{Name: name, Description: desc}, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		c := core.Caller{}
		if req != nil && req.Extra != nil && req.Extra.TokenInfo != nil {
			c.Via, _ = req.Extra.TokenInfo.Extra["via"].(string)
		}
		out, err := fn(core.WithCaller(ctx, c), in)
		return nil, out, err
	})
}

var scopeNote = map[auth.Scope]string{
	auth.ScopeRead: "\nThis token is read-only: it can look at everything but change nothing.",
	auth.ScopeEdit: "\nThis token cannot push: its commits stay on the server until a person or a token with full access pushes them.",
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
	return jobOut{ID: info.ID, State: info.State, Summary: info.Summary, Output: info.Output}
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

// New builds the MCP server for a token scope. Every tool calls the shared service.
func New(svc *core.Service, version string, scope auth.Scope) *mcp.Server {
	s := server{mcp.NewServer(&mcp.Implementation{Name: "nixos-manager", Version: version}, &mcp.ServerOptions{Instructions: instructions + scopeNote[scope]}), scope}

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
	tool(s, "commit", "Commit the staged changes. Refused unless run_check passed for exactly these staged changes; a pass that only evaluated the machines (checks.build = false) counts.", func(ctx context.Context, in struct {
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
	tool(s, "revert_commit", "Stage the inverse of a commit (nothing is committed): run_check, then commit, like any change. With since, the inverse of every commit after since up to rev, as one change: deployments names them as culprits for a machine whose deployment failed, to bring it back to what it last deployed. Refused if there are local changes, the commits are not on the current branch (pull first), the range has a merge, or it would conflict.", func(ctx context.Context, in struct {
		Rev   string `json:"rev"`
		Since string `json:"since,omitempty" jsonschema:"optional: revert every commit after this one, up to rev"`
	}) (done, error) {
		return done{true}, svc.Revert(ctx, in.Rev, in.Since)
	})
	tool(s, "merge_branch", "Stage another branch (e.g. testing-<host>) squashed into the current one and return a suggested commit message; run_check, then commit.", func(ctx context.Context, in struct {
		Branch string `json:"branch"`
	}) (map[string]string, error) {
		msg, err := svc.Merge(ctx, in.Branch)
		return map[string]string{"message": msg}, err
	})
	tool(s, "push", "Push the current branch to the remote. Refused until the commit being pushed has passed run_check, which built every machine the push changes, or only evaluated and dry-built them when the server sets checks.build = false (e.g. after a pull rebased local commits: run_check, then push).", func(ctx context.Context, _ struct{}) (map[string]string, error) {
		out, err := svc.Push(ctx, false)
		return map[string]string{"output": out}, err
	})
	tool(s, "try_on_machine", "Try changes on one machine before main: the staged changes (or, with nothing staged, the commits not pushed yet) are committed on the testing branch its comin follows (testing-<host> by default), on top of the main branch, and pushed. comin deploys that branch on this machine only, with switch-to-configuration test, so a reboot brings back what main gave it. Needs the main branch checked out and up to date with the remote (pull first), and a run_check that built this machine (or evaluated it, with checks.build = false). The working copy then stays on the testing branch: commit and push there to try more, and watch the machine with deployments. To promote: switch_branch to the main branch, merge_branch the testing branch, run_check, commit, push. To stop: end_trial. Refused when the testing branch holds an earlier trial the main branch does not have, unless replace.", func(ctx context.Context, in struct {
		Host    string `json:"host"`
		Message string `json:"message,omitempty" jsonschema:"the commit message for the staged changes"`
		Replace bool   `json:"replace,omitempty" jsonschema:"drop an earlier trial on the testing branch that the main branch does not have"`
	}) (core.Trial, error) {
		return svc.TryOn(ctx, in.Host, in.Message, in.Replace)
	})
	tool(s, "end_trial", "Send a machine back to its main branch after a trial: its testing branch on the remote is pointed at the main branch's last commit, which comin then deploys there. A working copy on the testing branch goes back to the main branch (a push from the testing branch would put the trial back); the trial's commits stay on the local testing branch. When its uncommitted changes conflict with the main branch, it stays and the reply says so.", func(ctx context.Context, in struct {
		Host string `json:"host"`
	}) (map[string]string, error) {
		msg, err := svc.EndTrial(ctx, in.Host)
		return map[string]string{"message": msg}, err
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
	tool(s, "fleet_insights", "Health and consistency findings for the fleet (evaluation errors, firewall, SSH hardening, garbage collection, outdated flake inputs, packages missing on one machine, failed, lagging or silent deployments, and the deployed branch's last commit when it has not passed a check: fix.check is that commit, for run_check), each with a suggested fix. noSshKey lists machines where no user has an SSH key: do not apply an SSH fix there before adding one, or nobody can log in.", func(ctx context.Context, _ struct{}) (map[string][]core.Insight, error) {
		m, err := working(ctx)
		if err != nil {
			return nil, err
		}
		ov, _ := svc.Deploys.Overview(ctx, m, time.Now()) // deployment findings are extra
		return map[string][]core.Insight{"insights": core.Insights(m, ov.Machines, svc.Remote(ctx), svc.TipCheck(ctx, ov.Branch), time.Now())}, nil
	})
	tool(s, "deployments", "What each machine runs, as comin reports it after deploying: the last commit it deployed (or built and found unchanged) and when, a failure with comin's error, and how many commits it is behind the branch its comin follows (the main branch set in services.comin.remotes, or its testing branch, testing-<host> by default, when that is on top). States: current, behind, failed, unknown (its commit is not in this working copy: pull first), never (has not reported a deployment: the token or the url in modules/common.nix is missing, unless lastSeen shows its heartbeats). silent, whatever the state: no report or heartbeat for more than silentAfter hours (0: never flagged; 168 and behind for machines without heartbeats). system is what it last said it runs, boots and boots next, and whether a reboot is pending; expected is what its commit builds, as comin evaluates it, and drift is manual (it runs a system no deployment made: nixos-rebuild by hand or a rollback) or unfinished (a switch that stopped or failed partway). branch is the main branch most machines follow and commits are its newest. Each machine also names its main and testing branch (try_on_machine pushes to the latest). culprits, for a machine whose deployment failed on its main branch, are the commits since the last one it deployed, newest first: revert_commit with rev the newest culprit and since the machine's commit stages their inverse, which rolls it back. A push is a deploy: call this after pushing to watch the machines catch up.", func(ctx context.Context, _ struct{}) (core.DeployOverview, error) {
		m, _, _ := svc.Jobs.Model(ctx, "working") // the machines and their comin branches, when an evaluation is at hand
		return svc.Deploys.Overview(ctx, m, time.Now())
	})
	tool(s, "machine_dependencies", "What one machine has installed, as it reported it: the closure of the system it runs, every store path with its version and size and what it references (needs the report module from the starter fleet that sends closures). Without name: the state and totals. state is ready, none (the machine does not report), shared (it reports with the shared fleet token, which cannot send closures: give it its own), old (its report module sends no closures: copy report-deployment.sh and report-deployments.nix from the starter fleet), waiting (asked; it arrives with its next report, within 15 minutes), refused or error (message says why). current is what it runs now; system, what the closure is of. With name: up to 25 entries whose name contains it, exact names first, each with uses and usedBy (at most 100 each) and why, a shortest chain from the system to it; more counts the matches left out.", func(ctx context.Context, in struct {
		Host string `json:"host"`
		Name string `json:"name,omitempty"`
	}) (map[string]any, error) {
		md, err := svc.Deploys.MachineDependencies(in.Host)
		if err != nil {
			return nil, err
		}
		out := map[string]any{"host": md.Host, "state": md.State, "message": md.Message, "system": md.System, "current": md.Current, "at": md.At, "size": md.Size, "count": len(md.Nodes)}
		if in.Name != "" {
			out["matches"], out["more"] = md.Lookup(in.Name, 25)
		}
		return out, nil
	})
	tool(s, "fleet_dependencies", "Which dependencies are installed on which machines, from the closures the machines report (see machine_dependencies): one row per name with a version (store paths without one, like unit files, are left out), with each machine's versions in hosts order (empty: not there). differs: the machines that have it disagree on its versions. Filter by a part of the name, and with differs only those rows. At most 300 rows; total counts every row that matched.", func(ctx context.Context, in struct {
		Name    string `json:"name,omitempty"`
		Differs bool   `json:"differs,omitempty"`
	}) (map[string]any, error) {
		m, _, _ := svc.Jobs.Model(ctx, "working") // the fleet's machines, when an evaluation is at hand
		f := svc.Deploys.FleetDependencies(m)
		q := strings.ToLower(in.Name)
		rows := slices.DeleteFunc(f.Rows, func(r core.DepRow) bool {
			return in.Differs && !r.Differs || !strings.Contains(strings.ToLower(r.Name), q)
		})
		return map[string]any{"hosts": f.Hosts, "rows": rows[:min(len(rows), 300)], "total": len(rows)}, nil
	})
	tool(s, "compare_option", "The effective value of NixOS options on every machine (or the given ones, where home:<name> is a standalone home and its home-manager options), with the files that set each value. Names are full option names such as services.openssh.settings.PermitRootLogin; quote segments containing dots.", func(ctx context.Context, in struct {
		Names []string `json:"names"`
		Hosts []string `json:"hosts,omitempty"`
	}) (map[string][]core.OptionValues, error) {
		res, err := ready(ctx, func() ([]core.OptionValues, *core.Job, error) { return svc.Jobs.OptionValues(ctx, in.Names, in.Hosts) })
		return map[string][]core.OptionValues{"values": res}, err
	})
	tool(s, "create_machine", "Add a machine: hosts/<name>/ with default.nix for its platform (and, for a machine installed by hand, its boot loader), hardware-configuration.nix (the pasted nixos-generate-config output, or a placeholder; none for a Proxmox LXC) and a managed.json preset for the role, with the first user when given (the server role allows SSH keys only, so give it a key). With a disk layout, also disk-config.nix for disko, and nixos-anywhere writes the hardware file when a person installs the machine from its page (this tool installs nothing). Says whether the flake picks it up by itself or which line to add, and the disko input when flake.nix lacks it.", func(ctx context.Context, in core.MachineSpec) (core.MachineCreated, error) {
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
	tool(s, "model_diff", "What the uncommitted changes do to each machine, compared with the last commit: packages, services/programs, users and ports added or removed, package and kernel versions. status is changed whenever the machine's system derivation changes; changed with none of those lists means another setting (a sysctl, a file outside /etc...). reboot: the kernel, its modules, the initrd or the kernel parameters change, which only a reboot applies. units: what switching does to each changed systemd unit if it runs (restart, reload, stop, keep: it runs the old version until restarted), or start for a new unit, started if a running target wants it. etc: the /etc files added, removed or changed. closure, only when this server has built both systems (a check builds them, unless checks.build = false): every dependency whose version or size changes (delta in bytes) and the closure size before and after.", func(ctx context.Context, _ struct{}) (map[string]any, error) {
		head, err := ready(ctx, func() (*core.Model, *core.Job, error) { return svc.Jobs.Model(ctx, "head") })
		if err != nil {
			return nil, err
		}
		work, err := working(ctx)
		if err != nil {
			return nil, err
		}
		hosts := core.DiffModels(head, work)
		svc.Jobs.AddClosures(ctx, head, work, hosts)
		return map[string]any{"hosts": hosts}, nil
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
	tool(s, "search_options", "Search the documented NixOS options of a machine (name, type, description, default, example), or with host home:<name> the home-manager options of a standalone home (homeConfigurations.<name>). A prefix such as services.openssh lists that module's options, enable first.", func(ctx context.Context, in struct {
		Host  string `json:"host" jsonschema:"machine name (nixosConfigurations attribute), or home:<name> for a standalone home"`
		Query string `json:"query"`
		Limit int    `json:"limit,omitempty"`
	}) (map[string][]core.OptionDoc, error) {
		res, err := ready(ctx, func() ([]core.OptionDoc, *core.Job, error) {
			return svc.Jobs.SearchOptions(ctx, in.Host, in.Query, in.Limit)
		})
		return map[string][]core.OptionDoc{"results": res}, err
	})

	type hostPackages struct {
		Host     string   `json:"host" jsonschema:"machine name (nixosConfigurations attribute), or * for every machine: modules/managed.json, which applies below each machine's own managed.json where its managed.nix is version 3; or home:<name> for a standalone home (homeConfigurations.<name>) that loads modules/managed-home.nix, whose file is homes/<name>/managed.json"`
		Packages []string `json:"packages" jsonschema:"nixpkgs attribute names, e.g. htop or python3Packages.requests"`
	}
	type hostPath struct {
		Host string   `json:"host" jsonschema:"machine name (nixosConfigurations attribute), or * for every machine: modules/managed.json, which applies below each machine's own managed.json where its managed.nix is version 3; or home:<name> for a standalone home (homeConfigurations.<name>) that loads modules/managed-home.nix, whose file is homes/<name>/managed.json"`
		Path []string `json:"path" jsonschema:"option path as separate parts, e.g. [\"services\", \"openssh\", \"enable\"]"`
	}
	tool(s, "add_packages", "Install nixpkgs packages on a machine, through its managed.json (hosts/<host>/managed.json).", func(ctx context.Context, in hostPackages) (core.ManagedView, error) {
		return svc.AddPackages(in.Host, in.Packages)
	})
	tool(s, "remove_packages", "Remove packages from a machine's managed.json. Packages added in hand-written Nix files must be edited there.", func(ctx context.Context, in hostPackages) (core.ManagedView, error) {
		return svc.RemovePackages(in.Host, in.Packages)
	})
	tool(s, "edit_packages", "Change a machine's packages through its managed.json, or with user a home-manager user's home.packages: add and remove attributes of managed.json's own list, and exclude package names (as host_details lists them) wherever they are added, NixOS defaults and hand-written Nix included; restore gives an excluded name back. With host *, for every machine: a name a machine excludes takes a shared package out of it, a shared exclusion never takes out what either file adds, and a user's shared home packages apply where that machine's managed.json has the user's home. host_details lists excluded ones as removedPackages, and corePackages that a working system needs.", func(ctx context.Context, in struct {
		Host string `json:"host" jsonschema:"machine name (nixosConfigurations attribute), or * for every machine: modules/managed.json, which applies below each machine's own managed.json where its managed.nix is version 3; or home:<name> for a standalone home (homeConfigurations.<name>) that loads modules/managed-home.nix, whose file is homes/<name>/managed.json"`
		core.PackageEdit
	}) (core.ManagedView, error) {
		return svc.EditPackages(in.Host, in.PackageEdit)
	})
	tool(s, "set_package_override", "Change how a package (by nixpkgs attribute) is built on a machine, for everything there that uses it: from, a flake input that is another nixpkgs (e.g. nixpkgs-unstable, added to flake.nix's inputs and locked with flake_lock) to take it from instead, arguments for .override, attributes for .overrideAttrs, a version with its source (a URL or a GitHub repository at a revision), and patches (a URL or a file in the machine's folder). An empty hash makes run_check fail and print the right one. Without override, the package is put back as nixpkgs has it. For anything else, write hosts/<host>/packages/<attribute>.nix with write_file: { pkgs, prev, lib, ... }: pkg: <package>.", func(ctx context.Context, in struct {
		Host     string                `json:"host" jsonschema:"machine name (nixosConfigurations attribute), or * for every machine: modules/managed.json, which applies below each machine's own managed.json where its managed.nix is version 3"`
		Attr     string                `json:"attr" jsonschema:"nixpkgs attribute, e.g. htop"`
		Override *core.PackageOverride `json:"override,omitempty"`
	}) (core.ManagedView, error) {
		return svc.SetPackageOverride(in.Host, in.Attr, in.Override, nil)
	})
	tool(s, "set_home_manager", "Turn home-manager on (with stateVersion for a user without it yet, e.g. the machine's release 26.05) or off for a user of a machine, through its managed.json. Then edit_packages with user, and set_option on home-manager.users.<user>.… (search_options with that prefix). Dotfiles are files in hosts/<host>/home/<user>/files/ (linked at ~/, copied to the Nix store: never a secret; see link_home_secret), and hosts/<host>/home/<user>/custom.nix is a home-manager module of your own. Needs home-manager's NixOS module in the machine's flake modules.", func(ctx context.Context, in struct {
		Host string `json:"host" jsonschema:"machine name (nixosConfigurations attribute)"`
		core.HomeEdit
	}) (core.ManagedView, error) {
		return svc.SetHome(in.Host, in.HomeEdit)
	})
	tool(s, "link_home_secret", "Link a secret the machine gets (host_details' secrets, from set_secret or Nix by hand) as a whole file in a user's home, e.g. ~/.netrc, through managed.json's home.<user>.secrets: the secret becomes the user's, and the file a link into /run/secrets, so its value never reaches the Nix store. Without secret, the link at path goes. For a file mixing text and secrets, write hosts/<host>/home/<user>/templates/<path> with write_file: it is linked at ~/<path>, with every @secret:<name>@ in it replaced by that secret on the machine. Both need managed.nix 5 (update_managed_module) and sops-nix (modules/secrets.nix).", func(ctx context.Context, in struct {
		Host string `json:"host" jsonschema:"machine name (nixosConfigurations attribute), or * for every machine whose managed.json has the user's home (modules/managed.json)"`
		core.HomeSecretEdit
	}) (core.ManagedView, error) {
		return svc.LinkHomeSecret(in.Host, in.HomeSecretEdit)
	})
	tool(s, "set_home_config", "Add an app's settings file in a user's home to managed.json's home.<user>.configs, or change how it is written, keeping its keys: for an app that rewrites its own settings (Supersonic's .config/supersonic/config.toml, Vesktop's settings.json, Claude Code's .claude/settings.json), where a file linked by home-manager would break it. On the machine, at every home-manager activation and login, mode merge writes the declared keys and keeps every other key the app wrote, and mode seed writes the whole file only when there is none. The file is rewritten only when it changes, keeps its mode (0600 once it holds a secret), and is left as it is, with a line in the activation log, when it does not parse. Set process for an app that keeps its settings in memory and writes them whole on exit, and lock for one that takes a lock folder. Then add keys with set_home_config_key. With remove, the file is no longer written; it stays on the machine. Logins an app keeps in its own databases or rotating tokens stay in the app. Needs managed.nix 6 (update_managed_module; host_details' managedVersion). The result is the managed file, which names secrets and never holds a value.", func(ctx context.Context, in struct {
		Host string `json:"host" jsonschema:"machine name (nixosConfigurations attribute), * for every machine whose managed.json has the user's home (modules/managed.json; the machine's own config at a path wins), or home:<name> for a standalone home, without user and without secrets"`
		core.HomeConfigEdit
	}) (core.ManagedView, error) {
		return svc.SetHomeConfig(in.Host, in.HomeConfigEdit)
	})
	type configKeyIn struct {
		Host string   `json:"host" jsonschema:"machine name (nixosConfigurations attribute), * for every machine whose managed.json has the user's home (modules/managed.json; the machine's own config at a path wins), or home:<name> for a standalone home, without user and without secrets"`
		User string   `json:"user,omitempty" jsonschema:"a home-manager user of the machine; none for a standalone home"`
		Path string   `json:"path" jsonschema:"the config's file in ~, added with set_home_config"`
		Key  []string `json:"key" jsonschema:"the key's path in the file, part by part: [\"Application\", \"EnableSystemTray\"]; [\"Servers[ID=8f2c…]\", \"Hostname\"] is the key Hostname of the item of the list Servers whose ID is 8f2c…, added when missing; an INI key is [section, key], a properties key is one part, dots included"`
	}
	tool(s, "set_home_config_key", "Set one key of an app config in a user's home (set_home_config) to a value, or to one of the machine's secrets (host_details' secrets, from set_secret). A value is copied to the Nix store, which every user of the machine can read: put passwords and tokens in secret. A secret's value is read on the machine, as the user, from /run/secrets, and written into the file there: managed.json, the Nix store and this tool's result hold its name, never its value. A list value replaces the app's list whole; to set one item of a list, name it Name[Field=Value] in key.", func(ctx context.Context, in struct {
		configKeyIn
		Value  any    `json:"value,omitempty" jsonschema:"the value: boolean, number, string, list or object"`
		Secret string `json:"secret,omitempty" jsonschema:"instead of value, a secret of the machine"`
	}) (core.ManagedView, error) {
		return svc.EditHomeConfigKey(in.Host, core.HomeConfigKeyEdit{User: in.User, Path: in.Path, Key: in.Key, Value: in.Value, Secret: in.Secret})
	})
	tool(s, "remove_home_config_key", "Take a key (a value or a secret) out of an app config in a user's home, so it is no longer written. The file on the machine keeps what it has; the app's own value stays. The result names secrets and never holds a value.", func(ctx context.Context, in configKeyIn) (core.ManagedView, error) {
		return svc.EditHomeConfigKey(in.Host, core.HomeConfigKeyEdit{User: in.User, Path: in.Path, Key: in.Key, Unset: true})
	})
	tool(s, "set_keyring_entry", "Store one of the machine's secrets in a user's keyring (Secret Service: GNOME Keyring) once their graphical session starts, for an app that reads its password there, such as Supersonic (go-keyring: attributes service=supersonic and username=<the server's ID>, in the login collection). It goes in managed.json's home.<user>.keyring; an entry with the same attributes (or those in was) is replaced. On the machine a user service stores it in the login collection, else the default one, only when it is missing or different, and never logs it. Never use it for a key an app made for itself (Brave Safe Storage, an app's store key): a new value breaks what that key encrypts. Needs managed.nix 6. The result names the secret and never holds its value.", func(ctx context.Context, in struct {
		Host string `json:"host" jsonschema:"machine name (nixosConfigurations attribute), or * for every machine whose managed.json has the user's home"`
		core.KeyringEdit
	}) (core.ManagedView, error) {
		in.Remove = false
		return svc.EditKeyring(in.Host, in.KeyringEdit)
	})
	tool(s, "remove_keyring_entry", "Take the keyring entry with these attributes out of a user's home in managed.json. The item already stored in the machine's keyring stays there. The result never holds a secret's value.", func(ctx context.Context, in struct {
		Host       string            `json:"host" jsonschema:"machine name (nixosConfigurations attribute), or * for modules/managed.json"`
		User       string            `json:"user" jsonschema:"a home-manager user of the machine"`
		Attributes map[string]string `json:"attributes" jsonschema:"the entry's attributes"`
	}) (core.ManagedView, error) {
		return svc.EditKeyring(in.Host, core.KeyringEdit{User: in.User, KeyringEntry: core.KeyringEntry{Attributes: in.Attributes}, Remove: true})
	})
	tool(s, "copy_to_machines", "Copy, from one machine to others, how a package is changed (packageOverride) or a user's home (home), as that machine's managed.json and folder have it. A machine that already has another value for something copied is refused whole and listed with its conflicts; nothing is overwritten. Home package lists are joined; home.stateVersion is not copied, so a machine where the user has no home yet also needs set_home_manager with its stateVersion.", func(ctx context.Context, in struct {
		Host string `json:"host" jsonschema:"the machine to copy from"`
		core.CopyRequest
	}) (copyOut, error) {
		r, err := svc.Copy(in.Host, in.CopyRequest)
		return copyOut{r}, err
	})
	tool(s, "update_managed_module", "Replace modules/managed.nix with this server's version, and write modules/managed-configs.py next to it, which it runs on the machines. host_details' managedVersion 6 is needed for set_home_config and set_keyring_entry, 5 for link_home_secret and home templates, 4 for an override's from, 3 for host * (modules/managed.json), and 2 for removed packages, overrides, home-manager and the files next to managed.json. With home, write modules/managed-home.nix instead, which applies a standalone home's homes/<name>/managed.json (host home:<name>; packages, removed packages and settings, plus custom.nix and files/<path> linked at ~/<path> next to it) once the home's modules in flake.nix have (import ./modules/managed-home.nix ./homes/<name>/managed.json), or (./homes + \"/<name>/managed.json\") for a name a Nix path cannot hold such as pablo@laptop; fleet_overview's homes show managedFile once it does. Pass base_hash from read_file, empty when the file does not exist.", func(ctx context.Context, in struct {
		BaseHash string `json:"base_hash,omitempty"`
		Home     bool   `json:"home,omitempty" jsonschema:"write modules/managed-home.nix, for standalone homes"`
	}) (map[string]string, error) {
		h, err := svc.UpdateManagedModule(in.BaseHash, in.Home)
		return map[string]string{"hash": h}, err
	})
	tool(s, "set_option", "Set a NixOS option on a machine through its managed.json, e.g. path [\"services\",\"openssh\",\"enable\"] with value true. Use search_options to find options and their types.", func(ctx context.Context, in struct {
		Host  string   `json:"host" jsonschema:"machine name (nixosConfigurations attribute), or * for every machine: modules/managed.json, which applies below each machine's own managed.json where its managed.nix is version 3; or home:<name> for a standalone home (homeConfigurations.<name>) that loads modules/managed-home.nix, whose file is homes/<name>/managed.json"`
		Path  []string `json:"path"`
		Value any      `json:"value" jsonschema:"the JSON value: boolean, number, string, list or object"`
	}) (core.ManagedView, error) {
		return svc.SetOption(in.Host, in.Path, in.Value, nil)
	})
	tool(s, "unset_option", "Remove an option setting from a machine's managed.json.", func(ctx context.Context, in hostPath) (core.ManagedView, error) {
		return svc.UnsetOption(in.Host, in.Path, nil)
	})

	if svc.Secrets != nil {
		type secretIn struct {
			Host string `json:"host,omitempty" jsonschema:"the machine that alone gets it; empty for every machine with a key"`
			Name string `json:"name" jsonschema:"its name: the file /run/secrets/<name> on the machine"`
		}
		tool(s, "list_secrets", "The fleet's secrets, kept with sops-nix: this server's age key, each machine's approved key (from its SSH host key; a machine without one gets no secrets; a person approves keys in Settings → Secrets, and proposed lists keys in secrets/hosts/ waiting for that), and the names in secrets/common.json (every machine with a key) and secrets/hosts/<machine>.json, never a value. stale: re-encrypting is needed (Settings → Secrets).", func(ctx context.Context, _ struct{}) (core.SecretsStatus, error) {
			return svc.Secrets.Status(ctx)
		})
		tool(s, "set_secret", "Add or replace a secret, encrypted before it is written: for every machine with a key (no host) or one machine. It is a change in the working copy, like any other: run_check, commit and push deploy it. Values are never shown back over MCP.", func(ctx context.Context, in struct {
			secretIn
			Value string `json:"value" jsonschema:"the value, text up to 64 KiB"`
		}) (done, error) {
			return done{true}, svc.Secrets.Set(ctx, in.Host, in.Name, in.Value)
		})
		tool(s, "delete_secret", "Remove a secret (for every machine with no host, or from one machine's file).", func(ctx context.Context, in secretIn) (done, error) {
			return done{true}, svc.Secrets.Delete(ctx, in.Host, in.Name)
		})
	}

	tool(s, "list_targets", "List the NixOS, home-manager and nix-darwin configurations in the staged flake, with the attribute to pass to run_check for an extra dry build.", func(ctx context.Context, _ struct{}) (map[string][]core.Target, error) {
		t, err := svc.Jobs.Targets(ctx)
		return map[string][]core.Target{"targets": t}, err
	})
	tool(s, "run_check", "Validate the staged changes: nix flake check, then a real build of every machine whose system they change compared with the newest pushed commit that passed a check (every machine when none has), then a dry build of each extra target. When the server sets checks.build = false, the changed machines get a dry build only and are listed in summary.evaluatedOnly instead of built: each builds itself when comin deploys, and a wrong fixed-output hash only fails there. Commit and push need it to pass. With commit, check that commit instead, compared with the newest commit before it that passed a check (for a commit pushed without one). Waits for the result.", func(ctx context.Context, in struct {
		Targets []string `json:"targets,omitempty" jsonschema:"extra attributes from list_targets to dry-build (evaluate only); the changed machines are always built, or dry-built with checks.build = false"`
		Commit  string   `json:"commit,omitempty" jsonschema:"a full commit SHA to check instead of the staged changes; takes no targets"`
	}) (jobOut, error) {
		j, err := svc.Jobs.Submit(ctx, core.JobRequest{Kind: "check", Targets: in.Targets, Commit: in.Commit})
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
	return s.Server
}

// Handler serves MCP over Streamable HTTP, with the tools of the request
// token's scope. Callers must check the token first (mcpauth.RequireBearerToken);
// DNS-rebinding protection is off because a reverse proxy (e.g. tailscale
// serve) forwards from localhost and the bearer token already blocks browsers.
func Handler(svc *core.Service, version string) http.Handler {
	servers := map[string]*mcp.Server{}
	for _, sc := range []auth.Scope{auth.ScopeRead, auth.ScopeEdit, auth.ScopeFull} {
		servers[string(sc)] = New(svc, version, sc)
	}
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		if ti := mcpauth.TokenInfoFromContext(r.Context()); ti != nil && len(ti.Scopes) == 1 {
			return servers[ti.Scopes[0]]
		}
		return nil // no token: refused
	}, &mcp.StreamableHTTPOptions{DisableLocalhostProtection: true})
}
