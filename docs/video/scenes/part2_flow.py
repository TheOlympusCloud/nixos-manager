"""Part 2, chapters 22 to 27: changes, checks, commits, trials, deployments, inputs, history and code."""

from kit import *  # noqa: F403

LEFT_PANE = (0.11, 0.06, 0.18, 0.6)
RIGHT_PANE = (0.29, 0.05, 0.71, 0.9)


def clear_to_manim(scene):
    scene.play(*[FadeOut(m) for m in scene.mobjects if m is not scene.camera.frame],
               scene.camera.frame.animate.set_width(config.frame_width).move_to(ORIGIN), run_time=0.6)
    scene.marks = []


class S22_1(Capture):
    sid = "S22.1"

    def construct(self):
        img = self.show("changes")
        self.look(img, (0.11, 0.05, 0.89, 0.9), "every click", margin=1.02, mark=False)
        self.look(img, (0.29, 0.25, 0.71, 0.6), "text diff", margin=1.1)
        img = self.show("changes-staged", "stage")
        self.look(img, LEFT_PANE, margin=1.2)
        self.finish()


LEGEND = [("+ packages, services, users, ports", GREEN), ("units: restart · reload · start · stop · not restarted", YELLOW_),
          ("/etc files added, changed, removed", ACCENT), ("reboot needed: kernel, initrd, kernel parameters", RED_),
          ("after a check: closure size and version changes", PURPLE_), ("unchanged machines listed as unchanged", MUTED)]


class S22_2(Capture):
    sid = "S22.2"

    def construct(self):
        img = self.show("changes")
        self.look(img, (0.29, 0.08, 0.71, 0.2), "below the diff", margin=1.1)
        self.lower("A machine changes when its system toplevel changes", "top level", ACCENT, 26)
        img = self.show("changes-2", "for each changed machine")
        self.look(img, (0.29, 0.05, 0.71, 0.9), margin=1.02, mark=False)
        self.until("decided", 0.3)
        clear_to_manim(self)
        rows = VGroup(*[T(t, 26, c) for t, c in LEGEND]).arrange(DOWN, aligned_edge=LEFT, buff=0.3)
        title = T("On the machines", 34, FG_STRONG, weight=BOLD).next_to(rows, UP, buff=0.5).align_to(rows, LEFT)
        self.play(FadeIn(title), FadeIn(rows[0]), FadeIn(rows[1]), run_time=0.6)
        for cue, i in (("slash", 2), ("reboot", 3), ("closure size", 4), ("unchanged", 5)):
            self.play_at(cue, FadeIn(rows[i], shift=RIGHT * 0.2), run_time=0.4)
        self.finish()


class S22_3(Capture):
    sid = "S22.3"

    def construct(self):
        img = self.show("checks-panel")
        self.look(img, LEFT_PANE, "run the check", margin=1.2)
        img = self.show("check-running", "flake check")
        self.look(img, RIGHT_PANE, margin=1.02, mark=False)
        self.until("really builds", 0.3)
        clear_to_manim(self)
        line = commit_line(4, gap=1.6).move_to(LEFT * 2 + UP * 1.2)
        ok = Dot(radius=0.16, color=GREEN).move_to(line[1][2])
        lab = T("newest pushed commit that passed a check", 22, GREEN).next_to(ok, UP, buff=0.3)
        staged = box("staged content", YELLOW_, 24).move_to(RIGHT * 4 + UP * 1.2)
        cmp = DashedLine(ok.get_right(), staged.get_left(), stroke_color=YELLOW_)
        self.play(FadeIn(line), FadeIn(ok), FadeIn(lab), FadeIn(staged), Create(cmp), run_time=0.8)
        hosts = VGroup(*[box(n, c, 22, font=MONO) for n, c in (("homeserver", YELLOW_), ("pi", YELLOW_), ("workstation", LINE_STRONG))])
        hosts.arrange(RIGHT, buff=0.6).move_to(DOWN * 1.4)
        tags = VGroup(T("toplevel differs → build", 20, YELLOW_), T("toplevel differs → build", 20, YELLOW_),
                      T("same → skip", 20, MUTED))
        for t, h in zip(tags, hosts):
            t.next_to(h, DOWN, buff=0.15)
        self.play_at("so a package", FadeIn(hosts), LaggedStart(*[FadeIn(t) for t in tags], lag_ratio=0.3), run_time=1.0)
        img = shot("check-done")
        self.play_at("log streams", *[FadeOut(m) for m in self.mobjects if m is not self.camera.frame], FadeIn(img), run_time=0.6)
        self.look(img, RIGHT_PANE, margin=1.02, mark=False)
        img = self.show("checks-panel", "tick other")
        self.look(img, (0.11, 0.1, 0.18, 0.25), margin=1.4)
        self.lower("checks.build = false: evaluate and dry-build only", "small server", YELLOW_, 24)
        self.mark(img, (0.115, 0.85, 0.17, 0.06), "one at a time")
        self.finish()


