"""Part 1: how Nix and NixOS work (chapters 0 to 10)."""

from kit import *  # noqa: F403


def folder(path, entries, color=LINE_STRONG, w=3.6):
    head = M(path, 26, ACCENT)
    rows = VGroup(*[M(e, 22, FG) for e in entries]).arrange(DOWN, aligned_edge=LEFT, buff=0.16)
    body = VGroup(head, rows).arrange(DOWN, aligned_edge=LEFT, buff=0.3)
    r = RoundedRectangle(corner_radius=0.15, width=w, height=3.4, stroke_color=color, stroke_width=2.5,
                         fill_color=RAISED, fill_opacity=1)
    body.move_to(r).align_to(r, UP).shift(DOWN * 0.3)
    body.align_to(r, LEFT).shift(RIGHT * 0.3)
    g = VGroup(r, body)
    g.rows = rows
    return g


class S1_1(Narrated):
    sid = "S1.1"

    def construct(self):
        bin_ = folder("/usr/bin", ["bash", "ssh", "python3"], w=3.2)
        lib = folder("/usr/lib", ["libc.so", "libz.so"], w=3.2)
        etc = folder("/etc", ["hosts", "fstab", "ssh/sshd_config"], w=3.9)
        tree = VGroup(bin_, lib, etc).arrange(RIGHT, buff=0.4).shift(DOWN * 0.45)
        self.play(LaggedStart(*[FadeIn(f, shift=UP * 0.2) for f in tree], lag_ratio=0.25), run_time=1.5)
        cap = self.caption("One shared place, overwritten in place")
        self.play_at("shared", FadeIn(cap))
        for word, f in (("libraries", lib), ("programs", bin_), ("settings", etc)):
            self.play_at(word, f[0].animate.set_stroke(ACCENT, 4), run_time=0.4)
            self.play(f[0].animate.set_stroke(LINE_STRONG, 2.5), run_time=0.4)

        a = box("package A", GREEN, 24).move_to([lib.get_x() - 1.9, 2.85, 0])
        b = box("package B", GREEN, 24).move_to([lib.get_x() + 1.9, 2.85, 0])
        ssl30 = M("libssl.so.3.0", 22, GREEN).next_to(lib.rows, DOWN, aligned_edge=LEFT, buff=0.16)
        self.until("two programs", 0.2)
        arrow_a = Arrow(a.get_bottom(), lib[0].get_top() + LEFT * 0.5, buff=0.05, stroke_color=GREEN, stroke_width=3)
        self.play(FadeIn(a, shift=LEFT * 0.3), run_time=0.6)
        self.play(GrowArrow(arrow_a), Write(ssl30), run_time=0.8)
        self.play(FadeIn(b, shift=LEFT * 0.3), run_time=0.6)
        ssl32 = M("libssl.so.3.2", 22, YELLOW_).move_to(ssl30, aligned_edge=LEFT)
        arrow_b = Arrow(b.get_bottom(), lib[0].get_top() + RIGHT * 0.5, buff=0.05, stroke_color=YELLOW_, stroke_width=3)
        self.play_at("second", GrowArrow(arrow_b), run_time=0.6)
        self.play(Transform(ssl30, ssl32), Flash(ssl30, color=YELLOW_), run_time=0.8)
        self.play_at("breaks", a[0].animate.set_stroke(RED_), a[1].animate.set_color(RED_),
                     arrow_a.animate.set_color(RED_), run_time=0.6)
        self.play(Wiggle(a), run_time=0.8)

        line = etc.rows[2]
        pencil = M("✎", 30, YELLOW_).next_to(line, RIGHT, buff=0.15)
        self.play_at("edit", line.animate.set_color(YELLOW_), FadeIn(pencil, shift=DOWN * 0.2), run_time=0.6)
        note = VGroup(*[M(q, 22, MUTED) for q in ("who?", "why?", "what was it before?")]).arrange(DOWN, buff=0.1,
                                                                                                aligned_edge=LEFT)
        note.next_to(etc.rows, DOWN, buff=0.35, aligned_edge=LEFT)
        self.play_at("nothing", LaggedStart(*[FadeIn(n) for n in note], lag_ratio=0.4), run_time=1.2)

        self.until("two machines", 0.4)
        self.clear(run_time=0.6)
        pkgs = ["nginx 1.24", "openssl 3.0", "python 3.11", "sshd: port 22"]

        def host(name):
            m = machine(name, s=0.9)
            items = VGroup(*[M(p, 22, FG) for p in pkgs]).arrange(DOWN, aligned_edge=LEFT, buff=0.15)
            items.next_to(m, DOWN, buff=0.35)
            return Group(m, items)

        left, right = host("homeserver"), host("workstation")
        Group(left, right).arrange(RIGHT, buff=3.0).shift(UP * 0.3)
        self.play(FadeIn(left), FadeIn(right), run_time=0.8)
        drift = [(right[1][1], "openssl 3.2", YELLOW_), (left[1][3], "sshd: port 2222", YELLOW_),
                 (right[1][0], "nginx 1.26", YELLOW_), (left[1][2], "python 3.12", YELLOW_)]
        self.play_at("drift", LaggedStart(*[Transform(m, M(t, 22, c).move_to(m, aligned_edge=LEFT))
                                            for m, t, c in drift], lag_ratio=0.5), run_time=2.4)
        extra = M("+ hand-fixed cron job", 22, RED_).next_to(right[1], DOWN, aligned_edge=LEFT, buff=0.15)
        self.play(FadeIn(extra, shift=UP * 0.1), run_time=0.5)
        neq = T("≠", 90, RED_).move_to(Group(left, right))
        self.play(FadeIn(neq, scale=0.5), run_time=0.5)
        quote = T("“it works on my machine”", 34, FG_STRONG, slant=ITALIC).to_edge(DOWN, buff=0.6)
        self.play_at("works", FadeIn(quote, shift=UP * 0.2), run_time=0.8)
        self.finish()


