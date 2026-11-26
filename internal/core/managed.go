package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/TheOlympusCloud/nixos-manager/templates"
)

// A machine's managed file holds what the fleet views edit by clicking:
//
//	{
//	  "packages": ["git", "python3Packages.requests"],
//	  "removedPackages": ["perl"],
//	  "packageOverrides": { "htop": { "override": { "sensorsSupport": false }, "patches": [{ "file": "patches/htop.patch" }] } },
//	  "home": { "pablo": { "packages": ["ripgrep"], "removedPackages": [], "secrets": { ".netrc": "netrc" },
//	    "configs": { ".config/app/settings.json": { "format": "json", "settings": { "theme": "dark" } } },
//	    "keyring": [ { "label": "…", "attributes": { "service": "app" }, "secret": "app-password" } ] } },
//	  "settings": { "services": { "openssh": { "enable": true } } }
//	}
//
// modules/managed.nix in the flake turns it into NixOS configuration. The app
// never rewrites hand-written Nix; JSON round-trips safely. Raw Nix and
// dotfiles go in real files next to it (see managed.nix), edited as any file.
//
// modules/managed.json has the same shape and applies to every machine that
// loads managed.nix 3, below each machine's own file (see managed.nix for how
// they merge). Every edit below takes SharedHost for it instead of a machine.
//
// A standalone home (homeConfigurations.<name>) has its own file,
// homes/<name>/managed.json, which modules/managed-home.nix applies: packages,
// removedPackages and settings, its home-manager options, and configs (see
// configs.go). Every edit below takes HomePrefix+<name> for it instead of a machine.
//
// A fleet module, modules/fleet/<name>/managed.json, has packages and settings,
// a description and the options it declares (modules.go), and applies where a
// machine turns it on (fleet.modules.<name>.enable, managed.nix 7). Every edit
// below takes ModulePrefix+<name> for it instead of a machine.

// ManagedView is a machine's managed file as the UI and MCP see it.
type ManagedView struct {
	Host     string   `json:"host"`
	Path     string   `json:"path"`
	Exists   bool     `json:"exists"`
	Hash     string   `json:"hash"`
	Packages []string `json:"packages"`
	// RemovedPackages are package names taken out of the system wherever they
	// are added: by NixOS, a flake input or hand-written Nix.
	RemovedPackages []string `json:"removedPackages"`
	// PackageOverrides change a package, by attribute, for everything on the machine.
	PackageOverrides map[string]PackageOverride `json:"packageOverrides"`
	// Home lists the home-manager users this file manages, with their packages.
	Home     map[string]HomeManaged `json:"home"`
	Settings map[string]any         `json:"settings"`
	// Configs are a standalone home's app configs (managed-home.nix), without secrets.
	Configs map[string]HomeConfig `json:"configs,omitempty"`
	// Description and Options are a fleet module's (modules.go): what it is,
	// and the options it declares, fleet.modules.<name>.<option>.
	Description string                  `json:"description,omitempty"`
	Options     map[string]ModuleOption `json:"options,omitempty"`
}

// PackageOverride is how managed.nix changes one package: taken from the
// flake input From (another nixpkgs) instead of the machine's own, then
// .override with Override, then .overrideAttrs with OverrideAttrs, Version,
// Src and Patches appended to the package's own. An empty hash builds with
// lib.fakeHash, so the check fails and prints the right one.
type PackageOverride struct {
	From          string         `json:"from,omitempty" jsonschema:"a flake input in flake.nix that is another nixpkgs (e.g. nixpkgs-unstable): the package comes from it; managed.nix 4 and specialArgs.inputs"`
	Override      map[string]any `json:"override,omitempty"`
	OverrideAttrs map[string]any `json:"overrideAttrs,omitempty"`
	Version       string         `json:"version,omitempty"`
	Src           *PackageSource `json:"src,omitempty"`
	Patches       []PackagePatch `json:"patches,omitempty"`
}

// PackageSource is a source to build instead: a URL (fetchurl) or a GitHub
// repository at a revision (fetchFromGitHub).
type PackageSource struct {
	URL    string `json:"url,omitempty"`
	GitHub string `json:"github,omitempty"` // owner/repo
	Rev    string `json:"rev,omitempty"`
	Hash   string `json:"hash,omitempty"`
}

