"""Console captures for Part 2: the real console on fake Nix with the fixture fleet, driven by Playwright.

    uv run python capture.py serve fresh|fleet       run a demo server in the foreground
    uv run python capture.py shots fresh|fleet [NAME ...]   take screenshots (server must be running)

Never point this at a real console. `fresh` is a new install (starter flake, no remote, no lock);
`fleet` is the fixture fleet (homeserver, pi, workstation, pablo@laptop) cloned from a local bare
repository that a stand-in ssh serves as ssh://git@forgejo.home.lan:2222/fleet/nixos.git, served at
http://nixos-manager.home.lan:8080 (the browser maps that name to 127.0.0.1; `fresh` uses port 18339).
Screenshots go to build/shots/<name>.png, with build/shots/<name>.json holding {size, boxes}:
the pixel rectangles of named elements, which the scenes highlight and zoom to.
"""

import base64
import hashlib
import json
import os
import random
import re
import shutil
import subprocess
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
DEMO = HERE / "build" / "demo"
SHOTS = HERE / "build" / "shots"
BIN = DEMO / "nixos-manager"
REMOTE_URL = "ssh://git@forgejo.home.lan:2222/fleet/nixos.git"
HOSTNAME = "nixos-manager.home.lan"
PORT = {"fresh": 18339, "fleet": 8080}
VIEW = {"width": 1440, "height": 810}
SCALE = 2
PASSWORD = "demo"
AUTHOR = ("Demo", "demo@example.com")


# A well-formed ed25519 public key made of the bytes 0..31: a placeholder that is obviously not anyone's.
DEMO_KEY = "ssh-ed25519 " + base64.b64encode(b"\x00\x00\x00\x0bssh-ed25519\x00\x00\x00\x20" + bytes(range(32))).decode() + \
    " pablo@workstation"


# --- the demo servers ------------------------------------------------------------------------
def build_server():
    if BIN.exists():
        return
    DEMO.mkdir(parents=True, exist_ok=True)
    subprocess.run(["npm", "ci"], cwd=REPO / "web", check=True)
    subprocess.run(["npm", "run", "build"], cwd=REPO / "web", check=True)
    subprocess.run(["go", "build", "-o", str(BIN), "./cmd/nixos-manager"], cwd=REPO, check=True)


def git(*args, cwd, env=None):
    return subprocess.run(["git", *args], cwd=cwd, check=True, capture_output=True, text=True,
                          env={**os.environ, **(env or {})}).stdout.strip()


def fixtures(root):
    """The repository's fixture fleet, without its broken machine, and with pi editable."""
    out = root / "fixtures"
    if out.exists():
        return out
    shutil.copytree(REPO / "internal/core/testdata/fleet", out)
    disc = json.loads((out / "discover.json").read_text())
    disc["nixos"] = [h for h in disc["nixos"] if h != "oldbox"]
    (out / "discover.json").write_text(json.dumps(disc))
    pi = json.loads((out / "host-pi.json").read_text())
    pi.update(managedFile="hosts/pi/managed.json", managedVersion=2)
    (out / "host-pi.json").write_text(json.dumps(pi))
    found = json.loads((out / "packages.json").read_text())
    for name, version, what in (("ripgrep", "14.1.1", "Recursively searches directories for a regex pattern"),
                                ("ncdu", "2.9.1", "Disk usage analyzer with an ncurses interface"),
                                ("nodejs_24", "24.11.0", "Event-driven I/O framework for the V8 JavaScript engine"),
                                ("ffmpeg", "7.1.2", "A complete, cross-platform solution to record, convert and stream audio and video")):
        found[f"legacyPackages.x86_64-linux.{name}"] = {"pname": name, "version": version, "description": what}
    (out / "packages.json").write_text(json.dumps(found))
    return out


COMMITS = [
    ("Initial fleet: homeserver, pi and workstation", None),
    ("homeserver: jellyfin and nextcloud", "hosts/homeserver/default.nix"),
    ("pi: home-assistant and mosquitto", "hosts/pi/default.nix"),
    ("workstation: Plasma desktop", "hosts/workstation/default.nix"),
    ("all: tailscale", "modules/common.nix"),
]


def seed_remote(root):
    """A bare repository with the starter flake shaped as the fixture fleet, a few commits deep.
    Its managed.nix is an older version, so Health offers the update (S30)."""
    remote = root / "remote.git"
    if remote.exists():
        return remote
    git("init", "--bare", "-b", "main", str(remote), cwd=root)
    work = root / "seed"
    shutil.copytree(REPO / "templates/fleet", work)
    ex = work / "hosts/example"
    for host in ("homeserver", "pi", "workstation"):
        shutil.copytree(ex, work / "hosts" / host)
        nix = work / "hosts" / host / "default.nix"
        nix.write_text(nix.read_text().replace('"example"', f'"{host}"'))
    shutil.rmtree(ex)
    managed = work / "modules/managed.nix"
    managed.write_text(re.sub(r"(nixosManager\.version = lib\.mkOption \{[^}]*default = )\d+;", r"\g<1>6;",
                              managed.read_text()))
    shutil.copy(REPO / "internal/core/testdata/fleet/flake.lock", work / "flake.lock")
    env = {"GIT_AUTHOR_NAME": AUTHOR[0], "GIT_AUTHOR_EMAIL": AUTHOR[1], "GIT_COMMITTER_NAME": AUTHOR[0],
           "GIT_COMMITTER_EMAIL": AUTHOR[1]}
    git("init", "-b", "main", cwd=work)
    git("config", "user.name", AUTHOR[0], cwd=work)
    git("config", "user.email", AUTHOR[1], cwd=work)
    for i, (msg, touch) in enumerate(COMMITS):
        if touch:
            f = work / touch
            f.write_text(f.read_text() + f"\n# {msg}\n")
        git("add", "-A", cwd=work)
        when = f"2026-11-{20 + i}T10:00:00+01:00"
        git("commit", "-q", "-m", msg, cwd=work, env={**env, "GIT_AUTHOR_DATE": when, "GIT_COMMITTER_DATE": when})
    git("push", "-q", str(remote), "main", cwd=work)
    shutil.rmtree(work)
    return remote


NIX_WRAPPER = r"""#!/usr/bin/env python3
# Fake Nix for the video demo, plus one thing real Nix does: a machine's toplevel changes when the
# files the console edits for it change, so checks build it, and it can be tried on.
import hashlib, os, re, subprocess, sys
args = sys.argv[1:]
r = subprocess.run([os.environ["FAKE_NIX"], *args], capture_output=True, text=True)
out, all_ = r.stdout, " ".join(args)
if r.returncode == 0 and "nixmgr:host" in all_:
    flake = re.search(r'flakeRef = "path:([^"?]*)', all_)
    host = re.search(r' host = "([^"]*)";', all_)
    if flake and host:
        h = hashlib.sha256()
        for f in (f"hosts/{host.group(1)}/managed.json", "modules/managed.json"):
            try:
                h.update(open(os.path.join(flake.group(1), f), "rb").read())
            except OSError:
                pass
        out = re.sub(r'("toplevel": "/nix/store/)[0-9a-z]+', lambda m: m.group(1) + h.hexdigest()[:6], out, count=1)
sys.stdout.write(out)
sys.stderr.write(r.stderr)
sys.exit(r.returncode)
"""


def nix_wrapper(root):
    bin_ = root / "bin"
    bin_.mkdir(parents=True, exist_ok=True)
    w = bin_ / "nix"
    w.write_text(NIX_WRAPPER)
    w.chmod(0o755)
    return w


