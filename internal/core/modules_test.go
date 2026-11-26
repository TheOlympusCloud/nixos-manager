package core

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TheOlympusCloud/nixos-manager/templates"
)

func TestFleetModules(t *testing.T) {
	s := newService(t)
	if ms := must[[]FleetModule](t)(s.ListModules()); len(ms) != 0 {
		t.Fatalf("modules of a fleet without any: %+v", ms)
	}
	m := must[FleetModule](t)(s.CreateModule("media", " Jellyfin with its firewall "))
	if m.Path != "modules/fleet/media/managed.json" || m.Host != "module:media" || m.Description != "Jellyfin with its firewall" || m.Custom {
		t.Fatalf("new module %+v", m)
	}
	_, err := s.CreateModule("media", "again")
	isErr(t, err, ErrConflict)

	port := json.Number("8096")
	must[FleetModule](t)(s.EditModule("media", ModuleEdit{Options: map[string]*ModuleOption{
		"port":  {Type: "port", Default: port, Description: "Where Jellyfin listens"},
		"users": {Type: "listOf str"}, // no default: each machine sets it
	}}))
	// Its packages and settings are edited as any managed file's, by target.
	must[ManagedView](t)(s.AddPackages("module:media", []string{"ffmpeg"}))
	must[ManagedView](t)(s.SetOption("module:media", []string{"services", "jellyfin", "enable"}, true, nil))
	must[ManagedView](t)(s.SetOption("module:media", []string{"networking", "firewall", "allowedTCPPorts"}, []any{"@option:port@"}, nil))
	write(t, s.Repo, "modules/fleet/media/custom.nix", "{ ... }: { }\n")

	content, _, _ := s.Read("modules/fleet/media/managed.json")
	var doc map[string]any
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"description": "Jellyfin with its firewall",
		"options": map[string]any{
			"port":  map[string]any{"type": "port", "default": float64(8096), "description": "Where Jellyfin listens"},
			"users": map[string]any{"type": "listOf str"},
		},
		"packages": []any{"ffmpeg"},
		"settings": map[string]any{
			"services":   map[string]any{"jellyfin": map[string]any{"enable": true}},
			"networking": map[string]any{"firewall": map[string]any{"allowedTCPPorts": []any{"@option:port@"}}},
		},
	}
	if !reflect.DeepEqual(doc, want) {
		t.Fatalf("module file\n%s", content)
	}

	// A folder without managed.json is something else, not a module.
	write(t, s.Repo, "modules/fleet/notes/README.md", "x\n")
	ms := must[[]FleetModule](t)(s.ListModules())
	if len(ms) != 1 || ms[0].Name != "media" || !ms[0].Custom || len(ms[0].Options) != 2 || ms[0].Packages[0] != "ffmpeg" {
		t.Fatalf("listed %+v", ms)
	}
	// One that does not parse is listed with its error.
	write(t, s.Repo, "modules/fleet/broken/managed.json", "{")
	ms = must[[]FleetModule](t)(s.ListModules())
	if len(ms) != 2 || ms[0].Name != "broken" || !strings.Contains(ms[0].Error, "not valid JSON") || ms[1].Error != "" {
		t.Fatalf("listed with a broken one %+v", ms)
	}
}

