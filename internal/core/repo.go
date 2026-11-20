// Package core holds the logic shared by the web UI and the MCP server: the git
// working copy, file editing, and the Nix jobs.
package core

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalid       = errors.New("invalid input")
	ErrNotFound      = errors.New("not found")
	ErrExists        = errors.New("already exists")
	ErrConflict      = errors.New("conflict")
	ErrNothingStaged = errors.New("nothing is staged")
	ErrNotChecked    = errors.New("not checked")
	ErrSyntax        = errors.New("syntax error")
	ErrForbidden     = errors.New("forbidden")
)

// Error is a user-facing message tagged with one of the Err* kinds above.
type Error struct {
	Kind error
	Msg  string
}

func (e *Error) Error() string { return e.Msg }
func (e *Error) Unwrap() error { return e.Kind }

func errorf(kind error, format string, a ...any) error {
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, a...)}
}

// Repo is the server's single working copy of the flake repository.
// Every mutation holds mu, so the UI and agents never interleave git operations.
type Repo struct {
	Dir         string
	AuthorName  string
	AuthorEmail string
	SSHDir      string // the app's deploy key and known_hosts (forgejo.go); "" uses ssh's defaults

	mu       sync.Mutex
	root     *os.Root
	gitFiles []string // the index and info/exclude: git add -A reads them too
	wt       struct {
		mu   sync.Mutex
		sum  [32]byte
		tree string // "" while nothing is cached
	}
}

// OpenRepo opens the working copy in dir. A missing or empty dir is cloned from
// remote, or initialised empty when there is no remote yet. In an existing repo
// a non-empty remote becomes (or replaces) origin.
func OpenRepo(dir, remote, name, email string) (*Repo, error) {
	return OpenRepoSSH(dir, remote, name, email, "")
}

// OpenRepoSSH is OpenRepo with git's SSH using the deploy key in sshDir when it
// exists. A clone that fails (the key is not on Forgejo yet, or it is down)
// starts an empty working copy with origin set, so Settings can connect it.
func OpenRepoSSH(dir, remote, name, email, sshDir string) (*Repo, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if sshDir != "" { // git runs ssh from the working copy, so a relative path would break
		if sshDir, err = filepath.Abs(sshDir); err != nil {
			return nil, err
		}
	}
	r := &Repo{Dir: dir, AuthorName: name, AuthorEmail: email, SSHDir: sshDir}
	ctx := context.Background()
	if _, err := os.Stat(filepath.Join(dir, ".git")); errors.Is(err, fs.ErrNotExist) {
		cloned := false
		if remote != "" {
			if _, err = r.git(ctx, "clone", "-q", "--", remote, dir); err != nil {
				slog.Warn("cannot clone the remote; starting an empty working copy (connect it in Settings)", "remote", redact(remote), "err", strings.ReplaceAll(err.Error(), remote, redact(remote)))
			}
			cloned = err == nil
		}
		if !cloned {
			_, err = r.git(ctx, "init", "-q", "-b", "main", dir)
		}
		if err == nil && !cloned && remote != "" {
			_, err = r.git(ctx, "remote", "add", "origin", remote)
		}
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else if remote != "" {
		cur, err := r.git(ctx, "remote", "get-url", "origin")
		if err != nil {
			_, err = r.git(ctx, "remote", "add", "origin", remote)
		} else if strings.TrimSpace(cur) != remote {
			_, err = r.git(ctx, "remote", "set-url", "origin", remote)
		}
		if err != nil {
			return nil, err
		}
	}
	if r.root, err = os.OpenRoot(dir); err != nil {
		return nil, err
	}
	out, err := r.git(ctx, "rev-parse", "--git-path", "index", "--git-path", "info/exclude")
	if err != nil {
		return nil, err
	}
	for _, p := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		r.gitFiles = append(r.gitFiles, p)
	}
	return r, nil
}

// GitError is a failed git command; Msg is git's own explanation.
type GitError struct {
	Args []string
	Code int
	Msg  string
}

func (e *GitError) Error() string {
	return fmt.Sprintf("git %s failed: %s", e.Args[0], e.Msg)
}

func (r *Repo) git(ctx context.Context, args ...string) (string, error) {
	out, _, err := r.gitRun(ctx, nil, nil, args...)
	return out, err
}

