"""Part 2, chapters 13 to 21: first steps, finding your way, machines, the machine page, fleet views."""

from kit import *  # noqa: F403

RAIL = (0.005, 0.055, 0.15, 0.29)
TOPBAR = (0.4, 0.0, 0.6, 0.045)
TABS = (0.105, 0.245, 0.62, 0.045)
WIZARD = (0.205, 0.085, 0.52, 0.76)  # the New machine dialog
PICKER = (0.22, 0.12, 0.56, 0.75)  # Add packages
DRAWER = (0.56, 0.05, 0.44, 0.9)  # a drawer on the right


class S13_1(Capture):
    sid = "S13.1"

    def construct(self):
        img = self.show("intro-hero")
        self.play(self.camera.frame.animate.scale(0.85).shift(RIGHT * 0.8), run_time=3, rate_func=smooth)
        self.show("intro-drift", "story")
        self.play(self.camera.frame.animate.scale(0.9), run_time=2.5, rate_func=smooth)
        img = self.show("settings-top", "watch it again")
        self.look(img, "about", margin=1.6)
        img = self.show("login", "log in")
        self.look(img, "password", margin=3.0)
        self.lower("10 attempts / 10 min per address · sessions last 7 days", "rate limited", YELLOW_, 26)
        self.finish()


STARTER = ["flake.nix", "hosts/example/default.nix", "hosts/example/hardware-configuration.nix",
           "hosts/example/managed.json", "home/pablo.nix", "modules/common.nix", "modules/managed.nix",
           "modules/report-deployments.nix", "modules/secrets.nix", "modules/binary-cache.nix",
           ".forgejo/workflows/check.yml"]


class S13_2(Capture):
    sid = "S13.2"

    def construct(self):
        img = self.show("fresh-code")
        self.look(img, (0.10, 0.06, 0.13, 0.55), "starter fleet", margin=1.2)
        self.until("every folder", 0.3)
        tree = VGroup(*[M(f, 26, FG) for f in STARTER]).arrange(DOWN, aligned_edge=LEFT, buff=0.14).move_to(LEFT * 1.2)
        self.play(FadeOut(img), *[FadeOut(m) for m in self.marks], self.camera.frame.animate.set_width(config.frame_width)
                  .move_to(ORIGIN), FadeIn(tree), run_time=0.8)
        self.marks = []
        glow = {2: "hardware", 3: "where the console", 5: "common", 6: "applies", 4: "example"}

        def lit(i, color=YELLOW_):
            return [tree[i].animate.set_color(color), Indicate(tree[i], color=color, scale_factor=1.04)]

        note = T("folder name = host name = nixosConfigurations.<name>", 24, ACCENT).to_edge(DOWN, buff=0.6)
        self.play(*lit(1), run_time=0.6)
        self.play_at("host name", FadeIn(note), run_time=0.5)
        self.play_at("hand-written", *lit(1, GREEN), run_time=0.6)
        for i in (2, 3, 5, 6, 4):
            self.play_at(glow[i], *lit(i, ACCENT if i != 3 else YELLOW_), run_time=0.6)
        self.finish()


class S13_3(Capture):
    sid = "S13.3"

    def construct(self):
        img = self.show("fresh-machines")
        self.look(img, (0.55, 0.84, 0.44, 0.1), "no lock file", margin=1.5)
        img = self.show("fresh-inputs", "click lock")
        self.look(img, "lock", margin=4)
        img = self.show("fresh-lock-log", "records")
        self.look(img, (0.11, 0.5, 0.55, 0.32), margin=1.2)
        img = self.show("fresh-changes", "stage")
        self.look(img, (0.11, 0.07, 0.29, 0.62), margin=1.2)
        self.finish()


