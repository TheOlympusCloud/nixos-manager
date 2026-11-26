package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"regexp"
	"slices"
	"strings"
)

// A fleet module bundles packages and settings that machines turn on one by
// one. modules/fleet/<name>/managed.json has the shape of a machine's
// managed.json (packages and settings) plus what makes it a module:
//
//	{
//	  "description": "Jellyfin with its firewall",
//	  "options": { "port": { "type": "port", "default": 8096, "description": "Where Jellyfin listens" } },
//	  "packages": ["ffmpeg"],
//	  "settings": { "services": { "jellyfin": { "enable": true } },
//	                "networking": { "firewall": { "allowedTCPPorts": ["@option:port@"] } } }
//	}
//
// managed.nix 7 makes each a NixOS module: fleet.modules.<name>.enable and an
// option fleet.modules.<name>.<option> for each declared one, and, under
// lib.mkIf enable, its packages and settings, where "@option:<option>@" stands
// for that option's value on the machine. A machine turns it on and sets its
// options as any NixOS option: in its managed.json's settings (SetOption), in
// modules/managed.json for every machine, or in hand-written Nix. custom.nix
// next to it is a NixOS module of the module's own, for what JSON cannot say.

// ModulePrefix, followed by a module's name, stands for that fleet module
// where a machine name goes: module:media. No machine name has a colon.
const ModulePrefix = "module:"

const moduleFolder = "modules/fleet"

var (
	// A module's name is a folder and a Nix attribute: fleet.modules.<name>.
	moduleName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,62}$`)
	// An option's name, as managed.nix finds it in "@option:<name>@".
	moduleOptionName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,62}$`)
	optionRef        = regexp.MustCompile(`@option:([A-Za-z_][A-Za-z0-9_-]*)@`)
)

// ModuleOptionTypes are the types an option of a module can have, as managed.nix maps them.
var ModuleOptionTypes = []string{"bool", "int", "port", "str", "listOf str", "anything"}

// ModuleOption is an option a fleet module declares. Without a default, each
// machine that turns the module on must set it.
type ModuleOption struct {
	Type        string `json:"type" jsonschema:"bool, int, port, str, listOf str or anything"`
	Default     any    `json:"default,omitempty" jsonschema:"its value where a machine sets none; leave out to make each machine set it"`
	Description string `json:"description,omitempty"`
	// nullDefault: "default": null (type anything), which omitempty would turn into none.
	nullDefault bool
}

// moduleOption is ModuleOption without its JSON methods.
type moduleOption ModuleOption

// UnmarshalJSON keeps "default": null apart from no default, refuses unknown
// fields and keeps numbers exact, as strictDecode does.
func (o *ModuleOption) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	var p moduleOption
	if err := dec.Decode(&p); err != nil {
		return err
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		return err
	}
	_, has := keys["default"]
	p.nullDefault = has && p.Default == nil
	*o = ModuleOption(p)
	return nil
}

// MarshalJSON writes "default": null for a null default.
func (o ModuleOption) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false) // as editManaged writes the rest of the file
	if err := enc.Encode(moduleOption(o)); err != nil {
		return nil, err
	}
	out := bytes.TrimSpace(b.Bytes())
	if o.nullDefault {
		out = append(append(out[:len(out)-1:len(out)-1], `,"default":null`...), '}')
	}
	return out, nil
}

// FleetModule is a fleet module as the UI and MCP list it.
type FleetModule struct {
	Name string `json:"name"`
	ManagedView
	Custom bool   `json:"custom"`          // modules/fleet/<name>/custom.nix exists
	Error  string `json:"error,omitempty"` // its managed.json cannot be read: fix it in Code
}

// ModuleEdit changes what a module is: its description, and options added,
// replaced or (with null) removed. Removing an option also removes the value
// each machine set for it.
type ModuleEdit struct {
	Description *string                  `json:"description,omitempty"`
	Options     map[string]*ModuleOption `json:"options,omitempty" jsonschema:"option name -> its declaration, or null to remove it"`
	BaseHash    *string                  `json:"base_hash,omitempty" jsonschema:"the module's hash the edit is based on; refused if it changed"`
}

// ModuleDeleted is what deleting a module changed: the managed files whose
// settings for it were removed, so neither a machine nor another module still
// turns it on there.
type ModuleDeleted struct {
	Path     string   `json:"path"`
	Machines []string `json:"machines"` // managed.json paths whose fleet.modules.<name> went, other modules' included
}

