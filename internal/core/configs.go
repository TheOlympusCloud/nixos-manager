package core

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

// App configs and keyring entries in homes (managed.nix 6): for apps that
// rewrite their own settings, where a file home-manager links would break them.
// modules/managed-configs.py writes them on the machine, as the user:
//
//	"home": { "pablo": {
//	  "configs": { ".config/supersonic/config.toml": {
//	    "format": "toml", "mode": "merge", "process": "supersonic",
//	    "settings": { "Servers[ID=8f2c…]": { "Hostname": "https://music.example" } } } },
//	  "keyring": [ { "label": "Password for '8f2c…' on 'supersonic'",
//	    "attributes": { "service": "supersonic", "username": "8f2c…" }, "secret": "music-password" } ] } }
//
// managed.json holds a secret's name, never its value, which the machine reads
// from /run/secrets. A standalone home has configs at the top, without secrets.

// HomeConfig is an app's settings file in a home, which the app also writes.
type HomeConfig struct {
	Format   string         `json:"format" jsonschema:"json, toml, yaml, ini or properties (Java)"`
	Mode     string         `json:"mode,omitempty" jsonschema:"merge (the default): the declared keys at every activation and login, every other key the app wrote stays; seed: the whole file, only when there is none"`
	Settings map[string]any `json:"settings,omitempty" jsonschema:"the declared keys, nested as in the file; an INI file is {section: {key: value}}, a properties file {key: value}"`
	Secrets  map[string]any `json:"secrets,omitempty" jsonschema:"the shape of settings, with the name of one of the machine's secrets where its value goes"`
	Process  string         `json:"process,omitempty" jsonschema:"text in the app's command line: while such a process of the user runs, the file is left alone (the app would write over it) until the next activation or login"`
	Lock     string         `json:"lock,omitempty" jsonschema:"a lock folder the app takes while it writes (mkdir), a path in ~ such as .claude/settings.json.lock: held while the file is written"`
}

// KeyringEntry is an item stored in the user's keyring (Secret Service, GNOME
// Keyring) once the graphical session starts, for an app that reads it there.
type KeyringEntry struct {
	Label      string            `json:"label" jsonschema:"the item's label, as the app names it"`
	Attributes map[string]string `json:"attributes" jsonschema:"the attributes the app looks the item up by, e.g. service and username"`
	Secret     string            `json:"secret" jsonschema:"one of the machine's secrets: the item's value"`
}

var (
	configFormats = []string{"json", "toml", "yaml", "ini", "properties"}
	// A key that is one item of a list: Servers[ID=8f2c…].
	listItemKey = regexp.MustCompile(`^(.+)\[([^=\[\]]+)=([^\[\]]*)\]$`)
)

func hasControl(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

func scalar(v any) bool {
	switch v.(type) {
	case string, bool, json.Number, float64, int:
		return true
	}
	return false
}

// checkConfig validates a config as managed-configs.py will read it.
func checkConfig(p string, c HomeConfig, secrets bool) error {
	if err := checkHomePath(p); err != nil {
		return err
	}
	what := "~/" + p
	if !slices.Contains(configFormats, c.Format) {
		return errorf(ErrInvalid, "%s: the format is one of %s", what, strings.Join(configFormats, ", "))
	}
	if c.Mode != "" && c.Mode != "merge" && c.Mode != "seed" {
		return errorf(ErrInvalid, "%s: the mode is merge or seed", what)
	}
	if hasControl(c.Process) || len(c.Process) > 200 {
		return errorf(ErrInvalid, "%s: invalid process", what)
	}
	if c.Lock != "" {
		if err := checkHomePath(c.Lock); err != nil {
			return err
		}
	}
	if len(c.Secrets) > 0 && !secrets {
		return errorf(ErrInvalid, "%s: a standalone home gets no machine's secrets, so its configs cannot hold any; put them in a NixOS machine's home", what)
	}
	// INI has sections of keys, properties keys only; the others nest freely.
	depth := map[string]int{"ini": 2, "properties": 1}[c.Format]
	if err := checkTree(what, c.Format, c.Settings, nil, depth, func(v any) bool { return scalar(v) || depth == 0 && noNull(v) }); err != nil {
		return err
	}
	if err := checkTree(what, c.Format, c.Secrets, nil, depth, func(v any) bool { s, ok := v.(string); return ok && secretName.MatchString(s) }); err != nil {
		return err
	}
	return bothSet(what, c.Settings, c.Secrets, nil)
}

// noNull reports whether v holds no null, however deep: TOML has none, and a
// list or object of the others would write one where the app may not expect it.
func noNull(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case []any:
		return !slices.ContainsFunc(x, func(e any) bool { return !noNull(e) })
	case map[string]any:
		for _, e := range x {
			if !noNull(e) {
				return false
			}
		}
	}
	return true
}