class S13_4(Capture):
    sid = "S13.4"

    def construct(self):
        img = self.show("forgejo-1")
        self.look(img, (0.11, 0.08, 0.53, 0.2), "numbered", mark=False, margin=1.15)
        self.look(img, (0.11, 0.22, 0.53, 0.15), "one", margin=1.3)
        self.look(img, (0.11, 0.37, 0.53, 0.25), "two", margin=1.3)
        self.mark(img, (0.13, 0.56, 0.5, 0.06), "copy")
        self.look(img, (0.11, 0.62, 0.53, 0.1), "three", margin=1.4)
        img = self.show("forgejo-2", "four")
        self.look(img, (0.11, 0.0, 0.53, 0.33), margin=1.2)
        self.look(img, (0.11, 0.5, 0.53, 0.2), "five", margin=1.3)
        self.finish()


SHORTCUTS = ["g f", "g m", "g p", "g s", "g u", "g o", "g a", "g d", "g n", "g i", "g c", "g h", "g e", "g ,"]
RAIL_ITEMS = ["Fleet", "Machines", "Packages", "Services", "Modules", "Options", "Health", "Deployments", "Dependencies",
              "Inputs", "Changes", "History", "Code", "Settings"]


class S14_1(Capture):
    sid = "S14.1"

    def construct(self):
        img = self.show("fleet")
        self.look(img, RAIL, "rail", mark=False, margin=1.3)
        self.until("each has", 0.2)
        for item in RAIL_ITEMS[:13]:
            r = region(img, item).set_stroke(ACCENT, 3)
            self.play(Create(r), run_time=0.12)
            self.play(FadeOut(r), run_time=0.12)
        keys = VGroup(*[M(k, 13, YELLOW_) for k in SHORTCUTS[:13]])
        for k, item in zip(keys, RAIL_ITEMS[:13]):
            k.next_to(region(img, item), RIGHT, buff=0.05)
        self.play_at("press g", LaggedStart(*[FadeIn(k) for k in keys], lag_ratio=0.08), run_time=1.2)
        for cue, item in (("g f", "Fleet"), ("g m", "Machines"), ("g c", "Changes")):
            try:
                self.mark(img, item, cue, ACCENT)
            except ValueError:
                pass
        self.play_at("badges", FadeOut(keys), run_time=0.4)
        self.unmark()
        for item in ("Changes", "Deployments", "Health"):
            self.mark(img, item, color=RED_ if item != "Changes" else ACCENT)
        self.finish()


class S14_2(Capture):
    sid = "S14.2"

    def construct(self):
        img = self.show("fleet")
        self.look(img, TOPBAR, "top bar", mark=False, margin=1.05)
        self.mark(img, "status", "state of the model")
        self.mark(img, "branch", "current branch")
        img = self.show("branch-menu", "branch menu")
        self.look(img, (0.4, 0.03, 0.35, 0.18), margin=1.3)
        img = self.show("fleet", "ahead")
        self.look(img, (0.7, 0.0, 0.3, 0.045), margin=1.1)
        self.mark(img, "pull", "pull and push")
        self.mark(img, "push")
        img = self.show("palette", "control k")
        self.look(img, (0.25, 0.08, 0.62, 0.35), margin=1.2)
        self.lower("/ searches inside the current view", "slash", ACCENT, 26)
        img = self.show("settings-top", "lists all")
        self.look(img, "shortcuts", margin=1.25)
        self.look(img, "theme", "two themes", margin=1.8)
        self.show("fleet-print", "print")
        self.play(self.camera.frame.animate.scale(0.85), run_time=2.5, rate_func=smooth)
        self.finish()


