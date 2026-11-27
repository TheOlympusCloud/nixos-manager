"""Shared look and timing for every scene: palette, recurring icons, narration cues, captures."""

import json
import sys
from functools import cache
from pathlib import Path

from manim import *  # noqa: F403

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent
BUILD = ROOT / "build"
ASSETS = ROOT / "assets"
sys.path.insert(0, str(ROOT))
import narrate  # noqa: E402

# The console's Night theme (web/src/app.css).
BG = "#0a0d15"
RAISED = "#10141f"
HOVER = "#171d2c"
ACTIVE = "#1f273b"
BORDER = "#333b54"
LINE = "#3d4763"
LINE_STRONG = "#6b78a0"
FG = "#d3cfc3"
FG_STRONG = "#f6f2e6"
MUTED = "#a6abbb"
FAINT = "#8f95ab"
ACCENT = "#7ebae4"
ACCENT_FILL = "#5277c3"
GREEN = "#8dd28a"
RED_ = "#ff7468"
YELLOW_ = "#f0b950"
PURPLE_ = "#b9a0f5"
CYAN = "#6fd2c6"

SANS = "Noto Sans"
MONO = "DejaVu Sans Mono"

config.background_color = BG
config.frame_rate = 30
config.pixel_width, config.pixel_height = 1920, 1080


def T(text, size=28, color=FG, font=SANS, weight=NORMAL, **kw):
    return Text(text, font=font, font_size=size, color=color, weight=weight, **kw)


def M(text, size=24, color=FG, **kw):
    return Text(text, font=MONO, font_size=size, color=color, **kw)


def MK(markup, size=24, font=MONO, color=FG):
    return MarkupText(markup, font=font, font_size=size, color=color)


def span(text, color):
    return f'<span foreground="{color}">{text}</span>'


def card(mob, pad=0.3, color=BORDER, fill=RAISED, radius=0.15, width=2):
    r = RoundedRectangle(corner_radius=radius, width=mob.width + 2 * pad, height=mob.height + 2 * pad,
                         stroke_color=color, stroke_width=width, fill_color=fill, fill_opacity=1)
    r.move_to(mob)
    return VGroup(r, mob)


def box(text, color=ACCENT, size=26, w=None, h=None, fill=RAISED, font=SANS):
    label = T(text, size, FG_STRONG, font=font) if isinstance(text, str) else text
    r = RoundedRectangle(corner_radius=0.15, width=w or label.width + 0.6, height=h or label.height + 0.5,
                         stroke_color=color, stroke_width=2.5, fill_color=fill, fill_opacity=1)
    return VGroup(r, label.move_to(r))


def pill(name, hash_="7x2kq…", prefix="/nix/store/", size=24):
    """A store path: /nix/store/<hash>-name, the hash dimmer."""
    t = MK(span(prefix, MUTED) + span(hash_, FAINT) + span("-" + name, FG_STRONG), size)
    g = card(t, pad=0.18, color=LINE_STRONG, radius=0.25)
    g.hash = t[len(prefix):len(prefix) + len(hash_)]
    g.name = t[len(prefix) + len(hash_):]
    return g


def padlock(color=YELLOW_, s=0.35):
    body = RoundedRectangle(corner_radius=0.05 * s / 0.35, width=s, height=s * 0.8, stroke_width=0,
                            fill_color=color, fill_opacity=1)
    shackle = Arc(radius=s * 0.32, start_angle=0, angle=PI, stroke_color=color, stroke_width=4 * s / 0.35)
    shackle.next_to(body, UP, buff=-0.02)
    return VGroup(shackle, body)


KIND = {"homeserver": "homeserver", "pi": "pi", "workstation": "workstation", "laptop": "laptop",
        "pablo@laptop": "laptop", "forgejo": "gitserver", "git": "gitserver"}


