package core

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// HostValue is an option's effective value on one machine, and the files that set it.
type HostValue struct {
	Host   string   `json:"host"`
	Exists bool     `json:"exists"` // the machine declares this option
	Value  any      `json:"value"`
	Files  []string `json:"files"` // repo files that define it; "nixos" = from NixOS or an input
	Type   string   `json:"type"`
	Error  string   `json:"error,omitempty"`
}

// OptionValues is one option across the fleet.
type OptionValues struct {
	Name  string      `json:"name"`
	Path  []string    `json:"path"`
	Hosts []HostValue `json:"hosts"`
}

const keepValues = 32

// SplitOptionName turns services.nginx.virtualHosts."a.lan".root into its attribute path.
// Placeholders like <name> are refused: a value needs a concrete option.
func SplitOptionName(name string) ([]string, error) {
	bad := errorf(ErrInvalid, "invalid option name %q", name)
	var path []string
	var seg strings.Builder
	quoted, wasQuoted := false, false
	for _, r := range name {
		switch {
		case quoted && r == '"':
			quoted = false
		case quoted:
			if unicode.IsControl(r) {
				return nil, bad
			}
			seg.WriteRune(r)
		case r == '"':
			if seg.Len() > 0 {
				return nil, bad
			}
			quoted, wasQuoted = true, true
		case r == '.':
			if seg.Len() == 0 && !wasQuoted {
				return nil, bad
			}
			path = append(path, seg.String())
			seg.Reset()
			wasQuoted = false
		case r == '<' || r == '>' || unicode.IsSpace(r) || unicode.IsControl(r):
			return nil, bad
		default:
			if wasQuoted {
				return nil, bad
			}
			seg.WriteRune(r)
		}
	}
	if quoted || (seg.Len() == 0 && !wasQuoted) {
		return nil, bad
	}
	return append(path, seg.String()), nil
}

// OptionValues evaluates options on the machines of the working copy (all when
// hosts is empty). Until the evaluation is done it returns the job instead.
func (js *Jobs) OptionValues(ctx context.Context, names, hosts []string) ([]OptionValues, *Job, error) {
	if len(names) == 0 || len(names) > 40 {
		return nil, nil, errorf(ErrInvalid, "ask for 1 to 40 options")
	}
	for _, n := range names {
		if _, err := SplitOptionName(n); err != nil {
			return nil, nil, err
		}
	}
	for _, h := range hosts {
		if !validTarget(h) {
			return nil, nil, errorf(ErrInvalid, "invalid machine name %q", h)
		}
	}
	tree, err := js.Repo.WorkingTree(ctx)
	if err != nil {
		return nil, nil, err
	}
	sn, sh := slices.Sorted(slices.Values(names)), slices.Sorted(slices.Values(hosts))
	key := "values " + tree + " " + strings.Join(sn, "\x00") + " @" + strings.Join(sh, ",")
	js.mu.Lock()
	defer js.mu.Unlock()
	if v, ok := js.indexes[key]; ok {
		return v.([]OptionValues), nil, nil
	}
	if err, ok := js.indexErrs[key]; ok {
		delete(js.indexErrs, key) // reported once: asking again retries, in case the cause was passing
		return nil, nil, err
	}
	for _, j := range js.jobs {
		if j.key == key && !j.finishedState() {
			return nil, j, nil
		}
	}
	j, err := js.enqueue(JobRequest{Kind: "values", Names: names, Hosts: hosts}, tree, key)
	return nil, j, err
}

func (js *Jobs) runValues(ctx context.Context, job *Job) (err error) {
	defer func() {
		if err != nil {
			js.mu.Lock()
			js.indexErrs[job.key] = err
			js.keepValues(job.key) // a failure nobody asks about again is forgotten too
			js.mu.Unlock()
		}
	}()
	src, cleanup, err := js.snapshot(ctx, job.tree)
	if err != nil {
		return err
	}
	defer cleanup()
	discovered, err := js.discover(ctx, src)
	if err != nil {
		return err
	}
	hosts := discovered["nixos"]
	if len(job.Req.Hosts) > 0 {
		hosts = slices.DeleteFunc(slices.Clone(hosts), func(h string) bool { return !slices.Contains(job.Req.Hosts, h) })
	}
	out := make([]OptionValues, len(job.Req.Names))
	paths := make([][]string, len(job.Req.Names))
	for i, n := range job.Req.Names {
		paths[i], _ = SplitOptionName(n) // validated before queueing
		out[i] = OptionValues{Name: n, Path: paths[i], Hosts: []HostValue{}}
	}
	pathsJSON, _ := json.Marshal(paths)
	for _, h := range hosts {
		fmt.Fprintf(job, "Evaluating %d option(s) on %s…\n", len(paths), h)
		var raw map[string]HostValue
		evalErr := js.evalJSON(ctx, src, map[string]string{"host": h, "pathsJSON": string(pathsJSON)}, nixExpr("values.nix"), &raw)
		for i, p := range paths {
			hv := HostValue{Host: h, Files: []string{}}
			if evalErr != nil {
				hv.Error = evalErr.Error()
			} else {
				k, _ := json.Marshal(p)
				got := raw[string(k)]
				hv.Exists, hv.Value, hv.Type = got.Exists, got.Value, got.Type
				hv.Files = orEmpty(got.Files)
			}
			out[i].Hosts = append(out[i].Hosts, hv)
		}
	}
	js.mu.Lock()
	js.indexes[job.key] = out
	js.keepValues(job.key)
	js.mu.Unlock()
	return nil
}

// keepValues records key as the newest option values result or failure, and
// forgets the oldest beyond keepValues; js.mu must be held.
func (js *Jobs) keepValues(key string) {
	js.valuesOrder = append(slices.DeleteFunc(js.valuesOrder, func(k string) bool { return k == key }), key)
	for len(js.valuesOrder) > keepValues {
		delete(js.indexes, js.valuesOrder[0])
		delete(js.indexErrs, js.valuesOrder[0])
		js.valuesOrder = js.valuesOrder[1:]
	}
}