MARKER = ".nixos-manager-video-demo"


def data_dir(name):
    # The fleet's data path shows in check logs: keep it short and plain (and not ~/nixos-manager, where a clone
    # of this repository or a real console's data may live).
    return Path.home() / "nixos-manager-demo" if name == "fleet" else DEMO / name / "data"


def own_data(name, create=False):
    """The demo's data folder, only if this script made it: never adopt or reset a folder it did not create."""
    d = data_dir(name)
    if d.exists() and not (d / MARKER).exists():
        sys.exit(f"{d} exists and was not made by capture.py (no {MARKER}): move it away or set another path")
    if create:
        d.mkdir(parents=True, exist_ok=True)
        (d / MARKER).write_text("Data of the video demo console (docs/video/capture.py); safe to delete.\n")
    return d


def serve(name):
    build_server()
    root = DEMO / name
    root.mkdir(parents=True, exist_ok=True)
    env = {**os.environ, "NIXMGR_NIX": str(nix_wrapper(root)), "NIXMGR_FAKE_DELAY": "1",
           "NIXMGR_FAKE_FIXTURES": str(fixtures(root)), "NIXMGR_AUTHOR_NAME": AUTHOR[0],
           "NIXMGR_AUTHOR_EMAIL": AUTHOR[1], "NIXMGR_DATA": str(own_data(name, create=True)),
           "NIXMGR_LISTEN": f"127.0.0.1:{PORT[name]}", "TMPDIR": "/tmp",
           "FAKE_NIX": str(REPO / "scripts/dev/fake-nix")}
    env["NIXMGR_PASSWORD_HASH"] = subprocess.run([str(BIN), "hash-password"], input=PASSWORD + "\n", text=True,
                                                 capture_output=True, check=True).stdout.strip()
    if name == "fleet":
        # git reaches the "Forgejo" through ssh; this ssh serves the local bare repository instead,
        # so every message keeps the Forgejo address.
        remote = seed_remote(root)
        bin_ = root / "bin"
        bin_.mkdir(exist_ok=True)
        ssh = bin_ / "ssh"
        ssh.write_text("#!/bin/sh\n# Stand-in for ssh in the video demo: the fleet's Forgejo is a local bare repository.\n"
                       'for last; do :; done\ncase $last in\n'
                       '  *git-upload-pack*) exec git-upload-pack "$DEMO_REMOTE" ;;\n'
                       '  *git-receive-pack*) exec git-receive-pack "$DEMO_REMOTE" ;;\nesac\n'
                       'echo "demo ssh: cannot run $last" >&2\nexit 1\n')
        ssh.chmod(0o755)
        env.update(NIXMGR_REMOTE=REMOTE_URL, DEMO_REMOTE=str(remote), PATH=f"{bin_}:{env['PATH']}")
    os.execve(str(BIN), [str(BIN)], env)


# --- shots ---------------------------------------------------------------------------------
# Boxes of what a scene may want to point at, keyed by their text: h: headings, sec: the section a
# heading titles, btn: buttons, tabs and links, row: table rows by first cell, nav: rail items.
AUTO_BOXES = """() => {
  const out = {};
  const put = (k, el) => {
    const r = el.getBoundingClientRect();
    if (r.width > 2 && r.height > 2 && r.bottom > 0 && r.top < innerHeight && r.right > 0 && r.left < innerWidth && !(k in out))
      out[k] = [r.x, r.y, r.width, r.height];
  };
  const txt = (el) => (el.innerText || el.textContent || '').trim().replace(/\\s+/g, ' ').slice(0, 60);
  document.querySelectorAll('h1,h2,h3,h4,legend,summary').forEach((h) => {
    const t = txt(h);
    if (!t) return;
    put('h:' + t, h);
    const sec = h.closest('section,fieldset,details,article,[role=dialog]');
    if (sec) put('sec:' + t, sec);
  });
  document.querySelectorAll('button,[role=tab],a,label').forEach((b) => { const t = txt(b); if (t) put('btn:' + t, b); });
  document.querySelectorAll('tr').forEach((r) => { const c = r.querySelector('td'); if (c) put('row:' + txt(c), r); });
  document.querySelectorAll('nav a, nav button, nav li').forEach((n) => put('nav:' + txt(n), n));
  document.querySelectorAll('[role=dialog]').forEach((d, i) => put('dialog' + (i || ''), d));
  return out;
}"""


class Shooter:
    def __init__(self, name, page):
        self.name, self.page = name, page
        self.base = f"http://{HOSTNAME}:{PORT[name]}/"

    def go(self, route, wait=2.5):
        self.page.goto(self.base + "#/" + route)
        self.page.reload()  # drops any dialog or drawer an earlier step left open
        self.settle(wait)

    def settle(self, wait=1.5):
        self.page.wait_for_load_state("networkidle")
        time.sleep(wait)

    def text(self, text, exact=False, nth=0):
        return self.page.get_by_text(text, exact=exact).nth(nth)

    def button(self, name, exact=False, nth=0):
        return self.page.get_by_role("button", name=name, exact=exact).nth(nth)

    def snap(self, shot, boxes=None, full=False):
        """Screenshot, plus the pixel rectangles of the named locators that are visible."""
        SHOTS.mkdir(parents=True, exist_ok=True)
        rects = {}
        for key, loc in (boxes or {}).items():
            try:
                b = loc.bounding_box(timeout=2000)
            except Exception:  # noqa: BLE001  a missing element only loses its highlight
                b = None
            if b:
                rects[key] = [round(v * SCALE) for v in (b["x"], b["y"], b["width"], b["height"])]
            else:
                print(f"  {shot}: no box for {key}", flush=True)
        for key, b in self.page.evaluate(AUTO_BOXES).items():
            rects.setdefault(key, [round(v * SCALE) for v in b])
        self.page.screenshot(path=str(SHOTS / f"{shot}.png"), full_page=full)
        size = [VIEW["width"] * SCALE, VIEW["height"] * SCALE]
        (SHOTS / f"{shot}.json").write_text(json.dumps({"size": size, "boxes": rects}))
        print(f"  {shot} ({len(rects)} boxes)", flush=True)


def login(s):
    s.page.goto(s.base)
    s.settle(1.5)
    enter = s.page.get_by_role("button", name="▐ Enter the console ▌")
    if enter.count():
        enter.first.click()
        time.sleep(0.8)
    pw = s.page.get_by_role("textbox", name="Password")
    if pw.count():
        pw.fill(PASSWORD)
        pw.press("Enter")
        s.settle(2)


def rewrite_info(route):
    """The demo's data folder is not the one a real install uses: show the real default."""
    # Playwright fetches outside the browser, where the demo hostname does not resolve.
    resp = route.fetch(url=route.request.url.replace(HOSTNAME, "127.0.0.1"), headers=route.request.all_headers())
    body = resp.text()
    for name in PORT:
        body = body.replace(str(data_dir(name)), "/var/lib/nixos-manager")
    route.fulfill(response=resp, body=body)


SHOT_FUNCS = {}


def shot(instance):
    def deco(fn):
        SHOT_FUNCS[fn.__name__] = (instance, fn)
        return fn
    return deco