class S22_4(Capture):
    sid = "S22.4"

    def construct(self):
        img = self.show("commit-message")
        self.look(img, "message", "write a message", margin=1.8)
        img = self.show("history-show", "trailer")
        self.look(img, RIGHT_PANE, margin=1.02, mark=False)
        self.lower("Via: web session · or Via: token <name>", "how it arrived", ACCENT, 26)
        self.finish()


class S22_5(Capture):
    sid = "S22.5"

    def construct(self):
        img = self.show("push-dialog")
        self.look(img, (0.24, 0.21, 0.52, 0.57), "push opens", margin=1.1)
        self.until("goes to", 0.3)
        clear_to_manim(self)
        srv = box("nixos-manager", ACCENT, 24).move_to(LEFT * 5 + UP * 1.5)
        fj = machine("forgejo", s=0.8).move_to(UP * 1.5)
        hosts = Group(*[machine(n, s=0.7) for n in ("homeserver", "pi", "workstation")]).arrange(RIGHT, buff=1.5).move_to(DOWN * 2)
        self.play(FadeIn(srv), FadeIn(fj), FadeIn(hosts), run_time=0.6)
        c = Dot(radius=0.15, color=YELLOW_).move_to(srv.get_right())
        self.play(c.animate.move_to(fj.get_left()), run_time=1.0)
        pulls = VGroup(*[Arrow(h.get_top(), fj.get_bottom(), buff=0.1, stroke_color=CYAN, stroke_width=3) for h in hosts[:2]])
        self.play_at("comin takes over", LaggedStart(*[GrowArrow(p) for p in pulls], lag_ratio=0.3),
                     hosts[0][0].animate.set_stroke(GREEN, 4), hosts[1][0].animate.set_stroke(GREEN, 4),
                     hosts[2].animate.set_opacity(0.35), run_time=1.0)
        warn = card(T("Pull rebased local commits → never checked as a whole → Push refuses.\n"
                      "Run the check again with nothing staged, then push.", 22, YELLOW_), color=YELLOW_, pad=0.25)
        warn.to_edge(DOWN, buff=0.3)
        self.play_at("rebases", FadeIn(warn), FadeOut(hosts), FadeOut(pulls), run_time=0.6)
        self.finish()