func TestFleetModuleValidation(t *testing.T) {
	s := newService(t)
	for _, bad := range []string{"", "a b", "../x", "-x", "a.b", "1st"} {
		_, err := s.CreateModule(bad, "")
		isErr(t, err, ErrInvalid)
	}
	must[FleetModule](t)(s.CreateModule("desk_top-2", ""))
	for name, o := range map[string]ModuleOption{
		"enable":  {Type: "bool"},
		"a.b":     {Type: "bool"},
		"t":       {Type: "float"},
		"b":       {Type: "bool", Default: "yes"},
		"p":       {Type: "port", Default: json.Number("70000")},
		"i":       {Type: "int", Default: json.Number("1.5")},
		"l":       {Type: "listOf str", Default: []any{"a", 1}},
		"s":       {Type: "str", Default: false},
		"control": {Type: "str", Description: "a\x00b"},
	} {
		_, err := s.EditModule("desk_top-2", ModuleEdit{Options: map[string]*ModuleOption{name: &o}})
		isErr(t, err, ErrInvalid)
	}
	for name, o := range map[string]ModuleOption{
		"b": {Type: "bool", Default: false}, "p": {Type: "port", Default: json.Number("0")}, "i": {Type: "int", Default: json.Number("-3")},
		"l": {Type: "listOf str", Default: []any{}}, "s": {Type: "str", Default: ""}, "any": {Type: "anything", Default: map[string]any{"x": []any{1}}},
	} {
		must[FleetModule](t)(s.EditModule("desk_top-2", ModuleEdit{Options: map[string]*ModuleOption{name: &o}}))
	}
	// A setting may only use the options the module declares.
	_, err := s.SetOption("module:desk_top-2", []string{"services", "x", "port"}, "@option:nope@", nil)
	isErr(t, err, ErrInvalid)
	must[ManagedView](t)(s.SetOption("module:desk_top-2", []string{"services", "x", "url"}, "http://localhost:@option:p@/", nil))
	_, err = s.EditModule("desk_top-2", ModuleEdit{Options: map[string]*ModuleOption{"p": nil}})
	isErr(t, err, ErrInvalid) // still used in the settings
	// Homes, removed and customised packages are the machines'.
	_, err = s.EditPackages("module:desk_top-2", PackageEdit{Exclude: []string{"perl"}})
	isErr(t, err, ErrInvalid)
	_, err = s.SetHome("module:desk_top-2", HomeEdit{User: "pablo", Enable: true})
	isErr(t, err, ErrInvalid)
	_, err = s.SetPackageOverride("module:desk_top-2", "htop", &PackageOverride{Version: "1"}, nil)
	isErr(t, err, ErrInvalid)
	// Editing one that does not exist does not make it: a misspelt name is an error.
	_, err = s.EditModule("ghost", ModuleEdit{Description: new(string)})
	isErr(t, err, ErrNotFound)
	_, err = s.SetOption("module:ghost", []string{"services", "x", "enable"}, true, nil)
	isErr(t, err, ErrNotFound)
	_, err = s.AddPackages("module:ghost", []string{"git"})
	isErr(t, err, ErrNotFound)
	empty := ""
	_, err = s.SetOption("module:ghost", []string{"services", "x", "enable"}, true, &empty)
	isErr(t, err, ErrNotFound)
	if _, _, err := s.Read("modules/fleet/ghost/managed.json"); err == nil {
		t.Fatal("an edit of a missing module created it")
	}
	_, err = s.DeleteModule("ghost")
	isErr(t, err, ErrNotFound)
	// A conflicting edit is refused.
	stale := "stale"
	_, err = s.EditModule("desk_top-2", ModuleEdit{Description: new(string), BaseHash: &stale})
	isErr(t, err, ErrConflict)
}

// A machine's description and options keys mean nothing outside a module and stay as they are.
func TestMachineManagedKeepsModuleKeys(t *testing.T) {
	s := newService(t)
	write(t, s.Repo, "hosts/web/managed.json", `{"description": 3, "options": "x", "packages": []}`)
	v := must[ManagedView](t)(s.AddPackages("web", []string{"git"}))
	if v.Description != "" || v.Options != nil {
		t.Fatalf("machine view %+v", v)
	}
	content, _, _ := s.Read("hosts/web/managed.json")
	if !strings.Contains(content, `"description": 3`) || !strings.Contains(content, `"options": "x"`) {
		t.Fatalf("machine file lost its keys:\n%s", content)
	}
}