def run_shots(name, only):
    from playwright.sync_api import sync_playwright

    with sync_playwright() as p:
        browser = p.chromium.launch(executable_path=shutil.which("chromium") or None,
                                    args=[f"--host-resolver-rules=MAP {HOSTNAME} 127.0.0.1"])
        state = DEMO / name / "browser-state.json"
        ctx = browser.new_context(viewport=VIEW, device_scale_factor=SCALE,
                                  storage_state=str(state) if state.exists() else None)
        page = ctx.new_page()
        page.set_default_timeout(10000)
        page.route("**/api/info", rewrite_info)
        s = Shooter(name, page)
        login(s)
        ctx.storage_state(path=str(state))
        for fname in only or STORY[name]:
            inst, fn = SHOT_FUNCS[fname]
            if inst == name:
                print(fname, flush=True)
                try:
                    fn(s)
                except Exception as e:  # noqa: BLE001  one failed shot should not stop the rest
                    print(f"  FAILED {fname}: {str(e).splitlines()[0]}", flush=True)
                    page.screenshot(path=str(SHOTS / f"_failed-{fname}.png"))
                    FAILURES.append(fname)
        browser.close()


# --- the shot list, in the order the console's state needs -----------------------------------
# (appended below)
def nav(s, label):
    return s.page.locator("nav").get_by_text(label, exact=True).first


@shot("fresh")
def intro(s):
    ctx = s.page.context.browser.new_context(viewport=VIEW, device_scale_factor=SCALE)
    page = ctx.new_page()
    t = Shooter(s.name, page)
    page.goto(t.base)
    t.settle(3)
    t.snap("intro-hero")
    for anchor, name in (("The challenge", "intro-drift"), ("Console", "intro-console"), ("Deploy", "intro-deploy")):
        page.get_by_role("link", name=anchor, exact=True).first.click()
        t.settle(3)
        t.snap(name)
    page.get_by_role("button", name="▐ Enter the console ▌").first.click()
    t.settle(1.5)
    t.snap("login", {"password": page.get_by_role("textbox", name="Password")})
    ctx.close()


@shot("fresh")
def fresh_code(s):
    s.go("code")
    for d in ("hosts", "modules", "home", ".forgejo"):
        s.page.get_by_text(d, exact=True).first.click()
        time.sleep(0.4)
    s.page.get_by_text("example", exact=True).first.click()
    time.sleep(0.6)
    s.snap("fresh-code", {"tree": s.page.get_by_text("Files", exact=True).first.locator("..")})


@shot("fresh")
def fresh_lock(s):
    s.go("machines")
    s.snap("fresh-machines", {"lock": s.page.get_by_text("Lock", exact=False).first})
    s.go("inputs")
    s.snap("fresh-inputs", {"lock": s.button("Lock the flake")})
    s.button("Lock the flake").click()
    time.sleep(1.2)
    s.snap("fresh-lock-log")
    s.go("changes", 3)
    s.snap("fresh-changes", {"lock": s.page.get_by_text("flake.lock").first})


@shot("fresh")
def forgejo(s):
    s.go("settings")
    head = s.page.get_by_text("Forgejo", exact=True).first
    head.scroll_into_view_if_needed()
    s.page.mouse.wheel(0, 250)
    time.sleep(1)
    s.snap("forgejo-1", {"remote": s.page.get_by_placeholder(re.compile("ssh://")).first,
                         "key": s.page.get_by_text("Deploy key", exact=True).first})
    s.page.mouse.wheel(0, 700)
    time.sleep(1)
    s.snap("forgejo-2", {"test": s.button("Test connection"), "webhook": s.page.get_by_text("webhook", exact=False).first})


# --- fleet: the machines report back (what comin's report hook would send) ----------------------
ALPHABET = "0123456789abcdfghijklmnpqrsvwxyz"


def store(name):
    h = hashlib.sha256(name.encode()).digest()
    return "/nix/store/" + "".join(ALPHABET[b % 32] for b in h[:32]) + "-" + name


LIBS = {"glibc": "2.40-66", "zlib": "1.3.1", "ncurses": "6.5", "readline": "8.2p13", "pcre2": "10.44",
        "libidn2": "2.3.8", "nghttp2": "1.65.0", "libssh2": "1.11.1", "brotli": "1.1.0", "xz": "5.8.1",
        "bash": "5.2p37", "coreutils": "9.7", "systemd": "257.9", "dbus": "1.14.10", "util-linux": "2.41",
        "python3": "3.13.7", "sqlite": "3.50.4", "libffi": "3.5.1", "expat": "2.7.1", "acl": "2.3.2"}


def closure(host, system):
    """A plausible closure: system -> system-path, etc, kernel -> packages -> libraries."""
    rnd = random.Random(host)
    fixture = json.loads((DEMO / "fleet/fixtures" / f"host-{host}.json").read_text())
    openssl = "3.0.15" if host == "pi" else "3.4.1"
    libs = {**LIBS, "openssl": openssl}
    paths, index = [], {}

    def add(name, refs=(), size=None):
        p = store(name)
        if p not in index:
            index[p] = len(paths)
            paths.append({"path": p, "narSize": size or rnd.randint(40_000, 9_000_000), "refs": []})
        node = paths[index[p]]
        node["refs"] = sorted({*node["refs"], *[index[store(r)] for r in refs]})
        return name

    for lib, v in libs.items():
        add(f"{lib}-{v}")
    glibc = f"glibc-{libs['glibc']}"
    for lib, v in libs.items():
        if lib != "glibc":
            add(f"{lib}-{v}", [glibc])
    add(f"openssl-{openssl}", [glibc, f"zlib-{libs['zlib']}"])
    curl = add(f"curl-{'8.16.0'}-lib", [f"openssl-{openssl}", f"zlib-{libs['zlib']}", f"nghttp2-{libs['nghttp2']}",
                                        f"libssh2-{libs['libssh2']}", f"brotli-{libs['brotli']}", glibc])
    pkgs = []
    for pk in fixture["packages"]:
        deps = [glibc] + rnd.sample([f"{k}-{v}" for k, v in libs.items() if k != "glibc"], 3)
        if pk["name"] in ("git", "curl", "wget", "rclone", "restic"):
            deps.append(curl)
        pkgs.append(add(f"{pk['name']}-{pk['version']}", deps, rnd.randint(200_000, 120_000_000)))
    sp = add("system-path", pkgs, 60_000)
    kernel = add(f"linux-{fixture.get('kernel') or '6.12.58'}", [], 140_000_000)
    initrd = add(f"initrd-linux-{fixture.get('kernel') or '6.12.58'}", [kernel], 30_000_000)
    etc = add("etc", [f"systemd-{libs['systemd']}", f"bash-{libs['bash']}"], 90_000)
    top = system.rsplit("/", 1)[1].split("-", 1)[1]
    add(top, [sp, kernel, initrd, etc, f"systemd-{libs['systemd']}"], 30_000)
    return {"paths": [{**p, "path": p["path"]} for p in paths]}


def http(s, method, path, body=None, token=None, query=""):
    import urllib.request

    req = urllib.request.Request(f"http://127.0.0.1:{PORT['fleet']}{path}{query}", method=method,
                                 data=json.dumps(body).encode() if body is not None else None)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    else:
        cookies = "; ".join(f"{c['name']}={c['value']}" for c in s.page.context.cookies())
        req.add_header("Cookie", cookies)
        req.add_header("Origin", f"http://127.0.0.1:{PORT['fleet']}")
    with urllib.request.urlopen(req) as r:
        raw = r.read()
    return json.loads(raw) if raw else None