class S23_1(Capture):
    sid = "S23.1"

    def construct(self):
        img = self.show("trial-ready")
        self.look(img, LEFT_PANE, "try it on one machine", margin=1.2, mark=False)
        self.mark(img, "try", "try on pi")
        img = self.show("trial-confirm", "testing pi branch")
        self.look(img, "dialog", margin=1.3, mark=False)
        img = self.show("trial-started", "only the pi")
        self.look(img, (0.11, 0.0, 0.89, 0.5), margin=1.05, mark=False)
        self.until("watch it", 0.3)
        img = self.show("trial-changes")
        self.look(img, (0.11, 0.05, 0.89, 0.9), margin=1.02, mark=False)
        self.until("why does", 0.3)
        clear_to_manim(self)
        pi = machine("pi", s=1.0).move_to(LEFT * 3)
        cut = cross(Line(LEFT, RIGHT).move_to(LEFT * 0.3)).scale(0.6)
        fj = machine("forgejo", s=0.9).move_to(RIGHT * 3.5)
        net = DashedLine(pi.get_right(), fj.get_left(), stroke_color=RED_)
        self.play(FadeIn(pi), FadeIn(fj), Create(net), run_time=0.6)
        self.play_at("loses", Create(cut), run_time=0.5)
        ok = T("a trial: a reboot is always enough", 30, GREEN).to_edge(DOWN, buff=1.0)
        self.play_at("with a trial", FadeIn(ok), run_time=0.5)
        self.finish()


class S24_1(Capture):
    sid = "S24.1"

    def construct(self):
        img = self.show("deploy-setup")
        self.look(img, "step1", "report back", margin=1.2)
        self.look(img, "step2", "report token", margin=1.2)
        img = self.show("deploy-setup-2", "one click")
        self.look(img, (0.11, 0.05, 0.89, 0.9), margin=1.02, mark=False)
        self.lower("A token reports for its own machine only", "own machine", ACCENT, 26)
        self.finish()


class S24_2(Capture):
    sid = "S24.2"

    def construct(self):
        img = self.show("deployments")
        self.look(img, (0.11, 0.05, 0.89, 0.9), "latest commits", margin=1.02, mark=False)
        self.mark(img, "pi", "error", RED_)
        img = self.show("deployments-2", "heartbeat")
        self.look(img, (0.11, 0.05, 0.89, 0.9), margin=1.02, mark=False)
        self.lower("silent after 2 h without a heartbeat · laptops: silentAfter = null", "silent", YELLOW_, 24)
        self.lower("changed by hand · switch unfinished · reboot pending", "changed by hand", FG_STRONG, 24)
        self.finish()


class S24_3(Capture):
    sid = "S24.3"

    def construct(self):
        img = self.show("revert-dialog")
        self.look(img, (0.11, 0.05, 0.89, 0.9), "revert the commit", margin=1.02, mark=False)
        img = self.show("revert-changes", "stages a revert")
        self.look(img, (0.11, 0.05, 0.89, 0.9), margin=1.02, mark=False)
        self.until("no", 0.3)
        clear_to_manim(self)
        line = commit_line(4, gap=1.5).move_to(LEFT * 1.5)
        bad = line[1][3]
        self.play(FadeIn(line), bad.animate.set_color(RED_), run_time=0.6)
        rev = Dot(radius=0.13, color=GREEN).move_to(bad.get_center() + RIGHT * 1.5)
        rl = Line(bad.get_center(), rev.get_center(), stroke_color=LINE_STRONG, stroke_width=3)
        tl = T("revert: a new commit with the old configuration", 24, GREEN).next_to(rev, UP, buff=0.4)
        self.play_at("forward", Create(rl), FadeIn(rev, scale=0.5), FadeIn(tl), run_time=0.8)
        note = card(T("Network cut off? Push the fix, then pick the previous generation\nin the boot menu: comin picks the fix up once online.",
                      22, FG), color=YELLOW_, pad=0.25).to_edge(DOWN, buff=0.4)
        self.play_at("cut a machine", FadeIn(note), run_time=0.6)
        self.finish()


