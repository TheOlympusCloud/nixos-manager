"""Part 2, chapters 28 to 32: secrets, binary cache, homes and upgrades, automation, recipes, closing."""

from kit import *  # noqa: F403
from part2_flow import clear_to_manim

LAYOUT = [(".sops.yaml", "who can open each file"), ("secrets/common.json", "every machine with a key"),
          ("secrets/hosts/<name>.pub", "a machine's SSH host key (public)"), ("secrets/hosts/<name>.json", "that machine alone")]


class S28_1(Capture):
    sid = "S28.1"

    def construct(self):
        rows = VGroup(*[VGroup(M(f, 26, ACCENT), T(d, 24, MUTED)) for f, d in LAYOUT])
        for r in rows:
            r[1].next_to(r[0].get_left() + RIGHT * 6.4, RIGHT, buff=0)
        rows.arrange(DOWN, aligned_edge=LEFT, buff=0.35).move_to(ORIGIN)
        self.play_at("keeps", FadeIn(rows[0]), run_time=0.5)
        self.play_at("common", FadeIn(rows[1], shift=RIGHT * 0.2), run_time=0.5)
        self.play_at("named after", FadeIn(rows[2:], shift=RIGHT * 0.2), run_time=0.6)
        img = shot("secrets")
        self.play_at("age key", FadeOut(rows), FadeIn(img), run_time=0.6)
        self.look(img, (0.11, 0.05, 0.6, 0.6), margin=1.05, mark=False)
        self.lower("Back up $NIXMGR_DATA/sops-age-key", "back it up", YELLOW_, 26)
        img = self.show("secrets-2", "approved")
        self.look(img, (0.11, 0.05, 0.6, 0.9), margin=1.05, mark=False)
        self.lower("A key from a pull or a token waits for a person's approval", "person's", ACCENT, 24)
        self.finish()


WIFI = """sops.secrets.wifi.owner = "wpa_supplicant";
networking.wireless.secretsFile = config.sops.secrets.wifi.path;"""


class S28_2(Capture):
    sid = "S28.2"

    def construct(self):
        img = self.show("secrets")
        self.look(img, (0.11, 0.05, 0.6, 0.9), "add a secret", margin=1.05, mark=False)
        self.lower("Only the encrypted form is written · Show decrypts for you alone", "encrypted", GREEN, 24)
        self.lower("No MCP tool can ever read a value", "no", RED_, 26)
        self.until("on the machine", 0.3)
        clear_to_manim(self)
        run = card(M("/run/secrets/wifi", 28, GREEN), color=GREEN, pad=0.25).move_to(UP * 1.6)
        code = code_card(WIFI, size=22).move_to(DOWN * 0.6)
        self.play(FadeIn(run), run_time=0.5)
        self.play_at("point", type_on(code, 1.0))
        self.play(Create(hl(lines(code, 1))), run_time=0.5)
        img = shot("secrets-2")
        self.play_at("two setup", *[FadeOut(m) for m in self.mobjects if m is not self.camera.frame], FadeIn(img), run_time=0.6)
        self.look(img, (0.11, 0.05, 0.6, 0.9), margin=1.05, mark=False)
        self.lower("Every change here is a working-copy change: check, commit, push", "every change", FG_STRONG, 24)
        self.finish()


CACHE_SRV = "services.nixos-manager.binaryCache = { enable = true; openFirewall = true; };"
CACHE_FLEET = 'fleet.binaryCache = {\n  url = "http://nixos-manager.home.lan:5000";\n  publicKey = "nixos-manager-1:…";\n};'