// gitRun runs git in the working copy and returns stdout, stderr and an error
// for a non-zero exit. The commit identity is always the configured human.
func (r *Repo) gitRun(ctx context.Context, env []string, stdin []byte, args ...string) (string, string, error) {
	full := append([]string{"-c", "user.name=" + r.AuthorName, "-c", "user.email=" + r.AuthorEmail, "-c", "core.quotePath=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = r.Dir
	// Literal pathspecs: a path like "*.nix" names that file, never every .nix file.
	cmd.Env = append(append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C", "GIT_LITERAL_PATHSPECS=1"), env...)
	if os.Getenv("GIT_SSH_COMMAND") == "" { // the user's own command wins
		cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND="+sshCommand(r.SSHDir))
	}
	cmd.WaitDelay = 5 * time.Second // a timed-out ssh child must not keep the pipes open forever
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		code := -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		err = &GitError{Args: args, Code: code, Msg: msg}
	}
	return stdout.String(), stderr.String(), err
}

// cleanPath validates a user-supplied repo-relative path.
func cleanPath(p string) (string, error) {
	if !filepath.IsLocal(p) {
		return "", errorf(ErrInvalid, "invalid path %q: must be relative and stay inside the repository", p)
	}
	p = filepath.ToSlash(filepath.Clean(p))
	if p == "." || p == ".git" || strings.HasPrefix(p, ".git/") {
		return "", errorf(ErrInvalid, "invalid path %q", p)
	}
	return p, nil
}

func cleanPaths(ps []string) ([]string, error) {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		c, err := cleanPath(p)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func hashOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Entry is one file or folder in the working copy.
type Entry struct {
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
}

// Tree lists every file and folder in the working copy except .git, sorted.
func (r *Repo) Tree() ([]Entry, error) {
	out := []Entry{}
	err := fs.WalkDir(r.root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		out = append(out, Entry{Path: p, Dir: d.IsDir()})
		return nil
	})
	return out, err
}

func textOrError(p string, b []byte) (string, error) {
	if !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
		return "", errorf(ErrInvalid, "%s is a binary file and cannot be edited here", p)
	}
	return string(b), nil
}

// Read returns a text file and the hash a later Write must present.
func (r *Repo) Read(p string) (string, string, error) {
	p, err := cleanPath(p)
	if err != nil {
		return "", "", err
	}
	b, err := r.root.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return "", "", errorf(ErrNotFound, "%s does not exist", p)
	}
	if err != nil {
		return "", "", r.folderOr(p, err)
	}
	s, err := textOrError(p, b)
	if err != nil {
		return "", "", err
	}
	return s, hashOf(b), nil
}

// Write saves a file. baseHash is the hash from Read; "" creates a new file.
// A mismatch means someone else changed the file, so nothing is overwritten.
func (r *Repo) Write(p, content, baseHash string) (string, error) {
	p, err := cleanPath(p)
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkBase(p, baseHash); err != nil {
		return "", err
	}
	if err := r.writeFile(p, []byte(content)); err != nil {
		return "", err
	}
	return hashOf([]byte(content)), nil
}

// checkBase makes sure p on disk is still the version the caller saw: hash
// baseHash, or absent when baseHash is "". Needs r.mu held.
func (r *Repo) checkBase(p, baseHash string) error {
	cur, err := r.root.ReadFile(p)
	switch {
	case err == nil && baseHash == "":
		return errorf(ErrExists, "%s already exists", p)
	case err == nil && hashOf(cur) != baseHash:
		return errorf(ErrConflict, "%s changed since it was loaded", p)
	case errors.Is(err, fs.ErrNotExist) && baseHash != "":
		return errorf(ErrConflict, "%s was deleted since it was loaded", p)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return r.folderOr(p, err)
	}
	return nil
}

// folderOr explains a failed file operation on a folder, else returns err.
func (r *Repo) folderOr(p string, err error) error {
	if fi, serr := r.root.Stat(p); serr == nil && fi.IsDir() {
		return errorf(ErrInvalid, "%s is a folder", p)
	}
	return err
}

