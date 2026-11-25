"""Writes the app configs and keyring entries of a managed.json home.

modules/managed.nix (and managed-home.nix) run it as the home's user, with a
JSON spec that holds the declared settings and, for each secret, the path of
its file (/run/secrets/<name>), never its value:

  managed-configs.py configs <spec>   at each home-manager activation and login
  managed-configs.py keyring <spec>   after the graphical session starts

configs: {"configs": {"<path in ~>": {"format": "json" | "toml" | "yaml" |
"ini" | "properties", "mode": "merge" | "seed", "settings": {...},
"secrets": {<same shape; leaves are secret file paths>}, "process": "...",
"lock": "<path in ~>"}}}

- merge writes the declared keys and keeps every other key the app wrote.
  An object merges key by key; any other value, a list included, is
  replaced whole. A key "Name[Field=Value]" is the item of the list Name
  whose Field is Value, added when missing, and merges as an object.
- seed writes the whole file only when there is none.
- process: while one of the user's processes has this text in its command
  line, the file is left alone (the app would overwrite it on exit) and the
  merge waits for the next activation or login.
- lock: a lock folder the app takes while it writes (mkdir), held while
  the file is written; still held after a few seconds, the merge waits.

A file is written only when its content changes, through a new file renamed
over it, and parsed again first. It keeps its mode, less any access for
others when it holds a secret; a new one is 0600 with a secret, else 0644.
A file that does not parse is left as it is. Nothing here prints a value.

keyring: {"entries": [{"label": "...", "attributes": {...}, "secret":
"<file>"}]} stores each entry with the Secret Service (GNOME Keyring) in
the login collection, as go-keyring does, else the default one, unless an
item with exactly those attributes already has that secret and label.
"""

import io
import json
import os
import re
import sys
import time

SELECTOR = re.compile(r"^(.+)\[([^=\[\]]+)=([^\[\]]*)\]$")


class Unparseable(Exception):
    pass


def say(msg):
    print("nixos-manager: " + msg, flush=True)


def where(e):
    """Where a parser failed, without the text around it (it may be a secret)."""
    for line_attr, col_attr in (("lineno", "colno"), ("line", "col")):
        line = getattr(e, line_attr, None)
        if isinstance(line, int):
            col = getattr(e, col_attr, None)
            return "line %d" % line + (", column %d" % col if isinstance(col, int) else "")
    mark = getattr(e, "problem_mark", None)
    if mark is not None:
        return "line %d" % (mark.line + 1)
    return type(e).__name__


def read_secret(path):
    with open(path, encoding="utf-8") as f:
        value = f.read()
    return value[:-1] if value.endswith("\n") else value


def resolve(tree):
    """The secrets tree with each file path replaced by the secret it holds."""
    return {k: resolve(v) if isinstance(v, dict) else read_secret(v) for k, v in tree.items()}


def combine(a, b):
    """b's keys over a's, object by object; neither is changed."""
    out = dict(a)
    for k, v in b.items():
        out[k] = combine(out[k], v) if isinstance(v, dict) and isinstance(out.get(k), dict) else v
    return out


def same(have, want):
    """A list item's key field against the text after the = in Name[Field=Value]."""
    if isinstance(have, bool):
        return json.dumps(have) == want
    if isinstance(have, (str, int, float)):
        return str(have) == want
    return False


# ---- formats whose documents are nested objects: JSON, TOML, YAML ----


class Tree:
    """Merges into a parsed document through new_map/new_list of its library."""

    def merge(self, node, settings):
        for key, want in settings.items():
            m = SELECTOR.match(key)
            if m:
                name, field, value = m.groups()
                items = node.get(name)
                if not isinstance(items, list):
                    node[name] = self.new_list()
                    items = node[name]
                item = next((x for x in items if self.is_map(x) and same(x.get(field), value)), None)
                if item is None:
                    items.append(self.new_map())
                    item = items[-1]
                    item[field] = value
                self.merge(item, want if isinstance(want, dict) else {})
            elif isinstance(want, dict):
                if not self.is_map(node.get(key)):
                    node[key] = self.new_map()
                self.merge(node[key], want)
            else:
                node[key] = want

    def is_map(self, x):
        return isinstance(x, dict)

    def new_map(self):
        return {}

    def new_list(self):
        return []


