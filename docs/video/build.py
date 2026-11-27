"""Render scenes and cut the video.

    uv run python build.py render [--jobs 6] [S1.1 S1.2 | 1 2 ...]   render scenes (ids or chapter numbers)
    uv run python build.py cut [1 2 ...]                              one MP4 per chapter + the full cut
                                                                      (with chapter numbers: only those chapters)

Scenes are classes named after their id (S1.1 -> S1_1) in scenes/*.py. Outputs go to build/.
"""

import argparse
import json
import re
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

import narrate

HERE = Path(__file__).resolve().parent
BUILD = HERE / "build"
MEDIA = BUILD / "media"
OUT = BUILD / "out"
TITLE = "Nix, NixOS and nixos-manager"
LOUD = "loudnorm=I=-16:TP=-1.5:LRA=11"
PAD = 1.0  # seconds after each scene's narration


def scene_files():
    found = {}
    for f in sorted((HERE / "scenes").glob("*.py")):
        for name in re.findall(r"^class (S\d+_\d+)\(", f.read_text(), re.M):
            found[name.replace("_", ".")] = f
    return found


def video(sid, files):
    return MEDIA / "videos" / files[sid].stem / "1080p30" / f"{sid.replace('.', '_')}.mp4"


def pick(args, scenes):
    if not args:
        return [s["id"] for s in scenes]
    return [s["id"] for s in scenes if s["id"] in args or str(s["chapter"]) in args]


def render(ids, jobs):
    files = scene_files()
    missing = [i for i in ids if i not in files]
    if missing:
        sys.exit(f"no scene class for {', '.join(missing)}")

    def one(sid):
        log = BUILD / "logs" / f"{sid}.log"
        log.parent.mkdir(parents=True, exist_ok=True)
        cmd = ["manim", "render", "--disable_caching", "--media_dir", str(MEDIA), "-v", "WARNING",
               files[sid].name, sid.replace(".", "_")]
        with log.open("w") as fh:
            ok = subprocess.run(cmd, cwd=files[sid].parent, stdout=fh, stderr=subprocess.STDOUT).returncode == 0
        print(f"{sid} {'ok' if ok else 'FAILED, see ' + str(log)}", flush=True)
        return ok

    with ThreadPoolExecutor(jobs) as pool:
        results = list(pool.map(one, ids))
    if not all(results):
        sys.exit(1)


def duration(path, stream=None):
    what = ["-select_streams", stream, "-show_entries", "stream=duration"] if stream else ["-show_entries", "format=duration"]
    out = subprocess.run(["ffprobe", "-v", "error", *what, "-of", "csv=p=0", str(path)],
                         capture_output=True, text=True, check=True).stdout
    return float(out.split()[0])


def fit(src):
    """The scene with its last frame held, and silence after its voice, to the narration plus PAD seconds:
    every scene lasts exactly as long as its audio, so chapters stay in sync."""
    dest = BUILD / "fit" / src.with_suffix(".mkv").name  # PCM audio: AAC priming would add up across scenes
    if dest.exists() and dest.stat().st_mtime > src.stat().st_mtime:
        return dest
    dest.parent.mkdir(parents=True, exist_ok=True)
    target = round(duration(src, "a:0") + PAD, 3)
    hold = max(0.0, target - duration(src, "v:0"))
    subprocess.run(["ffmpeg", "-v", "error", "-y", "-i", str(src), "-vf", f"tpad=stop_mode=clone:stop_duration={hold}",
                    "-af", f"apad=whole_dur={target}", "-t", str(target), "-c:v", "libx264", "-preset", "veryfast",
                    "-crf", "16", "-pix_fmt", "yuv420p", "-r", "30", "-c:a", "pcm_s16le", "-ar", "48000",
                    str(dest)], check=True)
    return dest


def concat(parts, dest, reencode, metadata=None):
    lst = dest.with_suffix(".txt")
    lst.write_text("".join(f"file '{p}'\n" for p in parts))
    cmd = ["ffmpeg", "-v", "error", "-y", "-f", "concat", "-safe", "0", "-i", str(lst)]
    if metadata:
        cmd += ["-i", str(metadata), "-map_metadata", "1", "-map_chapters", "1"]
    if reencode:  # two-pass loudnorm: speech at -16 LUFS
        probe = subprocess.run(["ffmpeg", "-v", "info"] + cmd[3:] + ["-af", LOUD + ":print_format=json", "-vn", "-f", "null", "-"],
                               capture_output=True, text=True, check=True).stderr
        m = json.loads(re.findall(r"\{[^{}]*\}", probe)[-1])
        af = (f"{LOUD}:linear=true:measured_I={m['input_i']}:measured_TP={m['input_tp']}:"
              f"measured_LRA={m['input_lra']}:measured_thresh={m['input_thresh']}:offset={m['target_offset']}")
        cmd += ["-af", af, "-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p",
                "-c:a", "aac", "-b:a", "160k", "-ar", "48000"]
    else:
        cmd += ["-c", "copy"]
    cmd += ["-movflags", "+faststart", str(dest)]
    subprocess.run(cmd, check=True)
    lst.unlink()


def slug(text):
    return re.sub(r"[^a-z0-9]+", "-", text.lower()).strip("-")


def cut(only):
    scenes = narrate.parse(narrate.SCRIPT.read_text())[0]
    files = scene_files()
    OUT.mkdir(parents=True, exist_ok=True)
    chapters = {}
    for s in scenes:
        chapters.setdefault(s["chapter"], (s["chapter_title"], []))[1].append(s["id"])
    made, meta, marks, t = [], [";FFMETADATA1", f"title={TITLE}"], [], 0.0
    for n, (title, ids) in chapters.items():
        if only and str(n) not in only:
            continue
        parts = [video(i, files) for i in ids]
        absent = [str(p) for p in parts if not p.exists()]
        if absent:
            sys.exit(f"chapter {n}: not rendered: {', '.join(absent)}")
        parts = [fit(p) for p in parts]
        dest = OUT / f"{n:02d}-{slug(title)}.mp4"
        if not dest.exists() or dest.stat().st_mtime < max(p.stat().st_mtime for p in parts):
            concat(parts, dest, reencode=True)
        d = duration(dest)
        meta += ["[CHAPTER]", "TIMEBASE=1/1000", f"START={int(t * 1000)}", f"END={int((t + d) * 1000)}",
                 f"title={n} {title}"]
        marks.append(f"{int(t) // 3600}:{int(t) % 3600 // 60:02d}:{int(t) % 60:02d} {n} {title}"
                     if t >= 3600 else f"{int(t) // 60}:{int(t) % 60:02d} {n} {title}")
        made.append(dest)
        t += d
        print(f"{dest.name} {d / 60:.1f} min", flush=True)
    if only:
        return
    (OUT / "chapters.ffmeta").write_text("\n".join(meta) + "\n")
    (OUT / "chapters.txt").write_text("\n".join(marks) + "\n")
    full = OUT / f"{slug(TITLE)}-full.mp4"
    concat(made, full, reencode=False, metadata=OUT / "chapters.ffmeta")
    print(f"{full.name} {t / 60:.1f} min, {len(made)} chapters", flush=True)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("what", choices=["render", "cut"])
    ap.add_argument("ids", nargs="*")
    ap.add_argument("--jobs", type=int, default=6)
    a = ap.parse_args()
    if a.what == "render":
        render(pick(a.ids, narrate.parse(narrate.SCRIPT.read_text())[0]), a.jobs)
    else:
        cut(a.ids)


if __name__ == "__main__":
    main()