// PackagePatch is a patch from a URL (fetchpatch) or a file in the machine's folder.
type PackagePatch struct {
	URL  string `json:"url,omitempty"`
	Hash string `json:"hash,omitempty"`
	File string `json:"file,omitempty"`
}

// HomeManaged is one home-manager user in a managed file.
type HomeManaged struct {
	Packages        []string `json:"packages"`
	RemovedPackages []string `json:"removedPackages"`
	// Secrets links a secret of the machine at a path in ~ (".netrc": "netrc"),
	// which makes it the user's (managed.nix 5).
	Secrets map[string]string `json:"secrets,omitempty"`
	// Configs are app settings files the apps also write, by path in ~, and
	// Keyring the items stored in the user's keyring (managed.nix 6; configs.go).
	Configs map[string]HomeConfig `json:"configs,omitempty"`
	Keyring []KeyringEntry        `json:"keyring,omitempty"`
}

var (
	hostName    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]*$`)
	packageAttr = regexp.MustCompile(`^[A-Za-z0-9_+-][A-Za-z0-9_+'-]*(\.[A-Za-z0-9_+-][A-Za-z0-9_+'-]*)*$`)
	// A package's name as the model shows it (pname), e.g. python3.13-requests.
	packageName = regexp.MustCompile(`^[A-Za-z0-9_+][A-Za-z0-9_+.'-]{0,199}$`)
	userName    = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	sriHash     = regexp.MustCompile(`^(sha256|sha512)-[A-Za-z0-9+/]+=*$`)
	gitHubRepo  = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
)