@shot("fleet")
def deploy_setup(s):
    s.go("deployments")
    s.snap("deploy-setup", {"step1": s.page.get_by_text("Tell the machines where this console is").first,
                            "step2": s.page.get_by_text("Put each machine's report token on it").first})
    s.page.mouse.wheel(0, 500)
    time.sleep(0.8)
    s.snap("deploy-setup-2")


@shot("fleet")
def reports(s):
    repo = own_data("fleet") / "repo"
    head, prev = git("rev-parse", "main", cwd=repo), git("rev-parse", "main~1", cwd=repo)
    plan = {"homeserver": [(prev, "done", ""), (head, "done", "")],
            "workstation": [(prev, "done", "")],
            "pi": [(prev, "done", ""), (head, "failed", ("error: builder for '/nix/store/0kq7…-home-assistant-2026.11.3.drv' "
                                                        "failed with exit code 1"))]}
    for host, steps in plan.items():
        token = http(s, "GET", "/api/deployments/token", query=f"?host={host}")["token"]
        system = store(f"nixos-system-{host}-26.05.20261124.a3f19c2")
        for commit, status, err in steps:
            body = {"host": host, "commit": commit, "phase": "deploy", "status": status, "ref": "refs/heads/main",
                    "generation": "", "error": err, "current": system if status == "done" else "", "booted": system,
                    "profile": system, "reboot": False, "silentAfter": 7200}
            if status == "failed":
                body.update(current=system)
            http(s, "POST", "/api/deployments/report", body, token)
        http(s, "POST", "/api/deployments/report", {"host": host, "phase": "heartbeat", "status": "", "current": system,
                                                    "booted": system, "profile": system, "reboot": False,
                                                    "silentAfter": 7200, "sendsClosure": True}, token)
        http(s, "POST", "/api/deployments/closure", closure(host, system), token,
             query=f"?host={host}&system={system}")
        print(f"  reported {host}", flush=True)


def rail(s):
    names = ["Fleet", "Machines", "Packages", "Services", "Modules", "Options", "Health", "Deployments", "Dependencies",
             "Inputs", "Changes", "History", "Code", "Settings"]
    return {n: s.page.locator("nav").get_by_text(n, exact=True).first for n in names}



@shot("fleet")
def topbar(s):
    s.go("fleet", 3)
    s.page.get_by_text("main", exact=True).first.click()
    time.sleep(0.8)
    s.snap("branch-menu")
    s.page.keyboard.press("Escape")
    s.page.keyboard.press("Control+k")
    time.sleep(0.6)
    s.page.keyboard.type("jelly", delay=120)
    time.sleep(1.2)
    s.snap("palette")
    s.page.keyboard.press("Escape")
    s.go("settings", 2)
    s.snap("settings-top", {"shortcuts": s.page.get_by_text("Keyboard shortcuts").first,
                            "theme": s.page.get_by_text("Theme", exact=True).first,
                            "about": s.page.get_by_text("About", exact=True).first})
    s.page.get_by_text("Print", exact=True).first.click()
    s.go("fleet", 4)
    s.snap("fleet-print")
    s.go("settings", 2)
    s.page.get_by_text("Night", exact=True).first.click()
    time.sleep(0.5)



TABS = ["overview", "essentials", "packages", "dependencies", "services", "modules", "users", "home", "network"]






def visible(loc):
    return loc.locator("visible=true").first


def row(s, text):
    return s.page.locator("tr", has=s.page.get_by_text(text, exact=True)).first


def switch_near(s, label):
    """The on/off switch on the same line as a label."""
    lb = visible(s.page.get_by_text(label, exact=True)).bounding_box()
    best, dist = None, 1e9
    sw = s.page.get_by_role("switch")
    for i in range(sw.count()):
        b = sw.nth(i).bounding_box()
        if b and abs(b["y"] - lb["y"]) < dist:
            best, dist = sw.nth(i), abs(b["y"] - lb["y"])
    return best


def click_at(s, loc):
    """Click the middle of an element, for graph nodes whose labels do not take clicks."""
    b = loc.bounding_box()
    s.page.mouse.click(b["x"] + b["width"] / 2, b["y"] + b["height"] / 2)


FAILURES = []  # shots and steps that failed: the run carries on, then exits non-zero


def step(s, name, fn, *boxes):
    """Do one interaction; whatever happens, keep a screenshot named after it."""
    try:
        fn()
        s.settle(1.2)
    except Exception as e:  # noqa: BLE001
        print(f"  step {name} failed: {str(e).splitlines()[0]}", flush=True)
        FAILURES.append(name)
    s.snap(name, dict(boxes) if boxes else None)


@shot("fleet")
def fleet_inspector(s):
    s.go("fleet", 5)
    click_at(s, visible(s.page.get_by_text("homeserver", exact=True)))
    s.settle(1.5)
    s.snap("fleet-inspector")
    s.page.keyboard.press("Escape")
    click_at(s, visible(s.page.get_by_text("tailscale", exact=True)))
    s.settle(1.5)
    s.snap("fleet-inspector-2")


@shot("fleet")
def palette(s):
    s.go("fleet", 3)
    s.page.keyboard.press("Control+k")
    time.sleep(0.6)
    s.page.keyboard.type("jelly", delay=120)
    time.sleep(2)
    s.snap("palette")
    s.page.keyboard.press("Escape")


@shot("fleet")
def essentials(s):
    s.go("machines/homeserver/essentials", 3)
    lab = visible(s.page.get_by_text("fail2ban", exact=False))
    lab.scroll_into_view_if_needed()
    time.sleep(0.6)
    s.snap("ess-fail2ban-before", {"row": lab})
    step(s, "ess-fail2ban", lambda: switch_near(s, lab.inner_text().strip()).click(), ("row", lab))


@shot("fleet")
def packages_tab(s):
    s.go("machines/homeserver/packages", 3)
    s.snap("pkg-list", {"nano": row(s, "nano"), "btop": row(s, "btop"), "add": s.button("Add packages"),
                        "every": s.page.get_by_label("Where changes go"), "nix": s.page.get_by_text("Nix for homeserver")})
    s.button("Add packages").click()
    time.sleep(1)
    box = s.page.get_by_placeholder("Search packages by name or description")
    box.fill("neovim")
    s.settle(3)
    s.snap("pkg-add", {"search": box})
    dialog = s.page.get_by_role("dialog").last
    step(s, "pkg-add-picked", lambda: visible(dialog.get_by_text("neovim", exact=True)).click())
    step(s, "pkg-add-done", lambda: dialog.get_by_role("button", name=re.compile("Add")).last.click())
    s.go("machines/homeserver/packages", 2)
    s.page.get_by_label("Remove nano from homeserver").click()
    s.settle(1)
    s.snap("pkg-remove-confirm")
    step(s, "pkg-removed", lambda: s.page.get_by_role("dialog").get_by_role("button", name=re.compile("Remove")).click(),
         ("nano", row(s, "nano")))
    step(s, "pkg-replace", lambda: s.page.get_by_label("Replace vim").click())
    try:
        s.page.get_by_placeholder("Search packages by name or description").fill("neovim")
        s.settle(2.5)
        s.snap("pkg-replace-search")
    except Exception as e:  # noqa: BLE001
        print("  replace search", e)
    s.page.keyboard.press("Escape")
    s.go("machines/homeserver/packages", 2)
    step(s, "pkg-customise", lambda: s.page.get_by_label("Customise btop").click())
    s.page.mouse.move(1100, 500)
    s.page.mouse.wheel(0, 600)
    time.sleep(0.8)
    s.snap("pkg-customise-2")
    s.page.mouse.wheel(0, 900)
    time.sleep(0.8)
    s.snap("pkg-customise-3")
    s.page.keyboard.press("Escape")
    s.page.keyboard.press("Escape")
    s.go("machines/homeserver/packages", 2)
    step(s, "nix-for", lambda: visible(s.page.get_by_text("Nix for homeserver")).click())
    s.go("machines/homeserver/packages", 2)
    step(s, "pkg-every", lambda: s.page.get_by_label("Where changes go").get_by_text("Every machine").click(),
         ("every", s.page.get_by_label("Where changes go")))