class S15_1(Capture):
    sid = "S15.1"

    def construct(self):
        img = self.show("fleet")
        self.look(img, (0.15, 0.08, 0.85, 0.85), "home view", mark=False, margin=1.0)
        self.mark(img, "homeserver", "each machine")
        self.mark(img, "pi")
        self.mark(img, "workstation")
        self.look(img, (0.56, 0.17, 0.25, 0.65), "around them", margin=1.15)
        img = self.show("fleet-files", "files")
        self.play(self.camera.frame.animate.scale(0.9), run_time=1.5)
        img = self.show("fleet-packages", "package groups")
        img = self.show("fleet-users", "users")
        img = self.show("fleet-inputs", "flake inputs")
        img = self.show("fleet", "share a service")
        self.look(img, (0.3, 0.17, 0.5, 0.65), margin=1.1, mark=False)
        img = self.show("fleet-inspector", "inspector")
        self.look(img, (0.62, 0.05, 0.38, 0.9), margin=1.1)
        img = self.show("fleet", "deployment state")
        self.mark(img, "homeserver", color=GREEN)
        self.mark(img, "pi", color=RED_)
        self.mark(img, "workstation", color=YELLOW_)
        self.lower("deployed · behind · failed · silent", None, FG_STRONG, 26)
        self.finish()


ROLES = [("Role", "Packages", "Settings"),
         ("Minimal", "git, htop", "none"),
         ("Server", "btop, git, htop, tmux", "SSH keys only, no root login · fail2ban · firewall · weekly GC (14 days) · optimise"),
         ("Desktop", "firefox, git, htop, vlc", "Plasma 6 + SDDM · PipeWire · printing · NetworkManager · Bluetooth · zram · GC")]


class S16_1(Capture):
    sid = "S16.1"

    def construct(self):
        img = self.show("machines")
        self.look(img, (0.11, 0.08, 0.89, 0.25), "every", margin=1.1)
        img = self.show("machines-2", "standalone")
        self.look(img, (0.11, 0.1, 0.89, 0.6), margin=1.1, mark=False)
        img = self.show("machines", "open")
        self.mark(img, "new", "create")
        self.mark(img, "iso", "installer")
        self.finish()


class S16_2(Capture):
    sid = "S16.2"

    def construct(self):
        img = self.show("nm-name")
        self.look(img, WIZARD, margin=1.05, mark=False)
        img = self.show("nm-role", "pick a role")
        self.look(img, WIZARD, margin=1.05, mark=False)
        t = table(ROLES, size=18, col_buff=0.4, row_buff=0.25, font=SANS)
        side = card(t, color=ACCENT, pad=0.25)
        frame = self.camera.frame
        side.scale_to_fit_width(frame.width * 0.9).move_to(frame.get_center() + DOWN * frame.height * 0.3)
        self.play_at("minimal", FadeIn(side, shift=UP * 0.2), Indicate(t[1], color=YELLOW_, scale_factor=1.03), run_time=0.8)
        for cue, i in (("server turns", 2), ("desktop adds", 3)):
            self.play_at(cue, Indicate(t[i], color=YELLOW_, scale_factor=1.03), run_time=0.6)
        self.play_at("everything", FadeOut(side), run_time=0.4)
        img = self.show("nm-platform", "platform")
        self.look(img, (0.15, 0.1, 0.32, 0.2), margin=1.4)
        img = self.show("nm-user-filled", "no role")
        self.look(img, WIZARD, margin=1.05, mark=False)
        img = self.show("nm-disk", "physical machine")
        self.look(img, WIZARD, margin=1.05, mark=False)
        img = self.show("nm-review", "nothing is committed")
        self.look(img, WIZARD, margin=1.05, mark=False)
        self.finish()


class S16_3(Capture):
    sid = "S16.3"

    def construct(self):
        img = self.show("install-card")
        self.look(img, "card", "install card", margin=1.6)
        img = self.show("install-hostkey", "fetch its host key")
        self.look(img, (0.11, 0.3, 0.6, 0.5), margin=1.15, mark=False)
        self.lower("No password sent, nothing run on the machine", "only reads", GREEN, 26)
        img = self.show("install-command", "command")
        self.look(img, (0.11, 0.2, 0.6, 0.6), margin=1.15, mark=False)
        self.finish()


BOOT = ["boot the USB stick", "sudo fleet-install: addresses + host key", "console: fetch, compare, approve the key",
        "Enter: fetch the repository, show the disks", "type the machine's name", "partition, nixos-install --flake",
        "set the users' passwords", "push the hardware configuration", "reboot → comin pulls"]