def _glyph(kind, s):
    """Vector stand-in for a machine picture."""
    if kind == "pi":
        b = RoundedRectangle(corner_radius=0.05, width=s * 1.1, height=s * 0.7, stroke_color=GREEN, fill_color="#16301f",
                             fill_opacity=1, stroke_width=2)
        chip = Square(s * 0.25, fill_color=FAINT, fill_opacity=1, stroke_width=0).move_to(b)
        pins = VGroup(*[Dot(radius=0.018, color=YELLOW_) for _ in range(10)]).arrange(RIGHT, buff=0.05)
        return VGroup(b, chip, pins.next_to(b.get_top(), DOWN, buff=0.07))
    if kind in ("workstation", "laptop"):
        scr = RoundedRectangle(corner_radius=0.05, width=s * 1.1, height=s * 0.7, stroke_color=ACCENT, fill_color=HOVER,
                               fill_opacity=1, stroke_width=2)
        base = (Rectangle(width=s * 1.3, height=0.07, fill_color=ACCENT, fill_opacity=1, stroke_width=0)
                .next_to(scr, DOWN, buff=0.03) if kind == "laptop"
                else Rectangle(width=s * 0.4, height=0.07, fill_color=ACCENT, fill_opacity=1, stroke_width=0)
                .next_to(scr, DOWN, buff=0.12))
        return VGroup(scr, base)
    tower = RoundedRectangle(corner_radius=0.05, width=s * 0.7, height=s, stroke_color=ACCENT, fill_color=HOVER,
                             fill_opacity=1, stroke_width=2)
    slots = VGroup(*[Line(LEFT * s * 0.22, RIGHT * s * 0.22, stroke_color=LINE_STRONG, stroke_width=2) for _ in range(3)])
    slots.arrange(DOWN, buff=0.08).move_to(tower).shift(UP * s * 0.18)
    led = Dot(radius=0.03, color=GREEN if kind != "gitserver" else YELLOW_).move_to(tower).shift(DOWN * s * 0.3)
    return VGroup(tower, slots, led)


def machine(name, kind=None, s=1.0, color=ACCENT, label=True):
    """A machine: its picture (assets/<kind>.png when present) and its name."""
    kind = kind or KIND.get(name, "homeserver")
    png = ASSETS / f"{kind}.png"
    pic = ImageMobject(str(png)).scale_to_fit_height(s * 1.2) if png.exists() else _glyph(kind, s)
    text = M(name, 22, FG_STRONG)
    frame = RoundedRectangle(corner_radius=0.18, width=max(s * 1.9, text.width + 0.4),
                             height=s * 1.75 + (0.35 if label else 0),
                             stroke_color=color, stroke_width=2.5, fill_color=RAISED, fill_opacity=1)
    pic.move_to(frame).shift(UP * (0.17 if label else 0))
    g = Group(frame, pic)
    if label:
        g.add(text.next_to(frame.get_bottom(), UP, buff=0.12))
    return g


def commit_line(n=5, gap=1.0, color=ACCENT):
    dots = VGroup(*[Dot(radius=0.11, color=color) for _ in range(n)]).arrange(RIGHT, buff=gap)
    line = Line(dots[0].get_center(), dots[-1].get_center(), stroke_color=LINE_STRONG, stroke_width=3)
    return VGroup(line, dots)


def cross(mob, color=RED_):
    return VGroup(Line(mob.get_corner(UL), mob.get_corner(DR)), Line(mob.get_corner(DL), mob.get_corner(UR))).set_stroke(
        color, 6)


def code_card(src, lang="nix", size=22, lines=True):
    c = Code(code_string=src.strip("\n"), language=lang, formatter_style="nord", add_line_numbers=False,
             background="window", paragraph_config={"font": MONO, "font_size": size},
             background_config={"fill_color": RAISED, "stroke_color": BORDER, "stroke_width": 2})
    return c


def bullet_list(items, size=26, color=FG, buff=0.22, mark="•"):
    rows = VGroup(*[VGroup(T(mark, size, ACCENT), T(i, size, color)).arrange(RIGHT, buff=0.2) for i in items])
    return rows.arrange(DOWN, aligned_edge=LEFT, buff=buff)