class Json(Tree):
    name = "JSON"

    def parse(self, text):
        try:
            doc = json.loads(text)
        except ValueError as e:
            raise Unparseable(where(e)) from None
        if not isinstance(doc, dict):
            raise Unparseable("not an object")
        return doc

    def plain(self, doc):
        return json.loads(json.dumps(doc))

    def dump(self, doc, old):
        # The app's own layout: its indent, or one line when it wrote one.
        if old is not None and "\n" not in old.strip():
            text = json.dumps(doc, ensure_ascii=False, separators=(",", ":"))
        else:
            m = old is not None and re.search(r"^\{\s*\n([ \t]+)\S", old)
            text = json.dumps(doc, ensure_ascii=False, indent=m.group(1) if m else 2)
        return text + ("\n" if old is None or old.endswith("\n") else "")


class Toml(Tree):
    name = "TOML"

    def __init__(self):
        import tomlkit

        self.lib = tomlkit

    def parse(self, text):
        try:
            return self.lib.parse(text)
        except Exception as e:
            raise Unparseable(where(e)) from None

    def plain(self, doc):
        return doc.unwrap()

    def new_map(self):
        return self.lib.table()

    def new_list(self):
        return self.lib.aot()

    def dump(self, doc, old):
        return self.lib.dumps(doc)


class Yaml(Tree):
    name = "YAML"

    def __init__(self):
        from ruamel.yaml import YAML
        from ruamel.yaml.comments import CommentedMap, CommentedSeq

        self.yaml = YAML()  # round-trip: comments, order and quotes stay
        self.yaml.preserve_quotes = True
        self.map, self.seq = CommentedMap, CommentedSeq

    def parse(self, text):
        try:
            doc = self.yaml.load(text)
        except Exception as e:
            raise Unparseable(where(e)) from None
        if doc is None:
            return self.map()
        if not isinstance(doc, dict):
            raise Unparseable("not a mapping")
        return doc

    def plain(self, doc):
        return json.loads(json.dumps(doc, default=str))

    def new_map(self):
        return self.map()

    def new_list(self):
        return self.seq()

    def dump(self, doc, old):
        out = io.StringIO()
        self.yaml.dump(doc, out)
        return out.getvalue()


# ---- line formats: INI and Java .properties, edited in place so comments stay ----


def text_of(v):
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, (dict, list)) or v is None:
        return json.dumps(v)
    return str(v)


class Ini:
    """[section] and key = value lines; settings are {section: {key: value}}."""

    name = "INI"
    SECTION = re.compile(r"^\s*\[([^\]]*)\]\s*$")
    KEY = re.compile(r"^\s*([^=:;#\s][^=:]*?)\s*[=:]\s?(.*)$")

    def parse(self, text):
        lines, doc, section = text.splitlines(), {}, None
        for n, line in enumerate(lines, 1):
            s = line.strip()
            if not s or s[0] in "#;":
                continue
            m = self.SECTION.match(line)
            if m:
                section = m.group(1).strip()
                doc.setdefault(section, {})
                continue
            m = self.KEY.match(line)
            if not m or section is None:
                raise Unparseable("line %d" % n)
            doc[section][m.group(1)] = m.group(2).strip()
        return {"lines": lines, "doc": doc}

    def plain(self, parsed):
        return parsed["doc"]

    def merge(self, parsed, settings):
        lines = parsed["lines"]
        for section, keys in settings.items():
            keys = keys if isinstance(keys, dict) else {}
            start = next((i for i, x in enumerate(lines) if (m := self.SECTION.match(x)) and m.group(1).strip() == section), None)
            if start is None:
                if lines and lines[-1].strip():
                    lines.append("")
                lines.append("[%s]" % section)
                start = len(lines) - 1
            end = next((i for i in range(start + 1, len(lines)) if self.SECTION.match(lines[i])), len(lines))
            for key, value in keys.items():
                line = "%s=%s" % (key, text_of(value))
                at = next((i for i in range(start + 1, end) if (m := self.KEY.match(lines[i])) and m.group(1) == key and lines[i].strip()[0] not in "#;"), None)
                if at is None:
                    last = max([start] + [i for i in range(start + 1, end) if lines[i].strip()])
                    lines.insert(last + 1, line)
                    end += 1
                else:
                    lines[at] = line
        parsed["doc"] = self.parse("\n".join(lines))["doc"]

    def new_doc(self):
        return {"lines": [], "doc": {}}

    def dump(self, parsed, old):
        return "\n".join(parsed["lines"]) + "\n"


