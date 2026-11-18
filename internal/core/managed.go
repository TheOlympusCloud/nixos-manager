package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
)

// A machine's managed file holds what the fleet views edit by clicking:
//
//	{ "packages": ["git", "python3Packages.requests"], "settings": { "services": { "openssh": { "enable": true } } } }
//
// modules/managed.nix in the flake turns it into NixOS configuration. The app
// never rewrites hand-written Nix; JSON round-trips safely.

// ManagedView is a machine's managed file as the UI and MCP see it.
type ManagedView struct {
	Host     string         `json:"host"`
	Path     string         `json:"path"`
	Exists   bool           `json:"exists"`
	Hash     string         `json:"hash"`
	Packages []string       `json:"packages"`
	Settings map[string]any `json:"settings"`
}

var (
	hostName    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]*$`)
	packageAttr = regexp.MustCompile(`^[A-Za-z0-9_+-][A-Za-z0-9_+'-]*(\.[A-Za-z0-9_+-][A-Za-z0-9_+'-]*)*$`)
)

func managedPath(host string) (string, error) {
	if !hostName.MatchString(host) {
		return "", errorf(ErrInvalid, "invalid machine name %q", host)
	}
	return "hosts/" + host + "/managed.json", nil
}

type managedDoc struct {
	view ManagedView
	doc  map[string]any // the whole file, so keys we do not know survive
}

func (s *Service) readManaged(host string) (managedDoc, error) {
	p, err := managedPath(host)
	if err != nil {
		return managedDoc{}, err
	}
	md := managedDoc{view: ManagedView{Host: host, Path: p, Packages: []string{}, Settings: map[string]any{}}, doc: map[string]any{}}
	content, hash, err := s.Read(p)
	if errors.Is(err, ErrNotFound) {
		return md, nil
	}
	if err != nil {
		return md, err
	}
	md.view.Exists, md.view.Hash = true, hash
	dec := json.NewDecoder(strings.NewReader(content))
	dec.UseNumber() // keep large integers exact
	if err := dec.Decode(&md.doc); err != nil || md.doc == nil {
		return md, errorf(ErrInvalid, "%s is not valid JSON (%v); fix it in the code view", p, err)
	}
	if raw, ok := md.doc["packages"]; ok {
		list, ok := raw.([]any)
		if !ok {
			return md, errorf(ErrInvalid, "%s: \"packages\" must be a list of package names", p)
		}
		for _, x := range list {
			name, ok := x.(string)
			if !ok {
				return md, errorf(ErrInvalid, "%s: \"packages\" must be a list of package names", p)
			}
			md.view.Packages = append(md.view.Packages, name)
		}
	}
	if raw, ok := md.doc["settings"]; ok {
		settings, ok := raw.(map[string]any)
		if !ok {
			return md, errorf(ErrInvalid, "%s: \"settings\" must be an object", p)
		}
		md.view.Settings = settings
	}
	return md, nil
}

// ManagedView reads a machine's managed file (empty when it does not exist yet).
func (s *Service) ManagedView(host string) (ManagedView, error) {
	md, err := s.readManaged(host)
	return md.view, err
}

// editManaged applies edit and saves the file only if nobody changed it since it was read.
func (s *Service) editManaged(host string, edit func(v *ManagedView) error) (ManagedView, error) {
	md, err := s.readManaged(host)
	if err != nil {
		return md.view, err
	}
	if err := edit(&md.view); err != nil {
		return md.view, err
	}
	slices.Sort(md.view.Packages)
	md.view.Packages = slices.Compact(md.view.Packages)
	md.doc["packages"], md.doc["settings"] = md.view.Packages, md.view.Settings
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(md.doc); err != nil {
		return md.view, err
	}
	hash, err := s.Write(md.view.Path, b.String(), md.view.Hash)
	if err != nil {
		return md.view, err
	}
	md.view.Exists, md.view.Hash = true, hash
	return md.view, nil
}

func checkPackages(names []string) error {
	for _, n := range names {
		if !packageAttr.MatchString(n) {
			return errorf(ErrInvalid, "invalid package name %q", n)
		}
	}
	return nil
}

// AddPackages installs nixpkgs packages (attribute names) on a machine.
func (s *Service) AddPackages(host string, names []string) (ManagedView, error) {
	if err := checkPackages(names); err != nil {
		return ManagedView{}, err
	}
	return s.editManaged(host, func(v *ManagedView) error {
		v.Packages = append(v.Packages, names...)
		return nil
	})
}

// RemovePackages removes packages from a machine's managed list.
func (s *Service) RemovePackages(host string, names []string) (ManagedView, error) {
	if err := checkPackages(names); err != nil {
		return ManagedView{}, err
	}
	return s.editManaged(host, func(v *ManagedView) error {
		v.Packages = slices.DeleteFunc(v.Packages, func(p string) bool { return slices.Contains(names, p) })
		return nil
	})
}

func checkPath(path []string) error {
	if len(path) == 0 || len(path) > 32 {
		return errorf(ErrInvalid, "an option path needs 1 to 32 parts")
	}
	for _, seg := range path {
		if seg == "" || strings.ContainsFunc(seg, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			return errorf(ErrInvalid, "invalid option path %q", strings.Join(path, "."))
		}
	}
	return nil
}

// SetOption sets a NixOS option (path is its attribute path) on a machine.
func (s *Service) SetOption(host string, path []string, value any) (ManagedView, error) {
	if err := checkPath(path); err != nil {
		return ManagedView{}, err
	}
	if value == nil {
		return ManagedView{}, errorf(ErrInvalid, "a value is required; unset the option instead")
	}
	return s.editManaged(host, func(v *ManagedView) error {
		node := v.Settings
		for i, seg := range path[:len(path)-1] {
			next, ok := node[seg]
			if !ok {
				child := map[string]any{}
				node[seg], node = child, child
				continue
			}
			child, ok := next.(map[string]any)
			if !ok {
				return errorf(ErrInvalid, "%s already has a value, so it cannot contain %s", strings.Join(path[:i+1], "."), path[i+1])
			}
			node = child
		}
		node[path[len(path)-1]] = value
		return nil
	})
}

// UnsetOption removes a setting and any parent left empty.
func (s *Service) UnsetOption(host string, path []string) (ManagedView, error) {
	if err := checkPath(path); err != nil {
		return ManagedView{}, err
	}
	return s.editManaged(host, func(v *ManagedView) error {
		unset(v.Settings, path)
		return nil
	})
}

func unset(node map[string]any, path []string) {
	if len(path) == 1 {
		delete(node, path[0])
		return
	}
	child, ok := node[path[0]].(map[string]any)
	if !ok {
		return
	}
	unset(child, path[1:])
	if len(child) == 0 {
		delete(node, path[0])
	}
}
