"""Render scenes on the Cellwork stage (film/): step the console's 80 ms clock in Chromium, keep one image per
tick that changed, and encode them with the scene's voice.

    uv run python film.py [--jobs 6] [S1.1 S1.2 | 1 2 ...]

Writes build/scenes/<scene>.mkv. A scene whose layout breaks the grid (text off the grid, two elements on the
same cells) is still written, but listed, and the run exits non-zero. --check runs only those checks.
"""

import argparse
import json
import math
import shutil
import subprocess
import sys
from concurrent.futures import ProcessPoolExecutor
from pathlib import Path

import narrate

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
BUILD = HERE / "build"
OUT = BUILD / "scenes"
TICK = 0.08  # seconds: lib/ascii.ts TICK
CARD = 2.4  # seconds of chapter card before a chapter's first scene speaks
OPENING = 4.0  # seconds of title before the first words of the video
PAD = 1.0  # seconds held after a scene's narration
VIEW = {"width": 1920, "height": 1080}


def bundle():
    """film/scenes.ts and everything it imports (lib/ascii.ts, the intro's scenes) as one script for film.html."""
    dest = BUILD / "film" / "film.js"
    dest.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run([str(REPO / "web/node_modules/.bin/rolldown"), str(HERE / "film/scenes.ts"), "--file", str(dest),
                    "--format", "iife", "--platform", "browser"], check=True, capture_output=True)


def intro_for(scene, scenes):
    """What plays before a scene's voice: the title of the video, a chapter card, or nothing."""
    i = scenes.index(scene)
    if i == 0:
        return None
    if i == 1:
        return {"kind": "opening"}
    if scenes[i - 1]["chapter"] != scene["chapter"]:
        return {"kind": "chapter", "n": scene["chapter"], "title": scene["chapter_title"],
                "part": {1: "Part 1 · How Nix and NixOS work", 11: "Part 2 · nixos-manager"}.get(scene["chapter"], "")}
    return None


def render(sid, check=False):
    from playwright.sync_api import sync_playwright

    scenes = narrate.parse(narrate.SCRIPT.read_text())[0]
    scene = next(s for s in scenes if s["id"] == sid)
    audio = json.loads((BUILD / "audio" / f"{sid}.json").read_text())
    intro = intro_for(scene, scenes)
    t0 = {None: 0.0, "opening": OPENING, "chapter": CARD}[intro and intro["kind"]]
    ticks = math.ceil((t0 + audio["duration"] + PAD) / TICK)
    frames = BUILD / "frames" / sid
    if not check:
        shutil.rmtree(frames, ignore_errors=True)
        frames.mkdir(parents=True)
    shots = {p.stem: json.loads(p.read_text()) for p in (BUILD / "shots").glob("*.json")}
    with sync_playwright() as p:
        browser = p.chromium.launch(executable_path=shutil.which("chromium") or None)
        page = browser.new_page(viewport=VIEW)
        page.add_init_script(f"window.SHOTS = {json.dumps(shots)}")
        page.goto((HERE / "film/film.html").as_uri() + "?manual")
        page.evaluate("document.fonts.ready.then(() => 0)")
        info = {"n": scene["chapter"], "chapter": scene["chapter_title"], "title": scene["title"]}
        page.evaluate("([sid, words, t0, intro, info]) => film.start(sid, words, t0, intro, info)",
                      [sid, audio["words"], t0, intro, info])
        runs, last = [], None
        if check:  # the layout checks only: no images, no video
            page.evaluate(f"(async () => {{ for (let i = 0; i < {ticks}; i++) await film.step() }})()")
            problems = page.evaluate("film.problems()")
            browser.close()
            return sid, problems
        for i in range(ticks):
            h = page.evaluate("film.step()")
            if h != last:
                page.screenshot(path=str(frames / f"{i:06d}.png"))
                runs.append([f"{i:06d}.png", 0])
                last = h
            runs[-1][1] += 1
        problems = page.evaluate("film.problems()")
        steps = page.evaluate("film.steps()") + [ticks - 1]
        browser.close()
    # One frame per step, once its animations have settled (about 9 ticks), for the layout review.
    review = BUILD / "steps" / sid
    shutil.rmtree(review, ignore_errors=True)
    review.mkdir(parents=True)
    starts = [int(f[:6]) for f, _ in runs]
    for k, t in enumerate(steps):
        at = min(t + 9, ticks - 1)
        f = runs[max(i for i, st in enumerate(starts) if st <= at)][0]
        shutil.copy(frames / f, review / f"{k:02d}.png")
    lst = frames / "list.txt"
    lst.write_text("".join(f"file '{f}'\nduration {n * TICK:.3f}\n" for f, n in runs) + f"file '{runs[-1][0]}'\n")
    OUT.mkdir(parents=True, exist_ok=True)
    dest = OUT / f"{sid.replace('.', '_')}.mkv"
    subprocess.run(["ffmpeg", "-v", "error", "-y", "-f", "concat", "-safe", "0", "-i", str(lst),
                    "-i", str(BUILD / "audio" / f"{sid}.wav"),
                    # concat drops the last image's duration: hold the last frame, then cut to the scene's length
                    "-filter_complex", (f"[0:v]fps=25,tpad=stop_mode=clone:stop_duration=10[v];"
                                        f"[1:a]adelay={int(t0 * 1000)}:all=1,apad[a]"), "-map", "[v]", "-map", "[a]",
                    "-t", f"{ticks * TICK:.3f}", "-r", "25", "-c:v", "libx264", "-preset", "veryfast", "-crf", "16",
                    "-pix_fmt", "yuv420p", "-c:a", "pcm_s16le", "-ar", "48000", str(dest)], check=True)
    shutil.rmtree(frames)
    return sid, problems


def pick(args, scenes):
    return [s["id"] for s in scenes if not args or s["id"] in args or str(s["chapter"]) in args]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("ids", nargs="*")
    ap.add_argument("--jobs", type=int, default=6)
    ap.add_argument("--check", action="store_true", help="only run the layout checks, much faster")
    a = ap.parse_args()
    bundle()
    ids = pick(a.ids, narrate.parse(narrate.SCRIPT.read_text())[0])
    bad = 0
    with ProcessPoolExecutor(a.jobs) as pool:
        for sid, problems in pool.map(render, ids, [a.check] * len(ids)):
            print(f"{sid} {'ok' if not problems else 'LAYOUT'}", flush=True)
            for p in problems:
                print(f"  {p}", flush=True)
            bad += bool(problems)
    if bad:
        sys.exit(f"{bad} scene(s) break the grid")


if __name__ == "__main__":
    main()