class S29_1(Narrated):
    sid = "S29.1"

    def construct(self):
        srv = card(VGroup(T("nixos-manager", 24, ACCENT), T("check builds pi's system", 20, MUTED)).arrange(DOWN, buff=0.1),
                   color=ACCENT, pad=0.25).move_to(LEFT * 4 + UP * 1.5)
        pi = machine("pi", s=0.9).move_to(RIGHT * 4 + UP * 1.5)
        self.play_at("already builds", FadeIn(srv), FadeIn(pi), run_time=0.6)
        dl = Arrow(srv.get_right(), pi.get_left(), buff=0.15, stroke_color=CYAN, stroke_width=4)
        port = M(":5000 · signed", 20, CYAN).next_to(dl, UP, buff=0.1)
        self.play_at("turn on", GrowArrow(dl), FadeIn(port), run_time=0.8)
        nb = T("downloads instead of compiling", 22, GREEN).next_to(pi, DOWN, buff=0.2)
        self.play_at("downloads", FadeIn(nb), run_time=0.5)
        a = code_card(CACHE_SRV, size=18).move_to(DOWN * 0.9)
        b = code_card(CACHE_FLEET, size=18).next_to(a, DOWN, buff=0.25)
        self.play_at("enable", FadeIn(a), run_time=0.5)
        self.play_at("address", FadeIn(b), run_time=0.5)
        fb = T("away from home: build after a 5 s timeout", 22, MUTED).next_to(nb, DOWN, buff=0.1)
        self.play_at("away from home", FadeIn(fb), run_time=0.5)
        arm = card(T("ARM machine on an x86 server: emulation or a remote ARM builder (README)", 20, YELLOW_), color=YELLOW_,
                   pad=0.2).to_edge(DOWN, buff=0.25)
        self.play_at("emulation", FadeOut(a), FadeOut(b), FadeIn(arm), run_time=0.6)
        self.finish()


class S30_1(Capture):
    sid = "S30.1"

    def construct(self):
        img = self.show("laptop")
        self.look(img, (0.11, 0.05, 0.89, 0.9), "standalone", margin=1.02, mark=False)
        img = self.show("laptop-2", "kept in")
        self.look(img, (0.11, 0.05, 0.89, 0.9), margin=1.02, mark=False)
        self.lower("home-manager switch --flake .#pablo@laptop", "deploy it", ACCENT, 26)
        img = self.show("health-managed", "health")
        self.look(img, "finding", margin=1.2)
        self.mark(img, "button", "update")
        img = self.show("managed-diff", "review")
        self.look(img, (0.29, 0.25, 0.71, 0.3), margin=1.1)
        self.finish()


SCOPES = [("Read only", "files, history, the fleet, deployments", ACCENT),
          ("Edit, no push", "edit, stage, check, commit, write secrets", YELLOW_),
          ("Full", "push, merge an update, read a secret, approve keys", RED_)]


class S31_1(Capture):
    sid = "S31.1"

    def construct(self):
        img = self.show("tokens")
        self.look(img, (0.11, 0.05, 0.6, 0.6), "api tokens", margin=1.05, mark=False)
        self.until("scope", 0.3)
        clear_to_manim(self)
        rings = VGroup(*[Circle(radius=r, color=c, stroke_width=4) for r, (_, _, c) in zip((1.3, 2.3, 3.3), SCOPES)])
        rings.move_to(LEFT * 3.2)
        names = VGroup(*[T(n, 24, c).move_to(rings[i].get_top() + DOWN * 0.35) for i, (n, _, c) in enumerate(SCOPES)])
        descs = VGroup(*[T(d, 22, c) for _, d, c in SCOPES]).arrange(DOWN, aligned_edge=LEFT, buff=0.6).move_to(RIGHT * 3.4)
        for cue, i in (("read only", 0), ("edit", 1), ("full", 2)):
            self.play_at(cue, Create(rings[i]), FadeIn(names[i]), FadeIn(descs[i], shift=LEFT * 0.2), run_time=0.6)
        tip = T("smallest scope it needs · an expiry if temporary", 24, GREEN).to_edge(DOWN, buff=0.5)
        self.play_at("smallest", FadeIn(tip), run_time=0.5)
        self.finish()