// A homeConfigurations name, such as pablo@laptop: a folder name in homes/.
var homeConfigName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.@+-]{0,99}$`)

// SharedHost stands for modules/managed.json where a machine name goes.
// No machine can have this name.
const SharedHost = "*"

// HomePrefix, followed by a homeConfigurations name, stands for that standalone
// home where a machine name goes: home:pablo@laptop. No machine name has a colon.
const HomePrefix = "home:"

// validTarget accepts a machine name or a standalone home (HomePrefix+name).
func validTarget(t string) bool {
	if name, ok := strings.CutPrefix(t, HomePrefix); ok {
		return homeConfigName.MatchString(name)
	}
	return hostName.MatchString(t)
}

func managedPath(host string) (string, error) {
	if host == SharedHost {
		return "modules/managed.json", nil
	}
	if name, ok := strings.CutPrefix(host, HomePrefix); ok {
		if !homeConfigName.MatchString(name) {
			return "", errorf(ErrInvalid, "invalid home configuration name %q", name)
		}
		return "homes/" + name + "/managed.json", nil
	}
	if name, ok := strings.CutPrefix(host, ModulePrefix); ok {
		if !moduleName.MatchString(name) {
			return "", errorf(ErrInvalid, "invalid module name %q: letters, digits, _ and -, starting with a letter or _", name)
		}
		return moduleFolder + "/" + name + "/managed.json", nil
	}
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
	md := managedDoc{view: ManagedView{
		Host: host, Path: p, Packages: []string{}, RemovedPackages: []string{},
		PackageOverrides: map[string]PackageOverride{}, Home: map[string]HomeManaged{}, Settings: map[string]any{},
	}, doc: map[string]any{}}
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
	if dec.Decode(&struct{}{}) != io.EOF { // e.g. two objects left by a bad merge: rewriting would drop one
		return md, errorf(ErrInvalid, "%s holds more than one JSON value; fix it in the code view", p)
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
	keys := map[string]any{"removedPackages": &md.view.RemovedPackages, "packageOverrides": &md.view.PackageOverrides, "home": &md.view.Home, "configs": &md.view.Configs}
	if strings.HasPrefix(host, ModulePrefix) { // elsewhere these keys mean nothing, and stay as they are
		keys["description"], keys["options"] = &md.view.Description, &md.view.Options
	}
	// The newer keys are decoded strictly: an unknown field would be lost on the next write.
	for key, into := range keys {
		if raw, ok := md.doc[key]; ok {
			if err := strictDecode(raw, into); err != nil {
				return md, errorf(ErrInvalid, "%s: %q: %v; fix it in the code view", p, key, err)
			}
		}
	}
	md.view.RemovedPackages = orEmpty(md.view.RemovedPackages)
	if md.view.PackageOverrides == nil {
		md.view.PackageOverrides = map[string]PackageOverride{}
	}
	if md.view.Home == nil {
		md.view.Home = map[string]HomeManaged{}
	}
	for u, h := range md.view.Home {
		h.Packages, h.RemovedPackages = orEmpty(h.Packages), orEmpty(h.RemovedPackages)
		md.view.Home[u] = h
	}
	if err := md.view.check(); err != nil {
		return md, errorf(ErrInvalid, "%s: %v; fix it in the code view", p, err)
	}
	return md, nil
}

// strictDecode decodes a value of the parsed file into a typed one, refusing
// unknown fields and keeping numbers exact.
func strictDecode(raw, into any) error {
	b, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	return dec.Decode(into)
}

// check validates what managed.nix will read, so a bad value is refused when
// it is written (or read), not first at the next evaluation.
func (v *ManagedView) check() error {
	if strings.HasPrefix(v.Host, ModulePrefix) {
		if err := v.checkModule(); err != nil {
			return err
		}
	}
	standalone := strings.HasPrefix(v.Host, HomePrefix)
	if standalone && (len(v.Home) > 0 || len(v.PackageOverrides) > 0) {
		return errorf(ErrInvalid, "a standalone home has packages, removedPackages, settings (its home-manager options) and configs; home-manager users and package overrides are for NixOS machines")
	}
	if !standalone && len(v.Configs) > 0 {
		return errorf(ErrInvalid, "a machine's app configs are its users': home.<user>.configs")
	}
	for p, c := range v.Configs {
		if err := checkConfig(p, c, false); err != nil {
			return err
		}
	}
	if err := checkPackageNames(v.RemovedPackages); err != nil {
		return err
	}
	for attr, o := range v.PackageOverrides {
		if err := checkOverride(attr, o); err != nil {
			return err
		}
	}
	for u, h := range v.Home {
		if !userName.MatchString(u) {
			return fmt.Errorf("invalid user name %q", u)
		}
		if err := checkPackages(h.Packages); err != nil {
			return err
		}
		if err := checkPackageNames(h.RemovedPackages); err != nil {
			return err
		}
		for p, name := range h.Secrets {
			if err := checkHomePath(p); err != nil {
				return err
			}
			if !secretName.MatchString(name) {
				return errorf(ErrInvalid, "invalid secret name %q", name)
			}
		}
		for p, c := range h.Configs {
			if err := checkConfig(p, c, true); err != nil {
				return err
			}
			if _, ok := h.Secrets[p]; ok {
				return errorf(ErrInvalid, "~/%s of %s is a linked secret and a config: keep one of the two", p, u)
			}
		}
		if err := checkKeyring(h.Keyring); err != nil {
			return err
		}
	}
	return nil
}

// checkHomePath accepts a path inside a home folder, such as .config/gh/hosts.yml.
func checkHomePath(p string) error {
	if c, err := cleanPath(p); err != nil || c != p || strings.ContainsFunc(p, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return errorf(ErrInvalid, "invalid path in ~ %q: a path inside the home folder, such as .config/gh/hosts.yml", p)
	}
	return nil
}

func checkPackageNames(names []string) error {
	for _, n := range names {
		if !packageName.MatchString(n) {
			return errorf(ErrInvalid, "invalid package name %q", n)
		}
	}
	return nil
}

func checkHash(h, what string) error {
	if h != "" && !sriHash.MatchString(h) {
		return errorf(ErrInvalid, "%s: the hash must look like sha256-…= (empty: the check prints the right one)", what)
	}
	return nil
}

func checkURL(u, what string) error {
	if strings.ContainsFunc(u, func(r rune) bool { return r <= 0x20 || r == 0x7f }) {
		return errorf(ErrInvalid, "%s: the URL has spaces or control characters", what)
	}
	for _, scheme := range []string{"https://", "http://", "mirror://"} {
		if strings.HasPrefix(u, scheme) && len(u) > len(scheme) {
			return nil
		}
	}
	return errorf(ErrInvalid, "%s: the URL must start with https://, http:// or mirror://", what)
}

func checkOverride(attr string, o PackageOverride) error {
	what := "the override of " + attr
	if !packageAttr.MatchString(attr) {
		return errorf(ErrInvalid, "invalid package attribute %q", attr)
	}
	for _, m := range []map[string]any{o.Override, o.OverrideAttrs} {
		for k := range m {
			if k == "" || strings.ContainsFunc(k, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
				return errorf(ErrInvalid, "%s: invalid argument name %q", what, k)
			}
		}
	}
	if o.From != "" && !inputName.MatchString(o.From) {
		return errorf(ErrInvalid, "%s: %q is not a flake input name such as nixpkgs-unstable", what, o.From)
	}
	if strings.ContainsFunc(o.Version, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return errorf(ErrInvalid, "%s: invalid version", what)
	}
	if s := o.Src; s != nil {
		switch {
		case (s.URL == "") == (s.GitHub == ""):
			return errorf(ErrInvalid, "%s: a source is a URL or a GitHub repository, not both", what)
		case s.URL != "":
			if err := checkURL(s.URL, what); err != nil {
				return err
			}
			if s.Rev != "" {
				return errorf(ErrInvalid, "%s: a revision goes with a GitHub repository", what)
			}
		case !gitHubRepo.MatchString(s.GitHub):
			return errorf(ErrInvalid, "%s: the GitHub repository must be owner/repo", what)
		case s.Rev == "" || strings.ContainsFunc(s.Rev, func(r rune) bool { return r <= 0x20 || r == 0x7f }):
			return errorf(ErrInvalid, "%s: a GitHub source needs a revision (a tag or commit)", what)
		}
		if err := checkHash(s.Hash, what); err != nil {
			return err
		}
	}
	for _, p := range o.Patches {
		switch {
		case (p.URL == "") == (p.File == ""):
			return errorf(ErrInvalid, "%s: a patch is a URL or a file, not both", what)
		case p.File != "":
			if c, err := cleanPath(p.File); err != nil || c != p.File || strings.HasPrefix(p.File, "../") {
				return errorf(ErrInvalid, "%s: a patch file is a path inside the folder of its managed.json (a machine's, or modules/), such as patches/fix.patch", what)
			}
			if p.Hash != "" {
				return errorf(ErrInvalid, "%s: a patch file needs no hash", what)
			}
		default:
			if err := checkURL(p.URL, what); err != nil {
				return err
			}
			if err := checkHash(p.Hash, what); err != nil {
				return err
			}
		}
	}
	return nil
}

// ManagedView reads a machine's managed file (empty when it does not exist yet).
func (s *Service) ManagedView(host string) (ManagedView, error) {
	md, err := s.readManaged(host)
	return md.view, err
}

// editManaged applies edit and saves the file only if nobody changed it since it
// was read. With base set, the file must also still be the version the caller saw.
func (s *Service) editManaged(host string, base *string, edit func(v *ManagedView) error) (ManagedView, error) {
	md, err := s.readManaged(host)
	if err != nil {
		return md.view, err
	}
	if base != nil && *base != md.view.Hash {
		return md.view, errorf(ErrConflict, "%s changed since it was loaded", md.view.Path)
	}
	if name, ok := strings.CutPrefix(host, ModulePrefix); ok && !md.view.Exists && base != &newModule {
		return md.view, errorf(ErrNotFound, "there is no module %s: create it first (create_module)", name)
	}
	if err := edit(&md.view); err != nil {
		return md.view, err
	}
	if err := md.view.check(); err != nil {
		return md.view, err
	}
	sortSet := func(xs []string) []string {
		slices.Sort(xs)
		return orEmpty(slices.Compact(xs))
	}
	md.view.Packages, md.view.RemovedPackages = sortSet(md.view.Packages), sortSet(md.view.RemovedPackages)
	for u, h := range md.view.Home {
		h.Packages, h.RemovedPackages = sortSet(h.Packages), sortSet(h.RemovedPackages)
		md.view.Home[u] = h
	}
	md.doc["packages"], md.doc["settings"] = md.view.Packages, md.view.Settings
	newer := map[string]any{"removedPackages": md.view.RemovedPackages, "packageOverrides": md.view.PackageOverrides, "home": md.view.Home, "configs": md.view.Configs}
	if strings.HasPrefix(host, ModulePrefix) {
		newer["description"], newer["options"] = md.view.Description, md.view.Options
	}
	// The newer keys only when used, so a file that needs none still works with an older managed.nix.
	for key, v := range newer {
		if reflect.ValueOf(v).Len() == 0 {
			delete(md.doc, key)
		} else {
			md.doc[key] = v
		}
	}
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

// PackageEdit changes the packages of a machine, or of one of its
// home-manager users (home.packages) when User is set.
type PackageEdit struct {
	User    string   `json:"user,omitempty" jsonschema:"a home-manager user of the machine: edit that user's home.packages instead of the system's"`
	Add     []string `json:"add,omitempty" jsonschema:"nixpkgs attributes to install"`
	Remove  []string `json:"remove,omitempty" jsonschema:"attributes to take out of managed.json's list"`
	Exclude []string `json:"exclude,omitempty" jsonschema:"package names (as the model lists them) to remove wherever they are added: NixOS defaults, flake inputs and hand-written Nix included"`
	Restore []string `json:"restore,omitempty" jsonschema:"package names to give back: undoes exclude"`
}

// EditPackages applies a PackageEdit in one write.
func (s *Service) EditPackages(host string, e PackageEdit) (ManagedView, error) {
	if err := checkPackages(append(slices.Clone(e.Add), e.Remove...)); err != nil {
		return ManagedView{}, err
	}
	if err := checkPackageNames(append(slices.Clone(e.Exclude), e.Restore...)); err != nil {
		return ManagedView{}, err
	}
	if e.User != "" && !userName.MatchString(e.User) {
		return ManagedView{}, errorf(ErrInvalid, "invalid user name %q", e.User)
	}
	edit := func(pkgs, removed *[]string) {
		*pkgs = append(slices.DeleteFunc(*pkgs, func(p string) bool { return slices.Contains(e.Remove, p) }), e.Add...)
		// Adding vim after removing NixOS's vim means vim again. managed.nix keeps what is
		// added here anyway; this keeps the list from claiming otherwise.
		*removed = append(slices.DeleteFunc(*removed, func(p string) bool { return slices.Contains(e.Restore, p) || slices.Contains(e.Add, p) }), e.Exclude...)
	}
	return s.editManaged(host, nil, func(v *ManagedView) error {
		if e.User == "" {
			edit(&v.Packages, &v.RemovedPackages)
			return nil
		}
		h, known := v.Home[e.User] // a user with home-manager by hand gets an entry with the first edit
		edit(&h.Packages, &h.RemovedPackages)
		if known || len(h.Packages)+len(h.RemovedPackages) > 0 {
			v.Home[e.User] = h
		}
		return nil
	})
}

// AddPackages installs nixpkgs packages (attribute names) on a machine.
func (s *Service) AddPackages(host string, names []string) (ManagedView, error) {
	return s.EditPackages(host, PackageEdit{Add: names})
}

// RemovePackages removes packages from a machine's managed list.
func (s *Service) RemovePackages(host string, names []string) (ManagedView, error) {
	return s.EditPackages(host, PackageEdit{Remove: names})
}

// SetPackageOverride sets how a package (by attribute) is changed on a
// machine, or with nil or an empty override, puts it back as nixpkgs has it.
// base as for SetOption.
func (s *Service) SetPackageOverride(host, attr string, o *PackageOverride, base *string) (ManagedView, error) {
	if !packageAttr.MatchString(attr) {
		return ManagedView{}, errorf(ErrInvalid, "invalid package attribute %q", attr)
	}
	if o != nil && reflect.ValueOf(*o).IsZero() {
		o = nil
	}
	if o != nil {
		if err := checkOverride(attr, *o); err != nil {
			return ManagedView{}, err
		}
	}
	return s.editManaged(host, base, func(v *ManagedView) error {
		if o == nil {
			delete(v.PackageOverrides, attr)
		} else {
			v.PackageOverrides[attr] = *o
		}
		return nil
	})
}

// HomeEdit turns home-manager on or off for a user of a machine.
type HomeEdit struct {
	User   string `json:"user"`
	Enable bool   `json:"enable"`
	// StateVersion, when turning it on for a user without home-manager yet: the
	// release whose defaults it keeps (home.stateVersion), e.g. "26.05".
	StateVersion string `json:"stateVersion,omitempty"`
}

var stateVersion = regexp.MustCompile(`^[0-9]{2}\.[0-9]{2}$`)

// SetHome adds a user to managed.json's home-manager users (their packages,
// files and custom.nix next to it then apply), or takes the user out with
// everything set here for them. Files next to managed.json stay.
func (s *Service) SetHome(host string, e HomeEdit) (ManagedView, error) {
	if !userName.MatchString(e.User) {
		return ManagedView{}, errorf(ErrInvalid, "invalid user name %q", e.User)
	}
	if host == SharedHost {
		return ManagedView{}, errorf(ErrInvalid, "home-manager is turned on per machine; a user's packages and options for every machine apply where it is on for them")
	}
	if e.StateVersion != "" && (!e.Enable || !stateVersion.MatchString(e.StateVersion)) {
		return ManagedView{}, errorf(ErrInvalid, "a state version is a release such as 26.05, given when turning home-manager on")
	}
	hmPath := []string{"home-manager", "users", e.User}
	return s.editManaged(host, nil, func(v *ManagedView) error {
		if !e.Enable {
			delete(v.Home, e.User)
			unset(v.Settings, hmPath)
			return nil
		}
		if _, ok := v.Home[e.User]; !ok {
			v.Home[e.User] = HomeManaged{Packages: []string{}, RemovedPackages: []string{}}
		}
		if e.StateVersion != "" {
			return setIn(v.Settings, append(hmPath, "home", "stateVersion"), e.StateVersion)
		}
		return nil
	})
}

// HomeSecretEdit links a secret of the machine as a file in a user's home, or
// with Secret empty, takes the link at Path away.
type HomeSecretEdit struct {
	User   string `json:"user" jsonschema:"a home-manager user of the machine"`
	Path   string `json:"path" jsonschema:"the file in ~, e.g. .netrc or .config/rclone/rclone.conf"`
	Secret string `json:"secret,omitempty" jsonschema:"a secret the machine gets (host_details' secrets): the whole file; empty to unlink"`
}

// LinkHomeSecret sets or removes a link of managed.json's home.<user>.secrets.
// The secret becomes the user's on the machine; a missing one fails its check.
func (s *Service) LinkHomeSecret(host string, e HomeSecretEdit) (ManagedView, error) {
	if !userName.MatchString(e.User) {
		return ManagedView{}, errorf(ErrInvalid, "invalid user name %q", e.User)
	}
	return s.editManaged(host, nil, func(v *ManagedView) error {
		h, known := v.Home[e.User]
		if e.Secret == "" {
			delete(h.Secrets, e.Path)
		} else {
			if h.Secrets == nil {
				h.Secrets = map[string]string{}
			}
			h.Secrets[e.Path] = e.Secret
		}
		if known || len(h.Secrets) > 0 { // as EditPackages: the first link adds a user with home-manager by hand
			v.Home[e.User] = h
		}
		return nil
	})
}

// UpdateManagedModule writes this release's modules/managed.nix over the
// repository's (or creates it), if it is still the version the caller saw.
// With home, modules/managed-home.nix, for standalone homes. Either way it also
// writes modules/managed-configs.py, which both run on the machines.
func (s *Service) UpdateManagedModule(base string, home bool) (string, error) {
	path := managedModule
	if home {
		path = managedHomeModule
	}
	b, err := fs.ReadFile(templates.Fleet, path)
	if err != nil {
		return "", err
	}
	hash, err := s.Write(path, string(b), base)
	if err != nil {
		return "", err
	}
	py, err := fs.ReadFile(templates.Fleet, managedConfigsWriter)
	if err != nil {
		return "", err
	}
	cur, curHash, err := s.Read(managedConfigsWriter)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return "", err
	}
	if cur != string(py) {
		if _, err := s.Write(managedConfigsWriter, string(py), curHash); err != nil {
			return "", err
		}
	}
	return hash, nil
}

const (
	managedModule        = "modules/managed.nix"
	managedHomeModule    = "modules/managed-home.nix"
	managedConfigsWriter = "modules/managed-configs.py"
)

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
// base, when set, is the managed file's hash the caller's value is based on.
func (s *Service) SetOption(host string, path []string, value any, base *string) (ManagedView, error) {
	if err := checkPath(path); err != nil {
		return ManagedView{}, err
	}
	if value == nil {
		return ManagedView{}, errorf(ErrInvalid, "a value is required; unset the option instead")
	}
	return s.editManaged(host, base, func(v *ManagedView) error { return setIn(v.Settings, path, value) })
}

// setIn sets path in a settings tree, creating the objects on the way.
func setIn(node map[string]any, path []string, value any) error {
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
}

// UnsetOption removes a setting and any parent left empty; base as for SetOption.
func (s *Service) UnsetOption(host string, path []string, base *string) (ManagedView, error) {
	if err := checkPath(path); err != nil {
		return ManagedView{}, err
	}
	return s.editManaged(host, base, func(v *ManagedView) error {
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