// checkModule validates what managed.nix reads from a module's managed.json.
func (v *ManagedView) checkModule() error {
	if len(v.Home) > 0 || len(v.PackageOverrides) > 0 || len(v.RemovedPackages) > 0 || len(v.Configs) > 0 {
		return errorf(ErrInvalid, "a fleet module has a description, options, packages and settings; homes, removed and customised packages are set on machines")
	}
	if strings.ContainsFunc(v.Description, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return errorf(ErrInvalid, "a module's description is one line of text")
	}
	for name, o := range v.Options {
		if err := checkModuleOption(name, o); err != nil {
			return err
		}
	}
	var missing []string
	walkStrings(v.Settings, func(s string) {
		for _, m := range optionRef.FindAllStringSubmatch(s, -1) {
			if _, ok := v.Options[m[1]]; !ok {
				missing = append(missing, m[1])
			}
		}
	})
	if len(missing) > 0 {
		slices.Sort(missing)
		return errorf(ErrInvalid, "the settings use @option:%s@, which the module does not declare: add the option, or set the value itself", strings.Join(slices.Compact(missing), "@, @option:"))
	}
	return nil
}

func checkModuleOption(name string, o ModuleOption) error {
	if !moduleOptionName.MatchString(name) || name == "enable" {
		return errorf(ErrInvalid, "invalid option name %q: letters, digits, _ and -, starting with a letter or _ (enable is the module's own)", name)
	}
	if strings.ContainsFunc(o.Description, func(r rune) bool { return r < 0x20 && r != '\n' || r == 0x7f }) {
		return errorf(ErrInvalid, "the description of %s has control characters", name)
	}
	if !slices.Contains(ModuleOptionTypes, o.Type) {
		return errorf(ErrInvalid, "the option %s has the type %q; use one of %s", name, o.Type, strings.Join(ModuleOptionTypes, ", "))
	}
	if o.Default != nil && !fitsType(o.Type, o.Default) || o.nullDefault && o.Type != "anything" {
		return errorf(ErrInvalid, "the default of %s is not a %s", name, o.Type)
	}
	return nil
}

// fitsType reports whether a JSON value is one of an option type's values.
func fitsType(typ string, v any) bool {
	switch typ {
	case "bool":
		_, ok := v.(bool)
		return ok
	case "int", "port":
		var n int64
		switch x := v.(type) {
		case json.Number:
			i, err := x.Int64()
			if err != nil {
				return false
			}
			n = i
		case float64:
			if x != float64(int64(x)) {
				return false
			}
			n = int64(x)
		default:
			return false
		}
		return typ == "int" || n >= 0 && n <= 65535
	case "str":
		_, ok := v.(string)
		return ok
	case "listOf str":
		list, ok := v.([]any)
		return ok && !slices.ContainsFunc(list, func(x any) bool { _, s := x.(string); return !s })
	}
	return true // anything
}

func walkStrings(v any, fn func(string)) {
	switch x := v.(type) {
	case string:
		fn(x)
	case []any:
		for _, e := range x {
			walkStrings(e, fn)
		}
	case map[string]any:
		for _, e := range x {
			walkStrings(e, fn)
		}
	}
}

// newModule, as editManaged's base, creates a module's file; any other edit of
// module:<name> needs it to exist, so a misspelt name is an error, not a new module.
var newModule = ""

// inRepo reports whether a file of the working copy exists.
func (s *Service) inRepo(p string) bool {
	_, err := fs.Stat(s.root.FS(), p)
	return err == nil
}