// Removing an option, or the module, takes away what machines set for it, so
// none fails to evaluate on an option that no longer exists.
func TestFleetModuleRemovalForgetsMachineSettings(t *testing.T) {
	s := newService(t)
	must[FleetModule](t)(s.CreateModule("media", ""))
	must[FleetModule](t)(s.EditModule("media", ModuleEdit{Options: map[string]*ModuleOption{
		"port": {Type: "port", Default: json.Number("8096")}, "name": {Type: "str"},
	}}))
	must[ManagedView](t)(s.SetOption("pi", []string{"fleet", "modules", "media", "enable"}, true, nil))
	must[ManagedView](t)(s.SetOption("pi", []string{"fleet", "modules", "media", "port"}, 8097, nil))
	must[ManagedView](t)(s.SetOption("pi", []string{"time", "timeZone"}, "UTC", nil))
	must[ManagedView](t)(s.SetOption("web", []string{"fleet", "modules", "media", "name"}, "web", nil))
	must[ManagedView](t)(s.SetOption(SharedHost, []string{"fleet", "modules", "media", "enable"}, true, nil))
	must[ManagedView](t)(s.SetOption(SharedHost, []string{"fleet", "modules", "other", "enable"}, true, nil))
	// Another module that turns media on and sets its options: every machine imports it.
	must[FleetModule](t)(s.CreateModule("stack", ""))
	must[ManagedView](t)(s.SetOption(ModulePrefix+"stack", []string{"fleet", "modules", "media", "enable"}, true, nil))
	must[ManagedView](t)(s.SetOption(ModulePrefix+"stack", []string{"fleet", "modules", "media", "port"}, 80, nil))
	must[ManagedView](t)(s.SetOption(ModulePrefix+"stack", []string{"services", "nginx", "enable"}, true, nil))
	write(t, s.Repo, "hosts/untouched/managed.json", `{"packages": ["git"], "settings": {}}`)
	_, untouchedHash, _ := s.Read("hosts/untouched/managed.json")

	m := must[FleetModule](t)(s.EditModule("media", ModuleEdit{Options: map[string]*ModuleOption{"port": nil}}))
	if _, ok := m.Options["port"]; ok || len(m.Options) != 1 {
		t.Fatalf("options after removing port %+v", m.Options)
	}
	pi := must[ManagedView](t)(s.ManagedView("pi"))
	if !reflect.DeepEqual(pi.Settings["fleet"], map[string]any{"modules": map[string]any{"media": map[string]any{"enable": true}}}) {
		t.Fatalf("pi still sets the removed option: %+v", pi.Settings)
	}
	stack := must[ManagedView](t)(s.ManagedView(ModulePrefix + "stack"))
	if !reflect.DeepEqual(stack.Settings["fleet"], map[string]any{"modules": map[string]any{"media": map[string]any{"enable": true}}}) {
		t.Fatalf("the module stack still sets the removed option: %+v", stack.Settings)
	}

	d := must[ModuleDeleted](t)(s.DeleteModule("media"))
	if d.Path != "modules/fleet/media/managed.json" ||
		!reflect.DeepEqual(d.Machines, []string{"modules/managed.json", "hosts/pi/managed.json", "hosts/web/managed.json", "modules/fleet/stack/managed.json"}) {
		t.Fatalf("deleted %+v", d)
	}
	stack = must[ManagedView](t)(s.ManagedView(ModulePrefix + "stack"))
	if !reflect.DeepEqual(stack.Settings, map[string]any{"services": map[string]any{"nginx": map[string]any{"enable": true}}}) {
		t.Fatalf("the module stack after deleting media %+v", stack.Settings)
	}
	if _, _, err := s.Read("modules/fleet/media/managed.json"); err == nil {
		t.Fatal("the module's file is still there")
	}
	pi = must[ManagedView](t)(s.ManagedView("pi"))
	if _, ok := pi.Settings["fleet"]; ok || pi.Settings["time"] == nil {
		t.Fatalf("pi after deleting the module %+v", pi.Settings)
	}
	web := must[ManagedView](t)(s.ManagedView("web"))
	if len(web.Settings) != 0 {
		t.Fatalf("web after deleting the module %+v", web.Settings)
	}
	shared := must[ManagedView](t)(s.ManagedView(SharedHost))
	if !reflect.DeepEqual(shared.Settings, map[string]any{"fleet": map[string]any{"modules": map[string]any{"other": map[string]any{"enable": true}}}}) {
		t.Fatalf("shared after deleting the module %+v", shared.Settings)
	}
	if _, h, _ := s.Read("hosts/untouched/managed.json"); h != untouchedHash {
		t.Fatal("a machine without the module was rewritten")
	}

	// A machine file that does not parse stops the deletion before anything changes.
	must[FleetModule](t)(s.CreateModule("media", ""))
	must[ManagedView](t)(s.SetOption("pi", []string{"fleet", "modules", "media", "enable"}, true, nil))
	write(t, s.Repo, "hosts/broken/managed.json", "{")
	_, err := s.DeleteModule("media")
	isErr(t, err, ErrInvalid)
	if _, _, err := s.Read("modules/fleet/media/managed.json"); err != nil {
		t.Fatal("the module went although a machine file could not be read")
	}
	if pi := must[ManagedView](t)(s.ManagedView("pi")); pi.Settings["fleet"] == nil {
		t.Fatal("pi was changed although the deletion stopped")
	}
}