def prop_unescape(s):
    out, i = [], 0
    while i < len(s):
        c = s[i]
        if c == "\\" and i + 1 < len(s):
            i += 1
            c = s[i]
            if c == "u" and re.match(r"[0-9a-fA-F]{4}", s[i + 1:i + 5]):
                out.append(chr(int(s[i + 1:i + 5], 16)))
                i += 5
                continue
            out.append({"t": "\t", "n": "\n", "r": "\r", "f": "\f"}.get(c, c))
        else:
            out.append(c)
        i += 1
    return "".join(out)


def prop_escape(s, key):
    out = []
    for i, c in enumerate(s):
        if c in "\\=:#!" or (c == " " and (key or i == 0)):
            out.append("\\" + c)
        elif c in "\t\n\r\f":
            out.append({"\t": "\\t", "\n": "\\n", "\r": "\\r", "\f": "\\f"}[c])
        elif ord(c) < 0x20 or ord(c) > 0x7E:
            out.append("".join("\\u%04x" % u for u in _utf16(c)))
        else:
            out.append(c)
    return "".join(out)


def _utf16(c):
    b = c.encode("utf-16-be")
    return [int.from_bytes(b[i:i + 2], "big") for i in range(0, len(b), 2)]


class Properties:
    """Java .properties: settings are {key: value}; keys may hold dots."""

    name = "properties"

    def logical(self, lines):
        """(first line, last line, text) of each key line, continuations joined."""
        i = 0
        while i < len(lines):
            first, text = i, lines[i].lstrip()
            while re.search(r"(?<!\\)(\\\\)*\\$", text) and i + 1 < len(lines):
                i += 1
                text = text[:-1] + lines[i].lstrip()
            if text and text[0] not in "#!":
                yield first, i, text
            i += 1

    def split(self, text):
        m = re.match(r"((?:\\.|[^\\=:\s])*)\s*[=:\s]?\s*(.*)$", text)
        return prop_unescape(m.group(1)), prop_unescape(m.group(2))

    def parse(self, text):
        lines = text.splitlines()
        doc = {}
        for _, _, t in self.logical(lines):
            k, v = self.split(t)
            doc[k] = v
        return {"lines": lines, "doc": doc}

    def plain(self, parsed):
        return parsed["doc"]

    def merge(self, parsed, settings):
        lines = parsed["lines"]
        for key, value in settings.items():
            line = prop_escape(key, True) + "=" + prop_escape(text_of(value), False)
            at = [(a, b) for a, b, t in self.logical(lines) if self.split(t)[0] == key]
            if at:
                a, b = at[-1]
                lines[a:b + 1] = [line]
            else:
                lines.append(line)
        parsed["doc"] = self.parse("\n".join(lines))["doc"]

    def new_doc(self):
        return {"lines": [], "doc": {}}

    def dump(self, parsed, old):
        return "\n".join(parsed["lines"]) + "\n"


FORMATS = {"json": Json, "toml": Toml, "yaml": Yaml, "ini": Ini, "properties": Properties}


def new_doc(fmt):
    if hasattr(fmt, "new_doc"):
        return fmt.new_doc()
    return fmt.parse("{}" if isinstance(fmt, Json) else "")


# ---- the user's running apps and their locks ----


def running(pattern):
    """Whether a process of this user has pattern in its command line."""
    me, mine = os.getuid(), {os.getpid(), os.getppid()}
    for pid in os.listdir("/proc"):
        if not pid.isdigit() or int(pid) in mine:
            continue
        try:
            if os.stat("/proc/" + pid).st_uid != me:
                continue
            with open("/proc/%s/cmdline" % pid, "rb") as f:
                cmd = f.read().replace(b"\0", b" ").decode("utf-8", "replace")
        except OSError:
            continue
        if pattern in cmd:
            return True
    return False


LOCK_WAIT = 5.0


class Lock:
    def __init__(self, path):
        self.path, self.wait, self.held = path, LOCK_WAIT, False

    def __enter__(self):
        deadline = time.monotonic() + self.wait
        while True:
            try:
                os.makedirs(os.path.dirname(self.path), exist_ok=True)
                os.mkdir(self.path)
                self.held = True
                return self
            except FileExistsError:
                if time.monotonic() >= deadline:
                    return self
                time.sleep(0.2)

    def __exit__(self, *exc):
        if self.held:
            os.rmdir(self.path)


# ---- one file ----


def write_atomically(path, text, mode):
    d = os.path.dirname(path)
    os.makedirs(d, mode=0o700, exist_ok=True)
    tmp = os.path.join(d, ".%s.nixos-manager-%d" % (os.path.basename(path), os.getpid()))
    fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            f.write(text)
            f.flush()
            os.fsync(f.fileno())
        os.chmod(tmp, mode)
        os.replace(tmp, path)
    except BaseException:
        if os.path.lexists(tmp):
            os.unlink(tmp)
        raise
    dfd = os.open(d, os.O_RDONLY)
    try:
        os.fsync(dfd)
    finally:
        os.close(dfd)


