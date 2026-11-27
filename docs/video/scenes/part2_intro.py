"""Part 2, chapters 11 and 12: what nixos-manager is, and installing it."""

from kit import *  # noqa: F403


class S11_1(Narrated):
    sid = "S11.1"

    def construct(self):
        inner = VGroup(*[card(T(t, 22, FG), color=LINE_STRONG, pad=0.15) for t in
                         ("working copy (git)", "Nix: evaluate, check, build", "web console", "MCP endpoint")])
        inner.arrange_in_grid(2, 2, buff=0.25)
        server = VGroup(T("nixos-manager server", 26, ACCENT, weight=BOLD), inner).arrange(DOWN, buff=0.3)
        server = card(server, color=ACCENT, pad=0.35).move_to(LEFT * 0.4 + UP * 0.9)
        self.play_at("web console", FadeIn(server[0]), FadeIn(server[1][0]), run_time=0.8)
        for cue, i in (("working copy", 0), ("runs", 1)):
            self.play_at(cue, FadeIn(inner[i], shift=UP * 0.1), run_time=0.5)
        self.play_at("visually", FadeIn(inner[2], shift=UP * 0.1), run_time=0.5)
        forgejo = machine("forgejo", s=0.8).move_to(RIGHT * 5.2 + UP * 0.9)
        fl = M("Forgejo", 20, MUTED).next_to(forgejo, UP, buff=0.1)
        push = Arrow(server.get_right() + UP * 0.3, forgejo.get_left() + UP * 0.3, buff=0.1, stroke_color=GREEN, stroke_width=3)
        back = Arrow(forgejo.get_left() + DOWN * 0.3, server.get_right() + DOWN * 0.3, buff=0.1, stroke_color=LINE_STRONG,
                     stroke_width=2.5)
        pl = M("push", 18, GREEN).next_to(push, UP, buff=0.05)
        wl = M("webhook · pull · statuses", 16, MUTED).next_to(back, DOWN, buff=0.05)
        self.play_at("checks", Indicate(inner[1], color=YELLOW_), run_time=0.8)
        self.play_at("pushes", FadeIn(forgejo), FadeIn(fl), GrowArrow(push), FadeIn(pl), run_time=0.8)
        self.play(GrowArrow(back), FadeIn(wl), run_time=0.6)
        hosts = Group(*[machine(n, s=0.6) for n in ("homeserver", "pi", "workstation")]).arrange(RIGHT, buff=0.6)
        hosts.move_to(RIGHT * 3.6 + DOWN * 2.5)
        pulls = VGroup(*[Arrow(h.get_top(), forgejo.get_bottom(), buff=0.1, stroke_color=CYAN, stroke_width=2.5) for h in hosts])
        self.play_at("deploys", FadeIn(hosts), LaggedStart(*[GrowArrow(p) for p in pulls], lag_ratio=0.2), run_time=1.0)
        reports = VGroup(*[DashedLine(h.get_left(), server.get_bottom() + RIGHT * (0.8 - 0.8 * i), stroke_color=YELLOW_,
                                      stroke_width=2.5, dash_length=0.12) for i, h in enumerate(hosts)])
        rl = T("deployment reports, heartbeats, closures", 20, YELLOW_).next_to(server, DOWN, buff=0.15).shift(LEFT * 0.8)
        self.play_at("talk back", LaggedStart(*[Create(r) for r in reports], lag_ratio=0.2), run_time=1.0)
        self.play_at("reports", FadeIn(rl), run_time=0.5)
        browser = card(T("browser", 22, FG), color=LINE_STRONG, pad=0.2).move_to(LEFT * 5.6 + UP * 2.2)
        robot = card(T("AI assistant · script", 22, FG), color=PURPLE_, pad=0.2).move_to(LEFT * 5.2 + DOWN * 1.8)
        eb = Arrow(browser.get_right(), inner[2].get_left(), buff=0.1, stroke_color=LINE_STRONG, stroke_width=2.5)
        er = Arrow(robot.get_right(), inner[3].get_left(), buff=0.1, stroke_color=PURPLE_, stroke_width=2.5)
        self.play(FadeIn(browser), GrowArrow(eb), run_time=0.6)
        self.play_at("endpoint", FadeIn(inner[3]), FadeIn(robot), GrowArrow(er), run_time=0.8)
        self.play_at("same rules", Indicate(inner[3], color=PURPLE_), run_time=0.8)
        self.finish()


