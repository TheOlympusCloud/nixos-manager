package core

import (
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/TheOlympusCloud/nixos-manager/templates"
)

// An app config is added with its format, then edited key by key: a value, a
// secret's name in place of a value, a list item by its key field. Keys the
// file had stay, and taking the last key out leaves the file's entry.
func TestHomeConfigs(t *testing.T) {
	s := newService(t)
	edit := must[ManagedView](t)
	p := ".config/supersonic/config.toml"
	edit(s.SetHomeConfig("web", HomeConfigEdit{User: "pablo", Path: p, Format: "toml", Process: "supersonic"}))
	edit(s.EditHomeConfigKey("web", HomeConfigKeyEdit{User: "pablo", Path: p, Key: []string{"Application", "EnableSystemTray"}, Value: true}))
	edit(s.EditHomeConfigKey("web", HomeConfigKeyEdit{User: "pablo", Path: p, Key: []string{"Servers[ID=a1]", "Hostname"}, Value: "https://music.example"}))
	v := edit(s.EditHomeConfigKey("web", HomeConfigKeyEdit{User: "pablo", Path: p, Key: []string{"Servers[ID=a1]", "Password"}, Secret: "music"}))
	want := HomeConfig{Format: "toml", Process: "supersonic",
		Settings: map[string]any{"Application": map[string]any{"EnableSystemTray": true}, "Servers[ID=a1]": map[string]any{"Hostname": "https://music.example"}},
		Secrets:  map[string]any{"Servers[ID=a1]": map[string]any{"Password": "music"}},
	}
	if !reflect.DeepEqual(v.Home["pablo"].Configs[p], want) {
		t.Fatalf("got %+v\nwant %+v", v.Home["pablo"].Configs[p], want)
	}
	if got := must[ManagedView](t)(s.ManagedView("web")); !reflect.DeepEqual(got.Home["pablo"].Configs[p], want) {
		t.Fatalf("read back %+v", got.Home["pablo"].Configs[p])
	}
	// The same key as a value again: the secret goes.
	v = edit(s.EditHomeConfigKey("web", HomeConfigKeyEdit{User: "pablo", Path: p, Key: []string{"Servers[ID=a1]", "Password"}, Value: "plain"}))
	if c := v.Home["pablo"].Configs[p]; c.Secrets != nil || c.Settings["Servers[ID=a1]"].(map[string]any)["Password"] != "plain" {
		t.Fatalf("value over secret %+v", c)
	}
	// Changing how it is written keeps the keys.
	v = edit(s.SetHomeConfig("web", HomeConfigEdit{User: "pablo", Path: p, Format: "toml", Mode: "seed"}))
	if c := v.Home["pablo"].Configs[p]; c.Mode != "seed" || c.Process != "" || len(c.Settings) != 2 {
		t.Fatalf("after mode %+v", c)
	}
	for _, k := range [][]string{{"Application", "EnableSystemTray"}, {"Servers[ID=a1]", "Hostname"}, {"Servers[ID=a1]", "Password"}} {
		v = edit(s.EditHomeConfigKey("web", HomeConfigKeyEdit{User: "pablo", Path: p, Key: k, Unset: true}))
	}
	if c, ok := v.Home["pablo"].Configs[p]; !ok || c.Settings != nil || c.Secrets != nil {
		t.Fatalf("after unsetting every key %+v", v.Home["pablo"].Configs)
	}
	if c, _, _ := s.Read("hosts/web/managed.json"); strings.Contains(c, "Application") || strings.Contains(c, `"secrets"`) || !strings.Contains(c, `"format": "toml"`) {
		t.Fatalf("file:\n%s", c)
	}
	// The first config added pablo's home, which stays, as a linked secret's does.
	edit(s.SetHomeConfig("web", HomeConfigEdit{User: "pablo", Path: p, Remove: true}))
	if c, _, _ := s.Read("hosts/web/managed.json"); strings.Contains(c, "configs") || !strings.Contains(c, `"pablo"`) {
		t.Fatalf("after removing it:\n%s", c)
	}
	_, err := s.EditHomeConfigKey("web", HomeConfigKeyEdit{User: "pablo", Path: p, Key: []string{"a"}, Value: 1})
	isErr(t, err, ErrNotFound)
}