def table(rows, size=24, col_buff=0.6, row_buff=0.22, colors=None, font=MONO, header=True):
    cells = [[(M if font == MONO else T)(str(c), size, (colors or {}).get(j, FG)) for j, c in enumerate(r)] for r in rows]
    if header:
        for c in cells[0]:
            c.set_color(ACCENT)
    widths = [max(r[j].width for r in cells) for j in range(len(rows[0]))]
    out = VGroup()
    for r in cells:
        row = VGroup()
        x = 0
        for j, c in enumerate(r):
            c.move_to([x + c.width / 2, 0, 0])
            x += widths[j] + col_buff
            row.add(c)
        out.add(row)
    out.arrange(DOWN, aligned_edge=LEFT, buff=row_buff)
    for i, r in enumerate(out):  # re-align columns after arrange
        for j, c in enumerate(r):
            c.align_to(out[0][j], LEFT)
    return out


@cache
def scenes():
    return {s["id"]: s for s in narrate.parse(narrate.SCRIPT.read_text())[0]}


class Narrated(MovingCameraScene):
    """A scene timed by its voice-over. Set `sid` to the script's scene id (e.g. "S1.1")."""

    sid = None
    opening = False  # the video's title card before the narration

    def setup(self):
        info = scenes()[self.sid]
        meta = json.loads((BUILD / "audio" / f"{self.sid}.json").read_text())
        self.words, self.audio_len, self._cursor = meta["words"], meta["duration"], 0
        self.marks = []
        order = list(scenes())
        first = order.index(self.sid) == 0 or scenes()[order[order.index(self.sid) - 1]]["chapter"] != info["chapter"]
        self.t0 = 2.6 if first and info["chapter"] else 3.4 if self.opening else 0.0
        self.add_sound(str(BUILD / "audio" / f"{self.sid}.wav"), time_offset=self.t0)
        if self.opening:
            self.opening_card()
        elif self.t0:
            self.chapter_card(info)

    def opening_card(self):
        mark = M("❄", 64, ACCENT)
        title = T("Nix, NixOS and nixos-manager", 60, FG_STRONG, weight=BOLD)
        sub = T("How Nix works, and how to run a fleet with it", 30, MUTED)
        g = VGroup(mark, title, sub).arrange(DOWN, buff=0.35)
        self.play(FadeIn(mark, scale=0.6), run_time=0.6)
        self.play(Write(title), FadeIn(sub, shift=UP * 0.2), run_time=1.2)
        self.wait(1.0)
        self.play(FadeOut(g), run_time=0.5)
        self.wait(max(0, self.t0 - self.time))

    def chapter_card(self, info):
        n = info["chapter"]
        kicker = {1: "Part 1 · How Nix and NixOS work", 11: "Part 2 · nixos-manager"}.get(n, "")
        num = M(f"{n:02d}", 30, ACCENT)
        title = T(info["chapter_title"], 56, FG_STRONG, weight=BOLD)
        rule = Line(LEFT * 3, RIGHT * 3, stroke_color=ACCENT_FILL, stroke_width=3)
        g = VGroup(num, title, rule).arrange(DOWN, buff=0.3)
        if kicker:
            g = VGroup(T(kicker, 26, MUTED), g).arrange(DOWN, buff=0.6)
        self.play(FadeIn(g, shift=UP * 0.2), run_time=0.8)
        self.wait(1.2)
        self.play(FadeOut(g), run_time=0.5)
        self.wait(max(0, self.t0 - self.time))

    # --- timing -----------------------------------------------------------------------------
    def at(self, cue):
        """Start time of the next spoken occurrence of `cue` (one or more words)."""
        want = [narrate.norm(w) for w in cue.split()]
        ws = self.words
        for i in range(self._cursor, len(ws) - len(want) + 1):
            if all(ws[i + k]["w"] == want[k] for k in range(len(want))):
                self._cursor = i + 1
                return self.t0 + ws[i]["s"]
        raise ValueError(f"{self.sid}: cue {cue!r} not found after word {self._cursor}")

    def until(self, cue, lead=0.0):
        t = self.at(cue) - lead
        if t > self.time + 1 / 30:
            self.wait(t - self.time)

    def play_at(self, cue, *anims, lead=0.15, **kw):
        self.until(cue, lead)
        self.play(*anims, **kw)

    def finish(self, pad=1.0):
        end = self.t0 + self.audio_len + pad
        if end > self.time:
            self.wait(end - self.time)

    # --- overlays ---------------------------------------------------------------------------
    def caption(self, text, color=FG_STRONG, size=30):
        t = T(text, size, color)
        c = card(t, pad=0.25, color=ACCENT_FILL, fill=BG)
        c.to_edge(DOWN, buff=0.4)
        return c

    def clear(self, *keep, run_time=0.6):
        gone = [m for m in self.mobjects if m not in keep]
        if gone:
            self.play(*[FadeOut(m) for m in gone], run_time=run_time)