class S1_2(Narrated):
    sid = "S1.2"

    def construct(self):
        lib = folder("/usr/lib", ["libc.so", "libz.so", "libssl.so.3.2"]).shift(LEFT * 3.5)
        self.add(lib)
        self.until("never", 0.1)
        shards = VGroup(*[Square(0.45, fill_color=RAISED, fill_opacity=1, stroke_color=LINE_STRONG, stroke_width=2)
                          .move_to(lib[0].get_center() + np.array([(i % 6 - 2.5) * 0.55, (i // 6 - 2.5) * 0.55, 0]))
                          for i in range(36)])
        self.play(FadeOut(lib[1]), FadeIn(shards), FadeOut(lib[0]), run_time=0.3)
        self.play(LaggedStart(*[s.animate.shift(np.array([(i % 6 - 2.5) * 0.9, (i // 6 - 2.5) * 0.9 - 2, 0]))
                                .rotate((i % 5 - 2) * 0.6).set_opacity(0) for i, s in enumerate(shards)],
                              lag_ratio=0.02), run_time=1.2)
        self.remove(shards)

        paths = [pill("libssl-3.0", "7x2kq…"), pill("libssl-3.2", "b9mzc…"), pill("nginx-1.26", "p1lww…")]
        col = VGroup(*paths).arrange(DOWN, buff=0.45).shift(RIGHT * 1.6 + UP * 0.4)
        locks = VGroup(*[padlock(s=0.3).next_to(p, RIGHT, buff=0.2) for p in paths])
        self.play_at("every package", LaggedStart(*[FadeIn(p, shift=RIGHT * 0.3) for p in paths], lag_ratio=0.35),
                     run_time=1.5)
        self.play(LaggedStart(*[FadeIn(lk, scale=0.6) for lk in locks], lag_ratio=0.2), run_time=0.8)
        self.play_at("fingerprint", *[Indicate(p.hash, color=YELLOW_, scale_factor=1.12) for p in paths], run_time=1.2)
        self.play_at("two versions", *[Indicate(p, color=ACCENT, scale_factor=1.05) for p in paths[:2]], run_time=1.0)

        a = box("package A", GREEN, 24).move_to(LEFT * 4.6 + UP * 1.6)
        b = box("package B", GREEN, 24).move_to(LEFT * 4.6 + DOWN * 0.2)
        arr_a = Arrow(a.get_right(), paths[0].get_left(), buff=0.12, stroke_color=GREEN, stroke_width=3)
        arr_b = Arrow(b.get_right(), paths[1].get_left(), buff=0.12, stroke_color=GREEN, stroke_width=3)
        self.play_at("each program", FadeIn(a), FadeIn(b), run_time=0.5)
        self.play(GrowArrow(arr_a), GrowArrow(arr_b), run_time=0.8)
        cap = self.caption("Every version in its own place. Nothing is overwritten.")
        self.play(FadeIn(cap), run_time=0.6)

        c = box("package C", ACCENT, 24).move_to(LEFT * 4.6 + DOWN * 2.0)
        new = pill("libssl-3.4", "q8hn2…").next_to(col, DOWN, buff=0.45).align_to(col, LEFT)
        arr_c = Arrow(c.get_right(), new.get_left(), buff=0.12, stroke_color=ACCENT, stroke_width=3)
        self.play_at("installing", FadeOut(cap), FadeIn(c), FadeIn(new, shift=RIGHT * 0.3), run_time=0.8)
        self.play(GrowArrow(arr_c), run_time=0.6)
        checks = VGroup(*[T("✓", 34, GREEN).next_to(x, LEFT, buff=0.15) for x in (a, b, c)])
        self.play(LaggedStart(*[FadeIn(ck, scale=0.5) for ck in checks], lag_ratio=0.2), run_time=0.8)

        self.until("rebuild", 0.3)
        everything = VGroup(a, b, c, arr_a, arr_b, arr_c, checks, *paths, new, locks)
        self.play(everything.animate.scale_to_fit_width(6.0).to_edge(LEFT, buff=0.4), run_time=1.0)
        copy = everything.copy()
        self.play(copy.animate.to_edge(RIGHT, buff=0.4), run_time=1.4)
        eq = T("=", 90, GREEN).move_to(ORIGIN)
        self.play_at("same result", FadeIn(eq, scale=0.5), run_time=0.6)
        self.finish()


def drift_pile(names, color=YELLOW_):
    return VGroup(*[M(n, 18, color) for n in names]).arrange(DOWN, aligned_edge=LEFT, buff=0.08)


class S0_1(Narrated):
    sid = "S0.1"

    def construct(self):
        hosts = [machine("homeserver", s=1.0), machine("pi", s=1.0), machine("workstation", s=1.0)]
        row = Group(*hosts).arrange(RIGHT, buff=1.6).shift(DOWN * 0.6)
        for cue, h in zip(("home server", "raspberry", "desktop"), hosts):
            self.play_at(cue, FadeIn(h, shift=UP * 0.3), run_time=0.6)
        piles = [drift_pile(["apt install nginx", "sshd_config ✎", "cron: fix-disk.sh"]),
                 drift_pile(["pip install --user", "/etc/hosts ✎", "libssl 1.1 pinned"]),
                 drift_pile(["curl | sh", "~/.bashrc hack", "driver 535 by hand"])]
        for p, h in zip(piles, hosts):
            p.next_to(h, DOWN, buff=0.25)
        self.play_at("collects", LaggedStart(*[LaggedStart(*[FadeIn(r, shift=DOWN * 0.15) for r in p], lag_ratio=0.5)
                                              for p in piles], lag_ratio=0.3), run_time=2.4)
        self.play(hosts[1][0].animate.set_stroke(PURPLE_), hosts[2][0].animate.set_stroke(YELLOW_), run_time=0.8)
        self.play_at("breaks", hosts[0][0].animate.set_stroke(RED_, 5), Wiggle(hosts[0]), run_time=0.8)
        q = T("?", 80, RED_).next_to(hosts[0], UP, buff=0.2)
        self.play_at("rebuild", FadeIn(q, scale=0.5), run_time=0.5)

        self.play_at("different", *[FadeOut(p, shift=DOWN * 0.5) for p in piles], FadeOut(q),
                     *[h[0].animate.set_stroke(ACCENT, 2.5) for h in hosts], run_time=1.0)
        files = VGroup(*[M(f, 20, FG) for f in ("hosts/homeserver/default.nix", "hosts/pi/default.nix",
                                                 "hosts/workstation/default.nix", "modules/common.nix")])
        files.arrange(DOWN, aligned_edge=LEFT, buff=0.12)
        self.play_at("described", FadeIn(files.move_to(UP * 2.4)), run_time=0.8)
        repo = card(VGroup(M("fleet.git", 24, ACCENT), files.copy()).arrange(DOWN, buff=0.2), color=ACCENT)
        repo.move_to(UP * 2.3)
        self.play_at("repository", ReplacementTransform(files, repo[1]), FadeIn(repo[0]), FadeIn(repo[1][0]),
                     run_time=1.0)
        beams = VGroup(*[Arrow(repo.get_bottom(), h.get_top(), buff=0.1, stroke_color=ACCENT, stroke_width=3)
                         for h in hosts])
        self.play_at("built", LaggedStart(*[GrowArrow(b) for b in beams], lag_ratio=0.2), run_time=1.0)
        checks = VGroup(*[T("✓", 40, GREEN).next_to(h, DOWN, buff=0.2) for h in hosts])
        self.play_at("same way", *[h[0].animate.set_stroke(GREEN, 4) for h in hosts],
                     LaggedStart(*[FadeIn(c, scale=0.5) for c in checks], lag_ratio=0.2), run_time=1.0)
        self.finish()


class S0_2(Narrated):
    sid = "S0.2"
    opening = True

    def construct(self):
        p1 = ["The Nix store", "Derivations", "Flakes", "The module system", "NixOS: build, switch, roll back",
              "home-manager", "Secrets", "Deploying with comin"]
        p2 = ["Fleet", "Machines", "Packages", "Services", "Modules", "Options", "Health", "Deployments",
              "Dependencies", "Inputs", "Changes", "History", "Code", "Settings"]
        h1 = T("Part 1 · How Nix works", 34, ACCENT, weight=BOLD)
        h2 = T("Part 2 · nixos-manager", 34, ACCENT, weight=BOLD)
        l1 = bullet_list(p1, 26)
        l2 = VGroup(*[M(x, 22, FG) for x in p2]).arrange(DOWN, aligned_edge=LEFT, buff=0.1)
        c1 = VGroup(h1, l1).arrange(DOWN, aligned_edge=LEFT, buff=0.4)
        c2 = VGroup(h2, l2).arrange(DOWN, aligned_edge=LEFT, buff=0.3)
        VGroup(c1, c2).arrange(RIGHT, buff=2.2, aligned_edge=UP).move_to(ORIGIN)
        self.play_at("two parts", FadeIn(h1), FadeIn(h2), run_time=0.6)
        for cue, i in (("store", 0), ("derivations", 1), ("flakes", 2), ("module system", 3),
                       ("running machine", 4)):
            self.play_at(cue, FadeIn(l1[i], shift=RIGHT * 0.2), run_time=0.4)
        self.play(LaggedStart(*[FadeIn(r, shift=RIGHT * 0.2) for r in l1[5:]], lag_ratio=0.3), run_time=1.0)
        self.play_at("second", LaggedStart(*[FadeIn(r, shift=RIGHT * 0.1) for r in l2], lag_ratio=0.1),
                     run_time=1.6)
        self.until("every view", 0.3)
        img = shot("fleet")
        self.play(FadeOut(c1), FadeOut(c2), FadeIn(img), run_time=0.8)
        self.play(self.camera.frame.animate.set_width(config.frame_width * 0.6).move_to(img.get_center() + LEFT * 1.0),
                  run_time=max(1.0, self.t0 + self.duration - self.time - 0.2), rate_func=smooth)
        self.finish()


class S2_1(Narrated):
    sid = "S2.1"

    def construct(self):
        head = M("/nix/store", 34, ACCENT).to_edge(UP, buff=0.7)
        paths = VGroup(pill("openssl-3.0.15", "7x2kq…"), pill("openssl-3.2.4", "b9mzc…"),
                       pill("nginx-1.26.2", "p1lww…"), pill("bash-5.2", "0dh4a…")).arrange(DOWN, buff=0.3)
        paths.next_to(head, DOWN, buff=0.5)
        self.play(FadeIn(head), run_time=0.6)
        self.play_at("store", LaggedStart(*[FadeIn(p, shift=UP * 0.2) for p in paths], lag_ratio=0.25),
                     run_time=1.6)
        self.play_at("store path", Indicate(paths[2], color=ACCENT, scale_factor=1.05), run_time=0.8)
        big = MK(span("/nix/store/", MUTED) + span("p1lwwz0g8d2f6qk1y9mxcbn3hrj4sv5a", FAINT) +
                 span("-nginx-1.26.2", FG_STRONG), 30)
        big.scale_to_fit_width(min(big.width, 12.6)).move_to(DOWN * 0.2)
        self.play_at("three parts", FadeOut(paths[:2]), FadeOut(paths[3]), ReplacementTransform(paths[2], big),
                     FadeOut(head), run_time=1.0)
        parts = [big[:11], big[11:43], big[43:]]
        labels = ["the store", "hash (32 characters)", "name and version"]
        braces = VGroup(*[Brace(p, DOWN, color=LINE_STRONG) for p in parts])
        texts = VGroup(*[T(l, 24, MUTED).next_to(b, DOWN, buff=0.15) for l, b in zip(labels, braces)])
        for cue, i in (("store itself", 0), ("hash", 1), ("readable", 2)):
            self.play_at(cue, GrowFromCenter(braces[i]), FadeIn(texts[i]), run_time=0.5)
        cap = self.caption("/nix/store/<hash>-<name>-<version>", ACCENT)
        self.play(FadeIn(cap), run_time=0.5)

        self.play_at("important", FadeOut(braces[0]), FadeOut(braces[2]), FadeOut(texts[0]), FadeOut(texts[2]),
                     FadeOut(cap), big.animate.shift(DOWN * 1.6), FadeOut(braces[1]), FadeOut(texts[1]),
                     run_time=0.8)
        hashbox = box("hash function", ACCENT, 24).move_to(UP * 0.6)
        ins = VGroup(*[box(t, LINE_STRONG, 22) for t in ("source code", "compiler", "build script", "libraries")])
        ins.arrange(RIGHT, buff=0.35).move_to(UP * 2.6)
        arrows = VGroup(*[Arrow(i.get_bottom(), hashbox.get_top(), buff=0.1, stroke_color=LINE_STRONG,
                                stroke_width=2.5) for i in ins])
        out = Arrow(hashbox.get_bottom(), big[11:43].get_top(), buff=0.15, stroke_color=ACCENT, stroke_width=3)
        self.play(FadeIn(hashbox), run_time=0.4)
        for cue, i in (("source code", 0), ("compiler", 1), ("build script", 2), ("libraries", 3)):
            self.play_at(cue, FadeIn(ins[i], shift=DOWN * 0.2), GrowArrow(arrows[i]), run_time=0.4)
        self.play(GrowArrow(out), Indicate(big[11:43], color=YELLOW_, scale_factor=1.05), run_time=0.8)
        flag = M("-O2 → -O3", 20, YELLOW_).next_to(ins[1], UP, buff=0.1)
        self.play_at("change any", ins[1][0].animate.set_stroke(YELLOW_, 3.5), FadeIn(flag), run_time=0.6)
        big2 = MK(span("/nix/store/", MUTED) + span("9fkq2w7xv0m3c8hn1yrd5sb4lza6tgpe", YELLOW_) +
                  span("-nginx-1.26.2", FG_STRONG), 30).match_width(big).move_to(big)
        self.play_at("different hash", Transform(big, big2), run_time=0.8)
        lock = padlock(s=0.4).next_to(big, RIGHT, buff=0.25)
        self.play_at("nothing", FadeIn(lock, scale=0.5), run_time=0.5)
        ro = T("read only", 26, YELLOW_).next_to(lock, DOWN, buff=0.15)
        self.play_at("read only", FadeIn(ro), run_time=0.4)
        self.finish()


class S2_2(Narrated):
    sid = "S2.2"

    def construct(self):
        def node(name, color=LINE_STRONG):
            return box(name, color, 24, font=MONO)

        ng = node("nginx-1.26", ACCENT).move_to(UP * 2.4 + LEFT * 2.5)
        libs = VGroup(node("openssl-3.0"), node("zlib-1.3"), node("pcre2-10.44")).arrange(RIGHT, buff=0.5)
        libs.move_to(UP * 0.4 + LEFT * 2.5)
        glibc = node("glibc-2.40").move_to(DOWN * 1.6 + LEFT * 2.5)
        e1 = VGroup(*[Arrow(ng.get_bottom(), l.get_top(), buff=0.08, stroke_width=3, stroke_color=ACCENT) for l in libs])
        e_glibc = Arrow(ng.get_bottom(), glibc.get_top(), buff=0.08, stroke_width=3, stroke_color=ACCENT,
                        path_arc=-0.9)
        e2 = VGroup(*[Arrow(l.get_bottom(), glibc.get_top(), buff=0.08, stroke_width=2.5, stroke_color=LINE_STRONG)
                      for l in libs])
        self.play(FadeIn(ng), run_time=0.6)
        self.play_at("points", LaggedStart(*[AnimationGroup(FadeIn(l), GrowArrow(e)) for l, e in zip(libs, e1)],
                                           lag_ratio=0.5), run_time=1.6)
        self.play_at("c library", FadeIn(glibc), GrowArrow(e_glibc), run_time=0.6)
        self.play_at("follow", LaggedStart(*[GrowArrow(e) for e in e2], lag_ratio=0.3), run_time=1.0)
        tree = VGroup(ng, libs, glibc, e1, e2, e_glibc)
        outline = DashedVMobject(SurroundingRectangle(tree, color=YELLOW_, buff=0.3, corner_radius=0.2), num_dashes=60)
        label = T("closure of nginx", 26, YELLOW_).next_to(outline, UP, buff=0.1)
        self.play_at("closure", Create(outline), FadeIn(label), run_time=1.0)
        cap = self.caption("Closure: a path plus everything it references")
        self.play(FadeIn(cap), run_time=0.5)

        other = machine("pi", s=1.1).move_to(RIGHT * 4.6 + UP * 0.6)
        self.play_at("copy", FadeOut(cap), FadeIn(other), run_time=0.6)
        unit = VGroup(tree, outline).copy()
        self.play(unit.animate.scale(0.32).next_to(other, DOWN, buff=0.3), run_time=1.4)
        runs = T("runs ✓", 26, GREEN).next_to(unit, DOWN, buff=0.15)
        self.play(FadeIn(runs), run_time=0.4)
        cache = box("binary cache", CYAN, 24).move_to(RIGHT * 4.6 + UP * 3.1)
        dl = Arrow(cache.get_bottom(), other.get_top(), buff=0.1, stroke_color=CYAN, stroke_width=3)
        self.play_at("binary caches", FadeIn(cache), GrowArrow(dl), run_time=0.8)
        same = M("same hash → download", 20, CYAN).next_to(dl, LEFT, buff=0.15)
        self.play_at("download", FadeIn(same), run_time=0.5)
        self.play_at("keep", Indicate(label, color=YELLOW_), run_time=1.0)
        self.finish()


HELLO = '''stdenv.mkDerivation {
  pname = "hello";
  version = "2.12.1";
  src = fetchurl {
    url = "mirror://gnu/hello/hello-2.12.1.tar.gz";
    hash = "sha256-jZkUKv2SV28wsM18tCqNxoCZmLxdYH2Idh9RLibH2yA=";
  };
}'''


class S3_1(Narrated):
    sid = "S3.1"

    def construct(self):
        code = code_card(HELLO, size=24)
        self.play_at("derivation", type_on(code, 2.4))
        self.play_at("source", Create(hl(lines(code, 3, 6))), run_time=0.6)
        self.until("two steps", 0.3)
        self.clear(code, run_time=0.3)
        self.play(code.animate.scale(0.5).to_corner(UL, buff=0.4), run_time=0.8)
        expr = box("Nix expression", LINE_STRONG, 24)
        drv = pill("hello-2.12.1.drv", "a8x0f…", prefix="…/", size=20)
        build = box("build (realise)", ACCENT, 24)
        out = pill("hello-2.12.1", "4lk9c…", prefix="…/", size=20)
        row = VGroup(expr, drv, build, out).arrange(RIGHT, buff=1.0)
        row.scale_to_fit_width(min(row.width, 13.2)).move_to(UP * 0.4)
        arr = chain(expr, drv, build, out)
        ev = T("evaluate", 22, MUTED).next_to(arr[0], UP, buff=0.1)
        self.play_at("evaluates", FadeIn(expr), run_time=0.4)
        self.play(GrowArrow(arr[0]), FadeIn(ev), run_time=0.6)
        self.play_at("produces", FadeIn(drv, shift=RIGHT * 0.2), run_time=0.6)
        nb = T("nothing built yet", 22, FAINT).next_to(drv, DOWN, buff=0.2)
        self.play_at("nothing", FadeIn(nb), run_time=0.4)
        self.play_at("realises", GrowArrow(arr[1]), FadeIn(build), run_time=0.6)
        sandbox = DashedVMobject(SurroundingRectangle(build, color=YELLOW_, buff=0.25, corner_radius=0.15))
        sb = T("sandbox: no network, only declared inputs", 20, YELLOW_).next_to(sandbox, DOWN, buff=0.15)
        self.play_at("sandbox", Create(sandbox), FadeIn(sb), run_time=0.8)
        self.play_at("output", GrowArrow(arr[2]), FadeIn(out, shift=RIGHT * 0.2), run_time=0.8)
        cache = box("binary cache", CYAN, 24).move_to(DOWN * 2.4 + RIGHT * 1.0)
        ask = Arrow(drv.get_bottom(), cache.get_left(), buff=0.15, stroke_color=CYAN, stroke_width=3)
        q = T("already built?", 22, CYAN).next_to(ask, LEFT, buff=0.1)
        self.play_at("binary cache", FadeIn(cache), GrowArrow(ask), FadeIn(q), run_time=0.8)
        dl = Arrow(cache.get_right(), out.get_bottom(), buff=0.15, stroke_color=CYAN, stroke_width=3)
        dlt = T("download instead", 22, CYAN).next_to(dl, RIGHT, buff=0.1)
        self.play_at("download", GrowArrow(dl), FadeIn(dlt), run_time=0.8)
        self.finish()


class S3_2(Narrated):
    sid = "S3.2"

    def construct(self):
        trunk = box("nixpkgs", ACCENT, 34, font=MONO).move_to(LEFT * 3.5 + UP * 0.2)
        leaves = ["firefox", "jellyfin", "python3", "linuxPackages", "nixos/modules"]
        lv = VGroup(*[M(x, 24, FG) for x in leaves]).arrange(DOWN, aligned_edge=LEFT, buff=0.35).move_to(RIGHT * 0.5 + UP * 0.2)
        edges = VGroup(*[Line(trunk.get_right(), l.get_left() + LEFT * 0.15, stroke_color=LINE_STRONG, stroke_width=2.5)
                         for l in lv])
        self.play_at("nix packages is", FadeIn(trunk), run_time=0.6)
        self.play(LaggedStart(*[AnimationGroup(Create(e), FadeIn(l)) for e, l in zip(edges[:4], lv[:4])],
                              lag_ratio=0.3), run_time=1.4)
        n = ValueTracker(0)
        counter = always_redraw(lambda: T(f"{int(n.get_value()):,} packages".replace(",", " "), 34, YELLOW_)
                                .move_to(RIGHT * 4.8 + UP * 0.2))
        self.add(counter)
        self.play_at("hundred thousand", n.animate.set_value(120000), run_time=1.6, rate_func=rush_from)
        self.remove(counter)
        counter = T("120 000+ packages", 34, YELLOW_).move_to(RIGHT * 4.8 + UP * 0.2)
        self.add(counter)
        self.play_at("modules", Create(edges[4]), FadeIn(lv[4]), lv[4].animate.set_color(ACCENT), run_time=0.6)
        self.until("comes in branches", 0.3)
        self.clear(run_time=0.5)
        main = Line(LEFT * 6, RIGHT * 6, stroke_color=ACCENT, stroke_width=4).shift(UP * 1.2)
        stable = Line(LEFT * 2, RIGHT * 6, stroke_color=GREEN, stroke_width=4).shift(DOWN * 0.8)
        fork = Line(main.point_from_proportion(0.33), stable.get_start(), stroke_color=GREEN, stroke_width=4)
        tu = M("nixos-unstable", 26, ACCENT).next_to(main, UP, buff=0.15).to_edge(LEFT, buff=0.6)
        ts = M("nixos-26.05 (stable)", 26, GREEN).next_to(stable, DOWN, buff=0.15).align_to(stable, LEFT)
        dots_u = VGroup(*[Dot(main.point_from_proportion(i / 14), radius=0.07, color=ACCENT) for i in range(15)])
        dots_s = VGroup(*[Dot(stable.point_from_proportion(i / 4), radius=0.07, color=GREEN) for i in range(5)])
        self.play_at("stable", Create(main), Create(fork), Create(stable), FadeIn(tu), FadeIn(ts), run_time=1.2)
        sec = T("security fixes for about 7 months", 22, GREEN).next_to(ts, DOWN, buff=0.15).align_to(ts, LEFT)
        self.play_at("security", LaggedStart(*[FadeIn(d) for d in dots_s], lag_ratio=0.2), FadeIn(sec), run_time=1.0)
        self.play_at("unstable", LaggedStart(*[FadeIn(d) for d in dots_u], lag_ratio=0.05), run_time=1.0)
        m = machine("homeserver", s=0.8).move_to(RIGHT * 4.6 + DOWN * 2.6)
        follow = Arrow(m.get_top(), stable.point_from_proportion(0.8), buff=0.1, stroke_color=GREEN, stroke_width=3)
        self.play_at("follows", FadeIn(m), GrowArrow(follow), run_time=0.8)
        one = Arrow(dots_u[13].get_center(), m.get_top() + LEFT * 0.4, buff=0.12, stroke_color=ACCENT, stroke_width=3)
        lbl = M("one package", 20, ACCENT).next_to(one, LEFT, buff=0.1)
        self.play_at("single package", GrowArrow(one), FadeIn(lbl), run_time=0.8)

        self.until("a word", 0.3)
        self.clear(run_time=0.5)
        title = T("The Nix language in three shapes", 34, FG_STRONG).to_edge(UP, buff=0.7)
        cards = [code_card('{ name = "hello"; version = "2.12"; }', size=24),
                 code_card('x: x + 1', size=24),
                 code_card('let\n  port = 8096;\nin\n  { allowedTCPPorts = [ port ]; }', size=24)]
        names = ["attribute set", "function", "let binding"]
        col = VGroup(*[VGroup(T(nm, 24, ACCENT), c).arrange(DOWN, aligned_edge=LEFT, buff=0.15)
                       for nm, c in zip(names, cards)]).arrange(DOWN, aligned_edge=LEFT, buff=0.4).next_to(title, DOWN, buff=0.5)
        self.play(FadeIn(title), run_time=0.5)
        for cue, g in zip(("attribute sets", "functions", "let bindings"), col):
            self.play_at(cue, FadeIn(g, shift=UP * 0.2), run_time=0.6)
        self.finish()


FLAKE = '''{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
    home-manager.url = "github:nix-community/home-manager/release-26.05";
    comin.url = "github:nlewo/comin";
    sops-nix.url = "github:Mic92/sops-nix";
    disko.url = "github:nix-community/disko";
  };
  outputs = { nixpkgs, ... }@inputs: {
    nixosConfigurations.homeserver = nixpkgs.lib.nixosSystem { … };
  };
}'''


class S4_1(Narrated):
    sid = "S4.1"

    def construct(self):
        code = code_card(FLAKE, size=22)
        name = M("flake.nix", 24, ACCENT).next_to(code, UP, buff=0.2).align_to(code, LEFT)
        self.play_at("folder", FadeIn(name), type_on(code, 2.0))
        r_in = hl(lines(code, 1, 7), ACCENT)
        r_out = hl(lines(code, 8, 10), GREEN)
        self.play_at("inputs", Create(r_in), run_time=0.6)
        self.play_at("outputs", Create(r_out), run_time=0.6)
        self.until("fleet", 0.3)
        self.play(FadeOut(r_in), FadeOut(r_out), FadeOut(name), code.animate.scale(0.45).to_corner(UL, buff=0.3),
                  run_time=0.8)
        ins = VGroup(*[box(n, ACCENT, 22, font=MONO) for n in ("nixpkgs", "home-manager", "comin", "sops-nix", "disko")])
        ins.arrange(DOWN, buff=0.2).move_to(LEFT * 4.2 + DOWN * 1.3)
        fn = box("outputs = { … }: { … }", LINE_STRONG, 24, font=MONO).move_to(DOWN * 1.3 + RIGHT * 0.2)
        outs = VGroup(*[box(f"nixosConfigurations.{n}", GREEN, 20, font=MONO) for n in ("homeserver", "pi", "workstation")])
        outs.arrange(DOWN, buff=0.35).move_to(RIGHT * 4.6 + DOWN * 1.3)
        a_in = VGroup(*[Arrow(i.get_right(), fn.get_left(), buff=0.1, stroke_color=ACCENT, stroke_width=2.5) for i in ins])
        a_out = VGroup(*[Arrow(fn.get_right(), o.get_left(), buff=0.1, stroke_color=GREEN, stroke_width=2.5) for o in outs])
        self.play(LaggedStart(*[FadeIn(i) for i in ins], lag_ratio=0.15), FadeIn(fn), run_time=1.0)
        self.play(LaggedStart(*[GrowArrow(a) for a in a_in], lag_ratio=0.1), run_time=0.8)
        self.play_at("configurations", LaggedStart(*[AnimationGroup(GrowArrow(a), FadeIn(o)) for a, o in zip(a_out, outs)],
                                                   lag_ratio=0.3), run_time=1.2)
        self.play_at("named", *[Indicate(o[1][len("nixosConfigurations."):], color=YELLOW_) for o in outs], run_time=1.0)
        self.finish()


LOCK = '''"nixpkgs": {
  "locked": {
    "owner": "NixOS",
    "repo": "nixpkgs",
    "rev": "a3f19c2e7b1d…",
    "narHash": "sha256-Qm3v…",
    "lastModified": 1764000000
  }
}'''


class S4_2(Narrated):
    sid = "S4.2"

    def construct(self):
        code = code_card(LOCK, "json", size=24)
        name = M("flake.lock", 24, ACCENT).next_to(code, UP, buff=0.2).align_to(code, LEFT)
        self.play_at("next to", FadeIn(name), type_on(code, 1.6))
        self.play_at("revision", Create(hl(lines(code, 4))), run_time=0.5)
        self.play_at("hash", Create(hl(lines(code, 5))), run_time=0.5)
        cap = self.caption("Lock = the exact revision of every input")
        self.play(FadeIn(cap), run_time=0.5)
        self.until("whoever", 0.3)
        self.clear(run_time=0.5)
        ins = VGroup(*[box(n, ACCENT, 22, font=MONO) for n in ("nixpkgs", "home-manager", "comin")]).arrange(DOWN, buff=0.5)
        ins.move_to(LEFT * 4)
        fl = box("flake.nix", LINE_STRONG, 26, font=MONO).move_to(RIGHT * 0.5)
        arrows = VGroup(*[Arrow(i.get_right(), fl.get_left(), buff=0.1, stroke_color=ACCENT, stroke_width=3) for i in ins])
        locks = VGroup(*[padlock(s=0.32).move_to(a.get_center() + UP * 0.25) for a in arrows])
        self.play(FadeIn(ins), FadeIn(fl), LaggedStart(*[GrowArrow(a) for a in arrows], lag_ratio=0.2), run_time=1.0)
        self.play(LaggedStart(*[FadeIn(lk, shift=DOWN * 0.3) for lk in locks], lag_ratio=0.2), run_time=0.8)
        same = T("today, in a year, any machine: the same nixpkgs", 24, GREEN).to_edge(DOWN, buff=0.8)
        self.play_at("today", FadeIn(same), run_time=0.6)
        bump = M("rev a3f19c2 → 5d0e8b1", 22, YELLOW_).next_to(ins[0], UP, buff=0.2)
        self.play_at("update", FadeIn(bump, shift=UP * 0.2), Indicate(locks[0], color=YELLOW_), run_time=0.8)

        self.until("pure mode", 0.3)
        self.play(FadeOut(same), FadeOut(bump), run_time=0.4)
        bubble = Circle(radius=2.3, color=ACCENT, stroke_width=3).move_to(fl.get_center() + LEFT * 1.6)
        pure = T("pure evaluation", 24, ACCENT).next_to(bubble, UP, buff=0.1)
        self.play(Create(bubble), FadeIn(pure), run_time=0.8)
        targets = [("random files", "/var/lib/…", UP * 2.3 + RIGHT * 5.0), ("environment", "$HOME, $USER", RIGHT * 5.5),
                   ("network", "https://…", DOWN * 2.3 + RIGHT * 5.0)]
        err = None
        for cue, label, pos in targets:
            t = M(label, 22, FAINT).move_to(pos)
            a = Arrow(fl.get_right(), t.get_left(), buff=0.1, stroke_color=RED_, stroke_width=3)
            self.play_at(cue, FadeIn(t), GrowArrow(a), run_time=0.5)
            self.play(a.animate.put_start_and_end_on(fl.get_right(), bubble.point_at_angle(np.arctan2(*(pos - fl.get_center())[[1, 0]]))),
                      run_time=0.3)
            self.play(a.animate.set_opacity(0.25), t.animate.set_opacity(0.4), run_time=0.2)
        err = M("error: access to absolute path '/var/lib/…' is forbidden in pure evaluation mode", 18, RED_)
        err.to_edge(DOWN, buff=0.5)
        self.play_at("reproducible", FadeIn(err), run_time=0.6)
        need = self.caption("A new input needs a Lock before anything evaluates", YELLOW_, 26)
        self.play_at("keep this", FadeOut(err), FadeIn(need), run_time=0.6)
        self.finish()


class S5_1(Narrated):
    sid = "S5.1"

    def construct(self):
        a = VGroup(T("Module A (NixOS)", 24, MUTED), card(M("options.services.openssh.enable\n  : bool, default false", 22, FG),
                                                            color=ACCENT)).arrange(DOWN, aligned_edge=LEFT, buff=0.15)
        b = VGroup(T("Module B (your configuration)", 24, MUTED), card(M("services.openssh.enable = true;", 22, FG),
                                                                       color=GREEN)).arrange(DOWN, aligned_edge=LEFT, buff=0.15)
        a.move_to(LEFT * 3.8 + UP * 2.2)
        b.move_to(LEFT * 3.8 + DOWN * 0.3)
        self.play_at("modules", FadeIn(a[1][0], b[1][0]), run_time=0.6)
        self.play_at("declare", FadeIn(a), run_time=0.5)
        self.play_at("set values", FadeIn(b), run_time=0.5)
        self.play_at("thousands", Indicate(a, color=ACCENT, scale_factor=1.03), run_time=0.8)
        self.play_at("your own", Indicate(b, color=GREEN, scale_factor=1.03), run_time=0.8)
        ms = box("module system\nmerge", YELLOW_, 26).move_to(RIGHT * 1.6 + UP * 1.0)
        ea = Arrow(a.get_right(), ms.get_left(), buff=0.15, stroke_color=ACCENT, stroke_width=3)
        eb = Arrow(b.get_right(), ms.get_left(), buff=0.15, stroke_color=GREEN, stroke_width=3)
        self.play_at("collects", FadeIn(ms), GrowArrow(ea), GrowArrow(eb), run_time=0.8)
        cfg = M("config", 26, YELLOW_).next_to(ms, DOWN, buff=0.6)
        ec = Arrow(ms.get_bottom(), cfg.get_top(), buff=0.1, stroke_color=YELLOW_, stroke_width=3)
        self.play_at("merges", GrowArrow(ec), FadeIn(cfg), run_time=0.6)
        outs = VGroup(box("systemd.services.sshd", ACCENT, 22, font=MONO), box("/etc/ssh/sshd_config", ACCENT, 22, font=MONO))
        outs.arrange(RIGHT, buff=0.5).next_to(cfg, DOWN, buff=0.9).shift(RIGHT * 1.6)
        eo = VGroup(*[Arrow(cfg.get_bottom(), o.get_top(), buff=0.1, stroke_color=ACCENT, stroke_width=3) for o in outs])
        self.play_at("reacts", LaggedStart(GrowArrow(eo[0]), FadeIn(outs[0]), lag_ratio=0.5), run_time=0.8)
        self.play_at("writes", GrowArrow(eo[1]), FadeIn(outs[1]), run_time=0.6)
        cap = self.caption("You describe what you want; the module works out how.")
        self.play_at("describe", FadeIn(cap), run_time=0.6)
        self.finish()


class S5_2(Narrated):
    sid = "S5.2"

    def construct(self):
        def rule(n, left, right, result, color=GREEN):
            g = VGroup(T(n, 26, ACCENT, weight=BOLD),
                       VGroup(card(M(left, 20), color=LINE_STRONG), T("+", 30, MUTED), card(M(right, 20), color=LINE_STRONG),
                              T("→", 30, MUTED), card(M(result, 20, color), color=color)).arrange(RIGHT, buff=0.25))
            return g.arrange(DOWN, aligned_edge=LEFT, buff=0.15)

        r1 = rule("Attribute sets merge key by key", "settings.PermitRootLogin = \"no\";", "settings.X11Forwarding = false;",
                  "both keys")
        r2 = rule("Lists are joined", "allowedTCPPorts = [ 22 ];", "[ 80 443 ]", "[ 22 80 443 ]")
        r3 = rule("Two single values conflict", "time.timeZone = \"Europe/Madrid\";", "\"UTC\"", "error: conflicting definitions",
                  RED_)
        rules = VGroup(r1, r2, r3).arrange(DOWN, aligned_edge=LEFT, buff=0.55)
        rules.scale_to_fit_width(min(rules.width, 13.4)).move_to(ORIGIN)
        for cue, r in (("attribute sets", r1), ("lists", r2), ("single value", r3)):
            self.play_at(cue, FadeIn(r, shift=UP * 0.2), run_time=0.7)
        self.until("to resolve", 0.3)
        self.clear(run_time=0.5)

        rows = [("50", "lib.mkForce", PURPLE_), ("90", "set on one machine in nixos-manager", ACCENT),
                ("95", "set for every machine in nixos-manager", ACCENT), ("100", "plain hand-written value", FG),
                ("1000", "lib.mkDefault", MUTED), ("1500", "option's own default", FAINT)]
        ladder = VGroup()
        for p, w, c in rows:
            num = M(p, 30, c)
            ladder.add(VGroup(num, T(w, 26, c).next_to(num.get_left() + RIGHT * 1.5, RIGHT, buff=0)))
        ladder.arrange(DOWN, aligned_edge=LEFT, buff=0.32)
        ladder.move_to(LEFT * 0.8)
        arrow = Arrow(ladder.get_corner(DL) + LEFT * 0.5, ladder.get_corner(UL) + LEFT * 0.5, stroke_color=YELLOW_,
                      stroke_width=4, buff=0)
        wins = T("Lower number wins", 28, YELLOW_).rotate(PI / 2).next_to(arrow, LEFT, buff=0.15)
        self.play_at("priority", GrowArrow(arrow), FadeIn(wins), run_time=0.8)
        self.play_at("one hundred", FadeIn(ladder[3], shift=RIGHT * 0.2), run_time=0.5)
        self.play_at("make default", FadeIn(ladder[4], shift=RIGHT * 0.2), FadeIn(ladder[5], shift=RIGHT * 0.2), run_time=0.5)
        self.play_at("make force", FadeIn(ladder[0], shift=RIGHT * 0.2), run_time=0.5)
        self.play_at("ninety", FadeIn(ladder[1], shift=RIGHT * 0.2), FadeIn(ladder[2], shift=RIGHT * 0.2), run_time=0.6)
        box90 = hl(VGroup(ladder[1], ladder[2]), ACCENT, 0.15)
        self.play_at("clicking", Create(box90), run_time=0.6)
        self.play_at("beat", Indicate(ladder[3], color=GREEN), run_time=0.8)
        self.play(Indicate(ladder[0], color=PURPLE_), run_time=0.8)
        self.until("one detail", 0.3)
        self.play(VGroup(ladder, arrow, wins, box90).animate.scale(0.6).to_edge(LEFT, buff=0.4), run_time=0.8)
        demo = VGroup(M("hand-written: [ 22 80 ]", 22, FG), M("clicked (90): [ 22 443 ]", 22, ACCENT),
                      M("result:      [ 22 443 ]", 22, YELLOW_)).arrange(DOWN, aligned_edge=LEFT, buff=0.25)
        demo.to_edge(RIGHT, buff=0.6)
        note = T("at a winning priority, a list replaces the others whole", 22, YELLOW_).next_to(demo, DOWN, buff=0.3)
        self.play_at("list is not joined", LaggedStart(*[FadeIn(d) for d in demo], lag_ratio=0.4), run_time=1.2)
        self.play(FadeIn(note), run_time=0.5)
        self.until("finally", 0.3)
        self.clear(run_time=0.5)
        mk = code_card('{ config, lib, ... }: {\n  options.fleet.modules.media.enable = lib.mkEnableOption "media";\n'
                       '  config = lib.mkIf config.fleet.modules.media.enable {\n    services.jellyfin.enable = true;\n  };\n}',
                       size=24)
        self.play(type_on(mk, 1.6))
        self.play_at("enable option", Create(hl(lines(mk, 1), ACCENT)), run_time=0.6)
        self.play_at("only apply", Create(hl(lines(mk, 2, 4), GREEN)), run_time=0.6)
        self.finish()


class S6_1(Narrated):
    sid = "S6.1"

    def construct(self):
        cfg = box("nixosConfigurations.homeserver", ACCENT, 22, font=MONO)
        ev = box("evaluate all modules", LINE_STRONG, 22)
        top = box("system.build.toplevel", YELLOW_, 22, font=MONO)
        row = VGroup(cfg, ev, top).arrange(RIGHT, buff=0.8).scale_to_fit_width(13).to_edge(UP, buff=0.9)
        arr = chain(cfg, ev, top)
        self.play(FadeIn(cfg), run_time=0.5)
        self.play_at("evaluated", GrowArrow(arr[0]), FadeIn(ev), run_time=0.6)
        self.play_at("top level", GrowArrow(arr[1]), FadeIn(top), run_time=0.6)
        out = pill("nixos-system-homeserver-26.05", "8f3kx…", size=22).move_to(UP * 1.0)
        down = Arrow(top.get_bottom(), out.get_top() + RIGHT * 3, buff=0.1, stroke_color=YELLOW_, stroke_width=3)
        self.play_at("store path", GrowArrow(down), FadeIn(out), run_time=0.8)
        items = [("kernel", "kernel"), ("ramdisk", "initrd"), ("slash", "etc/"), ("program", "sw/bin/"),
                 ("activates", "activate · bin/switch-to-configuration")]
        tree = VGroup(*[M("├─ " + label, 24, FG) for _, label in items]).arrange(DOWN, aligned_edge=LEFT, buff=0.18)
        tree.next_to(out, DOWN, buff=0.35).align_to(out, LEFT).shift(RIGHT * 0.5)
        for (cue, _), t in zip(items, tree):
            self.play_at(cue, FadeIn(t, shift=RIGHT * 0.2), run_time=0.4)
        self.until("if two", 0.3)
        self.clear(run_time=0.5)
        a = box("configuration A", ACCENT, 24).move_to(LEFT * 4 + UP * 1.2)
        b = box("configuration B", ACCENT, 24).move_to(LEFT * 4 + DOWN * 1.2)
        p = pill("nixos-system-pi", "k2v9d…", size=22).move_to(RIGHT * 2)
        ea = Arrow(a.get_right(), p.get_left(), buff=0.1, stroke_color=ACCENT, stroke_width=3)
        eb = Arrow(b.get_right(), p.get_left(), buff=0.1, stroke_color=ACCENT, stroke_width=3)
        self.play(FadeIn(a), FadeIn(b), run_time=0.5)
        self.play_at("same top level", GrowArrow(ea), GrowArrow(eb), FadeIn(p), run_time=0.8)
        eq = T("same toplevel = same system", 26, GREEN).next_to(p, DOWN, buff=0.4)
        self.play_at("same system", FadeIn(eq), run_time=0.5)
        p2 = pill("nixos-system-pi", "x7c1m…", size=22).move_to(p)
        self.play_at("top level changes", Transform(p, p2), Indicate(p, color=YELLOW_), run_time=0.8)
        ch = T("toplevel changed → the machine changes", 26, YELLOW_).move_to(eq)
        self.play(Transform(eq, ch), run_time=0.5)
        self.finish()


class S6_2(Narrated):
    sid = "S6.2"

    def construct(self):
        stc = M("switch-to-configuration", 30, ACCENT).to_edge(UP, buff=0.8)
        self.play_at("switch to configuration", FadeIn(stc), run_time=0.6)
        doors = [("switch", "activate now\n+ boot default", GREEN), ("test", "activate now\nboot entry untouched", YELLOW_),
                 ("boot", "next reboot only", ACCENT)]
        ds = VGroup(*[VGroup(box(n, c, 30, font=MONO, w=3.4, h=1.0), T(d, 22, MUTED)).arrange(DOWN, buff=0.25)
                      for n, d, c in doors]).arrange(RIGHT, buff=0.7).next_to(stc, DOWN, buff=0.7)
        self.play_at("three modes", LaggedStart(*[FadeIn(d[0]) for d in ds], lag_ratio=0.2), run_time=0.8)
        for cue, d in zip(("switch activates", "test activates", "boot does"), ds):
            self.play_at(cue, FadeIn(d[1], shift=UP * 0.1), Indicate(d[0], color=d[0][0].get_stroke_color()), run_time=0.7)
        self.until("activating", 0.3)
        self.play(FadeOut(ds), run_time=0.4)
        units = [("sshd.service", "restart", YELLOW_, "restarted"), ("nginx.service", "reload", CYAN, "reloaded"),
                 ("jellyfin.service", "start", GREEN, "started"), ("old.service", "stop", RED_, "stopped")]
        rows = VGroup(*[VGroup(M(u, 26, FG), M(a, 26, c)).arrange(RIGHT, buff=0.6) for u, a, c, _ in units])
        rows.arrange(DOWN, aligned_edge=LEFT, buff=0.3).move_to(LEFT * 1.8 + DOWN * 0.6)
        for r in rows:
            r[1].align_to(rows[0][0].get_right() + RIGHT * 2.2, LEFT)
        for (_, _, _, cue), r in zip(units, rows):
            self.play_at(cue, FadeIn(r, shift=RIGHT * 0.2), run_time=0.4)
        kern = card(VGroup(M("new kernel", 24, YELLOW_), T("reboot needed", 22, MUTED)).arrange(DOWN, buff=0.1), color=YELLOW_)
        kern.move_to(RIGHT * 4.6 + DOWN * 0.6)
        self.play_at("kernel", FadeIn(kern, shift=LEFT * 0.2), run_time=0.6)
        cap = self.caption("nixos-manager shows this before you deploy")
        self.play_at("shows", FadeIn(cap), run_time=0.5)
        self.finish()


class S7_1(Narrated):
    sid = "S7.1"

    def construct(self):
        gens = VGroup(*[card(VGroup(M(f"generation {n}", 24, FG_STRONG), M(f"…-nixos-system-{h}", 18, FAINT))
                             .arrange(DOWN, aligned_edge=LEFT, buff=0.08), color=LINE_STRONG)
                        for n, h in ((41, "26.05.1"), (42, "26.05.2"), (43, "26.05.3"))]).arrange(DOWN, buff=0.3)
        gens.move_to(LEFT * 3)
        self.play_at("generation", LaggedStart(*[FadeIn(g, shift=UP * 0.2) for g in gens], lag_ratio=0.3), run_time=1.2)
        prof = M("/nix/var/nix/profiles/system", 22, ACCENT).move_to(RIGHT * 3.4 + DOWN * 1.4)
        ptr = Arrow(prof.get_top(), gens[2].get_right(), buff=0.1, stroke_color=ACCENT, stroke_width=3)
        self.play(FadeIn(prof), GrowArrow(ptr), gens[2][0].animate.set_stroke(GREEN, 3), run_time=0.8)
        menu = card(VGroup(T("Boot menu", 22, MUTED), *[M(f"NixOS – Configuration {n}", 20, FG) for n in (43, 42, 41)])
                    .arrange(DOWN, aligned_edge=LEFT, buff=0.12), color=BORDER).move_to(RIGHT * 3.4 + UP * 1.4)
        self.play_at("boot menu", FadeIn(menu), run_time=0.6)
        cmd = self.caption("nixos-rebuild switch --rollback", ACCENT)
        self.play_at("going back", FadeIn(cmd), run_time=0.5)
        ptr2 = Arrow(prof.get_top(), gens[1].get_right(), buff=0.1, stroke_color=YELLOW_, stroke_width=3)
        self.play_at("rollback", Transform(ptr, ptr2), gens[2][0].animate.set_stroke(LINE_STRONG, 2),
                     gens[1][0].animate.set_stroke(YELLOW_, 3), run_time=0.8)
        inst = T("instant: nothing rebuilt", 26, GREEN).next_to(gens, DOWN, buff=0.3)
        self.play_at("nothing", FadeIn(inst), run_time=0.5)
        self.finish()


class S7_2(Narrated):
    sid = "S7.2"

    def construct(self):
        roots = VGroup(*[box(t, GREEN, 20) for t in ("generation 42", "generation 43", "result link")]).arrange(DOWN, buff=0.35)
        roots.move_to(LEFT * 5)
        names = ["nixos-system-43", "nixos-system-42", "firefox-145", "nixos-system-38", "firefox-139", "gcc-13 (old build)"]
        paths = VGroup(*[pill(n, "…", size=20) for n in names]).arrange(DOWN, buff=0.22).move_to(RIGHT * 1.5)
        self.play_at("grows", LaggedStart(*[FadeIn(p) for p in paths], lag_ratio=0.15), run_time=1.2)
        hdr = T("GC roots", 24, GREEN).next_to(roots, UP, buff=0.2)
        live = [(0, 1), (1, 0), (1, 2)]
        edges = VGroup(*[Arrow(roots[a].get_right(), paths[b].get_left(), buff=0.1, stroke_color=GREEN, stroke_width=2.5)
                         for a, b in live])
        self.play_at("root", FadeIn(roots), FadeIn(hdr), LaggedStart(*[GrowArrow(e) for e in edges], lag_ratio=0.2),
                     run_time=1.0)
        cmd = M("nix-collect-garbage", 24, YELLOW_).to_edge(UP, buff=0.6)
        self.play_at("deleted", FadeIn(cmd), *[paths[i].animate.set_opacity(0.12) for i in (3, 4, 5)], run_time=1.0)
        slider = VGroup(Line(LEFT * 2, RIGHT * 2, stroke_color=LINE_STRONG, stroke_width=4),
                        Dot(RIGHT * 0.4, radius=0.12, color=ACCENT)).next_to(paths, DOWN, buff=0.5)
        st = T("keep 14 days", 22, ACCENT).next_to(slider, DOWN, buff=0.1)
        self.play_at("schedule", FadeIn(slider), FadeIn(st), run_time=0.6)
        hv = T("Health: “No automatic garbage collection”", 22, YELLOW_).next_to(st, DOWN, buff=0.2)
        self.play_at("health", FadeIn(hv), run_time=0.5)
        self.until("warning", 0.3)
        self.clear(run_time=0.5)
        sv = card(M('system.stateVersion = "26.05";', 32, FG_STRONG), color=YELLOW_, pad=0.4)
        lock = padlock(s=0.55).next_to(sv, RIGHT, buff=0.4)
        self.play(FadeIn(sv), run_time=0.6)
        notv = T("not the release the machine runs", 26, RED_).next_to(sv, DOWN, buff=0.4)
        self.play_at("not the version", FadeIn(notv), run_time=0.5)
        rec = T("the release it was first installed with: keeps data formats compatible", 24, MUTED).next_to(notv, DOWN, buff=0.2)
        self.play_at("records", FadeIn(rec), run_time=0.5)
        never = T("set once · never change", 30, YELLOW_).next_to(sv, UP, buff=0.4)
        self.play_at("never", FadeIn(lock, scale=0.5), FadeIn(never), run_time=0.6)
        self.finish()


class S8_1(Narrated):
    sid = "S8.1"

    def construct(self):
        sysl = box("system: NixOS", ACCENT, 28, w=9, h=1.0).move_to(DOWN * 1.8)
        user = box("", GREEN, 24, w=9, h=2.4).next_to(sysl, UP, buff=0.25)
        ul = T("user pablo: home-manager", 26, GREEN).next_to(user.get_top(), DOWN, buff=0.2)
        self.play_at("describes", FadeIn(sysl), run_time=0.5)
        self.play_at("same", FadeIn(user), FadeIn(ul), run_time=0.6)
        chips = VGroup(*[card(M(t, 20), color=GREEN, pad=0.15) for t in ("home.packages", "programs.zsh", "programs.git",
                                                                          "~/.config/kitty/kitty.conf")]).arrange(RIGHT, buff=0.25)
        chips.next_to(ul, DOWN, buff=0.35)
        for cue, c in zip(("packages", "shell", "dotfiles"), (chips[0], VGroup(chips[1], chips[2]), chips[3])):
            self.play_at(cue, FadeIn(c, shift=UP * 0.1), run_time=0.5)
        st = pill("hm_kitty.conf", "c71bq…", size=20).to_edge(UP, buff=0.4).shift(RIGHT * 3)
        link = Arrow(chips[3].get_top(), st.get_bottom(), buff=0.1, stroke_color=YELLOW_, stroke_width=3)
        self.play_at("link", FadeIn(st), GrowArrow(link), run_time=0.8)
        self.until("two ways", 0.3)
        self.clear(run_time=0.5)
        a = card(VGroup(T("As a NixOS module", 26, ACCENT), M("home-manager.users.pablo =\n  import ../../home/pablo.nix;", 20))
                 .arrange(DOWN, aligned_edge=LEFT, buff=0.2), color=ACCENT)
        b = card(VGroup(T("Standalone", 26, GREEN), M("home-manager switch --flake .#pablo@laptop", 20),
                        T("macOS, another Linux…", 22, MUTED)).arrange(DOWN, aligned_edge=LEFT, buff=0.2), color=GREEN)
        VGroup(a, b).arrange(DOWN, buff=0.5, aligned_edge=LEFT).move_to(UP * 0.3)
        self.play_at("module", FadeIn(a, shift=UP * 0.2), run_time=0.6)
        self.play_at("standalone", FadeIn(b, shift=UP * 0.2), run_time=0.6)
        both = T("nixos-manager handles both", 28, FG_STRONG).to_edge(DOWN, buff=0.8)
        self.play_at("both", FadeIn(both), run_time=0.5)
        self.finish()


class S9_1(Narrated):
    sid = "S9.1"

    def construct(self):
        code = card(M('password = "hunter2";', 28, FG), color=LINE_STRONG).move_to(LEFT * 3.5 + UP * 1.5)
        sp = pill("etc-wifi.conf", "q1mz8…", size=22).move_to(RIGHT * 2.5 + UP * 1.5)
        arr = Arrow(code.get_right(), sp.get_left(), buff=0.15, stroke_color=LINE_STRONG, stroke_width=3)
        self.play_at("everything", FadeIn(code), GrowArrow(arr), FadeIn(sp), run_time=1.0)
        eye = T("👁 every user can read it", 24, RED_).next_to(sp, DOWN, buff=0.25)
        self.play_at("every user", FadeIn(eye), run_time=0.5)
        x = cross(VGroup(code, sp, arr))
        self.play_at("password", Create(x), run_time=0.6)
        self.until("solves", 0.3)
        self.clear(run_time=0.5)
        f = card(VGroup(M("secrets/hosts/homeserver.json", 22, ACCENT), M('"wifi": "ENC[AES256_GCM,data:…]"', 20, FAINT))
                 .arrange(DOWN, aligned_edge=LEFT, buff=0.15), color=ACCENT).move_to(LEFT * 3.3 + UP * 1.2)
        self.play_at("encrypted", FadeIn(f), run_time=0.6)
        keys = VGroup(VGroup(padlock(s=0.3), T("server's key", 22)).arrange(RIGHT, buff=0.15),
                      VGroup(padlock(s=0.3), T("homeserver's SSH host key", 22)).arrange(RIGHT, buff=0.15))
        keys.arrange(DOWN, aligned_edge=LEFT, buff=0.2).next_to(f, DOWN, buff=0.3).align_to(f, LEFT)
        self.play(FadeIn(keys), run_time=0.6)
        m = machine("homeserver", s=1.0).move_to(RIGHT * 3.4 + UP * 1.2)
        dec = Arrow(f.get_right(), m.get_left(), buff=0.15, stroke_color=GREEN, stroke_width=3)
        self.play_at("decrypt", FadeIn(m), GrowArrow(dec), run_time=0.8)
        run = card(VGroup(M("/run/secrets/wifi", 24, GREEN), M("-r-------- wpa_supplicant", 20, MUTED))
                   .arrange(DOWN, aligned_edge=LEFT, buff=0.1), color=GREEN).next_to(m, DOWN, buff=0.5)
        self.play_at("activation", FadeIn(run, shift=DOWN * 0.2), run_time=0.6)
        self.play_at("readable", Indicate(run[1][1], color=GREEN), run_time=0.8)
        ptr = card(M("networking.wireless.secretsFile = config.sops.secrets.wifi.path;", 20), color=LINE_STRONG)
        ptr.to_edge(DOWN, buff=0.5)
        self.play_at("points", FadeIn(ptr), run_time=0.6)
        self.finish()


def forgejo_and_fleet():
    repo = machine("forgejo", s=1.0).move_to(UP * 1.6)
    rl = M("fleet.git · main", 22, ACCENT).next_to(repo, UP, buff=0.15)
    hosts = Group(*[machine(n, s=0.85) for n in ("homeserver", "pi", "workstation")]).arrange(RIGHT, buff=1.8)
    hosts.move_to(DOWN * 2.0)
    return repo, rl, hosts


class S10_1(Narrated):
    sid = "S10.1"

    def construct(self):
        repo, rl, hosts = forgejo_and_fleet()
        self.play_at("many machines", FadeIn(hosts), run_time=0.8)
        self.play_at("repository", FadeIn(repo), FadeIn(rl), run_time=0.8)
        pulls = VGroup(*[Arrow(h.get_top(), repo.get_bottom(), buff=0.12, stroke_color=CYAN, stroke_width=3)
                         for h in hosts])
        tags = VGroup(*[M("comin", 18, CYAN).next_to(p.get_center(), LEFT, buff=0.1) for p in pulls])
        self.play_at("each machine", LaggedStart(*[GrowArrow(p) for p in pulls], lag_ratio=0.2), FadeIn(tags), run_time=1.0)
        self.play_at("fetches", *[Indicate(p, color=CYAN, scale_factor=1.0) for p in pulls], run_time=1.0)
        commit = Dot(radius=0.14, color=YELLOW_).next_to(rl, RIGHT, buff=0.3)
        ct = M("a3f19c2", 20, YELLOW_).next_to(commit, RIGHT, buff=0.1)
        self.play_at("new commit", FadeIn(commit, scale=0.5), FadeIn(ct), run_time=0.6)
        builds = VGroup(*[M(f"build .#{n}", 18, YELLOW_).next_to(h, DOWN, buff=0.15)
                          for n, h in zip(("homeserver", "pi", "workstation"), hosts)])
        self.play_at("builds", LaggedStart(*[FadeIn(b) for b in builds], lag_ratio=0.2), run_time=0.8)
        self.play_at("switches", *[h[0].animate.set_stroke(GREEN, 4) for h in hosts], run_time=0.6)
        cap = self.caption("A push to main is a deploy")
        self.play_at("push", FadeIn(cap), run_time=0.6)
        self.finish()


class S10_2(Narrated):
    sid = "S10.2"

    def construct(self):
        main = commit_line(5, gap=1.4).move_to(LEFT * 1.5 + DOWN * 0.3)
        ml = M("main", 24, ACCENT).next_to(main, LEFT, buff=0.3)
        self.play(FadeIn(main), FadeIn(ml), run_time=0.8)
        tip = main[1][-1]
        t1 = Dot(radius=0.11, color=YELLOW_).move_to(tip.get_center() + UP * 1.4 + RIGHT * 1.2)
        tb = Line(tip.get_center(), t1.get_center(), stroke_color=YELLOW_, stroke_width=3)
        tl = M("testing-pi", 24, YELLOW_).next_to(t1, RIGHT, buff=0.2)
        self.play_at("second branch", Create(tb), FadeIn(t1), FadeIn(tl), run_time=0.8)
        pi = machine("pi", s=0.8).move_to(RIGHT * 4.8 + UP * 2.0)
        follow = DashedLine(pi.get_left(), t1.get_center(), stroke_color=YELLOW_, buff=0.15)
        self.play_at("on top", FadeIn(pi), Create(follow), run_time=0.8)
        test = T("test mode: no boot entry", 22, YELLOW_).next_to(pi, DOWN, buff=0.15)
        self.play_at("test mode", FadeIn(test), run_time=0.5)
        rb = T("reboot → back to main", 22, GREEN).next_to(test, DOWN, buff=0.1)
        self.play_at("reboot", FadeIn(rb), run_time=0.5)
        self.until("forward", 0.3)
        self.play(FadeOut(VGroup(tb, t1, tl, follow, test, rb)), FadeOut(pi), run_time=0.5)
        back = CurvedArrow(tip.get_center() + DOWN * 0.3, main[1][1].get_center() + DOWN * 0.3, angle=-1.2, color=RED_)
        self.play_at("refuses", Create(back), run_time=0.8)
        no = cross(back).scale(0.5)
        self.play(Create(no), run_time=0.4)
        rev = Dot(radius=0.14, color=GREEN).move_to(tip.get_center() + RIGHT * 1.4)
        rl = Line(tip.get_center(), rev.get_center(), stroke_color=LINE_STRONG, stroke_width=3)
        revl = M("revert", 22, GREEN).next_to(rev, UP, buff=0.2)
        self.play_at("reverts", FadeOut(back), FadeOut(no), Create(rl), FadeIn(rev, scale=0.5), FadeIn(revl), run_time=0.8)
        ideas = VGroup(card(T("Trials", 26, YELLOW_), color=YELLOW_), card(T("Reverts", 26, GREEN), color=GREEN))
        ideas.arrange(RIGHT, buff=0.8).to_edge(DOWN, buff=0.7)
        self.play_at("two ideas", FadeIn(ideas, shift=UP * 0.2), run_time=0.6)
        self.finish()


class S10_3(Narrated):
    sid = "S10.3"

    def construct(self):
        steps = ["store paths", "derivations", "flake + lock", "modules", "toplevel", "switch", "generations"]
        row = VGroup(*[box(s, ACCENT, 18) for s in steps]).arrange(RIGHT, buff=0.45).scale_to_fit_width(13.2).move_to(UP * 1.6)
        arr = chain(*row, buff=0.06)
        cues = ["packages", "derivations", "flake", "modules", "system", "switching", "generations"]
        for cue, b, a in zip(cues, row, [None, *arr]):
            anims = [FadeIn(b, shift=RIGHT * 0.1)] + ([GrowArrow(a)] if a else [])
            self.play_at(cue, *anims, run_time=0.4)
        repo = box("repository", CYAN, 22).move_to(LEFT * 3 + DOWN * 1.2)
        cm = box("comin", CYAN, 22).move_to(DOWN * 1.2)
        ms = box("machines", GREEN, 22).move_to(RIGHT * 3 + DOWN * 1.2)
        a2 = chain(repo, cm, ms)
        self.play_at("deploys", FadeIn(repo), FadeIn(cm), FadeIn(ms), GrowArrow(a2[0]), GrowArrow(a2[1]), run_time=0.8)
        side = VGroup(card(T("sops-nix: secrets", 20, PURPLE_), color=PURPLE_, pad=0.15),
                      card(T("home-manager: users", 20, GREEN), color=GREEN, pad=0.15)).arrange(RIGHT, buff=0.5)
        side.to_edge(DOWN, buff=0.6)
        self.play(FadeIn(side), run_time=0.5)
        self.until("console", 0.3)
        self.play(*[m.animate.set_opacity(0.25) for m in self.mobjects], run_time=0.6)
        nm = T("nixos-manager", 48, FG_STRONG, weight=BOLD)
        self.play(FadeIn(nm, scale=0.8), run_time=0.6)
        self.finish()