class S11_2(Narrated):
    sid = "S11.2"

    def construct(self):
        names = ["Edit", "Check", "Commit", "Push = deploy"]
        colors = [ACCENT, YELLOW_, GREEN, CYAN]
        steps = VGroup(*[box(n, c, 30, w=3.3, h=1.1) for n, c in zip(names, colors)])
        pos = [UP * 1.8 + LEFT * 3.2, UP * 1.8 + RIGHT * 3.2, DOWN * 1.6 + RIGHT * 3.2, DOWN * 1.6 + LEFT * 3.2]
        for b, p in zip(steps, pos):
            b.move_to(p)
        arrows = VGroup(*[Arrow(steps[i].get_center(), steps[(i + 1) % 4].get_center(), buff=1.0, stroke_color=LINE_STRONG,
                                stroke_width=4) for i in range(4)])
        self.play(LaggedStart(*[FadeIn(b) for b in steps], lag_ratio=0.15), run_time=1.0)
        self.play(LaggedStart(*[GrowArrow(a) for a in arrows], lag_ratio=0.15), run_time=0.8)
        l1 = padlock(s=0.4).move_to(arrows[1].get_center() + RIGHT * 0.5)
        l2 = padlock(s=0.4).move_to(arrows[2].get_center() + DOWN * 0.45)
        self.play_at("cannot commit", FadeIn(l1, scale=0.5), run_time=0.5)
        self.play_at("cannot push", FadeIn(l2, scale=0.5), run_time=0.5)
        rule = T("a commit needs a passing check of exactly what is staged", 26, YELLOW_).move_to(ORIGIN)
        self.play_at("exactly", FadeIn(rule), run_time=0.6)
        who = VGroup(T("person in the web page: can override, explicitly", 22, FG),
                     T("automation (API, MCP): never", 22, RED_)).arrange(DOWN, buff=0.15).next_to(rule, DOWN, buff=0.3)
        self.play_at("person", FadeIn(who[0]), run_time=0.5)
        self.play_at("automation", FadeIn(who[1]), run_time=0.5)
        self.until("edit check", 0.2)
        for b in steps:
            self.play(Indicate(b, color=b[0].get_stroke_color(), scale_factor=1.08), run_time=0.4)
        self.finish()


PCT = """echo 'ssh-ed25519 AAAA... you@laptop' > /root/me.pub
pct create 120 local:vztmpl/nixos-26.05-proxmox-lxc.tar.xz \\
  --hostname nixos-manager \\
  --ostype nixos --unprivileged 1 --features nesting=1 \\
  --cores 4 --memory 4096 --swap 2048 \\
  --rootfs local-lvm:30 \\
  --net0 name=eth0,bridge=vmbr0,ip=dhcp \\
  --ssh-public-keys /root/me.pub \\
  --onboot 1 --start 1"""

MODULE = """services.nixos-manager = {
  enable = true;
  openFirewall = true; # port 8080
  author = {
    name = "Your Name";
    email = "you@example.com";
  };
  passwordHashFile = "/var/lib/secrets/nixos-manager-password";
};"""

HASH = """printf '%s\\n' "$pw" | nixos-manager hash-password > /var/lib/secrets/nixos-manager-password
systemctl restart nixos-manager"""


class S12_1(Narrated):
    sid = "S12.1"

    def construct(self):
        lxc = card(VGroup(T("Proxmox", 22, MUTED), T("NixOS container", 28, ACCENT), T("nixos-manager", 26, FG_STRONG))
                   .arrange(DOWN, buff=0.15), color=ACCENT, pad=0.4)
        self.play_at("recommended", FadeIn(lxc, scale=0.9), run_time=0.8)
        real = T("evaluates and builds for real", 26, GREEN).next_to(lxc, DOWN, buff=0.4)
        self.play_at("builds for real", FadeIn(real), run_time=0.5)
        self.until("download", 0.3)
        self.clear(run_time=0.4)
        code = code_card(PCT, "bash", size=20)
        self.play(type_on(code, 2.0))
        self.play_at("nesting", Create(hl(lines(code, 3))), run_time=0.6)
        mem = self.caption("4 GB RAM to start · 8 GB past a dozen machines · 30 GB disk", YELLOW_, 26)
        self.play_at("memory", Create(hl(lines(code, 4), ACCENT)), FadeIn(mem), run_time=0.6)
        self.until("import", 0.3)
        self.clear(run_time=0.4)
        mod = code_card(MODULE, size=24)
        name = M("/etc/nixos/configuration.nix", 22, ACCENT).next_to(mod, UP, buff=0.2).align_to(mod, LEFT)
        self.play(FadeIn(name), type_on(mod, 1.6))
        self.play_at("author", Create(hl(lines(mod, 3, 6))), run_time=0.6)
        self.play_at("password's", Create(hl(lines(mod, 7), ACCENT)), run_time=0.6)
        self.play_at("pipe", VGroup(name, mod).animate.scale(0.7).to_edge(UP, buff=0.4), run_time=0.6)
        h = code_card(HASH, "bash", size=20).to_edge(DOWN, buff=0.8)
        self.play(type_on(h, 1.0))
        port = T("→ http://<container>:8080", 28, GREEN).next_to(h, UP, buff=0.3)
        self.play_at("port", FadeIn(port), run_time=0.5)
        self.finish()


