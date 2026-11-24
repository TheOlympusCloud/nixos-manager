package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Every commit pushed to the repository is checked (Forgejo.Hook), and each
// result goes to Forgejo as a commit status, when a status token is set.

const statusContext = "nixos-manager/check"

// statusRetry is how long to wait before each new try of a status Forgejo
// failed to take (a network error, or its own error).
var statusRetry = []time.Duration{10 * time.Second, time.Minute}

var apiClient = &http.Client{Timeout: 20 * time.Second}

// pushedCommit is a commit waiting for a check, or for its result to reach
// Forgejo, as pushed-checks.json keeps it over a restart.
type pushedCommit struct {
	Commit string `json:"commit"`
	Branch string `json:"branch"`
}

type pushEntry struct {
	branch string
	busy   bool // a check of it runs, or its result is being sent
}

// StatusDelivery is the last commit status sent to Forgejo, as Settings shows it.
type StatusDelivery struct {
	Time   time.Time `json:"time"`
	Commit string    `json:"commit"`
	State  string    `json:"state"` // pending, success, failure or error
	OK     bool      `json:"ok"`
	Result string    `json:"result"`
}

// statusConfig is what Settings keeps for commit statuses, 0600 in the data dir.
type statusConfig struct {
	Token   string `json:"token"`
	Console string `json:"console,omitempty"` // the console's address, for the statuses' links
}

func (f *Forgejo) pushedPath() string { return filepath.Join(f.DataDir, "pushed-checks.json") }
func (f *Forgejo) statusPath() string { return filepath.Join(f.DataDir, "forgejo-status.json") }

// Resume has every check of a commit reported to Forgejo, then checks the
// pushed commits that waited when the server stopped. It needs Jobs.
func (f *Forgejo) Resume() error {
	b, err := os.ReadFile(f.pushedPath())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	var list []pushedCommit
	if err == nil {
		if err := json.Unmarshal(b, &list); err != nil {
			slog.Warn("cannot read the pushed commits that wait for a check; they are not checked", "file", f.pushedPath(), "err", err)
			list = nil
		}
	}
	f.mu.Lock()
	f.pushed = map[string]*pushEntry{}
	for _, p := range list {
		if fullSHA.MatchString(p.Commit) {
			f.pushed[p.Commit] = &pushEntry{branch: p.Branch}
		}
	}
	waiting := len(f.pushed) > 0
	f.mu.Unlock()
	f.Jobs.WatchCommits(f.track)
	if waiting {
		go f.sync()
	}
	return nil
}

// queuePush records a pushed commit until its check is reported; f.mu held.
func (f *Forgejo) queuePush(sha, branch string) {
	if f.pushed == nil {
		f.pushed = map[string]*pushEntry{}
	}
	if f.pushed[sha] != nil {
		return
	}
	f.pushed[sha] = &pushEntry{branch: branch}
	f.savePushed()
}

// savePushed writes the commits waiting for a check; f.mu held. Failing only
// means a restart forgets them, so it is logged.
func (f *Forgejo) savePushed() {
	list := []pushedCommit{}
	for sha, e := range f.pushed {
		list = append(list, pushedCommit{Commit: sha, Branch: e.branch})
	}
	slices.SortFunc(list, func(a, b pushedCommit) int { return strings.Compare(a.Commit, b.Commit) })
	b, _ := json.Marshal(list) // strings always marshal
	if err := writeAtomic(f.pushedPath(), b); err != nil {
		slog.Warn("cannot save the pushed commits that wait for a check; a restart forgets them", "err", err)
	}
}

// claimPushes takes the pushed commits that wait for a check, before a fetch
// that brings them; f.mu held.
func (f *Forgejo) claimPushes() []pushedCommit {
	var todo []pushedCommit
	for sha, e := range f.pushed {
		if !e.busy {
			e.busy = true
			todo = append(todo, pushedCommit{Commit: sha, Branch: e.branch})
		}
	}
	slices.SortFunc(todo, func(a, b pushedCommit) int { return strings.Compare(a.Commit, b.Commit) })
	return todo
}

// checkPushes checks the commits claimPushes took, after the fetch. A commit
// the failed fetch did not bring waits for the next one.
func (f *Forgejo) checkPushes(todo []pushedCommit, fetchErr error) {
	for _, p := range todo {
		job, pass, err := f.Jobs.CheckCommit(context.Background(), p.Commit, p.Branch)
		if fetchErr != nil && errors.Is(err, ErrNotFound) {
			f.mu.Lock()
			if e := f.pushed[p.Commit]; e != nil {
				e.busy = false
			}
			f.mu.Unlock()
			continue
		}
		go f.report(p.Commit, job, pass, err)
	}
}