class S16_4(Capture):
    sid = "S16.4"

    def construct(self):
        img = self.show("iso-1")
        self.look(img, "iso", "installer", margin=1.5)
        img = self.show("iso-building", "pinned")
        self.look(img, (0.11, 0.1, 0.89, 0.8), margin=1.05, mark=False)
        img = self.show("iso-done", "usb stick")
        self.look(img, (0.11, 0.1, 0.89, 0.8), margin=1.05, mark=False)
        self.until("sudo", 0.3)
        self.play(*[FadeOut(m) for m in self.mobjects if m is not self.camera.frame],
                  self.camera.frame.animate.set_width(config.frame_width).move_to(ORIGIN), run_time=0.6)
        steps = VGroup(*[VGroup(M(f"{i + 1}", 22, ACCENT), T(b, 24, FG)).arrange(RIGHT, buff=0.3) for i, b in enumerate(BOOT)])
        steps.arrange(DOWN, aligned_edge=LEFT, buff=0.22).move_to(LEFT * 0.5)
        self.play(FadeIn(steps[0], shift=RIGHT * 0.2), run_time=0.4)
        for cue, i in (("shows", 1), ("approve", 2), ("enter", 3), ("type", 4), ("partitions", 5),
                       ("passwords", 6), ("hardware", 7), ("reboots", 8)):
            self.play_at(cue, FadeIn(steps[i], shift=RIGHT * 0.2), run_time=0.4)
        ok = T("Nothing secret is on the ISO", 28, YELLOW_).to_edge(DOWN, buff=0.5)
        self.play_at("nothing secret", FadeIn(ok), run_time=0.5)
        self.finish()


class S17_1(Capture):
    sid = "S17.1"

    def construct(self):
        img = self.show("m-overview")
        self.look(img, TABS, "nine tabs", margin=1.05)
        self.look(img, (0.11, 0.3, 0.89, 0.15), "summarises", margin=1.1, mark=False)
        self.mark(img, (0.115, 0.44, 0.47, 0.13), "services")
        self.mark(img, (0.6, 0.72, 0.38, 0.18), "users")
        img = self.show("m-overview-2", "secrets")
        self.look(img, (0.6, 0.2, 0.4, 0.6), margin=1.1, mark=False)
        img = self.show("m-overview", "deployment")
        self.look(img, (0.6, 0.44, 0.38, 0.27), margin=1.2)
        self.finish()


class S17_2(Capture):
    sid = "S17.2"

    def construct(self):
        img = self.show("m-essentials")
        self.look(img, (0.11, 0.28, 0.89, 0.7), "essentials", mark=False, margin=1.05)
        self.mark(img, (0.115, 0.3, 0.42, 0.25), "time zone")
        self.mark(img, (0.54, 0.3, 0.45, 0.45), "garbage")
        img = self.show("m-essentials-2", "compressed")
        self.look(img, (0.11, 0.05, 0.89, 0.9), mark=False, margin=1.05)
        img = self.show("ess-zram", "flip a switch")
        self.look(img, "row", margin=2.2)
        self.lower("→ hosts/homeserver/managed.json (not deployed yet)", "nothing reaches", YELLOW_, 26)
        self.finish()


class S17_3(Capture):
    sid = "S17.3"

    def construct(self):
        img = self.show("pkg-list")
        self.look(img, (0.11, 0.38, 0.89, 0.5), "packages lists", mark=False, margin=1.05)
        self.mark(img, (0.38, 0.4, 0.3, 0.45), "comes from")
        img = self.show("pkg-add", "add packages")
        self.look(img, PICKER, margin=1.1)
        img = self.show("pkg-add-picked", "what the machine")
        self.look(img, PICKER, margin=1.1, mark=False)
        img = self.show("pkg-removed", "remove works")
        self.look(img, "nano", margin=1.6)
        self.lower("⌫ Removed here · the Nix that adds it stays untouched", "records", YELLOW_, 24)
        img = self.show("pkg-replace", "replace")
        self.look(img, (0.43, 0.04, 0.57, 0.85), margin=1.05)
        self.finish()