// On the starter fleet, a machine's model says which fleet modules it has on and
// each option's value with where it is set, and the module's options are in its
// option docs, so the option forms and compare_option work on them.
func TestFleetModuleModelWithRealNix(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	ctx := context.Background()
	s := NewService(newRepo(t))
	if _, err := s.Repo.Seed(templates.Fleet); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ct", "other"} {
		must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: name, System: "x86_64-linux", Role: "minimal", Platform: "proxmox-lxc"}))
	}
	must[FleetModule](t)(s.CreateModule("media", "the media server"))
	must[FleetModule](t)(s.EditModule("media", ModuleEdit{Options: map[string]*ModuleOption{
		"port": {Type: "port", Default: json.Number("8096"), Description: "Where it listens"},
	}}))
	must[ManagedView](t)(s.AddPackages(ModulePrefix+"media", []string{"hello"}))
	must[ManagedView](t)(s.SetOption(ModulePrefix+"media", []string{"networking", "firewall", "allowedTCPPorts"}, []any{"@option:port@"}, nil))
	must[ManagedView](t)(s.SetOption("ct", []string{"fleet", "modules", "media", "enable"}, true, nil))
	must[ManagedView](t)(s.SetOption("ct", []string{"fleet", "modules", "media", "port"}, 8097, nil))
	lock := must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "lock"}))
	if info := wait40(t, lock); info.State != "passed" {
		t.Fatalf("lock: %s", info.Output)
	}
	_, j, err := s.Jobs.Model(ctx, "working")
	ok(t, err)
	select {
	case <-j.Done():
	case <-time.After(15 * time.Minute):
		t.Fatal("the model did not finish")
	}
	m, _, err := s.Jobs.Model(ctx, "working")
	ok(t, err)
	if m == nil || m.Error != "" {
		t.Fatalf("model: %+v\n%s", m, j.Info(true).Output)
	}
	hosts := map[string]Host{}
	for _, h := range m.Hosts {
		hosts[h.Name] = h
	}
	ct, other := hosts["ct"], hosts["other"]
	if ct.Error != "" || ct.ManagedVersion != 7 || len(ct.Modules) != 1 {
		t.Fatalf("ct: %+v", ct)
	}
	want := HostModule{Name: "media", Enable: true, Files: []string{"hosts/ct/managed.json"},
		Options: map[string]HostModuleOption{"port": {Value: float64(8097), Files: []string{"hosts/ct/managed.json"}}}}
	if !reflect.DeepEqual(ct.Modules[0], want) {
		t.Fatalf("ct's module %+v", ct.Modules[0])
	}
	if !slices.Contains(ct.Ports.TCP, 8097) || !slices.ContainsFunc(ct.Packages, func(p Package) bool {
		return p.Name == "hello" && slices.Contains(p.Files, "modules/fleet/media/managed.json")
	}) {
		t.Fatalf("ct's ports %v and packages %+v", ct.Ports.TCP, ct.Packages)
	}
	if len(other.Modules) != 1 || other.Modules[0].Enable || other.Modules[0].Options["port"].Value != float64(8096) ||
		!reflect.DeepEqual(other.Modules[0].Options["port"].Files, []string{"modules/fleet/media/managed.json"}) {
		t.Fatalf("other's module %+v", other.Modules)
	}
	hits, oj, err := s.Jobs.SearchOptions(ctx, "other", "fleet.modules.media", 10)
	ok(t, err)
	if hits == nil {
		if info := wait40(t, oj); info.State != "passed" {
			t.Fatalf("options of other: %s", info.Output)
		}
		hits, _, err = s.Jobs.SearchOptions(ctx, "other", "fleet.modules.media", 10)
		ok(t, err)
	}
	if !slices.ContainsFunc(hits, func(o OptionDoc) bool {
		return o.Name == "fleet.modules.media.port" && o.Description != nil && *o.Description == "Where it listens"
	}) || !slices.ContainsFunc(hits, func(o OptionDoc) bool { return o.Name == "fleet.modules.media.enable" }) {
		t.Fatalf("search fleet.modules.media: %+v", hits)
	}
}

// A null default (type anything) stays one through the console's writes; with no
// default key, each machine must set the option.
func TestFleetModuleNullDefault(t *testing.T) {
	s := newService(t)
	must[FleetModule](t)(s.CreateModule("m", ""))
	write(t, s.Repo, "modules/fleet/m/managed.json", `{"options": {"extra": {"type": "anything", "default": null}, "need": {"type": "str"}}, "packages": [], "settings": {}}`)
	must[ManagedView](t)(s.AddPackages("module:m", []string{"git"}))
	content, _, _ := s.Read("modules/fleet/m/managed.json")
	var doc struct{ Options map[string]map[string]any }
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		t.Fatal(err)
	}
	if v, ok := doc.Options["extra"]["default"]; !ok || v != nil {
		t.Fatalf("the null default went:\n%s", content)
	}
	if _, ok := doc.Options["need"]["default"]; ok {
		t.Fatalf("an option without a default got one:\n%s", content)
	}
	// Over the API too, and only for anything.
	var e ModuleEdit
	if err := json.Unmarshal([]byte(`{"options": {"x": {"type": "anything", "default": null}, "y": null}}`), &e); err != nil {
		t.Fatal(err)
	}
	if !e.Options["x"].nullDefault || e.Options["y"] != nil {
		t.Fatalf("decoded %+v", e.Options)
	}
	must[FleetModule](t)(s.EditModule("m", e))
	e = ModuleEdit{}
	if err := json.Unmarshal([]byte(`{"options": {"s": {"type": "str", "default": null}}}`), &e); err != nil {
		t.Fatal(err)
	}
	_, err := s.EditModule("m", e)
	isErr(t, err, ErrInvalid)
	if err := json.Unmarshal([]byte(`{"type": "str", "dflt": 1}`), &ModuleOption{}); err == nil {
		t.Fatal("an unknown field accepted")
	}
}