@shot("fleet")
def deps_tab(s):
    s.go("machines/homeserver/dependencies", 3)
    inp = s.page.get_by_placeholder(re.compile("Find a package"))
    inp.fill("openssl")
    s.settle(1.5)
    s.snap("deps-search", {"search": inp})
    step(s, "deps-openssl", lambda: visible(s.page.get_by_text(re.compile("^openssl"))).click())



@shot("fleet")
def home_tab(s):
    s.go("machines/homeserver/home", 3)
    step(s, "home-on", lambda: visible(s.page.get_by_text(re.compile("Turn on home-manager"))).click())
    for i in range(1, 4):
        s.page.mouse.move(800, 500)
        s.page.mouse.wheel(0, 650)
        time.sleep(0.8)
        s.snap(f"home-{i + 1}")


@shot("fleet")
def users_tab(s):
    s.go("machines/homeserver/users", 3)
    step(s, "users-add", lambda: s.button("Add user").click())




def reset_working_copy():
    repo = own_data("fleet") / "repo"
    if not repo.is_dir():
        raise RuntimeError(f"no demo working copy at {repo}")
    for attempt in range(5):  # the console may hold git's index lock for a moment
        try:
            git("reset", "-q", cwd=repo)
            git("checkout", "--", ".", cwd=repo)
            git("clean", "-fdq", cwd=repo)
            return
        except subprocess.CalledProcessError:
            time.sleep(1 + attempt)
    raise RuntimeError("cannot reset the demo working copy")


def stage_all(s):
    s.go("changes", 3)
    visible(s.page.locator("label", has_text="Files").locator("input")).click()


def run_check(s, name, timeout=90):
    visible(s.page.get_by_role("button", name=re.compile("Run checks|Check staged"))).click()
    time.sleep(2.5)
    s.snap(f"{name}-running")
    end = time.time() + timeout
    while time.time() < end and not s.page.get_by_text(re.compile("passed|failed")).locator("visible=true").count():
        time.sleep(2)
    s.settle(1.5)
    s.snap(f"{name}-done")


def commit(s, message, name):
    s.page.get_by_role("tab", name="Commit").click()
    visible(s.page.get_by_text(re.compile("Checks passed"))).wait_for(timeout=60000)
    box = s.page.get_by_placeholder("What changes, and why (Ctrl+Enter commits)")
    box.fill(message)
    s.snap(f"{name}-message", {"message": box})
    box.press("Control+Enter")
    s.settle(2)
    s.snap(f"{name}-committed")


def push(s, name):
    s.page.get_by_role("button", name=re.compile("Push")).first.click()
    s.settle(2.5)
    s.snap(f"{name}-dialog")
    dialog = s.page.get_by_role("dialog").last
    dialog.get_by_role("button", name=re.compile("Push")).last.click()
    s.settle(4)
    s.snap(f"{name}-pushed")


@shot("fleet")
def managed_update(s):
    reset_working_copy()
    s.go("health", 4)
    btn = visible(s.page.get_by_role("button", name=re.compile("Update managed.nix")))
    btn.wait_for(timeout=90000)  # Health waits for the fleet's evaluation after a restart
    s.settle(1)
    s.snap("health-managed", {"finding": s.page.get_by_text("modules/managed.nix is older than this console").first,
                              "button": btn})
    btn.click()
    s.settle(1)
    s.snap("health-managed-confirm")
    s.button("Update it").click()
    s.settle(3)
    s.go("changes", 4)
    s.snap("managed-diff")
    stage_all(s)
    s.settle(1.5)
    s.snap("managed-staged")
    run_check(s, "managed-check")
    managed_commit(s)


@shot("fleet")
def managed_commit(s):
    s.go("changes", 3)
    s.page.get_by_role("tab", name="Commit").click()
    time.sleep(0.5)
    if s.page.get_by_text(re.compile("Not checked yet|need a passing check")).locator("visible=true").count():
        run_check(s, "managed-check")
        s.page.get_by_role("tab", name="Commit").click()
    commit(s, "Update modules/managed.nix to this console's version", "managed")
    push(s, "managed-push")



def mock(s, pattern, payload):
    """Answer one API call with demo data (for what fake Nix cannot produce)."""
    s.page.route(pattern, lambda route: route.fulfill(status=200, content_type="application/json",
                                                      body=json.dumps(payload(route) if callable(payload) else payload)))


def scroll(s, dy, x=800, y=500):
    s.page.mouse.move(x, y)
    s.page.mouse.wheel(0, dy)
    time.sleep(0.8)


@shot("fleet")
def clean_views(s):
    """Every read-only view, after managed.nix is current (no upgrade banner)."""
    reset_working_copy()
    s.go("fleet", 5)
    s.snap("fleet", {**rail(s), "status": s.page.get_by_text("OK", exact=False).first,
                     "branch": s.page.get_by_title("Switch, create or merge branches"),
                     "pull": s.page.get_by_role("button", name=re.compile("Pull")).first,
                     "push": s.page.get_by_role("button", name=re.compile("Push")).first,
                     "account": s.page.get_by_label("Account menu"),
                     "homeserver": visible(s.page.get_by_text("homeserver", exact=True)),
                     "pi": visible(s.page.get_by_text("pi", exact=True)),
                     "workstation": visible(s.page.get_by_text("workstation", exact=True))})
    for tab in ("Files", "Packages", "Users", "Inputs"):
        try:
            visible(s.page.get_by_text(tab, exact=True)).click()
            s.settle(2)
            s.snap(f"fleet-{tab.lower()}")
        except Exception as e:  # noqa: BLE001
            print("  tab", tab, str(e).splitlines()[0])
    for tab in TABS:
        s.go(f"machines/homeserver/{tab}", 3)
        s.snap(f"m-{tab}")
    s.go("machines/homeserver/overview", 3)
    scroll(s, 500)
    s.snap("m-overview-2")
    s.go("machines/homeserver/essentials", 3)
    scroll(s, 600)
    s.snap("m-essentials-2")
    s.go("machines/homeserver/users", 3)
    s.snap("m-users")
    s.go("machines", 3)
    s.snap("machines", {"new": s.page.get_by_text("New machine").first, "iso": s.page.get_by_text("Installer ISO").first,
                        "laptop": s.page.get_by_text("pablo@laptop").first})
    scroll(s, 600)
    s.snap("machines-2")
    s.go("packages", 3)
    s.snap("matrix-packages", {"htop": row(s, "htop")})
    s.go("services", 3)
    s.snap("matrix-services")
    s.go("health", 5)
    s.snap("health")
    scroll(s, 700)
    s.snap("health-2")
    scroll(s, 900)
    s.snap("health-3")
    s.go("inputs", 3)
    s.snap("inputs")
    scroll(s, 700)
    s.snap("inputs-2")
    s.go("history", 3)
    s.snap("history")
    s.go("dependencies", 4)
    s.snap("dependencies")
    s.go("deployments", 3)
    s.snap("deployments")