class S17_4(Capture):
    sid = "S17.4"

    def construct(self):
        img = self.show("pkg-customise")
        self.look(img, (0.47, 0.05, 0.53, 0.9), "customise", margin=1.05, mark=False)
        self.mark(img, (0.5, 0.23, 0.48, 0.07), "another nix")
        self.mark(img, (0.5, 0.32, 0.48, 0.1), "arguments")
        self.mark(img, (0.5, 0.43, 0.48, 0.1), "build attributes")
        self.mark(img, (0.5, 0.55, 0.48, 0.07), "pin a version")
        self.mark(img, (0.5, 0.62, 0.48, 0.05), "patches")
        self.mark(img, (0.5, 0.68, 0.48, 0.07), "nix file")
        self.until("overlay", 0.3)
        self.play(*[FadeOut(m) for m in self.mobjects if m is not self.camera.frame],
                  self.camera.frame.animate.set_width(config.frame_width).move_to(ORIGIN), run_time=0.6)
        self.marks = []
        base = VGroup(*[box(n, LINE_STRONG, 22, font=MONO) for n in ("btop", "htop", "git", "curl")]).arrange(RIGHT, buff=0.4)
        base.move_to(DOWN * 1.2)
        bl = T("homeserver's packages (pkgs)", 22, MUTED).next_to(base, DOWN, buff=0.25)
        over = box("overlay: btop = btop.override { … }", YELLOW_, 22, font=MONO).move_to(UP * 1.3)
        self.play(FadeIn(base), FadeIn(bl), run_time=0.5)
        self.play(FadeIn(over, shift=DOWN * 0.3), run_time=0.6)
        new = box("btop (customised)", YELLOW_, 22, font=MONO).move_to(base[0])
        self.play(Transform(base[0], new), run_time=0.8)
        users = T("everything that uses it gets the customised one", 24, YELLOW_).next_to(over, UP, buff=0.3)
        self.play_at("everything", FadeIn(users), run_time=0.5)
        rst = card(T("Reset to default", 22, FG), color=LINE_STRONG, pad=0.15).to_edge(DOWN, buff=0.4)
        self.play_at("reset", FadeIn(rst), Transform(base[0], box("btop", LINE_STRONG, 22, font=MONO).move_to(base[0])),
                     run_time=0.6)
        self.finish()


class S17_5(Capture):
    sid = "S17.5"

    def construct(self):
        img = self.show("pkg-every")
        self.look(img, "every", "every machine", margin=1.8)
        self.look(img, (0.11, 0.44, 0.89, 0.05), "modules slash", margin=1.1)
        self.lower("A machine's own value wins where both set something", "machine sets", ACCENT, 24)
        img = self.show("options-tz-copy", "copy to")
        self.look(img, (0.3, 0.2, 0.7, 0.7), margin=1.05, mark=False)
        img = self.show("nix-for", "anything the console")
        self.look(img, (0.15, 0.05, 0.85, 0.9), margin=1.05, mark=False)
        self.finish()


class S17_6(Capture):
    sid = "S17.6"

    def construct(self):
        img = self.show("m-dependencies")
        self.look(img, (0.11, 0.3, 0.5, 0.6), "closure", margin=1.1, mark=False)
        img = self.show("deps-search", "search")
        self.look(img, "search", margin=2.5)
        img = self.show("deps-openssl", "version")
        self.look(img, (0.58, 0.39, 0.4, 0.12), margin=1.4)
        self.look(img, (0.58, 0.5, 0.4, 0.08), "why it is here", margin=1.4)
        self.finish()