MCP_JSON = """{
  "mcpServers": {
    "nixos-manager": {
      "type": "http",
      "url": "http://<server>:8080/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}"""

TOOLS = {"Fleet": ["fleet_overview", "fleet_insights", "host_details", "model_diff", "compare_option", "search_packages",
                   "search_options"],
         "Packages and options": ["add_packages", "remove_packages", "edit_packages", "set_package_override", "set_option",
                                  "unset_option", "copy_to_machines"],
         "Homes": ["set_home_manager", "link_home_secret", "set_home_config", "set_keyring_entry"],
         "Modules": ["list_modules", "create_module", "edit_module", "delete_module", "update_managed_module"],
         "Machines · Files": ["create_machine", "list_files", "read_file", "write_file", "format_nix"],
         "Git": ["status", "diff", "stage", "commit", "log", "revert_commit", "merge_branch", "push", "pull"],
         "Checks and inputs": ["list_targets", "run_check", "flake_lock", "flake_update"],
         "Deployments": ["deployments", "try_on_machine", "end_trial", "machine_dependencies", "fleet_dependencies"],
         "Secrets": ["list_secrets", "set_secret", "delete_secret"]}


class S31_2(Narrated):
    sid = "S31.2"

    def construct(self):
        code = code_card(MCP_JSON, "json", size=22)
        self.play_at("point any", type_on(code, 1.4))
        self.until("sixty three", 0.3)
        self.play(code.animate.scale(0.5).to_corner(UL, buff=0.3), run_time=0.6)
        groups = VGroup()
        for g, tools in TOOLS.items():
            tiles = VGroup(*[card(M(t, 13, FG), color=LINE_STRONG, pad=0.06, radius=0.05) for t in tools])
            tiles.arrange_in_grid(cols=4, buff=0.06, cell_alignment=LEFT)
            groups.add(VGroup(T(g, 18, ACCENT), tiles).arrange(DOWN, aligned_edge=LEFT, buff=0.08))
        groups.arrange_in_grid(cols=3, buff=(0.35, 0.3), cell_alignment=UL)
        groups.scale_to_fit_width(13.4).scale_to_fit_height(min(groups.height, 5.6)).to_edge(DOWN, buff=0.3)
        self.play(LaggedStart(*[FadeIn(g, shift=UP * 0.1) for g in groups], lag_ratio=0.12), run_time=1.6)
        self.play_at("scope allows", *[g[1].animate.set_opacity(0.35) for g in groups[5:]], run_time=0.6)
        self.play(*[g[1].animate.set_opacity(1) for g in groups[5:]], run_time=0.4)
        self.until("same code", 0.3)
        self.play(FadeOut(groups), FadeOut(code), run_time=0.5)
        rules = VGroup(T("same code as the web page, same rules", 28, FG_STRONG),
                       T("read a file before writing it", 26, ACCENT),
                       T("no commit, no push without a passing check", 26, YELLOW_),
                       T("every commit says so in its trailer: Via: token <name>", 26, GREEN)).arrange(DOWN, buff=0.4)
        self.play(FadeIn(rules[0]), run_time=0.5)
        for cue, i in (("read a file", 1), ("never commit", 2), ("trailer", 3)):
            self.play_at(cue, FadeIn(rules[i], shift=UP * 0.1), run_time=0.5)
        self.finish()


RECIPES = [("install a tool", "Install a tool on one machine", ["pkg-add", "changes-staged", "check-done", "push-dialog", "deployments"],
            "Packages → Add packages → Changes → Check → Commit → Push → Deployments"),
           ("run a service", "Run a service", ["catalog", "svc-configure", "check-done", "push-dialog"],
            "Services → Add service → Configure → Check → Commit → Push"),
           ("risky", "A risky network change", ["check-done", "trial-ready", "trial-changes"],
            "Stage → Check → Try on <machine> → watch → Promote to main"),
           ("breaks", "Something broke", ["deployments", "revert-dialog", "revert-changes", "push-dialog"],
            "Deployments → Revert the commit that broke it → Check → Commit → Push"),
           ("stay up to date", "Stay up to date", ["scheduled", "scheduled-2", "inputs"],
            "Inputs → Scheduled update → notification → review → Merge into main")]