@shot("fleet")
def interactions(s):
    """Machine page actions, each from a clean working copy."""
    for fn in (fleet_inspector, palette, essentials, packages_tab, deps_tab, home_tab, users_tab):
        reset_working_copy()
        print(" ", fn.__name__, flush=True)
        try:
            fn(s)
        except Exception as e:  # noqa: BLE001
            print(f"  FAILED {fn.__name__}: {str(e).splitlines()[0]}", flush=True)
    reset_working_copy()


@shot("fleet")
def wizard(s):
    try:
        new_machine(s)
    finally:
        fx = DEMO / "fleet/fixtures"
        disc = json.loads((fx / "discover.json").read_text())
        disc["nixos"] = [h for h in disc["nixos"] if h != "nas"]
        (fx / "discover.json").write_text(json.dumps(disc))
        reset_working_copy()


def new_machine(s):
    reset_working_copy()
    s.go("machines", 3)
    s.button("New machine").click()
    s.settle(1)
    s.page.get_by_placeholder("nas").fill("nas")
    s.snap("nm-name")
    nxt = lambda: visible(s.page.get_by_role("button", name=re.compile("Next"))).click()
    step(s, "nm-role", nxt)
    step(s, "nm-user", nxt)
    try:
        s.page.get_by_placeholder("alice", exact=True).fill("pablo")
        s.page.get_by_placeholder(re.compile("^ssh-ed25519")).fill(DEMO_KEY)
        s.snap("nm-user-filled")
    except Exception as e:  # noqa: BLE001
        print("  user step", str(e).splitlines()[0])
    step(s, "nm-platform", nxt)
    scroll(s, 400, 720, 450)
    s.snap("nm-platform-2")
    try:
        visible(s.page.get_by_text(re.compile(r"^\W*nixos-anywhere$"))).click()
        s.settle(1)
        visible(s.page.get_by_text(re.compile(r"^\W*ext4$"))).click()
        s.page.get_by_placeholder("/dev/nvme0n1").first.fill("/dev/nvme0n1")
        s.settle(0.5)
        s.snap("nm-disk")
        scroll(s, 400, 720, 450)
        s.snap("nm-disk-2")
    except Exception as e:  # noqa: BLE001
        print("  disk", str(e).splitlines()[0])
    step(s, "nm-review", nxt)
    scroll(s, 400, 720, 450)
    s.snap("nm-review-2")
    step(s, "nm-created", lambda: visible(s.page.get_by_role("button", name=re.compile("Create"))).click())
    s.go("changes", 4)
    s.snap("nm-changes")
    # Real Nix would now evaluate nas: tell fake Nix about it, as the copy of homeserver it resembles.
    fx = DEMO / "fleet/fixtures"
    disc = json.loads((fx / "discover.json").read_text())
    if "nas" not in disc["nixos"]:
        disc["nixos"].append("nas")
        (fx / "discover.json").write_text(json.dumps(disc))
        nas = json.loads((fx / "host-homeserver.json").read_text())
        nas.update(name="nas", hostName="nas", managedFile="hosts/nas/managed.json", toplevel=store("nixos-system-nas-26.05.drv"))
        (fx / "host-nas.json").write_text(json.dumps(nas))
    try:
        http(s, "POST", "/api/model/refresh", {})
    except Exception as e:  # noqa: BLE001
        print("  refresh", e)
    time.sleep(8)
    mock(s, "**/api/hostkey*", {"key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKq3v8mR0Placeholder0HostKey0ForTheVideo root@nixos",
                                "fingerprint": "SHA256:q8Yx3mWb0c9fT2e6RkLh1VnZpD4sJu7aE5iGoKwM0s4"})
    s.go("machines/nas/overview", 8)
    det = s.page.locator("details", has=s.page.get_by_text(re.compile("Install with nixos-anywhere", re.IGNORECASE))).first
    try:
        if det.get_attribute("open") is None:
            det.locator("summary").click()
            s.settle(1)
        det.scroll_into_view_if_needed()
        time.sleep(0.6)
        s.snap("install-card", {"card": det})
        det.get_by_role("textbox").first.fill("192.168.1.50")
        step(s, "install-hostkey", lambda: visible(det.get_by_role("button", name=re.compile("Fetch"))).click(), ("card", det))
        try:
            visible(det.get_by_role("button", name=re.compile("Use it"))).click()
            s.settle(1.5)
        except Exception as e:  # noqa: BLE001
            print("  use it for secrets", str(e).splitlines()[0])
        scroll(s, 400)
        s.snap("install-command", {"card": det})
    except Exception as e:  # noqa: BLE001
        print("  install", str(e).splitlines()[0])
    s.page.unroute("**/api/hostkey*")


@shot("fleet")
def iso(s):
    s.go("machines", 3)
    head = visible(s.page.get_by_text("Installer ISO", exact=True))
    head.scroll_into_view_if_needed()
    s.snap("iso-1", {"iso": head})
    step(s, "iso-building", lambda: visible(s.page.get_by_role("button", name=re.compile("Build ISO"))).click())
    time.sleep(6)
    s.settle(2)
    s.snap("iso-done")
    scroll(s, 500)
    s.snap("iso-done-2")


@shot("fleet")
def modules(s):
    reset_working_copy()
    s.go("modules", 3)
    step(s, "mod-new", lambda: visible(s.page.get_by_role("button", name=re.compile("New module"))).click())
    try:
        visible(s.page.get_by_role("textbox").first).fill("media")
        s.snap("mod-new-filled")
        visible(s.page.get_by_role("button", name=re.compile("^\\W*Create"))).click()
        s.settle(2)
    except Exception as e:  # noqa: BLE001
        print("  new module", str(e).splitlines()[0])
    s.snap("mod-page")
    try:
        visible(s.page.get_by_role("button", name=re.compile("Add option"))).click()
        s.settle(1)
        s.snap("mod-option-form")
        form = s.page.locator("fieldset, form, div", has=s.page.get_by_text("New option", exact=True)).last
        form.get_by_role("textbox").first.fill("port")
        form.get_by_role("combobox").first.select_option("port")
        form.get_by_role("textbox").nth(1).fill("8096")
        s.snap("mod-option-filled")
        s.page.get_by_role("button", name=re.compile("Add option")).last.click()
        s.settle(2)
    except Exception as e:  # noqa: BLE001
        print("  module option", str(e).splitlines()[0])
    try:
        visible(s.page.get_by_role("button", name=re.compile("Add package"))).click()
        s.settle(1)
        s.page.get_by_placeholder("Search packages by name or description").fill("ffmpeg")
        s.settle(2.5)
        dialog = s.page.get_by_role("dialog").last
        visible(dialog.get_by_text("ffmpeg", exact=True)).click()
        dialog.get_by_role("button", name=re.compile("Add")).last.click()
        s.settle(2)
    except Exception as e:  # noqa: BLE001
        print("  module package", str(e).splitlines()[0])
    for opt, val in (("services.jellyfin.enable", "true"), ("networking.firewall.allowedTCPPorts", '["@option:port@"]')):
        try:
            s.page.get_by_placeholder("services.jellyfin.enable").fill(opt)
            s.page.get_by_placeholder(re.compile("^true, 8080")).fill(val)
            visible(s.page.get_by_role("button", name=re.compile("^\\W*Set\\W*$"))).click()
            s.settle(1.5)
        except Exception as e:  # noqa: BLE001
            print("  module setting", opt, str(e).splitlines()[0])
    s.go("modules/media", 3)
    s.snap("mod-filled")
    for i in range(2, 5):
        scroll(s, 600)
        s.snap(f"mod-page-{i}")
    s.go("machines/homeserver/modules", 4)
    s.snap("m-modules-media")