# --- console captures --------------------------------------------------------------------
@cache
def _shot_meta(name):
    p = BUILD / "shots" / f"{name}.json"
    return json.loads(p.read_text()) if p.exists() else {}


def shot(name, width=None):
    """A console screenshot filling the frame (or `width` units wide)."""
    img = ImageMobject(str(BUILD / "shots" / f"{name}.png"))
    img.scale_to_fit_width(width or config.frame_width)
    img.shot_name = name
    return img


def region(img, key, pad=0.08):
    """The rectangle, in scene units, of a named element the capture recorded (or an [x, y, w, h] in px)."""
    meta = _shot_meta(img.shot_name)
    pw, ph = meta.get("size", [2880, 1620])
    if isinstance(key, str) and key not in meta.get("boxes", {}):
        print(f"WARNING region: {img.shot_name} has no box {key!r}", file=sys.stderr)
        key = (0.11, 0.06, 0.88, 0.9)  # the view, without the rail
    x, y, w, h = meta["boxes"][key] if isinstance(key, str) else key
    if max(x, y, w, h) <= 1:  # fractions of the screenshot
        x, y, w, h = x * pw, y * ph, w * pw, h * ph
    k = img.width / pw
    left, top = img.get_left()[0], img.get_top()[1]
    pad = min(pad, 0.25 * min(w, h) * k)  # a small element gets a snug outline
    r = Rectangle(width=w * k + 2 * pad, height=h * k + 2 * pad, stroke_color=YELLOW_, stroke_width=4)
    r.move_to([left + (x + w / 2) * k, top - (y + h / 2) * k, 0])
    return r


def has_region(img, key):
    return key in _shot_meta(img.shot_name).get("boxes", {})


class Capture(Narrated):
    """A narrated walk over console screenshots: zoom, highlight, spotlight."""

    def focus(self, target, margin=1.25, run_time=1.2):
        f = self.camera.frame
        w = max(target.width * margin, target.height * margin * config.frame_width / config.frame_height)
        w = min(w, config.frame_width)
        self.play(f.animate.set_width(w).move_to(target), run_time=run_time)

    def unfocus(self, run_time=1.0):
        self.play(self.camera.frame.animate.set_width(config.frame_width).move_to(ORIGIN), run_time=run_time)

    def mark(self, rect, cue=None, run_time=0.6):
        if cue:
            self.until(cue, 0.15)
        self.play(Create(rect), run_time=run_time)
        return rect

    def show(self, name, cue=None, run_time=0.8):
        """Cross-fade to another screenshot, resetting the camera."""
        if cue:
            self.until(cue, 0.3)
        img = shot(name)
        self._lower_cap = None  # faded out with everything else below
        old = [m for m in self.mobjects]
        anims = [FadeIn(img)] + [FadeOut(m) for m in old if m is not self.camera.frame]
        if self.camera.frame.width != config.frame_width:
            anims.append(self.camera.frame.animate.set_width(config.frame_width).move_to(ORIGIN))
        self.play(*anims, run_time=run_time)
        return img