class S17_7(Capture):
    sid = "S17.7"

    def construct(self):
        img = self.show("m-services")
        self.look(img, (0.11, 0.3, 0.89, 0.6), "services and programs", margin=1.05, mark=False)
        img = self.show("catalog", "catalog")
        self.look(img, DRAWER, margin=1.05)
        img = self.show("catalog-search", "jellyfin and immich")
        self.look(img, DRAWER, margin=1.05, mark=False)
        img = self.show("svc-jellyfin-added", "adding")
        self.look(img, (0.11, 0.3, 0.89, 0.5), margin=1.1, mark=False)
        img = self.show("svc-configure", "configure")
        self.look(img, (0.3, 0.05, 0.7, 0.9), margin=1.05, mark=False)
        self.finish()


class S17_8(Capture):
    sid = "S17.8"

    def construct(self):
        img = self.show("m-modules-media")
        self.look(img, (0.11, 0.28, 0.89, 0.5), "turns", margin=1.1)
        self.finish()


class S17_9(Capture):
    sid = "S17.9"

    def construct(self):
        img = self.show("m-users")
        self.look(img, (0.11, 0.28, 0.6, 0.5), "users lists", margin=1.1, mark=False)
        img = self.show("users-add", "add a user")
        self.look(img, (0.11, 0.2, 0.89, 0.7), margin=1.05, mark=False)
        img = self.show("m-users", "whole lists")
        self.look(img, (0.11, 0.3, 0.6, 0.05), margin=1.3)
        self.lower("Before turning password logins off, make sure a key is here", "before removing", YELLOW_, 24)
        self.finish()


class S17_10(Capture):
    sid = "S17.10"

    def construct(self):
        img = self.show("m-home")
        self.look(img, (0.11, 0.3, 0.89, 0.25), "turn it on", margin=1.1, mark=False)
        img = self.show("home-on", "add packages")
        self.look(img, (0.11, 0.38, 0.89, 0.3), margin=1.1, mark=False)
        img = self.show("home-2", "dotfiles")
        self.look(img, (0.11, 0.1, 0.89, 0.8), margin=1.05, mark=False)
        img = self.show("home-3", "never put a token")
        self.look(img, (0.11, 0.1, 0.89, 0.8), margin=1.05, mark=False)
        self.lower("@secret:name@ → filled in by sops-nix, readable only by you", "template", ACCENT, 24)
        img = self.show("home-4", "own")
        self.look(img, (0.11, 0.1, 0.89, 0.8), margin=1.05, mark=False)
        self.finish()


APPS = [("Kind", "Example", "Managed as"),
        ("A file the app never writes", "kitty, Waybar", "Files in ~, linked read-only"),
        ("A file the app also writes", "Supersonic config.toml", "App settings file: only the keys set here"),
        ("A password in the keyring", "Supersonic's server password", "Keyring entry"),
        ("Opaque or rotating storage", "Discord login, OAuth tokens", "Kept in the app only")]


class S17_11(Capture):
    sid = "S17.11"

    def construct(self):
        img = self.show("home-4")
        self.look(img, (0.11, 0.3, 0.89, 0.6), margin=1.05, mark=False)
        self.until("handles", 0.3)
        t = table(APPS, size=22, col_buff=0.5, row_buff=0.35, font=SANS)
        t.scale_to_fit_width(min(t.width, 13.2))
        self.play(FadeOut(img), self.camera.frame.animate.set_width(config.frame_width).move_to(ORIGIN),
                  FadeIn(t[0]), FadeIn(t[1]), run_time=0.8)
        self.play_at("also writes", FadeIn(t[2], shift=UP * 0.1), run_time=0.5)
        note = T("merge only the keys set here · JSON, TOML, YAML, INI, properties · wait while the app runs", 20, MUTED)
        note.next_to(t, DOWN, buff=0.5)
        self.play_at("wait while", FadeIn(note), run_time=0.5)
        self.play_at("keyring", FadeIn(t[3], shift=UP * 0.1), run_time=0.5)
        self.play_at("keep to themselves", FadeIn(t[4], shift=UP * 0.1), run_time=0.5)
        self.finish()