@shot("fleet")
def options(s):
    s.go("options", 4)
    s.snap("options")
    box = visible(s.page.get_by_placeholder(re.compile("Search options")))
    box.fill("openssh")
    s.settle(2)
    s.snap("options-search", {"search": box})
    step(s, "options-permit", lambda: visible(s.page.get_by_text("settings.PermitRootLogin", exact=True)).click())
    scroll(s, 600, 1000, 500)
    s.snap("options-permit-2")
    box = visible(s.page.get_by_placeholder(re.compile("Search options")))
    box.fill("time.timeZone")
    s.settle(2)
    step(s, "options-tz", lambda: visible(s.page.get_by_text(re.compile(r"^(time\.)?timeZone$"))).click())
    scroll(s, 600, 1000, 500)
    s.snap("options-tz-2")
    step(s, "options-tz-copy", lambda: visible(s.page.get_by_role("button", name=re.compile("Copy to"))).click())


MODEL_DIFF = {"state": "ready", "hosts": [
    {"host": "pi", "status": "changed", "packagesAdded": ["jellyfin", "jellyfin-web", "jellyfin-ffmpeg"],
     "featuresAdded": ["services.jellyfin"], "usersAdded": ["jellyfin"], "portsAdded": ["tcp/8096"],
     "units": [{"name": "jellyfin.service", "action": "start"}, {"name": "firewall.service", "action": "restart"},
               {"name": "systemd-sysusers.service", "action": "restart"}, {"name": "nscd.service", "action": "keep"}],
     "etc": [{"path": "/etc/systemd/system/jellyfin.service", "status": "added"},
             {"path": "/etc/group", "status": "changed"}, {"path": "/etc/passwd", "status": "changed"}],
     "closure": {"sizeFrom": 2481000000, "sizeTo": 2913000000,
                 "changes": [{"name": "jellyfin", "to": "10.11.2", "delta": 186000000},
                             {"name": "jellyfin-ffmpeg", "to": "7.1.2", "delta": 211000000},
                             {"name": "jellyfin-web", "to": "10.11.2", "delta": 35000000}]}},
    {"host": "homeserver", "status": "changed", "featuresAdded": ["zramSwap"],
     "units": [{"name": "systemd-zram-setup@zram0.service", "action": "start"}, {"name": "dev-zram0.swap", "action": "start"}],
     "etc": [{"path": "/etc/systemd/zram-generator.conf", "status": "added"}],
     "closure": {"sizeFrom": 3120000000, "sizeTo": 3121000000,
                 "changes": [{"name": "zram-generator", "to": "1.2.1", "delta": 1100000}]}},
    {"host": "workstation", "status": "unchanged"}]}


@shot("fleet")
def change_flow(s):
    """S22: fail2ban on homeserver and Jellyfin on pi, then diff, check, commit, push."""
    reset_working_copy()
    s.go("machines/homeserver/essentials", 3)
    lab = visible(s.page.get_by_text(re.compile("Compressed swap in RAM")))
    lab.scroll_into_view_if_needed()
    s.snap("ess-zram-before", {"row": lab})
    switch_near(s, lab.inner_text().strip()).click()
    s.settle(1.5)
    s.snap("ess-zram", {"row": lab})
    s.go("machines/pi/services", 3)
    s.button("Add service or program").click()
    s.settle(1)
    dialog = s.page.get_by_role("dialog").last
    s.snap("catalog")
    scroll(s, 500, 1100, 500)
    s.snap("catalog-2")
    dialog.get_by_placeholder(re.compile("Search services")).fill("jellyfin")
    s.settle(2)
    s.snap("catalog-search")
    dialog.get_by_role("button", name=re.compile("Enable")).first.click()
    s.settle(2)
    s.snap("svc-jellyfin-added")
    s.go("machines/pi/services", 4)
    s.snap("svc-pi-after")
    try:
        head = visible(s.page.get_by_text("jellyfin", exact=True)).bounding_box()
        cands = s.page.get_by_text(re.compile("Configure$"))
        best = min((cands.nth(i) for i in range(cands.count()) if cands.nth(i).bounding_box()),
                   key=lambda c: abs(c.bounding_box()["x"] - head["x"]) + abs(c.bounding_box()["y"] - head["y"] - 60))
        best.click()
        s.settle(2)
    except Exception as e:  # noqa: BLE001
        print("  configure", str(e).splitlines()[0])
    s.snap("svc-configure")
    mock(s, "**/api/model/diff*", MODEL_DIFF)
    s.go("changes", 4)
    s.snap("changes", {"files": s.page.get_by_text("Files", exact=True).first})
    scroll(s, 500, 1100, 500)
    s.snap("changes-2")
    stage_all(s)
    s.settle(1.5)
    s.snap("changes-staged")
    s.page.get_by_role("tab", name="Checks").click()
    s.settle(1)
    s.snap("checks-panel")
    s.page.get_by_role("tab", name="Commit").click()
    run_check(s, "check")
    s.page.get_by_role("tab", name="Commit").click()
    s.page.unroute("**/api/model/diff*")
    commit(s, "homeserver: zram swap; pi: Jellyfin", "commit")
    s.go("history", 3)
    s.snap("history-commit")
    step(s, "history-show", lambda: visible(s.page.get_by_text(re.compile("^homeserver: zram swap"))).click())
    mock(s, "**/api/model/diff*", MODEL_DIFF)
    s.go("changes", 3)
    push(s, "push")
    s.page.unroute("**/api/model/diff*")


@shot("fleet")
def reports2(s):
    """After the push: homeserver deployed it, workstation is behind, pi failed on it."""
    repo = own_data("fleet") / "repo"
    head, prev = git("rev-parse", "main", cwd=repo), git("rev-parse", "main~1", cwd=repo)
    err = ("error: builder for '/nix/store/5w2kd…-jellyfin-ffmpeg-7.1.2.drv' failed with exit code 137 "
           "(out of memory while building on the Pi)")
    plan = {"homeserver": [(head, "done", "")], "workstation": [(prev, "done", "")],
            "pi": [(prev, "done", ""), (head, "failed", err)]}
    for host, steps in plan.items():
        token = http(s, "GET", "/api/deployments/token", query=f"?host={host}")["token"]
        system = store(f"nixos-system-{host}-26.05.20261124.a3f19c2")
        for commit_, status, e in steps:
            http(s, "POST", "/api/deployments/report", {"host": host, "commit": commit_, "phase": "deploy", "status": status,
                                                        "ref": "refs/heads/main", "generation": "", "error": e,
                                                        "current": system, "booted": system, "profile": system,
                                                        "reboot": False, "silentAfter": 7200}, token)
        print(f"  reported {host}", flush=True)


@shot("fleet")
def deployments(s):
    s.go("deployments", 4)
    s.snap("deployments", {"pi": visible(s.page.get_by_text("pi", exact=True))})
    scroll(s, 600)
    s.snap("deployments-2")
    step(s, "revert-dialog", lambda: visible(s.page.get_by_role("button", name=re.compile("Revert the commit that broke"))).click())
    step(s, "revert-staged", lambda: visible(s.page.get_by_role("dialog").get_by_role("button", name=re.compile("Revert"))).click())
    s.go("changes", 4)
    s.snap("revert-changes")
    reset_working_copy()