def type_on(code, run_time=2.0):
    """Animations that draw a code card, then type its lines on."""
    return Succession(FadeIn(code.background, run_time=0.4) if hasattr(code, "background") else Wait(0.01),
                      LaggedStart(*[FadeIn(line, shift=RIGHT * 0.1) for line in code.code_lines],
                                  lag_ratio=0.6, run_time=run_time))


def lines(code, a, b=None):
    """Lines a..b (0-based, inclusive) of a code card."""
    return VGroup(*code.code_lines[a:(b if b is not None else a) + 1])


def hl(mob, color=YELLOW_, buff=0.08):
    return SurroundingRectangle(mob, color=color, buff=buff, corner_radius=0.06, stroke_width=3)


def chain(*mobs, buff=0.15, color=LINE_STRONG):
    return VGroup(*[Arrow(a.get_right(), b.get_left(), buff=buff, stroke_color=color, stroke_width=3,
                          max_tip_length_to_length_ratio=0.15) for a, b in zip(mobs, mobs[1:])])


def pin(scene, mob):
    """Keep an overlay at the same place on screen while the camera zooms and pans."""
    frame = scene.camera.frame
    w0, off = mob.width, mob.get_center() - frame.get_center()
    fw0 = frame.width

    def follow(m):
        k = frame.width / fw0
        m.set_width(w0 * k)
        m.move_to(frame.get_center() + off * k)

    mob.add_updater(follow)
    return mob


def _look(self, img, where, cue=None, mark=None, margin=1.35, run_time=1.2, color=YELLOW_):
    """Zoom to a part of a screenshot (named box, [x, y, w, h] px, or fractions) and outline it.
    Only a box the capture recorded gets an outline: an estimated one could frame the wrong thing."""
    if mark is None:
        mark = isinstance(where, str)
    r = region(img, where).set_stroke(color)
    if cue:
        self.until(cue, 0.4)
    self._drop_lower()
    self.focus(r, margin=margin, run_time=run_time)
    if mark:
        self.play(Create(r), run_time=0.5)
        self.marks = getattr(self, "marks", []) + [r]
    return r


def _back(self, cue=None, run_time=1.0):
    if cue:
        self.until(cue, 0.3)
    anims = [FadeOut(m) for m in getattr(self, "marks", [])]
    self.marks = []
    self._drop_lower()
    self.play(self.camera.frame.animate.set_width(config.frame_width).move_to(ORIGIN), *anims, run_time=run_time)


def _mark(self, img, where, cue=None, color=YELLOW_):
    r = region(img, where).set_stroke(color)
    if cue:
        self.until(cue, 0.2)
    if not isinstance(where, str):  # estimated, not recorded: no outline (see _look)
        return r
    self.play(Create(r), run_time=0.5)
    self.marks = getattr(self, "marks", []) + [r]
    return r


def _unmark(self):
    if getattr(self, "marks", []):
        self.play(*[FadeOut(m) for m in self.marks], run_time=0.4)
    self.marks = []


def _lower(self, text, cue=None, color=FG_STRONG, size=28):
    """A lower-third that stays put on screen whatever the camera does."""
    if cue:
        self.until(cue, 0.2)
    c = self.caption(text, color, size)
    frame = self.camera.frame
    k = frame.width / config.frame_width
    c.scale(k).move_to(frame.get_center() + (c.get_center()) * k)
    old = getattr(self, "_lower_cap", None)
    self.play(FadeIn(c), *([FadeOut(old)] if old else []), run_time=0.5)
    self._lower_cap = c
    return c


def _drop_lower(self):
    """Lower-thirds are placed for the current view: they leave before the camera moves."""
    if getattr(self, "_lower_cap", None):
        self.play(FadeOut(self._lower_cap), run_time=0.3)
        self._lower_cap = None


