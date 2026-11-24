package core

import (
	"errors"
	"maps"
	"path"
	"reflect"
	"slices"
	"strings"
)

// Copying writes what one machine has for a package's customisation, or for a
// user's home, into other machines' managed.json and folders. It never
// overwrites: a machine that already has another value for something copied
// is refused whole, and nothing is written to it.

// CopyRequest names what to copy from a machine (one of PackageOverride and
// Home) and the machines to copy it to.
type CopyRequest struct {
	PackageOverride string   `json:"packageOverride,omitempty" jsonschema:"a nixpkgs attribute: copy how the package is changed (managed.json's packageOverrides entry, its patch files and packages/<attribute>.nix)"`
	Home            string   `json:"home,omitempty" jsonschema:"a user: copy their home (managed.json's home entry, the home-manager options set for them except home.stateVersion, custom.nix and the files in ~)"`
	To              []string `json:"to" jsonschema:"the machines to copy to; each needs a managed.json"`
}

// CopyResult is what a copy did on one machine.
type CopyResult struct {
	Host    string `json:"host"`
	Changed bool   `json:"changed"` // false: the machine already had all of it, or was refused
	// Conflicts are what the machine already has with another value; when
	// there are any, nothing was written to it.
	Conflicts []string `json:"conflicts,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// copyPlan is what a copy writes on each machine.
type copyPlan struct {
	// apply adds the copied entries to a machine's managed file: what it
	// already has with another value, and whether anything changed.
	apply  func(v *ManagedView) (conflicts []string, changed bool)
	files  map[string]copyFile // by path inside the machine's folder
	absent []string            // paths inside the machine's folder that it must not have
}

// copyFile is a file a copy writes: its text, and whether it is executable,
// which git and home-manager's links both keep.
type copyFile struct {
	text string
	exec bool
}

func (s *Service) readCopy(p string) (copyFile, error) {
	text, _, err := s.Read(p)
	if err != nil {
		return copyFile{}, err
	}
	exec, err := s.Repo.executable(p)
	return copyFile{text, exec}, err
}

// Copy copies a package's customisation or a user's home from one machine to
// others. Each machine gets its own result; an error means nothing was written.
// A machine without a home for the user still needs home-manager turned on
// there (SetHome with its stateVersion), which a copy never sets.
func (s *Service) Copy(from string, r CopyRequest) ([]CopyResult, error) {
	if from == SharedHost {
		return nil, errorf(ErrInvalid, "copy from a machine; every machine already gets modules/managed.json")
	}
	if len(r.To) == 0 {
		return nil, errorf(ErrInvalid, "pick the machines to copy to")
	}
	for _, t := range r.To {
		if t == from || t == SharedHost {
			return nil, errorf(ErrInvalid, "copy to machines other than %s", from)
		}
		if _, err := managedPath(t); err != nil {
			return nil, err
		}
	}
	src, err := s.readManaged(from)
	if err != nil {
		return nil, err
	}
	var p copyPlan
	switch {
	case (r.PackageOverride == "") == (r.Home == ""):
		return nil, errorf(ErrInvalid, "copy a package's customisation or a home")
	case r.Home != "":
		p, err = s.homePlan(src, r.Home)
	default:
		p, err = s.overridePlan(src, r.PackageOverride)
	}
	if err != nil {
		return nil, err
	}
	out := make([]CopyResult, 0, len(r.To))
	for _, t := range r.To {
		res := CopyResult{Host: t}
		if err := s.copyTo(t, p, &res); err != nil {
			res.Error = err.Error()
		}
		out = append(out, res)
	}
	return out, nil
}

// copyTo applies p to one machine: files first, then its managed file, which
// must not have changed since the conflicts were looked for.
func (s *Service) copyTo(host string, p copyPlan, res *CopyResult) error {
	md, err := s.readManaged(host)
	if err != nil {
		return err
	}
	if !md.view.Exists {
		return errorf(ErrInvalid, "%s does not exist: turn on editing on %s first", md.view.Path, host)
	}
	conflicts, changed := p.apply(&md.view)
	dir := path.Dir(md.view.Path) + "/"
	var missing []string
	for _, f := range slices.Sorted(maps.Keys(p.files)) {
		cur, err := s.readCopy(dir + f)
		switch {
		case errors.Is(err, ErrNotFound):
			missing = append(missing, f)
		case err != nil:
			return err
		case cur != p.files[f]:
			conflicts = append(conflicts, dir+f)
		}
	}
	for _, f := range p.absent {
		if _, _, err := s.Read(dir + f); !errors.Is(err, ErrNotFound) {
			conflicts = append(conflicts, dir+f)
		}
	}
	if len(conflicts) > 0 {
		res.Conflicts = conflicts
		return nil
	}
	for _, f := range missing {
		if _, err := s.Write(dir+f, p.files[f].text, ""); err != nil {
			return err
		}
		if p.files[f].exec {
			if err := s.Repo.makeExecutable(dir + f); err != nil {
				return err
			}
		}
	}
	res.Changed = changed || len(missing) > 0
	if changed {
		_, err = s.editManaged(host, &md.view.Hash, func(v *ManagedView) error {
			p.apply(v)
			return nil
		})
	}
	return err
}

// overridePlan copies how a package is changed: its packageOverrides entry,
// the patch files it names, and packages/<attr>.nix. A machine that changes
// the package in either way the source does not is refused too, since it
// would not get the same package.
func (s *Service) overridePlan(src managedDoc, attr string) (copyPlan, error) {
	if !packageAttr.MatchString(attr) {
		return copyPlan{}, errorf(ErrInvalid, "invalid package attribute %q", attr)
	}
	dir := path.Dir(src.view.Path) + "/"
	o, has := src.view.PackageOverrides[attr]
	p := copyPlan{files: map[string]copyFile{}}
	for _, patch := range o.Patches {
		if patch.File == "" {
			continue
		}
		c, err := s.readCopy(dir + patch.File)
		if err != nil {
			return p, err
		}
		p.files[patch.File] = c
	}
	nixFile := "packages/" + attr + ".nix"
	switch c, err := s.readCopy(dir + nixFile); {
	case err == nil:
		p.files[nixFile] = c
	case errors.Is(err, ErrNotFound):
		if !has {
			return p, errorf(ErrInvalid, "%s does not change %s", src.view.Host, attr)
		}
		p.absent = []string{nixFile}
	default:
		return p, err
	}
	p.apply = func(v *ManagedView) ([]string, bool) {
		cur, ok := v.PackageOverrides[attr]
		switch {
		case ok && !(has && reflect.DeepEqual(cur, o)):
			return []string{"packageOverrides." + attr}, false
		case ok || !has:
			return nil, false
		}
		v.PackageOverrides[attr] = o
		return nil, true
	}
	return p, nil
}

// homePlan copies a user's home: managed.json's home entry (lists and secret
// links joined), the home-manager options set for them except
// home.stateVersion, which stays each machine's own, custom.nix and the files
// and templates linked into ~.
func (s *Service) homePlan(src managedDoc, user string) (copyPlan, error) {
	h, ok := src.view.Home[user]
	if !ok {
		return copyPlan{}, errorf(ErrInvalid, "%s's home is not managed on %s", user, src.view.Host)
	}
	hmPath := []string{"home-manager", "users", user}
	// src is read for this copy only, so taking stateVersion out of it changes no file.
	unset(src.view.Settings, append(hmPath, "home", "stateVersion"))
	settings := map[string]any{}
	if opts, ok := lookup(src.view.Settings, hmPath); ok {
		if err := setIn(settings, hmPath, opts); err != nil {
			return copyPlan{}, err
		}
	}
	dir := path.Dir(src.view.Path) + "/"
	home := "home/" + user + "/"
	entries, err := s.Tree()
	if err != nil {
		return copyPlan{}, err
	}
	p := copyPlan{files: map[string]copyFile{}}
	for _, e := range entries {
		f, in := strings.CutPrefix(e.Path, dir)
		if e.Dir || !in || (f != home+"custom.nix" && !strings.HasPrefix(f, home+"files/") && !strings.HasPrefix(f, home+"templates/")) {
			continue
		}
		// shortcut: files are read as text, so a binary dotfile stops the copy; copy bytes if one is needed.
		c, err := s.readCopy(e.Path)
		if err != nil {
			return p, err
		}
		p.files[f] = c
	}
	p.apply = func(v *ManagedView) ([]string, bool) {
		var conflicts []string
		cur, had := v.Home[user]
		for _, x := range h.Packages {
			if slices.Contains(cur.RemovedPackages, x) {
				conflicts = append(conflicts, "home."+user+".removedPackages: "+x)
			}
		}
		for _, x := range h.RemovedPackages {
			if slices.Contains(cur.Packages, x) {
				conflicts = append(conflicts, "home."+user+".packages: "+x)
			}
		}
		join := func(a, b []string) []string {
			out := slices.Clone(a)
			for _, x := range b {
				if !slices.Contains(out, x) {
					out = append(out, x)
				}
			}
			return out
		}
		next := HomeManaged{Packages: join(cur.Packages, h.Packages), RemovedPackages: join(cur.RemovedPackages, h.RemovedPackages), Secrets: maps.Clone(cur.Secrets)}
		for p, name := range h.Secrets {
			if other, ok := next.Secrets[p]; ok && other != name {
				conflicts = append(conflicts, "home."+user+".secrets: "+p)
			} else if !ok {
				if next.Secrets == nil {
					next.Secrets = map[string]string{}
				}
				next.Secrets[p] = name
			}
		}
		changed := !had || len(next.Packages) > len(cur.Packages) || len(next.RemovedPackages) > len(cur.RemovedPackages) || len(next.Secrets) > len(cur.Secrets)
		v.Home[user] = next
		if addMissing(v.Settings, settings, "settings", &conflicts) {
			changed = true
		}
		slices.Sort(conflicts)
		return conflicts, changed
	}
	return p, nil
}

// lookup is the value at path in a settings tree.
func lookup(node map[string]any, path []string) (any, bool) {
	var v any = node
	for _, seg := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		if v, ok = m[seg]; !ok {
			return nil, false
		}
	}
	return v, true
}

// addMissing sets src's values in dst where dst has none, and lists where dst
// has another value. Objects merge as managed.nix merges them; anything else
// is one value. Values from src are shared, never changed afterwards.
func addMissing(dst, src map[string]any, at string, conflicts *[]string) bool {
	changed := false
	for k, sv := range src {
		p := at + "." + k
		dv, ok := dst[k]
		sm, sObj := sv.(map[string]any)
		dm, dObj := dv.(map[string]any)
		switch {
		case !ok:
			dst[k], changed = sv, true
		case sObj && dObj:
			if addMissing(dm, sm, p, conflicts) {
				changed = true
			}
		case sObj || dObj || !reflect.DeepEqual(sv, dv):
			*conflicts = append(*conflicts, p)
		}
	}
	return changed
}