// ListModules reads every fleet module, by name. One that cannot be read is
// listed with its error, so the others still show.
func (s *Service) ListModules() ([]FleetModule, error) {
	entries, err := fs.ReadDir(s.root.FS(), moduleFolder)
	if errors.Is(err, fs.ErrNotExist) {
		return []FleetModule{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []FleetModule{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !s.inRepo(moduleFolder + "/" + e.Name() + "/managed.json") {
			continue // a folder of something else
		}
		m, err := s.Module(e.Name())
		if err != nil {
			m.Error = err.Error()
		}
		out = append(out, m)
	}
	return out, nil
}

// Module reads one fleet module.
func (s *Service) Module(name string) (FleetModule, error) {
	m := FleetModule{Name: name}
	m.Host = ModulePrefix + name
	if !moduleName.MatchString(name) {
		m.Path = moduleFolder + "/" + name + "/managed.json"
		return m, errorf(ErrInvalid, "%q is not a module name managed here (letters, digits, _ and -): edit it in Code", name)
	}
	view, err := s.ManagedView(ModulePrefix + name)
	m.ManagedView = view
	if err != nil {
		return m, err
	}
	if !view.Exists {
		return m, errorf(ErrNotFound, "there is no module %s", name)
	}
	m.Custom = s.inRepo(moduleFolder + "/" + name + "/custom.nix")
	return m, nil
}

// CreateModule makes a new, empty fleet module that no machine has on yet.
func (s *Service) CreateModule(name, description string) (FleetModule, error) {
	if !moduleName.MatchString(name) {
		return FleetModule{}, errorf(ErrInvalid, "invalid module name %q: letters, digits, _ and -, starting with a letter or _", name)
	}
	if _, err := s.editManaged(ModulePrefix+name, &newModule, func(v *ManagedView) error {
		v.Description = strings.TrimSpace(description)
		return nil
	}); err != nil {
		if errors.Is(err, ErrConflict) {
			return FleetModule{}, errorf(ErrConflict, "there is already a module %s", name)
		}
		return FleetModule{}, err
	}
	return s.Module(name)
}

// EditModule changes a module's description and options. An option removed
// also loses the values machines set for it (in their managed.json and
// modules/managed.json), which would otherwise fail their evaluation.
func (s *Service) EditModule(name string, e ModuleEdit) (FleetModule, error) {
	if !moduleName.MatchString(name) {
		return FleetModule{}, errorf(ErrInvalid, "invalid module name %q", name)
	}
	var removed []string
	_, err := s.editManaged(ModulePrefix+name, e.BaseHash, func(v *ManagedView) error {
		if !v.Exists {
			return errorf(ErrNotFound, "there is no module %s", name)
		}
		if e.Description != nil {
			v.Description = strings.TrimSpace(*e.Description)
		}
		if v.Options == nil {
			v.Options = map[string]ModuleOption{}
		}
		for o, d := range e.Options {
			if d == nil {
				if _, ok := v.Options[o]; ok {
					removed = append(removed, o)
				}
				delete(v.Options, o)
				continue
			}
			d.Description = strings.TrimSpace(d.Description)
			v.Options[o] = *d
		}
		return nil
	})
	if err != nil {
		return FleetModule{}, err
	}
	slices.Sort(removed)
	for _, o := range removed {
		if _, err := s.forgetSetting([]string{"fleet", "modules", name, o}, ""); err != nil {
			return FleetModule{}, err
		}
	}
	return s.Module(name)
}

// DeleteModule removes a module's folder, and turns it off where a managed
// file turns it on: its fleet.modules.<name> settings go too. Hand-written Nix
// that still sets them fails the next check, which names the file.
func (s *Service) DeleteModule(name string) (ModuleDeleted, error) {
	p, err := managedPath(ModulePrefix + name)
	if err != nil {
		return ModuleDeleted{}, err
	}
	if !s.inRepo(p) { // one that does not parse can still go
		return ModuleDeleted{}, errorf(ErrNotFound, "there is no module %s", name)
	}
	changed, err := s.forgetSetting([]string{"fleet", "modules", name}, ModulePrefix+name)
	if err != nil {
		return ModuleDeleted{}, err
	}
	if err := s.Delete(moduleFolder + "/" + name); err != nil {
		return ModuleDeleted{}, err
	}
	return ModuleDeleted{Path: p, Machines: changed}, nil
}

// forgetSetting unsets path in every managed file that sets it: each machine's
// hosts/<name>/managed.json, modules/managed.json and the fleet modules' own,
// which may turn on another module or set its options; skip is a target left
// out (a module being deleted). Every file is read first, so one that cannot
// be read stops it before anything is written.
func (s *Service) forgetSetting(path []string, skip string) ([]string, error) {
	targets := []string{SharedHost}
	for _, folder := range []struct {
		dir, prefix string
		name        *regexp.Regexp
	}{{"hosts", "", hostName}, {moduleFolder, ModulePrefix, moduleName}} {
		entries, err := fs.ReadDir(s.root.FS(), folder.dir)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		for _, e := range entries {
			// A module folder without managed.json is something else.
			if e.IsDir() && folder.name.MatchString(e.Name()) && folder.prefix+e.Name() != skip &&
				(folder.prefix == "" || s.inRepo(folder.dir+"/"+e.Name()+"/managed.json")) {
				targets = append(targets, folder.prefix+e.Name())
			}
		}
	}
	type hit struct{ target, hash, path string }
	var hits []hit
	for _, t := range targets {
		md, err := s.readManaged(t)
		if err != nil {
			return nil, err
		}
		if _, ok := lookup(md.view.Settings, path); ok {
			hits = append(hits, hit{t, md.view.Hash, md.view.Path})
		}
	}
	changed := []string{}
	for _, h := range hits {
		if _, err := s.UnsetOption(h.target, path, &h.hash); err != nil {
			return changed, err
		}
		changed = append(changed, h.path)
	}
	return changed, nil
}