OPTIONS = [("enable", "Run the console"), ("package", "The nixos-manager build (the flake sets it)"),
           ("listen", "Address and port; default 0.0.0.0:8080"), ("openFirewall", "Open that port"),
           ("dataDir", "Working copy, keys, tokens; /var/lib/nixos-manager"),
           ("author.name, author.email", "Identity of every commit"),
           ("passwordHashFile", "bcrypt hash of the login password"),
           ("remote", "Repository to clone and push (or set it in Settings)"),
           ("environmentFile", "Extra secrets as KEY=value"),
           ("extraEnvironment", "Extra variables, e.g. NIXMGR_EVAL_JOBS = \"1\""),
           ("checks.build", "false: checks only evaluate; each machine builds itself"),
           ("binaryCache.enable, .openFirewall", "Serve the store to the machines (port 5000)")]


class S12_2(Narrated):
    sid = "S12.2"

    def construct(self):
        head = M("services.nixos-manager.*", 26, ACCENT).to_edge(UP, buff=0.4)
        t = table([("Option", "Meaning")] + OPTIONS, size=20, row_buff=0.17, col_buff=0.5,
                  colors={0: FG_STRONG, 1: MUTED}).next_to(head, DOWN, buff=0.3)
        self.play(FadeIn(head), run_time=0.4)
        self.play_at("all the", LaggedStart(*[FadeIn(r, shift=UP * 0.1) for r in t], lag_ratio=0.08), run_time=2.0)
        self.play_at("checks dot build", Create(hl(t[11], YELLOW_)), t[11].animate.set_color(YELLOW_), run_time=0.6)
        self.play_at("binary cache", Create(hl(t[12], CYAN)), t[12].animate.set_color(CYAN), run_time=0.6)
        self.finish()


COMPOSE = """services:
  nixos-manager:
    build: .
    ports: ["8080:8080"]
    environment:
      NIXMGR_AUTHOR_NAME: "Your Name"
      NIXMGR_AUTHOR_EMAIL: "you@example.com"
      NIXMGR_PASSWORD_HASH_FILE: /run/secrets/nixmgr/password-hash
      # NIXMGR_CHECK_BUILD: "0"
  data-owner:   # gives the data volume to the image's user
    user: root
    entrypoint: ["chown", "-R", "-h", "nixos-manager:nixos-manager", "/var/lib/nixos-manager"]"""


class S12_3(Narrated):
    sid = "S12.3"

    def construct(self):
        code = code_card(COMPOSE, "yaml", size=20).to_edge(LEFT, buff=0.5)
        name = M("compose.yaml", 22, ACCENT).next_to(code, UP, buff=0.2).align_to(code, LEFT)
        self.play_at("docker image", FadeIn(name), type_on(code, 1.6))
        self.play_at("unprivileged", Create(hl(lines(code, 9, 11), MUTED)), run_time=0.6)
        warn = card(VGroup(T("Docker: builds can read the server's data,", 24, YELLOW_),
                           T("including the key that decrypts the secrets.", 24, YELLOW_),
                           T("Only when everyone who can push may see the secrets.", 22, FG))
                    .arrange(DOWN, aligned_edge=LEFT, buff=0.12), color=YELLOW_, pad=0.3)
        warn.next_to(code, RIGHT, buff=0.4).shift(UP * 0.8)
        self.play_at("cannot sandbox", FadeIn(warn, shift=LEFT * 0.2), run_time=0.8)
        alt = T("Otherwise: the NixOS container", 24, GREEN).next_to(warn, DOWN, buff=0.3).align_to(warn, LEFT)
        self.play_at("otherwise", FadeIn(alt), run_time=0.5)
        net = card(T("plain HTTP → trusted network, Tailscale or a TLS proxy", 22, FG), color=ACCENT, pad=0.2)
        net.next_to(alt, DOWN, buff=0.4).align_to(warn, LEFT)
        self.play_at("plain", FadeIn(net), run_time=0.6)
        self.finish()