func TestHomeConfigValidation(t *testing.T) {
	s := newService(t)
	edit := must[ManagedView](t)
	edit(s.SetHomeConfig("web", HomeConfigEdit{User: "pablo", Path: "a.ini", Format: "ini"}))
	edit(s.SetHomeConfig("web", HomeConfigEdit{User: "pablo", Path: "a.properties", Format: "properties"}))
	edit(s.SetHomeConfig("web", HomeConfigEdit{User: "pablo", Path: "a.json", Format: "json"}))
	edit(s.EditHomeConfigKey("web", HomeConfigKeyEdit{User: "pablo", Path: "a.ini", Key: []string{"General", "size"}, Value: 3}))
	edit(s.EditHomeConfigKey("web", HomeConfigKeyEdit{User: "pablo", Path: "a.properties", Key: []string{"leak.until"}, Value: "1"}))
	edit(s.EditHomeConfigKey("web", HomeConfigKeyEdit{User: "pablo", Path: "a.json", Key: []string{"list"}, Value: []any{1, "a"}}))
	for _, bad := range []HomeConfigEdit{
		{User: "pablo", Path: "/etc/passwd", Format: "json"}, {User: "pablo", Path: "../x", Format: "json"},
		{User: "pablo", Path: "a", Format: "xml"}, {User: "pablo", Path: "a", Format: "json", Mode: "replace"},
		{User: "pablo", Path: "a", Format: "json", Lock: "/tmp/x"}, {User: "pablo", Path: "a", Format: "json", Process: "a\nb"},
		{User: "Pablo", Path: "a", Format: "json"}, {Path: "a", Format: "json"},
	} {
		_, err := s.SetHomeConfig("web", bad)
		isErr(t, err, ErrInvalid)
	}
	for _, bad := range []HomeConfigKeyEdit{
		{Path: "a.ini", Key: []string{"size"}, Value: 3},                         // a key outside any section
		{Path: "a.ini", Key: []string{"General", "x", "y"}, Value: 3},            // deeper than a section's keys
		{Path: "a.ini", Key: []string{"General", "list"}, Value: []any{1}},       // INI has no lists
		{Path: "a.properties", Key: []string{"a", "b"}, Value: "1"},              // properties are flat
		{Path: "a.json", Key: []string{"servers[id=1]"}, Value: 1},               // a list item is an object
		{Path: "a.json", Key: []string{"servers[id]", "a"}, Value: 1},            // not Name[Field=Value]
		{Path: "a.json", Key: []string{"list", "x"}, Value: 1},                   // list already has a value
		{Path: "a.json", Key: []string{"token"}, Secret: "not a name"},           // a secret's name
		{Path: "a.json", Key: []string{"token"}, Value: "v", Secret: "token"},    // both
		{Path: "a.json", Key: []string{"token"}},                                 // neither
		{Path: "a.json", Key: []string{"a\nb"}, Value: 1},                        // control character
		{Path: "a.json", Key: []string{}, Value: 1},                              // no key
		{Path: "a.ini", Key: []string{"General", "pw"}, Secret: "pw", Value: ""}, // value "" and a secret
	} {
		bad.User = "pablo"
		if _, err := s.EditHomeConfigKey("web", bad); err == nil {
			t.Errorf("%+v was accepted", bad)
		}
	}
	// A path linked as a secret, a key both set and secret, two keyring entries with the
	// same attributes: written by hand, each is refused when read.
	for _, doc := range []string{
		`{"home": {"pablo": {"packages": [], "removedPackages": [], "secrets": {".netrc": "n"}, "configs": {".netrc": {"format": "ini"}}}}}`,
		`{"home": {"pablo": {"packages": [], "removedPackages": [], "configs": {"a.json": {"format": "json", "settings": {"a": {"b": 1}}, "secrets": {"a": {"b": "s"}}}}}}}`,
		`{"home": {"pablo": {"packages": [], "removedPackages": [], "configs": {"a.json": {"format": "json", "settings": {"a": null}}}}}}`,
		`{"home": {"pablo": {"packages": [], "removedPackages": [], "keyring": [{"label": "a", "attributes": {"s": "x"}, "secret": "s"}, {"label": "b", "attributes": {"s": "x"}, "secret": "t"}]}}}`,
		`{"home": {"pablo": {"packages": [], "removedPackages": [], "keyring": [{"label": "a", "attributes": {}, "secret": "s"}]}}}`,
		`{"home": {"pablo": {"packages": [], "removedPackages": [], "configs": {"a.json": {"format": "json", "unknown": 1}}}}}`,
		`{"packages": [], "configs": {"a.json": {"format": "json"}}}`,
	} {
		write(t, s.Repo, "hosts/db/managed.json", doc)
		if _, err := s.ManagedView("db"); err == nil {
			t.Errorf("accepted %s", doc)
		}
	}
}