// writeFile replaces p atomically, creating parent folders and keeping the mode.
func (r *Repo) writeFile(p string, data []byte) error {
	mode := fs.FileMode(0o644)
	if fi, err := r.root.Stat(p); err == nil {
		if fi.IsDir() {
			return errorf(ErrInvalid, "%s is a folder", p)
		}
		mode = fi.Mode().Perm()
	}
	if dir := path.Dir(p); dir != "." {
		if err := r.root.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := path.Join(path.Dir(p), fmt.Sprintf(".%s.%s.tmp", path.Base(p), rand.Text()[:8]))
	if err := r.root.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := r.root.Rename(tmp, p); err != nil {
		r.root.Remove(tmp)
		return err
	}
	return nil
}

// Mkdir creates a folder (and its parents).
func (r *Repo) Mkdir(p string) error {
	p, err := cleanPath(p)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.root.Lstat(p); err == nil {
		return errorf(ErrExists, "%s already exists", p)
	}
	return r.root.MkdirAll(p, 0o755)
}

// Move renames a file or folder.
func (r *Repo) Move(from, to string) error {
	from, err := cleanPath(from)
	if err != nil {
		return err
	}
	if to, err = cleanPath(to); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.root.Lstat(from); err != nil {
		return errorf(ErrNotFound, "%s does not exist", from)
	}
	if _, err := r.root.Lstat(to); err == nil {
		return errorf(ErrExists, "%s already exists", to)
	}
	if strings.HasPrefix(to+"/", from+"/") {
		return errorf(ErrInvalid, "cannot move %s into itself", from)
	}
	if dir := path.Dir(to); dir != "." {
		if err := r.root.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return r.root.Rename(from, to)
}

// Delete removes a file or a folder with everything in it.
func (r *Repo) Delete(p string) error {
	p, err := cleanPath(p)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.root.Lstat(p); err != nil {
		return errorf(ErrNotFound, "%s does not exist", p)
	}
	return r.root.RemoveAll(p)
}

// FileStatus is one changed path. Staged and Unstaged use git's letters
// (M, A, D, R, T, U, and ? for untracked); "" means no change on that side.
type FileStatus struct {
	Path     string `json:"path"`
	From     string `json:"from,omitempty"`
	Staged   string `json:"staged"`
	Unstaged string `json:"unstaged"`
}

type Status struct {
	Branch        string       `json:"branch"`
	Upstream      string       `json:"upstream"`
	Ahead         int          `json:"ahead"`
	Behind        int          `json:"behind"`
	Files         []FileStatus `json:"files"`
	StagedTree    string       `json:"staged_tree"`
	StagedChecked bool         `json:"staged_checked"`
	StagedCheck   *CheckPass   `json:"staged_check,omitempty"` // the passed check, with the machines it built
}

func (r *Repo) Status(ctx context.Context) (Status, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status(ctx)
}

func (r *Repo) status(ctx context.Context) (Status, error) {
	st := Status{Files: []FileStatus{}}
	out, err := r.git(ctx, "status", "--porcelain=v2", "-z", "--branch", "--untracked-files=all")
	if err != nil {
		return st, err
	}
	letter := func(c byte) string {
		if c == '.' {
			return ""
		}
		return string(c)
	}
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		switch {
		case strings.HasPrefix(f, "# branch.head "):
			st.Branch = strings.TrimPrefix(f, "# branch.head ")
		case strings.HasPrefix(f, "# branch.upstream "):
			st.Upstream = strings.TrimPrefix(f, "# branch.upstream ")
		case strings.HasPrefix(f, "# branch.ab "):
			fmt.Sscanf(strings.TrimPrefix(f, "# branch.ab "), "+%d -%d", &st.Ahead, &st.Behind)
		case strings.HasPrefix(f, "1 "):
			p := strings.SplitN(f, " ", 9)
			st.Files = append(st.Files, FileStatus{Path: p[8], Staged: letter(p[1][0]), Unstaged: letter(p[1][1])})
		case strings.HasPrefix(f, "2 "):
			p := strings.SplitN(f, " ", 10)
			fs := FileStatus{Path: p[9], Staged: letter(p[1][0]), Unstaged: letter(p[1][1])}
			if i+1 < len(fields) {
				fs.From = fields[i+1]
				i++
			}
			st.Files = append(st.Files, fs)
		case strings.HasPrefix(f, "u "):
			p := strings.SplitN(f, " ", 11)
			st.Files = append(st.Files, FileStatus{Path: p[10], Staged: "U", Unstaged: "U"})
		case strings.HasPrefix(f, "? "):
			st.Files = append(st.Files, FileStatus{Path: f[2:], Unstaged: "?"})
		}
	}
	st.StagedTree, err = r.stagedTree(ctx)
	return st, err
}

// stagedTree is the tree object of the index: the exact content a commit would record.
func (r *Repo) stagedTree(ctx context.Context) (string, error) {
	tree, err := r.git(ctx, "write-tree")
	return strings.TrimSpace(tree), err
}

// export writes the files of tree into dir, using a private index.
func (r *Repo) export(ctx context.Context, tree, dir string) error {
	env := []string{"GIT_INDEX_FILE=" + dir + ".index"}
	defer os.Remove(dir + ".index")
	if _, _, err := r.gitRun(ctx, env, nil, "read-tree", tree); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	_, _, err := r.gitRun(ctx, env, nil, "checkout-index", "-a", "--prefix="+dir+"/")
	return err
}

// lockFiles are the files nix flake lock and update read besides the tracked ones.
var lockFiles = []string{"flake.nix", "flake.lock"}

// lockSource is the tree that lock and update run on: the tracked files as
// they are in the working copy, plus flake.nix and flake.lock even untracked
// (a new repository locks before its first commit). Untracked and ignored
// files stay out. It also returns the two files' hashes ("" when absent), for
// putLock.
func (r *Repo) lockSource(ctx context.Context) (string, map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	base := map[string]string{}
	var present []string
	for _, f := range lockFiles {
		b, err := r.root.ReadFile(f)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			base[f] = ""
		case err != nil:
			return "", nil, r.folderOr(f, err)
		default:
			base[f] = hashOf(b)
			present = append(present, f)
		}
	}
	adds := [][]string{{"-u"}} // no pathspec: "." fails while nothing is tracked
	if len(present) > 0 {
		adds = append(adds, append([]string{"-f", "--"}, present...))
	}
	tree, err := r.indexTree(ctx, adds...)
	return tree, base, err
}

