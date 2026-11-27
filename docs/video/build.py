"""Cut the video from the rendered scenes.

    uv run python build.py cut [1 2 ...]       one MP4 per chapter and the full cut (chapter numbers: only those)
    uv run python build.py parts [MiB]         the full cut split at chapter starts into numbered parts under MiB

film.py renders the scenes (build/scenes/<scene>.mkv), each exactly as long as its voice and its pauses.
Outputs go to build/out/.
"""

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

import narrate

HERE = Path(__file__).resolve().parent
BUILD = HERE / "build"
SCENES = BUILD / "scenes"
OUT = BUILD / "out"
TITLE = "Nix, NixOS and nixos-manager"
LOUD = "loudnorm=I=-16:TP=-1.5:LRA=11"
FULL = OUT / "nix-nixos-and-nixos-manager-full.mp4"


def duration(path):
    out = subprocess.run(["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", str(path)],
                         capture_output=True, text=True, check=True).stdout
    return float(out)


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
        # Text on a still grid compresses far below a camera's rate: animation tuning, a long GOP.
        cmd += ["-af", af, "-c:v", "libx264", "-preset", "slow", "-crf", "24", "-tune", "animation", "-g", "250",
                "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "96k", "-ar", "48000"]
    else:
        cmd += ["-c", "copy"]
    cmd += ["-movflags", "+faststart", str(dest)]
    subprocess.run(cmd, check=True)
    lst.unlink()


def slug(text):
    return re.sub(r"[^a-z0-9]+", "-", text.lower()).strip("-")


def chapters():
    out = {}
    for s in narrate.parse(narrate.SCRIPT.read_text())[0]:
        out.setdefault(s["chapter"], (s["chapter_title"], []))[1].append(s["id"])
    return out


def cut(only):
    OUT.mkdir(parents=True, exist_ok=True)
    made, meta, marks, t = [], [";FFMETADATA1", f"title={TITLE}"], [], 0.0
    for n, (title, ids) in chapters().items():
        if only and str(n) not in only:
            continue
        parts = [SCENES / f"{i.replace('.', '_')}.mkv" for i in ids]
        absent = [str(p) for p in parts if not p.exists()]
        if absent:
            sys.exit(f"chapter {n}: not rendered: {', '.join(absent)}")
        dest = OUT / f"{n:02d}-{slug(title)}.mp4"
        if not dest.exists() or dest.stat().st_mtime < max(p.stat().st_mtime for p in parts):
            concat(parts, dest, reencode=True)
        d = duration(dest)
        meta += ["[CHAPTER]", "TIMEBASE=1/1000", f"START={int(t * 1000)}", f"END={int((t + d) * 1000)}",
                 f"title={n} {title}"]
        marks.append(f"{int(t) // 60}:{int(t) % 60:02d} {n} {title}")
        made.append(dest)
        t += d
        print(f"{dest.name} {d / 60:.1f} min", flush=True)
    if only:
        return
    (OUT / "chapters.ffmeta").write_text("\n".join(meta) + "\n")
    (OUT / "chapters.txt").write_text("\n".join(marks) + "\n")
    concat(made, FULL, reencode=False, metadata=OUT / "chapters.ffmeta")
    print(f"{FULL.name} {t / 60:.1f} min, {len(made)} chapters, {FULL.stat().st_size / 2**20:.0f} MiB", flush=True)


def parts(limit):
    """The full cut in numbered parts, each a run of whole chapters under limit MiB, with its own chapter list."""
    files = sorted(OUT.glob("[0-9][0-9]-*.mp4"))
    groups, size = [[]], 0
    for f in files:
        s = f.stat().st_size / 2**20
        if groups[-1] and size + s > limit:
            groups.append([])
            size = 0
        groups[-1].append(f)
        size += s
    for i, g in enumerate(groups, 1):
        dest = OUT / f"nix-nixos-and-nixos-manager-part{i}of{len(groups)}.mp4"
        concat(g, dest, reencode=False)
        print(f"{dest.name}: chapters {g[0].name[:2]} to {g[-1].name[:2]}, {dest.stat().st_size / 2**20:.0f} MiB")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("what", choices=["cut", "parts"])
    ap.add_argument("args", nargs="*")
    a = ap.parse_args()
    if a.what == "cut":
        cut(a.args)
    else:
        parts(float(a.args[0]) if a.args else 95)


if __name__ == "__main__":
    main()
