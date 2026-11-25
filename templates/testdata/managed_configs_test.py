"""Tests of fleet/modules/managed-configs.py, the writer of app configs in homes.

Run by TestManagedConfigsWriter (templates/configs_test.go) with a Python that
has tomlkit, ruamel.yaml and secretstorage, as managed.nix gives the writer.
"""

import contextlib
import importlib.util
import io
import json
import os
import subprocess
import sys
import tempfile
import time
import unittest

# No __pycache__ next to the writer: the fleet template is embedded as it is.
sys.dont_write_bytecode = True
HERE = os.path.dirname(os.path.abspath(__file__))
spec = importlib.util.spec_from_file_location("mc", os.path.join(HERE, "..", "fleet", "modules", "managed-configs.py"))
mc = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mc)

SECRET = "s3cr3t-value-never-logged"


class Home(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.home = self.tmp.name
        os.environ["HOME"] = self.home
        self.secret = os.path.join(self.home, "run-secret")
        with open(self.secret, "w") as f:
            f.write(SECRET + "\n")
        mc.LOCK_WAIT = 0.3

    def tearDown(self):
        self.tmp.cleanup()

    def path(self, rel):
        return os.path.join(self.home, rel)

    def write(self, rel, text, mode=0o644):
        p = self.path(rel)
        os.makedirs(os.path.dirname(p), exist_ok=True)
        with open(p, "w") as f:
            f.write(text)
        os.chmod(p, mode)

    def read(self, rel):
        with open(self.path(rel)) as f:
            return f.read()

    def run_configs(self, configs):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = mc.configs({"configs": configs})
        self.assertNotIn(SECRET, out.getvalue())
        return code, out.getvalue()


class JsonTest(Home):
    def test_merge_keeps_the_apps_keys(self):
        self.write("a.json", json.dumps({"app": 1, "ui": {"theme": "dark", "size": 3}, "list": [1, 2]}, indent=4) + "\n")
        code, out = self.run_configs({"a.json": {"format": "json", "settings": {"ui": {"theme": "light"}, "list": [9], "new": True}}})
        self.assertEqual(code, 0)
        self.assertEqual(json.loads(self.read("a.json")), {"app": 1, "ui": {"theme": "light", "size": 3}, "list": [9], "new": True})
        self.assertTrue(self.read("a.json").startswith('{\n    "app"'), "keeps the app's indent")
        self.assertIn("~/a.json: written", out)

    def test_unchanged_file_is_not_written(self):
        self.write("a.json", '{"ui": {"theme": "light"}, "x": 1}')
        os.utime(self.path("a.json"), (1000, 1000))
        code, out = self.run_configs({"a.json": {"format": "json", "settings": {"ui": {"theme": "light"}}}})
        self.assertEqual((code, out), (0, ""))
        self.assertEqual(os.stat(self.path("a.json")).st_mtime, 1000)

    def test_list_item_by_key_field(self):
        self.write("a.json", json.dumps({"servers": [{"id": "a", "host": "old", "keep": 1}, {"id": "b"}]}))
        self.run_configs({"a.json": {"format": "json", "settings": {"servers[id=a]": {"host": "new"}, "servers[id=c]": {"host": "c"}}}})
        self.assertEqual(json.loads(self.read("a.json"))["servers"], [{"id": "a", "host": "new", "keep": 1}, {"id": "b"}, {"id": "c", "host": "c"}])

    def test_secret_goes_in_and_the_mode_closes(self):
        self.write("a.json", "{}\n", 0o664)
        self.run_configs({"a.json": {"format": "json", "settings": {"user": "me"}, "secrets": {"auth": {"token": self.secret}}}})
        self.assertEqual(json.loads(self.read("a.json")), {"user": "me", "auth": {"token": SECRET}})
        self.assertEqual(os.stat(self.path("a.json")).st_mode & 0o777, 0o600)

    def test_new_file_with_a_secret_is_0600_and_without_0644(self):
        self.run_configs({
            ".config/x/s.json": {"format": "json", "secrets": {"token": self.secret}},
            ".config/x/p.json": {"format": "json", "settings": {"a": 1}},
        })
        self.assertEqual(os.stat(self.path(".config/x/s.json")).st_mode & 0o777, 0o600)
        self.assertEqual(os.stat(self.path(".config/x/p.json")).st_mode & 0o777, 0o644)

    def test_mode_is_kept(self):
        self.write("a.json", "{}", 0o640)
        self.run_configs({"a.json": {"format": "json", "settings": {"a": 1}}})
        self.assertEqual(os.stat(self.path("a.json")).st_mode & 0o777, 0o640)

    def test_link_is_replaced_by_a_file(self):
        self.write("store.json", '{"a": 0, "b": 2}', 0o444)
        os.symlink(self.path("store.json"), self.path("a.json"))
        self.run_configs({"a.json": {"format": "json", "settings": {"a": 1}}})
        self.assertFalse(os.path.islink(self.path("a.json")))
        self.assertEqual(json.loads(self.read("a.json")), {"a": 1, "b": 2})
        self.assertEqual(os.stat(self.path("a.json")).st_mode & 0o777, 0o644)

    def test_unparseable_file_is_left_alone(self):
        bad = '{"token": "%s", broken' % SECRET
        self.write("a.json", bad)
        code, out = self.run_configs({"a.json": {"format": "json", "settings": {"a": 1}}})
        self.assertEqual(code, 0)
        self.assertEqual(self.read("a.json"), bad)
        self.assertIn("~/a.json: left as it is: it is not valid JSON (line 1", out)

    def test_seed_writes_only_a_missing_file(self):
        conf = {"format": "json", "mode": "seed", "settings": {"a": 1, "s[id=x]": {"v": 2}}}
        self.run_configs({"a.json": conf})
        self.assertEqual(json.loads(self.read("a.json")), {"a": 1, "s": [{"id": "x", "v": 2}]})
        self.write("a.json", '{"a": 5}')
        self.run_configs({"a.json": conf})
        self.assertEqual(self.read("a.json"), '{"a": 5}')

    def test_missing_secret_fails_and_leaves_the_file(self):
        self.write("a.json", "{}")
        code, out = self.run_configs({"a.json": {"format": "json", "secrets": {"t": self.path("nope")}}})
        self.assertEqual(code, 1)
        self.assertIn("failed: No such file or directory", out)
        self.assertEqual(self.read("a.json"), "{}")

    def test_waits_while_the_app_runs(self):
        marker = "nixmgr-fake-app-%d" % os.getpid()
        app = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(30)", marker])
        try:
            self.write("a.json", '{"a": 0}')
            code, out = self.run_configs({"a.json": {"format": "json", "settings": {"a": 1}, "process": marker}})
            self.assertEqual(code, 0)
            self.assertIn("deferred: %s is running" % marker, out)
            self.assertEqual(self.read("a.json"), '{"a": 0}')
        finally:
            app.kill()
            app.wait()
        self.run_configs({"a.json": {"format": "json", "settings": {"a": 1}, "process": marker}})
        self.assertEqual(json.loads(self.read("a.json")), {"a": 1})

    def test_takes_the_apps_lock(self):
        self.write("a.json", '{"a": 0}')
        os.mkdir(self.path("a.json.lock"))
        code, out = self.run_configs({"a.json": {"format": "json", "settings": {"a": 1}, "lock": "a.json.lock"}})
        self.assertIn("deferred: ~/a.json.lock is locked", out)
        self.assertEqual(self.read("a.json"), '{"a": 0}')
        os.rmdir(self.path("a.json.lock"))
        self.run_configs({"a.json": {"format": "json", "settings": {"a": 1}, "lock": "a.json.lock"}})
        self.assertEqual(json.loads(self.read("a.json")), {"a": 1})
        self.assertFalse(os.path.exists(self.path("a.json.lock")), "released")


class ReviewTest(Home):
    """What the first review found: each file stands alone, whatever the others do."""

    def test_a_file_that_raises_does_not_stop_the_next(self):
        self.write("b.json", "{}")
        code, out = self.run_configs({
            "a.toml": {"format": "toml", "settings": {"x": [1, None]}},
            "b.json": {"format": "json", "settings": {"a": 1}},
        })
        self.assertEqual(code, 1)
        self.assertIn("~/a.toml: failed: ", out)
        self.assertEqual(json.loads(self.read("b.json")), {"a": 1})

    def test_a_file_that_is_not_utf8_is_left_alone(self):
        p = self.path("b.json")
        with open(p, "wb") as f:
            f.write(b'{"a": "\xff"}')
        code, out = self.run_configs({"b.json": {"format": "json", "settings": {"a": 1}}})
        self.assertEqual(code, 0)
        self.assertIn("~/b.json: left as it is: it is not valid JSON (not UTF-8)", out)
        with open(p, "rb") as f:
            self.assertEqual(f.read(), b'{"a": "\xff"}')

    def test_true_is_not_one(self):
        self.write("b.json", '{"k": 1, "z": 0}')
        self.run_configs({"b.json": {"format": "json", "settings": {"k": True, "z": False}}})
        self.assertEqual(json.loads(self.read("b.json")), {"k": True, "z": False})

    def test_an_ini_value_with_a_line_break_is_refused(self):
        with open(self.secret, "w") as f:
            f.write("line1\nadmin=true")
        self.write("c.ini", "[General]\nname=me\n")
        code, out = self.run_configs({"c.ini": {"format": "ini", "secrets": {"General": {"pw": self.secret}}}})
        self.assertEqual(code, 1)
        self.assertIn("~/c.ini: not written: General.pw has a line break", out)
        self.assertEqual(self.read("c.ini"), "[General]\nname=me\n")

    def test_a_stale_lock_is_taken_over(self):
        self.write("a.json", '{"a": 0}')
        os.mkdir(self.path("a.json.lock"))
        os.utime(self.path("a.json.lock"), (time.time() - 120, time.time() - 120))
        code, out = self.run_configs({"a.json": {"format": "json", "settings": {"a": 1}, "lock": "a.json.lock"}})
        self.assertEqual((code, json.loads(self.read("a.json"))), (0, {"a": 1}))
        self.assertFalse(os.path.exists(self.path("a.json.lock")))

    def test_crlf_files_keep_their_line_endings(self):
        p = self.path("u.properties")
        with open(p, "wb") as f:
            f.write(b"#c\r\ntheme=dark\r\n")
        self.run_configs({"u.properties": {"format": "properties", "settings": {"theme": "light", "x": "1"}}})
        with open(p, "rb") as f:
            self.assertEqual(f.read(), b"#c\r\ntheme=light\r\nx=1\r\n")


class TomlTest(Home):
    SUPERSONIC = """# Supersonic's settings
[Application]
WindowWidth = 1200 # remembered
EnableSystemTray = false

[[Servers]]
ID = "abc"
Hostname = "http://old"
Username = "me"

[[Servers]]
ID = "def"
Hostname = "http://other"
"""

    def test_merge_keeps_comments_and_other_servers(self):
        self.write("c.toml", self.SUPERSONIC)
        self.run_configs({"c.toml": {"format": "toml", "settings": {
            "Application": {"EnableSystemTray": True},
            "Servers[ID=abc]": {"Hostname": "https://fiesta"},
            "Servers[ID=new]": {"Hostname": "https://new"},
        }}})
        text = self.read("c.toml")
        self.assertIn("# Supersonic's settings", text)
        self.assertIn("WindowWidth = 1200 # remembered", text)
        import tomlkit
        doc = tomlkit.parse(text).unwrap()
        self.assertEqual(doc["Application"], {"WindowWidth": 1200, "EnableSystemTray": True})
        self.assertEqual(doc["Servers"], [
            {"ID": "abc", "Hostname": "https://fiesta", "Username": "me"},
            {"ID": "def", "Hostname": "http://other"},
            {"ID": "new", "Hostname": "https://new"},
        ])

    def test_new_file(self):
        self.run_configs({"c.toml": {"format": "toml", "settings": {"Servers[ID=a]": {"Hostname": "h"}, "Application": {"x": 1}}}})
        import tomlkit
        self.assertEqual(tomlkit.parse(self.read("c.toml")).unwrap(), {"Servers": [{"ID": "a", "Hostname": "h"}], "Application": {"x": 1}})

    def test_unparseable(self):
        self.write("c.toml", "[Application\nx = 1\n")
        code, out = self.run_configs({"c.toml": {"format": "toml", "settings": {"Application": {"x": 2}}}})
        self.assertEqual(self.read("c.toml"), "[Application\nx = 1\n")
        self.assertIn("not valid TOML", out)


class YamlTest(Home):
    def test_merge_keeps_comments(self):
        self.write("c.yaml", "# top\nui:\n  theme: dark  # the app's\nkeep: [1, 2]\n")
        self.run_configs({"c.yaml": {"format": "yaml", "settings": {"ui": {"theme": "light"}, "hosts[name=a]": {"port": 22}}}})
        text = self.read("c.yaml")
        self.assertIn("# top", text)
        self.assertIn("# the app's", text)
        from ruamel.yaml import YAML
        self.assertEqual(json.loads(json.dumps(YAML(typ="safe").load(text))), {"ui": {"theme": "light"}, "keep": [1, 2], "hosts": [{"name": "a", "port": 22}]})


class IniTest(Home):
    def test_merge_keeps_comments_and_order(self):
        self.write("c.ini", "; made by the app\n[General]\nname = me\n# its comment\nsize=3\n\n[Other]\nx=1\n")
        self.run_configs({"c.ini": {"format": "ini", "settings": {"General": {"size": 4, "tray": True}, "New": {"a": "b"}}}})
        self.assertEqual(self.read("c.ini"), "; made by the app\n[General]\nname = me\n# its comment\nsize=4\ntray=true\n\n[Other]\nx=1\n\n[New]\na=b\n")

    def test_unparseable(self):
        self.write("c.ini", "key-before-any-section\n")
        code, out = self.run_configs({"c.ini": {"format": "ini", "settings": {"a": {"b": 1}}}})
        self.assertIn("not valid INI (line 1)", out)


class PropertiesTest(Home):
    def test_merge_keeps_comments_and_escapes(self):
        self.write("u.properties", "#Fri Nov 21 10:00:00 CET 2026\ntheme=dark\nleak.until=123\nlong=a\\\n    b\nclose_to_tray=false\n")
        self.run_configs({"u.properties": {"format": "properties", "settings": {"theme": "light", "long": "x y", "path": "C:\\dir=1", "uni": "caf\u00e9"}}})
        text = self.read("u.properties")
        self.assertEqual(text, "#Fri Nov 21 10:00:00 CET 2026\ntheme=light\nleak.until=123\nlong=x y\nclose_to_tray=false\npath=C\\:\\\\dir\\=1\nuni=caf\\u00e9\n")
        self.assertEqual(mc.Properties().parse(text)["doc"], {"theme": "light", "leak.until": "123", "long": "x y", "close_to_tray": "false", "path": "C:\\dir=1", "uni": "caf\u00e9"})

    def test_reads_java_forms(self):
        doc = mc.Properties().parse("! comment\na b\nc:d\n  e = f\\\n   g\nk\\ ey=v\n")["doc"]
        self.assertEqual(doc, {"a": "b", "c": "d", "e": "fg", "k ey": "v"})


class KeyringTest(Home):
    """keyring() against a stand-in for secretstorage and its login collection."""

    def setUp(self):
        super().setUp()
        import types

        items = self.items = []

        class Item:
            def __init__(self, label, attrs, secret):
                self.label, self.attrs, self.secret = label, dict(attrs), secret

            def get_label(self):
                return self.label

            def get_attributes(self):
                return dict(self.attrs)

            def get_secret(self):
                return self.secret

        class Collection:
            def __init__(self, conn, path):
                self.path = path

            def is_locked(self):
                return False

            def search_items(self, attrs):
                return [i for i in items if all(i.attrs.get(k) == v for k, v in attrs.items())]

            def create_item(self, label, attrs, secret, replace=False):
                items[:] = [i for i in items if not (replace and i.attrs == attrs)] + [Item(label, attrs, secret)]

        fake = types.ModuleType("secretstorage")
        fake.dbus_init = lambda: None
        fake.Collection = Collection
        fake.exceptions = types.SimpleNamespace(ItemNotFoundException=KeyError)
        sys.modules["secretstorage"] = fake
        sys.modules["secretstorage.exceptions"] = fake.exceptions
        self.addCleanup(sys.modules.pop, "secretstorage")
        self.addCleanup(sys.modules.pop, "secretstorage.exceptions")

    def run_keyring(self, label="pw"):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = mc.keyring({"entries": [{"label": label, "attributes": {"service": "supersonic", "username": "a1"}, "secret": self.secret}]})
        self.assertNotIn(SECRET, out.getvalue())
        return code, out.getvalue()

    def test_stores_only_what_is_missing_or_different(self):
        self.assertEqual(self.run_keyring(), (0, "nixos-manager: keyring: pw: stored (it was missing)\n"))
        self.assertEqual([(i.label, i.attrs, i.secret) for i in self.items], [("pw", {"service": "supersonic", "username": "a1"}, SECRET.encode())])
        self.assertEqual(self.run_keyring(), (0, ""))
        self.assertEqual(self.run_keyring("new label"), (0, "nixos-manager: keyring: new label: stored (its label changed)\n"))
        with open(self.secret, "w") as f:
            f.write("another")
        self.assertEqual(self.run_keyring("new label"), (0, "nixos-manager: keyring: new label: stored (its value changed)\n"))
        self.assertEqual(len(self.items), 1)

    def test_missing_secret_fails_without_storing(self):
        os.unlink(self.secret)
        code, out = self.run_keyring()
        self.assertEqual(code, 1)
        self.assertIn("keyring: pw: failed", out)
        self.assertEqual(self.items, [])


class Cli(Home):
    def test_spec_from_a_file(self):
        p = self.path("spec.json")
        with open(p, "w") as f:
            json.dump({"configs": {"a.json": {"format": "json", "settings": {"a": 1}}}}, f)
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            self.assertEqual(mc.main(["managed-configs.py", "configs", p]), 0)
        self.assertEqual(json.loads(self.read("a.json")), {"a": 1})


if __name__ == "__main__":
    time.tzset()
    unittest.main()