class S24_4(Capture):
    sid = "S24.4"

    def construct(self):
        img = self.show("notifications")
        self.look(img, (0.11, 0.05, 0.6, 0.9), "notifications", margin=1.05, mark=False)
        phone = RoundedRectangle(corner_radius=0.3, width=2.6, height=4.6, stroke_color=LINE_STRONG, fill_color=BG, fill_opacity=1)
        msg = card(VGroup(T("ntfy · nixos-manager", 16, MUTED), T("pi failed to deploy a3f19c2", 18, FG_STRONG))
                   .arrange(DOWN, aligned_edge=LEFT, buff=0.08), color=RED_, pad=0.12).scale_to_fit_width(2.3)
        frame = self.camera.frame
        g = VGroup(phone, msg.move_to(phone.get_top() + DOWN * 0.8))
        g.scale(frame.width / config.frame_width).move_to(frame.get_center() + RIGHT * frame.width * 0.32)
        self.play_at("fails to deploy", FadeIn(g, shift=LEFT * 0.3), run_time=0.6)
        self.finish()


class S25_1(Capture):
    sid = "S25.1"

    def construct(self):
        img = self.show("dependencies")
        self.look(img, (0.11, 0.05, 0.89, 0.9), "side by side", margin=1.02, mark=False)
        self.lower("Versions differ → e.g. pi still runs openssl 3.0.15", "older", YELLOW_, 24)
        self.finish()


class S26_1(Capture):
    sid = "S26.1"

    def construct(self):
        img = self.show("inputs")
        self.look(img, (0.11, 0.05, 0.89, 0.6), "inputs lists", margin=1.05, mark=False)
        self.lower("amber after 30 days · red after 60", "amber", YELLOW_, 26)
        self.mark(img, "update", "update one")
        self.finish()


class S26_2(Capture):
    sid = "S26.2"

    def construct(self):
        img = self.show("scheduled")
        self.look(img, "card", "schedule", margin=1.3)
        self.until("commits the new lock", 0.3)
        clear_to_manim(self)
        main = commit_line(4, gap=1.5).move_to(LEFT * 1.5)
        ml = M("main", 24, ACCENT).next_to(main, LEFT, buff=0.3)
        up = Dot(radius=0.13, color=YELLOW_).move_to(main[1][3].get_center() + UP * 1.3 + RIGHT * 1.5)
        br = Line(main[1][3].get_center(), up.get_center(), stroke_color=YELLOW_, stroke_width=3)
        bl = M("flake-update (no machine follows it)", 22, YELLOW_).next_to(up, RIGHT, buff=0.2)
        self.play(FadeIn(main), FadeIn(ml), run_time=0.5)
        self.play(Create(br), FadeIn(up), FadeIn(bl), run_time=0.8)
        chk = T("checked ✓ · notification: ready", 22, GREEN).next_to(bl, DOWN, buff=0.2).align_to(bl, LEFT)
        self.play_at("checks it", FadeIn(chk), run_time=0.5)
        img = shot("scheduled-2")
        self.play_at("inputs then shows", *[FadeOut(m) for m in self.mobjects if m is not self.camera.frame],
                     FadeIn(img), run_time=0.6)
        self.look(img, (0.11, 0.05, 0.89, 0.9), margin=1.02, mark=False)
        self.lower("Merge into main = a fast-forward · or Discard", "merge into main", GREEN, 24)
        self.finish()


class S27_1(Capture):
    sid = "S27.1"

    def construct(self):
        img = self.show("history-commit")
        self.look(img, (0.11, 0.05, 0.89, 0.9), "every commit", margin=1.02, mark=False)
        img = self.show("history-show", "what it did")
        self.look(img, RIGHT_PANE, margin=1.02, mark=False)
        img = self.show("file-history", "single file")
        self.look(img, (0.11, 0.05, 0.89, 0.9), margin=1.02, mark=False)
        self.finish()


class S27_2(Capture):
    sid = "S27.2"

    def construct(self):
        img = self.show("code-file")
        self.look(img, (0.11, 0.05, 0.89, 0.9), "full editor", margin=1.02, mark=False)
        self.look(img, (0.25, 0.08, 0.6, 0.6), "completion", margin=1.1, mark=False)
        self.lower("Ctrl+S saves · Shift+Alt+F formats with nixfmt", "control", ACCENT, 26)
        self.lower("Clicks go to managed.json; your hand-written files stay yours", "never rewrites", GREEN, 24)
        self.finish()