func TestKeyringEntries(t *testing.T) {
	s := newService(t)
	edit := must[ManagedView](t)
	a := KeyringEntry{Label: "Password for 'a1' on 'supersonic'", Attributes: map[string]string{"service": "supersonic", "username": "a1"}, Secret: "music"}
	edit(s.EditKeyring("web", KeyringEdit{User: "pablo", KeyringEntry: a}))
	b := a
	b.Secret = "music2"
	v := edit(s.EditKeyring("web", KeyringEdit{User: "pablo", KeyringEntry: b})) // same attributes: replaced
	if got := v.Home["pablo"].Keyring; !reflect.DeepEqual(got, []KeyringEntry{b}) {
		t.Fatalf("got %+v", got)
	}
	c := b
	c.Attributes = map[string]string{"service": "supersonic", "username": "b2"}
	v = edit(s.EditKeyring("web", KeyringEdit{User: "pablo", KeyringEntry: c, Was: a.Attributes}))
	if got := v.Home["pablo"].Keyring; !reflect.DeepEqual(got, []KeyringEntry{c}) {
		t.Fatalf("after changing its attributes %+v", got)
	}
	edit(s.EditKeyring(SharedHost, KeyringEdit{User: "pablo", KeyringEntry: a}))
	if c, _, _ := s.Read("modules/managed.json"); !strings.Contains(c, `"username": "a1"`) {
		t.Fatalf("shared:\n%s", c)
	}
	_, err := s.EditKeyring("web", KeyringEdit{User: "pablo", KeyringEntry: KeyringEntry{Attributes: a.Attributes}, Remove: true})
	isErr(t, err, ErrNotFound)
	edit(s.EditKeyring("web", KeyringEdit{User: "pablo", KeyringEntry: KeyringEntry{Attributes: c.Attributes}, Remove: true}))
	if c, _, _ := s.Read("hosts/web/managed.json"); strings.Contains(c, "keyring") {
		t.Fatalf("after removing the last:\n%s", c)
	}
	for _, bad := range []KeyringEntry{
		{Label: "", Attributes: a.Attributes, Secret: "x"}, {Label: "a\nb", Attributes: a.Attributes, Secret: "x"},
		{Label: "a", Secret: "x"}, {Label: "a", Attributes: a.Attributes, Secret: "a b"},
		{Label: "a", Attributes: map[string]string{"": "x"}, Secret: "x"},
	} {
		_, err := s.EditKeyring("web", KeyringEdit{User: "pablo", KeyringEntry: bad})
		isErr(t, err, ErrInvalid)
	}
	_, err = s.EditKeyring(HomePrefix+"pablo@laptop", KeyringEdit{User: "pablo", KeyringEntry: a})
	isErr(t, err, ErrInvalid)
}

