"""Cut the video from the rendered scenes.

    uv run python build.py cut [1 2 ...]       one MP4 per chapter and the full cut (chapter numbers: only those)
    uv run python build.py fit [MiB]           the full cut re-encoded (two passes) to land under MiB, as an attachment
    uv run python build.py parts [MiB]         the full cut split at chapter starts into numbered parts under MiB

film.py renders the scenes (build/scenes/<scene>.mkv), each exactly as long as its voice and its pauses, and
writes their sound cues (build/cues/). Each chapter's sound is mixed here: the voice, the cues and the music bed
from audio.py, at -14 LUFS (YouTube's target). Outputs go to build/out/.
"""

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

import audio
import film
import narrate
import numpy as np
import soundfile as sf

HERE = Path(__file__).resolve().parent
BUILD = HERE / "build"
SCENES = BUILD / "scenes"
OUT = BUILD / "out"
TITLE = "Nix, NixOS and nixos-manager"
LOUD = "loudnorm=I=-14:TP=-1.5:LRA=11"
MIX = BUILD / "mix"
FULL = OUT / "nix-nixos-and-nixos-manager-full.mp4"


def duration(path):
    out = subprocess.run(["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", str(path)],
                         capture_output=True, text=True, check=True).stdout
    return float(out)


def chapter_sound(n, ids, lst):
    """The chapter's mix as a WAV: its voice (from the scenes), its cues and its music bed."""
    MIX.mkdir(parents=True, exist_ok=True)
    raw = subprocess.run(["ffmpeg", "-v", "error", "-f", "concat", "-safe", "0", "-i", str(lst), "-vn", "-ac", "1",
                          "-ar", str(audio.SR), "-f", "f32le", "-"], capture_output=True, check=True).stdout
    voice = np.frombuffer(raw, dtype=np.float32).astype(np.float64)
    scenes = narrate.parse(narrate.SCRIPT.read_text())[0]
    cues, t = [], 0.0
    for sid in ids:
        scene = next(s for s in scenes if s["id"] == sid)
        cues += [{"t": t + c["t"], "k": c["k"]} for c in json.loads((BUILD / "cues" / f"{sid}.json").read_text())]
        t += film.ticks_of(scene, scenes)[0] * film.TICK
    out, placed = audio.mix(voice, cues, n)
    dest = MIX / f"{n:02d}.wav"
    sf.write(dest, out.astype(np.float32), audio.SR, subtype="FLOAT")
    (MIX / f"{n:02d}-cues.json").write_text(json.dumps(placed))
    return dest


def concat(parts, dest, reencode, metadata=None, sound=None):
    lst = dest.with_suffix(".txt")
    lst.write_text("".join(f"file '{p}'\n" for p in parts))
    cmd = ["ffmpeg", "-v", "error", "-y", "-f", "concat", "-safe", "0", "-i", str(lst)]
    if metadata:
        cmd += ["-i", str(metadata), "-map_metadata", "1", "-map_chapters", "1"]
    if reencode:  # the chapter's mix, two-pass loudnorm to -14 LUFS
        mixed = sound(lst)
        cmd += ["-i", str(mixed), "-map", "0:v", "-map", "1:a"]
        probe = subprocess.run(["ffmpeg", "-v", "info", "-i", str(mixed), "-af", LOUD + ":print_format=json", "-f", "null", "-"],
                               capture_output=True, text=True, check=True).stderr
        m = json.loads(re.findall(r"\{[^{}]*\}", probe)[-1])
        af = (f"{LOUD}:linear=true:measured_I={m['input_i']}:measured_TP={m['input_tp']}:"
              f"measured_LRA={m['input_lra']}:measured_thresh={m['input_thresh']}:offset={m['target_offset']}")
        # Text on a still grid compresses far below a camera's rate: animation tuning, a long GOP.
        cmd += ["-af", af, "-c:v", "libx264", "-preset", "slow", "-crf", "24", "-tune", "animation", "-g", "250",
                "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-shortest"]
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
            concat(parts, dest, reencode=True, sound=lambda lst, n=n, ids=ids: chapter_sound(n, ids, lst))
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


def markers(files):
    """An FFMETADATA chapter list for a run of chapter files, as they follow each other."""
    meta, t = [";FFMETADATA1", f"title={TITLE}"], 0.0
    names = {n: title for n, (title, _) in chapters().items()}
    for f in files:
        d = duration(f)
        n = int(f.name[:2])
        meta += ["[CHAPTER]", "TIMEBASE=1/1000", f"START={int(t * 1000)}", f"END={int((t + d) * 1000)}", f"title={n} {names[n]}"]
        t += d
    return "\n".join(meta) + "\n"


def fit(limit):
    """
    The full cut re-encoded in two passes at the bitrate that lands it under limit MiB (attachments are capped at
    100 MiB): the video gets what the audio (96 kbit/s) leaves, chapters and markers kept. FULL is replaced.
    """
    secs = duration(FULL)
    audio_k = 96
    video_k = int((limit * 2**20 * 8 * 0.97) / secs / 1000) - audio_k  # 3 % for the container
    tmp = FULL.with_name("fit.mp4")
    log = MIX / "x264-fit"
    MIX.mkdir(parents=True, exist_ok=True)
    common = ["-c:v", "libx264", "-b:v", f"{video_k}k", "-tune", "animation", "-g", "250", "-pix_fmt", "yuv420p",
              "-passlogfile", str(log)]
    subprocess.run(["ffmpeg", "-v", "error", "-y", "-i", str(FULL), "-map", "0:v", "-preset", "medium"] + common
                   + ["-pass", "1", "-an", "-f", "null", "-"], check=True)
    subprocess.run(["ffmpeg", "-v", "error", "-y", "-i", str(FULL), "-map", "0:v", "-map", "0:a", "-map_metadata", "0",
                    "-map_chapters", "0", "-preset", "slow"] + common + ["-pass", "2",
                    # a lower AAC rate overshoots the chapters' -1.5 dB peaks: limit them first
                    "-af", "alimiter=limit=0.75:level=false", "-c:a", "aac", "-b:a", f"{audio_k}k",
                    "-movflags", "+faststart", str(tmp)], check=True)
    tmp.replace(FULL)
    for f in MIX.glob("x264-fit*"):
        f.unlink()
    print(f"{FULL.name}: {video_k} kbit/s video, {FULL.stat().st_size / 2**20:.1f} MiB")


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
        meta = dest.with_suffix(".ffmeta")
        meta.write_text(markers(g))
        concat(g, dest, reencode=False, metadata=meta)
        meta.unlink()
        print(f"{dest.name}: chapters {g[0].name[:2]} to {g[-1].name[:2]}, {dest.stat().st_size / 2**20:.0f} MiB")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("what", choices=["cut", "fit", "parts"])
    ap.add_argument("args", nargs="*")
    a = ap.parse_args()
    if a.what == "cut":
        cut(a.args)
    elif a.what == "fit":
        fit(float(a.args[0]) if a.args else 97)
    else:
        parts(float(a.args[0]) if a.args else 95)


if __name__ == "__main__":
    main()