// track reports a check of a commit queued by anyone else than checkPushes
// (the Check button in Health, MCP): Jobs calls it for every one.
func (f *Forgejo) track(job *Job) {
	sha := job.Req.Commit
	f.mu.Lock()
	e := f.pushed[sha]
	if e != nil && e.busy { // checkPushes or a report has it
		f.mu.Unlock()
		return
	}
	if e == nil {
		if f.pushed == nil {
			f.pushed = map[string]*pushEntry{}
		}
		e = &pushEntry{branch: job.Req.Branch}
		f.pushed[sha] = e
	}
	e.busy = true
	f.savePushed()
	f.mu.Unlock()
	go f.report(sha, job, nil, nil)
}

// report sends Forgejo what became of a commit: success at once when its tree
// had passed, error when it could not be checked, else pending and then the
// check's result. A check of the same commit asked for meanwhile is reported
// next; then the commit stops waiting.
func (f *Forgejo) report(sha string, job *Job, pass *CheckPass, err error) {
	ctx := context.Background()
	for {
		switch {
		case err != nil && errors.Is(err, ErrNotFound):
			f.postStatus(ctx, sha, "error", "Not checked: nixos-manager could not fetch this commit")
		case err != nil:
			f.postStatus(ctx, sha, "error", truncate("Not checked: "+err.Error(), 140))
		case pass != nil:
			f.postStatus(ctx, sha, "success", passedText(pass.Base, pass.Built, pass.EvaluatedOnly))
		default:
			build := "a build"
			if f.Jobs.EvalOnly {
				build = "a dry build"
			}
			f.postStatus(ctx, sha, "pending", "Checking: nix flake check, then "+build+" of every machine it changes")
			<-job.Done()
			state, desc := checkStatus(job.Info(true))
			f.postStatus(ctx, sha, state, desc)
		}
		f.mu.Lock()
		if job = f.Jobs.unfinishedCheck(sha); job == nil {
			delete(f.pushed, sha)
			f.savePushed()
			f.mu.Unlock()
			return
		}
		f.mu.Unlock()
		pass, err = nil, nil
	}
}

// passedText says what a passed check built, or only evaluated, for a commit status.
func passedText(base string, built, evaluated []string) string {
	verb, end := "built", ""
	if len(evaluated) > 0 { // a check either builds every changed machine or none
		verb, end, built = "evaluated", "; not built here", evaluated
	}
	switch {
	case base == "":
		return fmt.Sprintf("Passed: %s every machine (%d)%s", verb, len(built), end)
	case len(built) == 0:
		return "Passed: no machine changes"
	case len(built) <= 4:
		return "Passed: " + verb + " " + strings.Join(built, ", ") + end
	}
	return fmt.Sprintf("Passed: %s the %d machines it changes%s", verb, len(built), end)
}

// checkStatus is the commit status a finished check gives: success, failure,
// or error when it ran out of time before it could tell.
func checkStatus(info JobInfo) (string, string) {
	sum, last := info.Summary, string(lastLine([]byte(info.Output)))
	switch {
	case info.State == "passed" && sum != nil:
		return "success", passedText(sum.Base, sum.Built, sum.EvaluatedOnly)
	case info.State == "passed":
		return "success", "Passed"
	case strings.HasPrefix(last, "stopped:"):
		return "error", "Not checked to the end: the check ran out of time"
	case sum == nil:
		return "failure", "Failed: nix flake check or the evaluation failed; the log says why"
	case len(sum.Failed) > 0:
		return "failure", truncate(fmt.Sprintf("Failed: %s %s not build", list(sum.Failed), map[bool]string{true: "does", false: "do"}[len(sum.Failed) == 1]), 140)
	}
	return "failure", truncate("Failed: "+last, 140)
}

// apiBase is the repository in Forgejo's API, e.g.
// https://forgejo.lan/api/v1/repos/pablo/fleet, or "" when the remote has no web side.
func (f *Forgejo) apiBase(ctx context.Context) string {
	root, path := webRoot(f.Repo.originURL(ctx), f.Repo.WebBase(ctx))
	if root == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return root + "/api/v1/repos/" + strings.Join(parts, "/")
}

// apiCall sends one request to Forgejo's API with the token, and answers the
// status code and Forgejo's message, if any.
func apiCall(ctx context.Context, method, u, token string, body []byte) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := apiClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var m struct {
		Message string `json:"message"`
	}
	json.Unmarshal(b, &m)
	return resp.StatusCode, m.Message, nil
}