Capture._drop_lower = _drop_lower
Capture.look = _look
Capture.back = _back
Capture.mark = _mark
Capture.unmark = _unmark
Capture.lower = _lower


def _display_image_mobject(self, image_mobject, pixel_array):
    """Manim's image drawing, transforming only the part of the image the camera sees: zoomed-in
    screenshots otherwise get resampled at their full, many times larger, zoomed size every frame."""
    from PIL import Image

    src = image_mobject.get_pixel_array()
    h, w = src.shape[:2]
    original = np.array([[0, 0], [w, 0], [0, h], [w, h]], dtype=np.float64)
    target = self.points_to_subpixel_coords(image_mobject, image_mobject.points).astype(np.float64)
    x0, y0 = np.floor(target.min(axis=0)).astype(int)
    x1, y1 = np.ceil(target.max(axis=0)).astype(int)
    x0, y0, x1, y1 = max(x0, 0), max(y0, 0), min(x1, self.pixel_width), min(y1, self.pixel_height)
    if x1 - x0 < 1 or y1 - y0 < 1:
        return
    (tx0, ty0), (tx1, _), (_, ty2), _ = target
    if abs(target[1][1] - ty0) < 1e-6 and abs(target[2][0] - tx0) < 1e-6 and tx1 > tx0 and ty2 > ty0:
        # Not rotated (every screenshot): crop the part in view and resize it, much faster than a transform.
        kx, ky = w / (tx1 - tx0), h / (ty2 - ty0)
        x0, y0 = max(x0, int(np.ceil(tx0))), max(y0, int(np.ceil(ty0)))
        x1, y1 = min(x1, int(np.floor(tx1))), min(y1, int(np.floor(ty2)))
        if x1 - x0 < 1 or y1 - y0 < 1:
            return
        box = (max(0.0, (x0 - tx0) * kx), max(0.0, (y0 - ty0) * ky), min(w, (x1 - tx0) * kx), min(h, (y1 - ty0) * ky))
        alpha = src[:, :, 3]
        if alpha.min() == alpha.max():  # opaque or evenly faded: blend the RGB straight into the frame
            cached = getattr(image_mobject, "_rgb", None)
            if cached is None or cached[0] is not src:
                cached = (src, Image.fromarray(np.ascontiguousarray(src[:, :, :3]), mode="RGB"))
                image_mobject._rgb = cached
            rgb = np.asarray(cached[1].resize((x1 - x0, y1 - y0), Image.Resampling.BILINEAR, box=box))
            a = int(alpha[0, 0]) / 255
            region_ = pixel_array[y0:y1, x0:x1]
            region_[:, :, :3] = rgb if a == 1 else (rgb * a + region_[:, :, :3] * (1 - a)).astype(np.uint8)
            region_[:, :, 3] = 255
            return
        sub = Image.fromarray(src, mode="RGBA").resize((x1 - x0, y1 - y0), Image.Resampling.BILINEAR, box=box)
        full = Image.new("RGBA", (self.pixel_width, self.pixel_height), (0, 0, 0, 0))
        full.paste(sub, (x0, y0))
        self.overlay_PIL_image(pixel_array, full)
        return
    a, b = [], []
    for (x, y), (X, Y) in zip(target - [x0, y0], original):
        a += [[x, y, 1, 0, 0, 0, -X * x, -X * y], [0, 0, 0, x, y, 1, -Y * x, -Y * y]]
        b += [X, Y]
    try:
        coeffs = np.linalg.solve(np.array(a), np.array(b))
    except np.linalg.LinAlgError:
        return
    sub = Image.fromarray(src, mode="RGBA").transform((x1 - x0, y1 - y0), Image.Transform.PERSPECTIVE, coeffs,
                                                       resample=image_mobject.resampling_algorithm)
    full = Image.new("RGBA", (self.pixel_width, self.pixel_height), (0, 0, 0, 0))
    full.paste(sub, (x0, y0))
    self.overlay_PIL_image(pixel_array, full)


Camera.display_image_mobject = _display_image_mobject
