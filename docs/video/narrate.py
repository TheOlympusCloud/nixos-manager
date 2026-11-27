"""Voice-over: one WAV per scene of script.md, with word timings, using Kokoro locally.

    uv run python narrate.py [--jobs 4] [--voice af_heart] [S1.1 S1.2 ...]

Writes build/audio/<scene>.wav and build/audio/<scene>.json ({duration, words}).
A scene whose spoken text and voice are unchanged is skipped.
"""

import argparse
import hashlib
import json
import multiprocessing as mp
import multiprocessing.util  # mp.util
import os
import re
import shutil
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
SCRIPT = HERE / "script.md"
OUT = HERE / "build" / "audio"
RATE = 24000
LEAD = 0.3  # silence before the first word
GAP = 0.4  # silence between paragraphs (production notes)

# Words the script's pronunciation table leaves out that Kokoro reads wrongly.
EXTRA = {
    "nixos-manager": "nix O S manager",
    "nixos": "nix O S",
    "nginx": "engine X",
    "systemd": "system D",
    "toplevel": "top level",
    "openssl": "open S S L",
    "zlib": "zee lib",
    "TOML": "tom-ul",
    "x86": "x eighty six",
    "slash etc": "slash E T C",
}


def parse(text):
    """Scenes in order: [{id, chapter, title, paragraphs}] and the pronunciation table."""
    scenes, chapter, chapter_title, lexicon = [], None, None, {}
    in_table = False
    for line in text.splitlines():
        if line.startswith("Pronunciation"):
            in_table = True
            continue
        if in_table:
            if not line.startswith("|"):
                in_table = not lexicon  # the table ends at its first non-row line
                continue
            cells = [c.strip() for c in line.strip("|").split("|")]
            says = re.findall(r'"([^"]*)"', cells[1])
            written = [w.strip().strip("`") for w in cells[0].split(",")]
            for w, s in zip(written, says):
                lexicon[w] = s
            continue
        m = re.match(r"## Chapter (\d+): (.*?) \(", line)
        if m:
            chapter, chapter_title = int(m.group(1)), m.group(2)
            continue
        m = re.match(r"### (S[\d.]+) (.*?) \(", line)
        if m:
            scenes.append({"id": m.group(1), "chapter": chapter, "chapter_title": chapter_title,
                           "title": m.group(2), "paragraphs": [""]})
            continue
        if scenes and line.startswith(">"):
            body = line[1:].strip()
            paras = scenes[-1]["paragraphs"]
            if body:
                paras[-1] = (paras[-1] + " " + body).strip()
            elif paras[-1]:
                paras.append("")
    for s in scenes:
        s["paragraphs"] = [p for p in s["paragraphs"] if p]
    return scenes, lexicon


def speakable(text, lexicon):
    for written, say in sorted({**lexicon, **EXTRA}.items(), key=lambda kv: -len(kv[0])):
        text = re.sub(r"(?<![\w/.-])" + re.escape(written) + r"(?![\w-])", say, text)
    return text


def norm(word):
    return re.sub(r"[^a-z0-9]", "", word.lower())


_pipe = None


def _init(threads):
    global _pipe
    import torch

    torch.set_num_threads(threads)
    import espeakng_loader
    import misaki.espeak  # noqa: F401  sets the long default data path we override below
    from phonemizer.backend.espeak.wrapper import EspeakWrapper

    # espeak-ng ignores a data path over 160 characters (deep venvs hit it) and phonemizer
    # resolves symlinks, so give it a copy at a short path.
    tmp = tempfile.mkdtemp(prefix="espk")
    # Pool workers end with os._exit, past atexit: a multiprocessing finalizer removes the copy.
    mp.util.Finalize(None, shutil.rmtree, args=(tmp, True), exitpriority=0)
    short = os.path.join(tmp, "d")
    shutil.copytree(espeakng_loader.get_data_path(), short)
    EspeakWrapper.set_data_path(short)
    from kokoro import KPipeline

    _pipe = KPipeline(lang_code="a", repo_id="hexgrad/Kokoro-82M")


def synth(job):
    import numpy as np
    import soundfile as sf

    sid, paragraphs, voice, digest = job
    audio, words, t = [np.zeros(int(LEAD * RATE), dtype=np.float32)], [], LEAD
    for i, para in enumerate(paragraphs):
        if i:
            audio.append(np.zeros(int(GAP * RATE), dtype=np.float32))
            t += GAP
        for r in _pipe(para, voice=voice, speed=1.0):
            for tk in r.tokens or []:
                if norm(tk.text) and tk.start_ts is not None:
                    words.append({"w": norm(tk.text), "s": round(t + tk.start_ts, 3), "e": round(t + tk.end_ts, 3)})
            a = r.audio.numpy()
            audio.append(a)
            t += len(a) / RATE
    data = np.concatenate(audio)
    sf.write(OUT / f"{sid}.wav", data, RATE)
    meta = {"duration": round(len(data) / RATE, 3), "words": words, "digest": digest}
    (OUT / f"{sid}.json").write_text(json.dumps(meta))
    return sid, meta["duration"]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("scenes", nargs="*")
    ap.add_argument("--jobs", type=int, default=4)
    ap.add_argument("--voice", default="af_heart")
    args = ap.parse_args()
    scenes, lexicon = parse(SCRIPT.read_text())
    assert lexicon.get("NixOS") == "nix O S" and lexicon.get("sops-nix") == "sops nix", lexicon
    assert scenes and all(s["paragraphs"] and s["chapter_title"] for s in scenes), "a scene without narration"
    assert speakable("nixos-manager's NixOS, nixpkgs and sops-nix", lexicon) == \
        "nix O S manager's nix O S, nix packages and sops nix"
    OUT.mkdir(parents=True, exist_ok=True)
    jobs = []
    for s in scenes:
        if args.scenes and s["id"] not in args.scenes:
            continue
        paras = [speakable(p, lexicon) for p in s["paragraphs"]]
        digest = hashlib.sha256(json.dumps([args.voice, LEAD, GAP, paras]).encode()).hexdigest()[:16]
        meta = OUT / f"{s['id']}.json"
        if meta.exists() and json.loads(meta.read_text()).get("digest") == digest:
            continue
        jobs.append((s["id"], paras, args.voice, digest))
    print(f"{len(jobs)} scene(s) to synthesise", flush=True)
    threads = max(1, (os.cpu_count() or 4) // args.jobs)
    with mp.get_context("spawn").Pool(args.jobs, initializer=_init, initargs=(threads,)) as pool:
        for sid, dur in pool.imap_unordered(synth, jobs):
            print(f"{sid} {dur:.1f}s", flush=True)


if __name__ == "__main__":
    main()