// checkTree checks the keys of a settings or secrets tree and its values, with
// leaf. depth > 0 is how deep values are, as INI's and properties' are.
func checkTree(what, format string, t map[string]any, at []string, depth int, leaf func(any) bool) error {
	for k, v := range t {
		p := append(slices.Clone(at), k)
		name := strings.Join(p, ".")
		if k == "" || hasControl(k) {
			return errorf(ErrInvalid, "%s: invalid key %q", what, name)
		}
		item := listItemKey.MatchString(k)
		if strings.HasSuffix(k, "]") && !item {
			return errorf(ErrInvalid, "%s: %s: a key that ends in ] is a list item, Name[Field=Value]", what, name)
		}
		if item && depth > 0 {
			return errorf(ErrInvalid, "%s: %s: a %s file has no lists", what, name, format)
		}
		sub, isMap := v.(map[string]any)
		switch {
		case isMap && (depth == 0 || len(p) < depth):
			if err := checkTree(what, format, sub, p, depth, leaf); err != nil {
				return err
			}
		case item:
			return errorf(ErrInvalid, "%s: %s is a list item, so its value is an object of its keys", what, name)
		case depth > 0 && len(p) < depth:
			return errorf(ErrInvalid, "%s: %s is an INI section, so its value is an object of its keys", what, name)
		case !leaf(v):
			return errorf(ErrInvalid, "%s: %s: invalid value", what, name)
		}
	}
	return nil
}

// bothSet refuses a key that is both a setting and a secret.
func bothSet(what string, settings, secrets map[string]any, at []string) error {
	for k, sv := range secrets {
		v, ok := settings[k]
		if !ok {
			continue
		}
		p := append(slices.Clone(at), k)
		sm, sObj := sv.(map[string]any)
		m, obj := v.(map[string]any)
		if !sObj || !obj {
			return errorf(ErrInvalid, "%s: %s is both a setting and a secret", what, strings.Join(p, "."))
		}
		if err := bothSet(what, m, sm, p); err != nil {
			return err
		}
	}
	return nil
}

func checkKeyring(entries []KeyringEntry) error {
	for i, e := range entries {
		what := fmt.Sprintf("keyring entry %q", e.Label)
		switch {
		case e.Label == "" || hasControl(e.Label) || len(e.Label) > 200:
			return errorf(ErrInvalid, "keyring entry %d: invalid label", i+1)
		case len(e.Attributes) == 0 || len(e.Attributes) > 32:
			return errorf(ErrInvalid, "%s: it needs the attributes the app looks it up by", what)
		case !secretName.MatchString(e.Secret):
			return errorf(ErrInvalid, "%s: invalid secret name %q", what, e.Secret)
		}
		for k, v := range e.Attributes {
			if k == "" || hasControl(k) || hasControl(v) {
				return errorf(ErrInvalid, "%s: invalid attribute %q", what, k)
			}
		}
		for _, o := range entries[:i] {
			if maps.Equal(o.Attributes, e.Attributes) {
				return errorf(ErrInvalid, "%s: another keyring entry has the same attributes", what)
			}
		}
	}
	return nil
}