class S17_12(Capture):
    sid = "S17.12"

    def construct(self):
        img = self.show("m-network")
        self.look(img, (0.115, 0.3, 0.38, 0.22), "every open port", margin=1.2)
        self.look(img, (0.5, 0.3, 0.45, 0.22), "opening a port", margin=1.2)
        self.look(img, (0.11, 0.55, 0.89, 0.2), "file systems", margin=1.1)
        self.finish()


class S18_1(Capture):
    sid = "S18.1"

    def construct(self):
        img = self.show("matrix-packages")
        self.look(img, (0.11, 0.28, 0.89, 0.65), "matrices", margin=1.05, mark=False)
        self.mark(img, "htop", "gaps")
        img = self.show("matrix-services", "health")
        self.look(img, (0.11, 0.18, 0.89, 0.7), margin=1.05, mark=False)
        self.finish()


class S19_1(Capture):
    sid = "S19.1"

    def construct(self):
        img = self.show("mod-new")
        self.look(img, (0.11, 0.05, 0.89, 0.5), "bundles", margin=1.1, mark=False)
        img = self.show("mod-new-filled", "create a module")
        self.look(img, (0.17, 0.2, 0.45, 0.2), margin=1.3, mark=False)
        img = self.show("mod-filled", "give it an option")
        self.look(img, (0.11, 0.05, 0.89, 0.9), margin=1.02, mark=False)
        img = self.show("mod-page-2", "settings")
        self.look(img, (0.11, 0.05, 0.89, 0.9), margin=1.02, mark=False)
        self.lower("@option:port@ = this machine's value of the port option", "means", ACCENT, 24)
        self.until("behind the scenes", 0.3)
        self.play(*[FadeOut(m) for m in self.mobjects if m is not self.camera.frame],
                  self.camera.frame.animate.set_width(config.frame_width).move_to(ORIGIN), run_time=0.6)
        self.marks = []
        js = code_card('{\n  "options": { "port": { "type": "port", "default": 8096 } },\n  "packages": [ "ffmpeg" ],\n'
                       '  "settings": { "services": { "jellyfin": { "enable": true } },\n'
                       '    "networking": { "firewall": { "allowedTCPPorts": [ "@option:port@" ] } } }\n}', "json", size=20)
        nx = code_card("options.fleet.modules.media = {\n  enable = lib.mkEnableOption \"media\";\n"
                       "  port = lib.mkOption { type = types.port; default = 8096; };\n};\n"
                       "config = lib.mkIf cfg.enable { … };", size=20)
        jl = M("modules/fleet/media/managed.json", 20, ACCENT)
        nl = M("what NixOS sees", 20, GREEN)
        left = VGroup(jl, js).arrange(DOWN, aligned_edge=LEFT, buff=0.15)
        right = VGroup(nl, nx).arrange(DOWN, aligned_edge=LEFT, buff=0.15)
        VGroup(left, right).arrange(DOWN, buff=0.5, aligned_edge=LEFT).scale_to_fit_height(7.2)
        self.play(FadeIn(left), run_time=0.6)
        self.play(FadeIn(right, shift=DOWN * 0.2), run_time=0.6)
        self.play_at("hand-written", Create(hl(lines(js, 3, 4), YELLOW_)), run_time=0.5)
        on = T("on each machine: fleet.modules.media.enable = true → its managed.json", 22, FG).to_edge(DOWN, buff=0.3)
        self.play_at("on each machine", FadeIn(on), run_time=0.5)
        self.play_at("without a default", Indicate(lines(nx, 2), color=YELLOW_), run_time=0.8)
        img = shot("mod-filled")
        self.play_at("add nix", FadeOut(left), FadeOut(right), FadeOut(on), FadeIn(img), run_time=0.6)
        self.mark(img, (0.83, 0.1, 0.17, 0.04))
        self.finish()