def apply(home, rel, c):
    """Writes one config; returns what happened, for the log."""
    path = os.path.join(home, rel)
    fmt = FORMATS[c["format"]]()
    secrets = c.get("secrets") or {}
    if c.get("process") and running(c["process"]):
        return "deferred: %s is running, and would write over it; it is written at the next activation or login" % c["process"]
    lock = Lock(os.path.join(home, c["lock"])) if c.get("lock") else None
    if lock:
        lock.__enter__()
        if not lock.held:
            return "deferred: ~/%s is locked by the app; it is written at the next activation or login" % c["lock"]
    try:
        try:
            st = os.lstat(path)
        except FileNotFoundError:
            st = None
        if st is not None and c.get("mode") == "seed":
            return None
        old = None
        if st is not None:
            with open(path, encoding="utf-8") as f:
                old = f.read()
        doc = new_doc(fmt) if old is None else fmt.parse(old)
        before = None if old is None else fmt.plain(doc)
        fmt.merge(doc, combine(c.get("settings") or {}, resolve(secrets)))
        if before is not None and fmt.plain(doc) == before:
            return None
        text = fmt.dump(doc, old)
        try:
            fmt.parse(text)
        except Unparseable as e:
            return "not written: the result would not parse as %s (%s)" % (fmt.name, e)
        # A link (home-manager's, into the store) is replaced by a file of the user's own.
        mode = st.st_mode & 0o7777 if st is not None and not os.path.islink(path) else (0o600 if secrets else 0o644)
        if secrets:
            mode &= 0o7700
        write_atomically(path, text, mode)
        return "written" if old is not None else "created"
    finally:
        if lock:
            lock.__exit__(None, None, None)


def configs(spec):
    home, failed = os.path.expanduser("~"), False
    for rel, c in sorted(spec.get("configs", {}).items()):
        try:
            what = apply(home, rel, c)
        except Unparseable as e:
            what = "left as it is: it is not valid %s (%s)" % (FORMATS[c["format"]].name, e)
        except OSError as e:
            what, failed = "failed: %s (%s)" % (e.strerror or type(e).__name__, e.filename), True
        except UnicodeError as e:
            what, failed = "failed: %s" % type(e).__name__, True
        if what:
            say("~/%s: %s" % (rel, what))
    return 1 if failed else 0


def keyring(spec):
    import secretstorage
    from secretstorage.exceptions import ItemNotFoundException

    run = os.environ.get("XDG_RUNTIME_DIR", "/run/user/%d" % os.getuid())
    os.environ.setdefault("DBUS_SESSION_BUS_ADDRESS", "unix:path=%s/bus" % run)
    conn = secretstorage.dbus_init()
    try:
        coll = secretstorage.Collection(conn, "/org/freedesktop/secrets/collection/login")
    except ItemNotFoundException:
        coll = secretstorage.get_default_collection(conn)
    if coll.is_locked():
        coll.unlock()
    if coll.is_locked():
        say("keyring: %s is locked, so nothing was stored; unlock it and restart nixos-manager-keyring" % coll.get_label())
        return 1
    failed = False
    for e in spec.get("entries", []):
        attrs, label = e["attributes"], e["label"]
        try:
            value = read_secret(e["secret"]).encode("utf-8")
        except OSError as err:
            say("keyring: %s: failed: %s" % (label, err.strerror))
            failed = True
            continue
        # SearchItems matches every given attribute; the item create_item replaces has exactly these.
        items = [(i.get_label() == label, i.get_secret() == value) for i in coll.search_items(attrs)]
        if (True, True) in items:
            continue
        why = "it was missing" if not items else "its value changed" if (True, False) in items else "its label changed"
        coll.create_item(label, attrs, value, replace=True)
        say("keyring: %s: stored (%s)" % (label, why))
    return 1 if failed else 0


def main(argv):
    if len(argv) != 3 or argv[1] not in ("configs", "keyring"):
        print("usage: managed-configs.py configs|keyring <spec.json>", file=sys.stderr)
        return 2
    with open(argv[2], encoding="utf-8") as f:
        spec = json.load(f)
    return configs(spec) if argv[1] == "configs" else keyring(spec)


if __name__ == "__main__":
    sys.exit(main(sys.argv))