// putLock writes the flake.lock Nix made into the working copy, unless
// flake.nix or flake.lock changed since lockSource: the lock was made from the
// old ones, and a save made meanwhile must not be overwritten.
func (r *Repo) putLock(base map[string]string, lock []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, f := range lockFiles {
		if err := r.checkBase(f, base[f]); errors.Is(err, ErrConflict) || errors.Is(err, ErrExists) {
			return errorf(ErrConflict, "%s changed while Nix was locking, so the new flake.lock was not written: run it again", f)
		} else if err != nil {
			return err
		}
	}
	if cur, err := r.root.ReadFile("flake.lock"); err == nil && bytes.Equal(cur, lock) {
		return nil
	}
	return r.writeFile("flake.lock", lock)
}

func (r *Repo) untracked(ctx context.Context) (map[string]bool, error) {
	out, err := r.git(ctx, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	m := map[string]bool{}
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			m[p] = true
		}
	}
	return m, nil
}

// Diff returns a unified diff: index vs HEAD when staged, else working tree vs
// index with untracked files shown as new. p limits it to one path.
func (r *Repo) Diff(ctx context.Context, p string, staged bool) (string, error) {
	var err error
	if p != "" {
		if p, err = cleanPath(p); err != nil {
			return "", err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	args := []string{"diff", "--no-color", "--no-ext-diff", "-M"}
	if staged {
		args = append(args, "--cached")
	}
	if p != "" {
		args = append(args, "--", p)
	}
	out, err := r.git(ctx, args...)
	if err != nil || staged {
		return out, err
	}
	untracked, err := r.untracked(ctx)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(out)
	for _, u := range slices.Sorted(maps.Keys(untracked)) {
		if p != "" && u != p && !strings.HasPrefix(u, p+"/") {
			continue
		}
		d, _, err := r.gitRun(ctx, nil, nil, "diff", "--no-color", "--no-index", "--", os.DevNull, u)
		var ge *GitError
		if err != nil && !(errors.As(err, &ge) && ge.Code == 1) { // 1 = "files differ"
			return "", err
		}
		b.WriteString(d)
	}
	return b.String(), nil
}

// Stage adds paths (all changes when empty) to the index, deletions included.
func (r *Repo) Stage(ctx context.Context, paths []string) error {
	paths, err := cleanPaths(paths)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err = r.git(ctx, append([]string{"add", "-A", "--"}, paths...)...)
	return err
}

// Unstage removes paths (everything when empty) from the index, keeping the files.
func (r *Repo) Unstage(ctx context.Context, paths []string) error {
	paths, err := cleanPaths(paths)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.hasHead(ctx) {
		_, err = r.git(ctx, append([]string{"rm", "-r", "-q", "--cached", "--ignore-unmatch", "--"}, paths...)...)
		return err
	}
	_, err = r.git(ctx, append([]string{"reset", "-q", "--"}, paths...)...)
	return err
}

// Discard drops unstaged changes to paths; untracked files are deleted.
func (r *Repo) Discard(ctx context.Context, paths []string) error {
	paths, err := cleanPaths(paths)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return errorf(ErrInvalid, "name the paths to discard")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	untracked, err := r.untracked(ctx)
	if err != nil {
		return err
	}
	var tracked []string
	for _, p := range paths {
		if untracked[p] {
			if err := r.root.Remove(p); err != nil {
				return err
			}
		} else {
			tracked = append(tracked, p)
		}
	}
	if len(tracked) > 0 {
		_, err = r.git(ctx, append([]string{"restore", "--worktree", "--"}, tracked...)...)
	}
	return err
}

func (r *Repo) hasHead(ctx context.Context) bool {
	_, err := r.git(ctx, "rev-parse", "--verify", "-q", "HEAD")
	return err == nil
}

// Commit records the index with the configured human as author and committer.
func (r *Repo) Commit(ctx context.Context, message string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.commit(ctx, message)
}

// commit needs r.mu held.
func (r *Repo) commit(ctx context.Context, message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", errorf(ErrInvalid, "the commit message is empty")
	}
	if r.AuthorName == "" || r.AuthorEmail == "" {
		return "", errorf(ErrInvalid, "no commit identity configured")
	}
	if _, err := r.git(ctx, "diff", "--cached", "--quiet"); err == nil {
		return "", errorf(ErrNothingStaged, "nothing is staged to commit")
	}
	args := []string{"commit", "-q", "-F", "-"}
	if c, ok := callerOf(ctx); ok && c.Via != "" {
		trailers, _, err := r.gitRun(ctx, nil, []byte(message), "interpret-trailers", "--parse")
		if err != nil {
			return "", err
		}
		for line := range strings.Lines(strings.ToLower(trailers)) {
			if strings.HasPrefix(line, "via:") {
				return "", errorf(ErrInvalid, "the Via trailer is added by nixos-manager; remove it from the message")
			}
		}
		args = append(args, "--trailer", "Via: "+c.Via)
	}
	if _, _, err := r.gitRun(ctx, nil, []byte(message), args...); err != nil {
		return "", err
	}
	sha, err := r.git(ctx, "rev-parse", "HEAD")
	return strings.TrimSpace(sha), err
}

// resolve turns a user-supplied revision into a commit SHA.
func (r *Repo) resolve(ctx context.Context, rev string) (string, error) {
	if rev == "" || strings.HasPrefix(rev, "-") || strings.ContainsAny(rev, " \t\n\x00") {
		return "", errorf(ErrInvalid, "invalid revision %q", rev)
	}
	out, err := r.git(ctx, "rev-parse", "--verify", "-q", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", errorf(ErrNotFound, "unknown revision %q", rev)
	}
	return strings.TrimSpace(out), nil
}

type Commit struct {
	SHA     string `json:"sha"`
	Short   string `json:"short"`
	Author  string `json:"author"`
	Email   string `json:"email"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
}

// Log lists commits newest first, for the whole repo or (following renames) one file.
func (r *Repo) Log(ctx context.Context, p, rev string, limit int) ([]Commit, error) {
	var err error
	if p != "" {
		if p, err = cleanPath(p); err != nil {
			return nil, err
		}
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	args := []string{"log", logFormat, fmt.Sprintf("-n%d", limit)}
	if rev != "" {
		sha, err := r.resolve(ctx, rev)
		if err != nil {
			return nil, err
		}
		args = append(args, sha)
	} else if !r.hasHead(ctx) {
		return []Commit{}, nil
	}
	if p != "" {
		args = append(args, "--follow", "--", p)
	}
	out, err := r.git(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseLog(out), nil
}

// logFormat makes git log print what parseLog reads.
const logFormat = "--format=%H%x00%h%x00%an%x00%ae%x00%aI%x00%s%x1e"

func parseLog(out string) []Commit {
	commits := []Commit{}
	for _, rec := range strings.Split(out, "\x1e") {
		f := strings.Split(strings.TrimLeft(rec, "\n"), "\x00")
		if len(f) == 6 {
			commits = append(commits, Commit{SHA: f[0], Short: f[1], Author: f[2], Email: f[3], Date: f[4], Subject: f[5]})
		}
	}
	return commits
}

type CommitInfo struct {
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Email   string `json:"email"`
	Date    string `json:"date"`
	Message string `json:"message"`
	Diff    string `json:"diff"`
}

// CommitInfo returns a commit's message and its patch against its first parent.
func (r *Repo) CommitInfo(ctx context.Context, rev string) (CommitInfo, error) {
	sha, err := r.resolve(ctx, rev)
	if err != nil {
		return CommitInfo{}, err
	}
	meta, err := r.git(ctx, "show", "-s", "--format=%H%x00%an%x00%ae%x00%aI%x00%B", sha)
	if err != nil {
		return CommitInfo{}, err
	}
	f := strings.SplitN(meta, "\x00", 5)
	if len(f) != 5 {
		return CommitInfo{}, fmt.Errorf("unexpected git show output for %s", sha)
	}
	diff, err := r.git(ctx, "show", "--format=", "--no-color", "--no-ext-diff", "-M", "-m", "--first-parent", sha)
	if err != nil {
		return CommitInfo{}, err
	}
	return CommitInfo{SHA: f[0], Author: f[1], Email: f[2], Date: f[3], Message: strings.TrimRight(f[4], "\n") + "\n", Diff: strings.TrimLeft(diff, "\n")}, nil
}

// FileAt returns a file's content as of a commit.
func (r *Repo) FileAt(ctx context.Context, rev, p string) (string, error) {
	p, err := cleanPath(p)
	if err != nil {
		return "", err
	}
	sha, err := r.resolve(ctx, rev)
	if err != nil {
		return "", err
	}
	out, err := r.git(ctx, "cat-file", "blob", sha+":"+p)
	if err != nil {
		return "", errorf(ErrNotFound, "%s did not exist in %.12s", p, sha)
	}
	return textOrError(p, []byte(out))
}

// Restore writes a file as it was at rev into the working copy (not committed).
// Like Write, baseHash is the version on disk the caller saw ("" if absent).
func (r *Repo) Restore(ctx context.Context, p, rev, baseHash string) error {
	content, err := r.FileAt(ctx, rev, p)
	if err != nil {
		return err
	}
	p, _ = cleanPath(p)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkBase(p, baseHash); err != nil {
		return err
	}
	return r.writeFile(p, []byte(content))
}

// Remote is origin's URL with any password hidden, or "" when there is none.
func (r *Repo) Remote(ctx context.Context) string {
	out, _ := r.git(ctx, "remote", "get-url", "origin")
	return redact(strings.TrimSpace(out))
}

func redact(remote string) string {
	if u, err := url.Parse(remote); err == nil {
		return u.Redacted()
	}
	return remote
}

type Branches struct {
	Current  string   `json:"current"`
	Branches []string `json:"branches"`
}

func (r *Repo) Branches(ctx context.Context) (Branches, error) {
	cur, _ := r.git(ctx, "symbolic-ref", "--short", "-q", "HEAD")
	b := Branches{Current: strings.TrimSpace(cur), Branches: []string{}}
	out, err := r.git(ctx, "for-each-ref", "--format=%(refname:short)", "refs/heads/")
	if err != nil {
		return b, err
	}
	seen := false
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l != "" {
			b.Branches = append(b.Branches, l)
			seen = seen || l == b.Current
		}
	}
	if !seen && b.Current != "" { // unborn branch in a fresh repo
		b.Branches = append(b.Branches, b.Current)
	}
	return b, nil
}

func (r *Repo) checkBranch(ctx context.Context, name string) error {
	if strings.HasPrefix(name, "-") {
		return errorf(ErrInvalid, "invalid branch name %q", name)
	}
	if _, err := r.git(ctx, "check-ref-format", "--branch", name); err != nil {
		return errorf(ErrInvalid, "invalid branch name %q", name)
	}
	return nil
}

// CreateBranch creates name from rev (HEAD when empty) and switches to it.
func (r *Repo) CreateBranch(ctx context.Context, name, from string) error {
	if err := r.checkBranch(ctx, name); err != nil {
		return err
	}
	args := []string{"switch", "-q", "-c", name}
	if from != "" {
		sha, err := r.resolve(ctx, from)
		if err != nil {
			return err
		}
		args = append(args, sha)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.git(ctx, args...); err != nil {
		return errorf(ErrConflict, "%v", err)
	}
	return nil
}

// Switch checks out a local branch; git refuses when local changes would be lost.
func (r *Repo) Switch(ctx context.Context, name string) error {
	if err := r.checkBranch(ctx, name); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.git(ctx, "switch", "-q", "--no-guess", name); err != nil {
		return errorf(ErrConflict, "%v", err)
	}
	return nil
}

// clean refuses operations that would mix with uncommitted changes to tracked files. Needs r.mu.
func (r *Repo) clean(ctx context.Context) error {
	dirty, err := r.git(ctx, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return err
	}
	if dirty != "" {
		return errorf(ErrConflict, "commit or discard your changes first")
	}
	return nil
}

// isAncestor says whether commit a is b or one of its ancestors.
func (r *Repo) isAncestor(ctx context.Context, a, b string) bool {
	_, err := r.git(ctx, "merge-base", "--is-ancestor", "--end-of-options", a, b)
	return err == nil
}

// Revert stages the inverse of a commit; the user checks and commits it like
// any change. With since, it stages the inverse of every commit after since up
// to rev, newest first, as one change: what brings rev's changes back to since.
// The commits must be on the current branch.
func (r *Repo) Revert(ctx context.Context, rev, since string) error {
	sha, err := r.resolve(ctx, rev)
	if err != nil {
		return err
	}
	from := ""
	if since != "" {
		if from, err = r.resolve(ctx, since); err != nil {
			return err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.clean(ctx); err != nil {
		return err
	}
	if !r.isAncestor(ctx, sha, "HEAD") {
		cur, _ := r.git(ctx, "symbolic-ref", "--short", "-q", "HEAD")
		return errorf(ErrConflict, "%.7s is not on %s in this working copy: pull first if it was pushed from elsewhere, or switch to the branch it is on", sha, cmp.Or(strings.TrimSpace(cur), "the current commit"))
	}
	what := fmt.Sprintf("%.12s", sha)
	if from == "" {
		if parents, _ := r.git(ctx, "rev-list", "--parents", "-n", "1", sha); len(strings.Fields(parents)) > 2 {
			return errorf(ErrInvalid, "%.12s is a merge commit; revert the commits it brought in instead", sha)
		}
	} else {
		if from == sha || !r.isAncestor(ctx, from, sha) {
			return errorf(ErrInvalid, "%.7s does not come before %.7s", from, sha)
		}
		if merges, _ := r.git(ctx, "rev-list", "--merges", "--count", from+".."+sha); strings.TrimSpace(merges) != "0" {
			return errorf(ErrInvalid, "the commits after %.7s up to %.7s include a merge; revert them one by one instead", from, sha)
		}
		what = fmt.Sprintf("the commits after %.12s up to %.12s", from, sha)
		sha = from + ".." + sha // git reverts a range newest first
	}
	if _, err := r.git(ctx, "revert", "--no-commit", sha); err != nil {
		r.git(ctx, "revert", "--abort")
		return errorf(ErrConflict, "%s cannot be reverted cleanly; later commits changed the same lines.\n%v", what, err)
	}
	_, err = r.git(ctx, "revert", "--quit") // keep the staged changes, drop git's "revert in progress"
	return err
}

// Merge stages another branch squashed into the current one and returns a suggested
// commit message. Nothing is committed, and git is never left mid-merge.
func (r *Repo) Merge(ctx context.Context, branch string) (string, error) {
	if err := r.checkBranch(ctx, branch); err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.clean(ctx); err != nil {
		return "", err
	}
	cur, err := r.git(ctx, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return "", errorf(ErrInvalid, "not on a branch")
	}
	if _, err := r.git(ctx, "rev-parse", "--verify", "-q", "refs/heads/"+branch); err != nil {
		return "", errorf(ErrNotFound, "no branch named %s", branch)
	}
	if _, err := r.git(ctx, "merge", "--squash", "refs/heads/"+branch); err != nil {
		r.git(ctx, "reset", "-q", "--merge")
		return "", errorf(ErrConflict, "%s cannot be merged cleanly; the branches changed the same lines.\n%v", branch, err)
	}
	if staged, _ := r.git(ctx, "diff", "--cached", "--name-only"); strings.TrimSpace(staged) == "" {
		return "", errorf(ErrInvalid, "%s has nothing that %s does not already have", branch, strings.TrimSpace(cur))
	}
	return fmt.Sprintf("Merge %s into %s", branch, strings.TrimSpace(cur)), nil
}

const netTimeout = 2 * time.Minute

func (r *Repo) currentBranch(ctx context.Context) (string, error) {
	cur, err := r.git(ctx, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return "", errorf(ErrInvalid, "not on a branch")
	}
	if _, err := r.git(ctx, "remote", "get-url", "origin"); err != nil {
		return "", errorf(ErrInvalid, "no remote is configured")
	}
	return strings.TrimSpace(cur), nil
}

// pushBase is the tree a push of the current branch lands on: origin's copy of
// the branch, else origin's default branch (a new testing-<host> branch is
// deployed on top of it), else "" when the remote has neither, as last
// fetched. Needs r.mu.
func (r *Repo) pushBase(ctx context.Context) (string, error) {
	trees, err := r.pushedTrees(ctx)
	if len(trees) == 0 {
		return "", err
	}
	return trees[0], err
}

// pushedTrees are the trees of what a push lands on (see pushBase) and of the
// commits before it on its first-parent line, newest first, as far back as a
// check looks for one that passed; none when nothing is pushed. Needs r.mu.
func (r *Repo) pushedTrees(ctx context.Context) ([]string, error) {
	refs := []string{"refs/remotes/origin/HEAD"}
	if cur, err := r.git(ctx, "symbolic-ref", "--short", "-q", "HEAD"); err == nil {
		refs = append([]string{"refs/remotes/origin/" + strings.TrimSpace(cur)}, refs...)
	}
	for _, ref := range refs {
		if _, err := r.git(ctx, "rev-parse", "--verify", "-q", ref); err == nil {
			out, err := r.git(ctx, "log", "--first-parent", "--format=%T", fmt.Sprintf("--max-count=%d", checkedAncestors+1), "--end-of-options", ref, "--")
			return strings.Fields(out), err
		}
	}
	return nil, nil
}

// Push sends the current branch to origin and sets it as upstream.
func (r *Repo) Push(ctx context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.push(ctx)
}

// push needs r.mu held.
func (r *Repo) push(ctx context.Context) (string, error) {
	branch, err := r.currentBranch(ctx)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, netTimeout)
	defer cancel()
	out, errOut, err := r.gitRun(ctx, nil, nil, "push", "-u", "origin", "refs/heads/"+branch)
	if err != nil {
		if strings.Contains(errOut, "rejected") {
			return "", errorf(ErrConflict, "the remote has commits you do not have yet; pull first.\n%s", strings.TrimSpace(errOut))
		}
		return "", err
	}
	return strings.TrimSpace(out + errOut), nil
}

// Pull fetches origin and brings the current branch up to date: a fast-forward
// when possible, otherwise a rebase of the local commits that is aborted (and
// leaves everything as it was) if it conflicts.
func (r *Repo) Pull(ctx context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	branch, err := r.currentBranch(ctx)
	if err != nil {
		return "", err
	}
	nctx, cancel := context.WithTimeout(ctx, netTimeout)
	defer cancel()
	if _, err := r.git(nctx, "fetch", "-q", "origin"); err != nil {
		return "", err
	}
	up := "origin/" + branch
	if _, err := r.git(ctx, "rev-parse", "--verify", "-q", "refs/remotes/"+up); err != nil {
		return "", errorf(ErrInvalid, "%s does not exist on the remote yet; push it first", branch)
	}
	if r.hasHead(ctx) && !r.related(ctx, up) {
		return "", errorf(ErrConflict, "cannot pull: %s.", unrelated)
	}
	if !r.hasHead(ctx) {
		_, err := r.git(ctx, "reset", "-q", up) // unborn local branch: adopt the remote one
		if err == nil {
			_, err = r.git(ctx, "branch", "-q", "--set-upstream-to", up)
		}
		return "Took " + up + " from the remote.", err
	}
	counts, err := r.git(ctx, "rev-list", "--left-right", "--count", "HEAD..."+up)
	if err != nil {
		return "", err
	}
	var ahead, behind int
	fmt.Sscanf(counts, "%d %d", &ahead, &behind)
	switch {
	case behind == 0:
		return "Already up to date.", nil
	case ahead == 0:
		if _, err := r.git(ctx, "merge", "-q", "--ff-only", up); err != nil {
			return "", errorf(ErrConflict, "cannot fast-forward: %v", err)
		}
		return fmt.Sprintf("Fast-forwarded %d commit(s) from %s.", behind, up), nil
	}
	if dirty, err := r.git(ctx, "status", "--porcelain", "--untracked-files=no"); err != nil {
		return "", err
	} else if dirty != "" {
		return "", errorf(ErrConflict, "the branch and the remote both have new commits; commit or discard your local changes first")
	}
	if _, err := r.git(ctx, "rebase", "-q", up); err != nil {
		r.git(ctx, "rebase", "--abort")
		return "", errorf(ErrConflict, "your %d local commit(s) conflict with the %d new remote commit(s); nothing was changed. Resolve it on a branch or in Forgejo.\n%v", ahead, behind, err)
	}
	return fmt.Sprintf("Rebased %d local commit(s) onto %d new commit(s) from %s.", ahead, behind, up), nil
}

// Seed copies the files of fsys into a brand-new, empty working copy and
// reports whether it did. Nothing is staged or committed.
func (r *Repo) Seed(fsys fs.FS) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hasHead(context.Background()) {
		return false, nil
	}
	if entries, err := fs.ReadDir(r.root.FS(), "."); err != nil || len(entries) > 1 { // only .git
		return false, err
	}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		return r.writeFile(p, b)
	})
	return err == nil, err
}

// CommitsBetween counts the commits that to has and from lacks (from..to) and
// says when the oldest of them was committed. from and to are SHAs or full refs
// chosen by the caller, never user input.
func (r *Repo) CommitsBetween(ctx context.Context, from, to string) (int, time.Time, error) {
	out, err := r.git(ctx, "log", "--format=%ct", "--end-of-options", to, "^"+from, "--")
	if err != nil {
		return 0, time.Time{}, err
	}
	lines := strings.Fields(out)
	var oldest int64
	for _, l := range lines {
		var sec int64
		if _, err := fmt.Sscan(l, &sec); err == nil && (oldest == 0 || sec < oldest) {
			oldest = sec
		}
	}
	if len(lines) == 0 {
		return 0, time.Time{}, nil
	}
	return len(lines), time.Unix(oldest, 0).UTC(), nil
}