func (f *Forgejo) statusConfig() (statusConfig, error) {
	var c statusConfig
	b, err := os.ReadFile(f.statusPath())
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err == nil {
		err = json.Unmarshal(b, &c)
	}
	return c, err
}

// SetStatusToken keeps the Forgejo token commit statuses are sent with, and
// the console's address their links open (optional); an empty token removes
// both. The token must reach the repository through Forgejo's API first.
func (f *Forgejo) SetStatusToken(ctx context.Context, token, console string) error {
	token = strings.TrimSpace(token)
	console = strings.TrimRight(strings.TrimSpace(console), "/")
	if token == "" {
		f.mu.Lock()
		defer f.mu.Unlock()
		if err := os.Remove(f.statusPath()); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if len(token) > 512 || strings.ContainsFunc(token, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return errorf(ErrInvalid, "paste the token alone, as Forgejo shows it")
	}
	if console != "" && !httpBase(console) {
		return errorf(ErrInvalid, "the console's address must look like http://nixos-manager.lan:8080")
	}
	api := f.apiBase(ctx)
	if api == "" {
		return errorf(ErrInvalid, "connect the Forgejo repository over SSH or HTTPS first: a remote on this server has no Forgejo to send statuses to")
	}
	code, msg, err := apiCall(ctx, "GET", api, token, nil)
	switch {
	case err != nil:
		return errorf(ErrInvalid, "cannot reach Forgejo's API at %s: %v. Check the web address above", api, err)
	case code == 401 || code == 403:
		return errorf(ErrInvalid, "Forgejo refused the token (%s). Create one under Settings → Applications with the write:repository permission", cmpOr(msg, http.StatusText(code)))
	case code == 404:
		return errorf(ErrInvalid, "Forgejo has no repository %s that this token's user can see. Check the web address above, and that the user can write to the repository", api)
	case code/100 != 2:
		return errorf(ErrInvalid, "Forgejo's API answered %d at %s", code, api)
	}
	if err := f.outsideRepo(f.DataDir); err != nil {
		return err
	}
	b, _ := json.Marshal(statusConfig{Token: token, Console: console})
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.MkdirAll(f.DataDir, 0o700); err != nil {
		return err
	}
	return writeAtomic(f.statusPath(), b)
}

// LastStatus is the last commit status sent to Forgejo, or nil.
func (f *Forgejo) LastStatus() *StatusDelivery {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastStatus == nil {
		return nil
	}
	d := *f.lastStatus
	return &d
}

// postStatus tells Forgejo a commit's check state, when a status token is set.
// A network error or an error of Forgejo's own is tried again (statusRetry).
func (f *Forgejo) postStatus(ctx context.Context, sha, state, desc string) {
	cfg, err := f.statusConfig()
	if err != nil {
		slog.Warn("cannot read the commit status settings", "err", err)
		return
	}
	if cfg.Token == "" {
		return
	}
	done := func(ok bool, result string) {
		f.mu.Lock()
		f.lastStatus = &StatusDelivery{Time: time.Now().UTC(), Commit: sha, State: state, OK: ok, Result: result}
		f.mu.Unlock()
		if !ok {
			slog.Warn("cannot send a commit status to Forgejo", "commit", sha, "state", state, "err", result)
		}
	}
	api := f.apiBase(ctx)
	if api == "" {
		done(false, "The remote has no Forgejo web address, so no status was sent.")
		return
	}
	target := ""
	if cfg.Console != "" {
		target = cfg.Console + "/#/changes?check=" + sha
	}
	body, _ := json.Marshal(map[string]string{"state": state, "target_url": target, "description": desc, "context": statusContext})
	for try := 0; ; try++ {
		code, msg, err := apiCall(ctx, "POST", api+"/statuses/"+sha, cfg.Token, body)
		var result string
		switch {
		case err != nil:
			result = "Cannot reach Forgejo's API: " + err.Error()
		case code/100 == 2:
			done(true, fmt.Sprintf("Forgejo has %s for %.7s.", state, sha))
			return
		case code == 401 || code == 403:
			result = fmt.Sprintf("Forgejo refused the token (%d %s). It needs the write:repository permission, from a user who can write to the repository.", code, msg)
		default:
			result = strings.TrimSpace(fmt.Sprintf("Forgejo's API answered %d. %s", code, msg))
		}
		if (err == nil && code < 500 && code != 429) || try == len(statusRetry) {
			done(false, result)
			return
		}
		time.Sleep(statusRetry[try])
	}
}