class S32_1(Narrated):
    sid = "S32.1"

    def construct(self):
        title = T("Five everyday recipes", 44, FG_STRONG, weight=BOLD)
        self.play(FadeIn(title), run_time=0.6)
        current = VGroup(title)
        for i, (cue, name, shots, steps) in enumerate(RECIPES):
            self.until(cue, 0.4)
            head = T(f"{i + 1}  {name}", 34, ACCENT, weight=BOLD).to_edge(UP, buff=0.5)
            thumbs = Group(*[shot(s).scale_to_fit_width(3.0) for s in shots])
            frames = VGroup(*[SurroundingRectangle(t, color=LINE_STRONG, buff=0.03, stroke_width=2) for t in thumbs])
            row = Group(*[Group(t, f) for t, f in zip(thumbs, frames)]).arrange(RIGHT, buff=0.4)
            row.scale_to_fit_width(min(row.width, 13.2)).move_to(UP * 0.3)
            arrows = VGroup(*[Arrow(a.get_right(), b.get_left(), buff=0.03, stroke_color=ACCENT, stroke_width=3,
                                    max_tip_length_to_length_ratio=0.4) for a, b in zip(row, row[1:])])
            path = self.caption(steps, FG_STRONG, 24)
            self.play(FadeOut(current), FadeIn(head), run_time=0.5)
            self.play(LaggedStart(*[FadeIn(r, shift=UP * 0.2) for r in row], lag_ratio=0.25), Create(arrows),
                      FadeIn(path), run_time=1.4)
            current = Group(head, row, arrows, path)
        self.finish()


class S32_2(Narrated):
    sid = "S32.2"

    def construct(self):
        console = card(VGroup(M("❄", 40, ACCENT), T("nixos-manager", 26, FG_STRONG)).arrange(DOWN, buff=0.1), color=ACCENT,
                       pad=0.3)
        hosts = Group(*[machine(n, s=0.8) for n in ("homeserver", "pi", "workstation")])
        for h, p in zip(hosts, (LEFT * 4.5 + DOWN * 1.8, DOWN * 2.6, RIGHT * 4.5 + DOWN * 1.8)):
            h.move_to(p)
        repo = machine("forgejo", s=0.8).move_to(UP * 2.4)
        links = VGroup(*[Line(console.get_center(), h.get_center(), stroke_color=LINE_STRONG, stroke_width=2) for h in hosts],
                       Line(console.get_center(), repo.get_center(), stroke_color=ACCENT, stroke_width=3))
        self.play_at("every machine", FadeIn(hosts), FadeIn(repo), run_time=0.8)
        self.play(Create(links), FadeIn(console), run_time=0.8)
        self.bring_to_front(console)
        words = [("every change", "shown in NixOS terms", ACCENT), ("checked", "checked by a real build", YELLOW_),
                 ("deployed", "deployed by a push", GREEN), ("reversible", "every step reversible", PURPLE_)]
        ws = VGroup(*[T(t, 26, c) for _, t, c in words]).arrange(DOWN, aligned_edge=LEFT, buff=0.2).to_corner(UL, buff=0.5)
        for (cue, _, _), w in zip(words, ws):
            self.play_at(cue, FadeIn(w, shift=RIGHT * 0.2), run_time=0.4)
        final = VGroup(M("❄", 72, ACCENT), T("Nix · NixOS · nixos-manager", 48, FG_STRONG, weight=BOLD)).arrange(DOWN, buff=0.3)
        self.play_at("that's", *[FadeOut(m) for m in self.mobjects], FadeIn(final, scale=0.9), run_time=1.0)
        self.finish(pad=2.5)