@shot("fleet")
def trial(s):
    reset_working_copy()
    s.go("machines/pi/network", 3)
    port = s.page.get_by_label("TCP port to open")
    port.fill("2222")
    step(s, "trial-port", lambda: visible(s.page.get_by_role("button", name=re.compile("Open port"))).click())
    stage_all(s)
    s.settle(1)
    run_check(s, "trial-check")
    s.page.get_by_role("tab", name="Commit").click()
    visible(s.page.get_by_text(re.compile("Checks passed"))).wait_for(timeout=60000)
    s.page.get_by_placeholder("What changes, and why (Ctrl+Enter commits)").fill("pi: SSH on port 2222 as well")
    s.settle(0.5)
    s.snap("trial-ready", {"try": s.page.get_by_label("Try on one machine first")})
    step(s, "trial-confirm", lambda: visible(s.page.get_by_role("button", name=re.compile("Try on"))).click())
    step(s, "trial-started", lambda: s.page.get_by_role("dialog").get_by_role("button", name=re.compile("Try on")).click())
    s.settle(4)
    s.snap("trial-changes")
    scroll(s, 500, 1100, 500)
    s.snap("trial-changes-2")


@shot("fleet")
def inputs_update(s):
    repo = own_data("fleet") / "repo"
    head = git("rev-parse", "main", cwd=repo)
    now = int(time.time())
    upd = {"schedule": {"enabled": True, "day": 0, "hour": 4}, "next": "2026-11-29T04:00:00+01:00", "zone": "Europe/Madrid",
           "update": {"state": "ready", "scheduled": True, "started": "2026-11-22T04:00:03+01:00",
                      "finished": "2026-11-22T04:21:40+01:00", "branch": "flake-update", "base": head,
                      "commit": "5d0e8b1c4f2a9e7d3b6c0a1f8e5d2c9b7a4f6e3d",
                      "inputs": [{"name": "nixpkgs", "status": "updated", "fromRev": "a3f19c2e7b1d4f0a9c8e6b5d3a2f1e0c9b8a7d6e",
                                  "toRev": "5d0e8b1c4f2a9e7d3b6c0a1f8e5d2c9b7a4f6e3d", "fromDate": now - 40 * 86400,
                                  "toDate": now - 2 * 86400,
                                  "compareUrl": "https://github.com/NixOS/nixpkgs/compare/a3f19c2...5d0e8b1"},
                                 {"name": "home-manager", "status": "updated", "fromRev": "4f2b8c1d0e9a",
                                  "toRev": "9a1c3e5f7b2d", "fromDate": now - 55 * 86400, "toDate": now - 3 * 86400},
                                 {"name": "comin", "status": "updated", "fromRev": "c84768279d6c", "toRev": "e71f0a2b9c33",
                                  "fromDate": now - 135 * 86400, "toDate": now - 9 * 86400}],
                      "check": "j31", "summary": {"base": head, "changed": ["homeserver", "pi", "workstation"],
                                                  "built": ["homeserver", "pi", "workstation"],
                                                  "reboot": ["homeserver", "workstation"], "toBuild": 14, "toFetch": 412,
                                                  "download": "1.1 GiB", "unpacked": "4.6 GiB"},
                      "changed": ["homeserver", "pi", "workstation"]}}
    s.go("inputs", 3)
    s.snap("inputs", {"update": visible(s.page.get_by_role("button", name=re.compile("Update all")))})
    mock(s, "**/api/updates", upd)
    s.go("inputs", 3)
    head = visible(s.page.get_by_text("Scheduled update", exact=True))
    head.scroll_into_view_if_needed()
    time.sleep(0.8)
    s.snap("scheduled", {"card": head})
    scroll(s, 500)
    s.snap("scheduled-2")
    s.page.unroute("**/api/updates")


@shot("fleet")
def history_code(s):
    s.go("history", 3)
    s.snap("history")
    box = s.page.get_by_placeholder("Any file, e.g. flake.nix")
    box.fill("hosts/pi/managed.json")
    step(s, "file-history", lambda: visible(s.page.get_by_role("button", name=re.compile("Show"))).click())
    s.go("code", 3)
    for d in ("hosts", "homeserver"):
        visible(s.page.get_by_text(d, exact=True)).click()
        time.sleep(0.5)
    step(s, "code-file", lambda: visible(s.page.get_by_text("default.nix", exact=True)).click())


@shot("fleet")
def settings_sections(s):
    s.go("settings", 3)
    for title, name in (("Secrets", "secrets"), ("API tokens", "tokens"), ("Notifications", "notifications"),
                        ("Forgejo", "settings-forgejo")):
        try:
            h = visible(s.page.get_by_text(title, exact=True))
            h.scroll_into_view_if_needed()
            time.sleep(0.8)
            s.snap(name, {"head": h})
            scroll(s, 500)
            s.snap(f"{name}-2")
        except Exception as e:  # noqa: BLE001
            print("  section", title, str(e).splitlines()[0])


@shot("fleet")
def laptop(s):
    s.go("machines", 3)
    step(s, "laptop", lambda: visible(s.page.get_by_text("pablo@laptop")).click())
    scroll(s, 600)
    s.snap("laptop-2")



@shot("fleet")
def end_trial(s):
    s.go("changes", 3)
    s.snap("trial-view")
    step(s, "trial-end-confirm", lambda: visible(s.page.get_by_role("button", name=re.compile("End the trial"))).click())
    step(s, "trial-ended", lambda: s.page.get_by_role("dialog").get_by_role("button", name=re.compile("End the trial")).click())


@shot("fleet")
def extras(s):
    s.go("fleet", 4)
    try:
        visible(s.page.get_by_text(re.compile(r"Users$"))).click()
        s.settle(2)
        s.snap("fleet-users")
    except Exception as e:  # noqa: BLE001
        print("  users tab", str(e).splitlines()[0])
    s.go("machines/homeserver/services", 4)
    try:
        card = s.page.locator("div, li, article, section").filter(has=s.page.get_by_text("jellyfin", exact=True)) \
            .filter(has=s.page.get_by_text(re.compile("Configure$"))).last
        card.get_by_text(re.compile("Configure$")).click()
        s.settle(2.5)
        s.snap("svc-configure")
        scroll(s, 500, 1100, 500)
        s.snap("svc-configure-2")
    except Exception as e:  # noqa: BLE001
        print("  configure", str(e).splitlines()[0])

# The order the console's state needs: each step builds on what the earlier ones left.
STORY = {"fresh": ["intro", "fresh_code", "fresh_lock", "forgejo"],
         "fleet": ["deploy_setup", "managed_update", "reports", "clean_views", "topbar", "interactions", "wizard", "iso",
                   "modules", "options", "change_flow", "reports2", "deployments", "inputs_update", "history_code",
                   "settings_sections", "laptop", "trial", "end_trial", "extras"]}

if __name__ == "__main__":
    cmd, name, *rest = sys.argv[1:]
    if cmd == "serve":
        serve(name)
    else:
        run_shots(name, rest)  # in the order given: later shots build on the state earlier ones leave
        if FAILURES:
            sys.exit(f"{len(FAILURES)} shot(s) or step(s) failed: {', '.join(FAILURES)}")