// editConfigs applies fn to the configs of user's home on a machine (or
// modules/managed.json), or of a standalone home, where user is "".
func (s *Service) editConfigs(host, user string, fn func(c map[string]HomeConfig) error) (ManagedView, error) {
	standalone := strings.HasPrefix(host, HomePrefix)
	switch {
	case standalone && user != "":
		return ManagedView{}, errorf(ErrInvalid, "a standalone home has its configs at the top: no user")
	case !standalone && !userName.MatchString(user):
		return ManagedView{}, errorf(ErrInvalid, "invalid user name %q", user)
	}
	return s.editManaged(host, nil, func(v *ManagedView) error {
		if standalone {
			if v.Configs == nil {
				v.Configs = map[string]HomeConfig{}
			}
			return fn(v.Configs)
		}
		h, known := v.Home[user]
		if h.Configs == nil {
			h.Configs = map[string]HomeConfig{}
		}
		if err := fn(h.Configs); err != nil {
			return err
		}
		if len(h.Configs) == 0 {
			h.Configs = nil
		}
		if known || h.Configs != nil { // as EditPackages: the first config adds a user with home-manager by hand
			v.Home[user] = h
		}
		return nil
	})
}

// HomeConfigEdit adds an app's config file to a home or changes how it is
// written, keeping its keys; with Remove, it is no longer written.
type HomeConfigEdit struct {
	User    string `json:"user,omitempty" jsonschema:"a home-manager user of the machine; none for a standalone home (home:<name>)"`
	Path    string `json:"path" jsonschema:"the file in ~, e.g. .config/supersonic/config.toml"`
	Format  string `json:"format,omitempty" jsonschema:"json, toml, yaml, ini or properties (Java)"`
	Mode    string `json:"mode,omitempty" jsonschema:"merge (the default): the declared keys at every activation and login, every other key the app wrote stays; seed: the whole file, only when there is none"`
	Process string `json:"process,omitempty" jsonschema:"text in the app's command line: while such a process of the user runs, the file is left alone until the next activation or login, since the app would write over it on exit"`
	Lock    string `json:"lock,omitempty" jsonschema:"a lock folder the app takes while it writes (mkdir), a path in ~ such as .claude/settings.json.lock: held while the file is written"`
	Remove  bool   `json:"remove,omitempty" jsonschema:"stop writing the file; the file stays on the machine as it is"`
}

// SetHomeConfig applies a HomeConfigEdit.
func (s *Service) SetHomeConfig(host string, e HomeConfigEdit) (ManagedView, error) {
	return s.editConfigs(host, e.User, func(c map[string]HomeConfig) error {
		if e.Remove {
			delete(c, e.Path)
			return nil
		}
		cur := c[e.Path]
		cur.Format, cur.Mode, cur.Process, cur.Lock = e.Format, e.Mode, e.Process, e.Lock
		c[e.Path] = cur
		return nil
	})
}

// HomeConfigKeyEdit sets one key of a home's app config to a value or to one
// of the machine's secrets, or with Unset, takes it out.
type HomeConfigKeyEdit struct {
	User   string   `json:"user,omitempty" jsonschema:"a home-manager user of the machine; none for a standalone home (home:<name>)"`
	Path   string   `json:"path" jsonschema:"the config's file in ~, added first with set_home_config"`
	Key    []string `json:"key" jsonschema:"the key's path in the file, part by part: [\"Application\", \"EnableSystemTray\"]; [\"Servers[ID=8f2c…]\", \"Hostname\"] is the key Hostname of the item of the list Servers whose ID is 8f2c…, added when missing; an INI key is [section, key], a properties key is one part, dots included"`
	Value  any      `json:"value,omitempty" jsonschema:"its value: boolean, number, string, list (replaced whole) or object; it is copied to the Nix store, so never a secret"`
	Secret string   `json:"secret,omitempty" jsonschema:"instead of value, one of the machine's secrets (host_details' secrets, from set_secret): its value goes there, read on the machine and never stored in the repository unencrypted or in the Nix store"`
	Unset  bool     `json:"unset,omitempty"`
}