// A standalone home has configs at the top, without secrets.
func TestStandaloneHomeConfigs(t *testing.T) {
	s := newService(t)
	h := HomePrefix + "pablo@laptop"
	v := must[ManagedView](t)(s.SetHomeConfig(h, HomeConfigEdit{Path: "a.json", Format: "json"}))
	v = must[ManagedView](t)(s.EditHomeConfigKey(h, HomeConfigKeyEdit{Path: "a.json", Key: []string{"theme"}, Value: "dark"}))
	if !reflect.DeepEqual(v.Configs["a.json"], HomeConfig{Format: "json", Settings: map[string]any{"theme": "dark"}}) {
		t.Fatalf("got %+v", v.Configs)
	}
	_, err := s.EditHomeConfigKey(h, HomeConfigKeyEdit{Path: "a.json", Key: []string{"token"}, Secret: "token"})
	isErr(t, err, ErrInvalid)
	_, err = s.SetHomeConfig(h, HomeConfigEdit{User: "pablo", Path: "b.json", Format: "json"})
	isErr(t, err, ErrInvalid)
}

// Copying a home brings its configs and keyring entries along, keeps the target's
// own, and refuses a different config at the same path or entry with the same attributes.
func TestCopyHomeConfigs(t *testing.T) {
	s := newService(t)
	edit := must[ManagedView](t)
	k := KeyringEntry{Label: "pw", Attributes: map[string]string{"service": "a"}, Secret: "pw"}
	for _, h := range []string{"web", "srv", "db"} {
		edit(s.SetHome(h, HomeEdit{User: "pablo", Enable: true, StateVersion: "26.05"}))
	}
	edit(s.SetHomeConfig("web", HomeConfigEdit{User: "pablo", Path: "a.json", Format: "json"}))
	edit(s.EditHomeConfigKey("web", HomeConfigKeyEdit{User: "pablo", Path: "a.json", Key: []string{"x"}, Value: 1}))
	edit(s.EditKeyring("web", KeyringEdit{User: "pablo", KeyringEntry: k}))
	edit(s.SetHomeConfig("srv", HomeConfigEdit{User: "pablo", Path: "own.json", Format: "json"}))
	edit(s.SetHomeConfig("db", HomeConfigEdit{User: "pablo", Path: "a.json", Format: "yaml"}))
	got := must[[]CopyResult](t)(s.Copy("web", CopyRequest{Home: "pablo", To: []string{"srv", "db"}}))
	want := []CopyResult{{Host: "srv", Changed: true}, {Host: "db", Conflicts: []string{"home.pablo.configs: a.json"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("results %+v", got)
	}
	srv := must[ManagedView](t)(s.ManagedView("srv")).Home["pablo"]
	if len(srv.Configs) != 2 || srv.Configs["a.json"].Settings["x"] == nil || !reflect.DeepEqual(srv.Keyring, []KeyringEntry{k}) {
		t.Fatalf("srv %+v", srv)
	}
}

// Updating managed.nix brings the writer it runs on the machines along.
func TestUpdateManagedModuleWritesTheWriter(t *testing.T) {
	s := newService(t)
	write(t, s.Repo, "modules/managed-configs.py", "# an older one\n")
	must[string](t)(s.UpdateManagedModule("", false))
	want, _ := fs.ReadFile(templates.Fleet, "modules/managed-configs.py")
	if c, _, _ := s.Read("modules/managed-configs.py"); c != string(want) || !strings.Contains(c, "def configs(") {
		t.Fatalf("not the template:\n%s", c)
	}
	_, cur, _ := s.Read("modules/managed-home.nix")
	must[string](t)(s.UpdateManagedModule(cur, true))
}