class S20_1(Capture):
    sid = "S20.1"

    def construct(self):
        img = self.show("options")
        self.look(img, (0.11, 0.06, 0.89, 0.6), "browses", margin=1.05, mark=False)
        img = self.show("options-search", "select one")
        self.look(img, "search", margin=2.5)
        img = self.show("options-permit", "type")
        self.look(img, (0.3, 0.05, 0.7, 0.9), margin=1.05, mark=False)
        img = self.show("options-permit-2", "form")
        self.look(img, (0.3, 0.05, 0.7, 0.9), margin=1.05, mark=False)
        self.lower("NixOS computes it · inside a list of submodules → set it in Code", "can't be set", YELLOW_, 24)
        self.finish()


class S20_2(Capture):
    sid = "S20.2"

    def construct(self):
        img = self.show("options-tz-2")
        self.look(img, (0.3, 0.3, 0.7, 0.6), "on every machine", margin=1.05)
        img = self.show("options-tz-copy", "copy to")
        self.look(img, (0.3, 0.2, 0.7, 0.7), margin=1.05, mark=False)
        self.lower("lib.mkForce in hand-written Nix: a click cannot beat it", "make force", PURPLE_, 24)
        self.finish()


FINDINGS = [("✖", "Machine does not evaluate", RED_), ("▲", "Firewall disabled → Turn the firewall on", YELLOW_),
            ("▲", "SSH accepts passwords → Allow SSH keys only", YELLOW_),
            ("▲", "SSH allows root logins → Forbid root logins", YELLOW_),
            ("◆", "SSH without fail2ban → Enable fail2ban", ACCENT),
            ("◆", "No automatic garbage collection → Collect weekly, keep 14 days", ACCENT),
            ("◆", "Store not optimised → Optimise the store automatically", ACCENT),
            ("▲", "Input N days old → Update input", YELLOW_),
            ("▲", "Release at or near end of life · mixed releases", YELLOW_),
            ("◆", "Package on every machine except one", ACCENT),
            ("▲", "No comin, or comin pulling another repository", YELLOW_),
            ("▲", "Last pushed commit not checked → Check", YELLOW_),
            ("✖", "Deployment failed · behind > 1 h · silent · changed by hand · reboot pending", RED_)]


class S21_1(Capture):
    sid = "S21.1"

    def construct(self):
        img = self.show("health")
        self.look(img, (0.11, 0.05, 0.89, 0.9), "checklist", margin=1.02, mark=False)
        self.until("flags", 0.3)
        self.play(FadeOut(img), self.camera.frame.animate.set_width(config.frame_width).move_to(ORIGIN), run_time=0.5)
        rows = VGroup(*[VGroup(T(i, 24, c), T(t, 24, FG)).arrange(RIGHT, buff=0.3) for i, t, c in FINDINGS])
        rows.arrange(DOWN, aligned_edge=LEFT, buff=0.2).to_edge(LEFT, buff=0.8).to_edge(UP, buff=0.5)
        for cue, i in (("evaluate", 0), ("firewall", 1), ("passwords", 2), ("root", 3), ("without", 4), ("garbage", 5),
                       ("optimisation", 6), ("old", 7), ("releases", 8), ("except", 9)):
            self.play_at(cue, FadeIn(rows[i], shift=RIGHT * 0.2), run_time=0.35)
        self.play_at("delivery", rows.animate.shift(UP * 2.0), run_time=0.6)
        for cue, i in (("without co-min", 10), ("not passed", 11), ("deployment problems", 12)):
            try:
                self.play_at(cue, FadeIn(rows[i], shift=RIGHT * 0.2), run_time=0.35)
            except ValueError:
                self.play(FadeIn(rows[i], shift=RIGHT * 0.2), run_time=0.35)
        img = shot("health-2")
        self.play_at("fix button", FadeOut(rows), FadeIn(img), run_time=0.6)
        self.look(img, (0.11, 0.05, 0.89, 0.9), margin=1.02, mark=False)
        self.lower("Make sure each machine has an SSH key first", "before you press", YELLOW_, 24)
        self.finish()