// EditHomeConfigKey applies a HomeConfigKeyEdit.
func (s *Service) EditHomeConfigKey(host string, e HomeConfigKeyEdit) (ManagedView, error) {
	if err := checkPath(e.Key); err != nil {
		return ManagedView{}, err
	}
	if !e.Unset && (e.Value == nil) == (e.Secret == "") {
		return ManagedView{}, errorf(ErrInvalid, "give a value or a secret")
	}
	return s.editConfigs(host, e.User, func(c map[string]HomeConfig) error {
		cur, ok := c[e.Path]
		if !ok {
			return errorf(ErrNotFound, "~/%s is not one of the home's configs: add it first (its format)", e.Path)
		}
		cur.Settings, cur.Secrets = orMap(cur.Settings), orMap(cur.Secrets)
		unset(cur.Settings, e.Key)
		unset(cur.Secrets, e.Key)
		var err error
		switch {
		case e.Unset:
		case e.Secret != "":
			err = setIn(cur.Secrets, e.Key, e.Secret)
		default:
			err = setIn(cur.Settings, e.Key, e.Value)
		}
		if len(cur.Settings) == 0 {
			cur.Settings = nil
		}
		if len(cur.Secrets) == 0 {
			cur.Secrets = nil
		}
		c[e.Path] = cur
		return err
	})
}

func orMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// KeyringEdit adds a keyring entry to a user's home, or replaces the one with
// the same attributes (or Was); with Remove, the one with those attributes goes.
type KeyringEdit struct {
	User string `json:"user" jsonschema:"a home-manager user of the machine"`
	KeyringEntry
	Was    map[string]string `json:"was,omitempty" jsonschema:"the attributes of the entry this one replaces, when they change"`
	Remove bool              `json:"remove,omitempty"`
}

// EditKeyring applies a KeyringEdit. The item already in a machine's keyring stays there.
func (s *Service) EditKeyring(host string, e KeyringEdit) (ManagedView, error) {
	if strings.HasPrefix(host, HomePrefix) {
		return ManagedView{}, errorf(ErrInvalid, "keyring entries hold a machine's secret, which a standalone home does not get")
	}
	if !userName.MatchString(e.User) {
		return ManagedView{}, errorf(ErrInvalid, "invalid user name %q", e.User)
	}
	return s.editManaged(host, nil, func(v *ManagedView) error {
		h, known := v.Home[e.User]
		old := e.Was
		if old == nil {
			old = e.Attributes
		}
		at := slices.IndexFunc(h.Keyring, func(x KeyringEntry) bool { return maps.Equal(x.Attributes, old) })
		switch {
		case e.Remove && at < 0:
			return errorf(ErrNotFound, "%s's home has no keyring entry with those attributes", e.User)
		case e.Remove:
			h.Keyring = slices.Delete(h.Keyring, at, at+1)
		case at < 0 && e.Was != nil:
			return errorf(ErrNotFound, "%s's home has no keyring entry with the attributes it replaces", e.User)
		case at < 0:
			h.Keyring = append(h.Keyring, e.KeyringEntry)
		default:
			h.Keyring = slices.Clone(h.Keyring)
			h.Keyring[at] = e.KeyringEntry
		}
		if len(h.Keyring) == 0 {
			h.Keyring = nil
		}
		if known || h.Keyring != nil {
			v.Home[e.User] = h
		}
		return nil
	})
}

// joinConfigs adds src's configs and keyring entries to dst's, for a copy: a
// config at a path dst has, or an entry with attributes it has, must be the same.
func joinConfigs(user string, dst *HomeManaged, src HomeManaged, conflicts *[]string) bool {
	changed := false
	for p, c := range src.Configs {
		have, ok := dst.Configs[p]
		switch {
		case !ok:
			if dst.Configs == nil {
				dst.Configs = map[string]HomeConfig{}
			}
			dst.Configs[p], changed = c, true
		case !reflect.DeepEqual(have, c):
			*conflicts = append(*conflicts, "home."+user+".configs: "+p)
		}
	}
	for _, e := range src.Keyring {
		at := slices.IndexFunc(dst.Keyring, func(x KeyringEntry) bool { return maps.Equal(x.Attributes, e.Attributes) })
		switch {
		case at < 0:
			dst.Keyring, changed = append(dst.Keyring, e), true
		case !reflect.DeepEqual(dst.Keyring[at], e):
			*conflicts = append(*conflicts, "home."+user+".keyring: "+e.Label)
		}
	}
	return changed
}
