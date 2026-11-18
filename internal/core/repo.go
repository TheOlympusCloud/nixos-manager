// Package core holds the logic shared by the web UI and the MCP server: the git
// working copy, file editing, and the Nix jobs.
package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
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

	mu   sync.Mutex
	root *os.Root
}

// OpenRepo opens the working copy in dir. A missing or empty dir is cloned from
// remote, or initialised empty when there is no remote yet. In an existing repo
// a non-empty remote becomes (or replaces) origin.
func OpenRepo(dir, remote, name, email string) (*Repo, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	r := &Repo{Dir: dir, AuthorName: name, AuthorEmail: email}
	ctx := context.Background()
	if _, err := os.Stat(filepath.Join(dir, ".git")); errors.Is(err, fs.ErrNotExist) {
		if remote != "" {
			_, err = r.git(ctx, "clone", "-q", "--", remote, dir)
		} else {
			_, err = r.git(ctx, "init", "-q", "-b", "main", dir)
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
	if os.Getenv("GIT_SSH_COMMAND") == "" {
		cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new")
	}
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
	if _, _, err := r.gitRun(ctx, nil, []byte(message), "commit", "-q", "-F", "-"); err != nil {
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
	args := []string{"log", "--format=%H%x00%h%x00%an%x00%ae%x00%aI%x00%s%x1e", fmt.Sprintf("-n%d", limit)}
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
	commits := []Commit{}
	for _, rec := range strings.Split(out, "\x1e") {
		f := strings.Split(strings.TrimLeft(rec, "\n"), "\x00")
		if len(f) == 6 {
			commits = append(commits, Commit{SHA: f[0], Short: f[1], Author: f[2], Email: f[3], Date: f[4], Subject: f[5]})
		}
	}
	return commits, nil
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
	remote := strings.TrimSpace(out)
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
