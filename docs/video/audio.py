"""The video's sound beside the voice, all synthesised here: nothing recorded, sampled or licensed, so nothing a
Content ID claim could match (see CREDITS.md).

- Cues: short 8-bit effects for what happens on screen (film.py writes build/cues/<scene>.json). They are sparse
  by design: each kind has a minimum spacing, and a cue that would land on a speech peak moves into the next gap
  in the voice, or is dropped (or, for a success, failure or transition, played far below the voice).
- Music: a chiptune bed per chapter, its key, tempo, mode, chords and arpeggio chosen per part and per chapter,
  its layers changing every eight bars, so 45 minutes never loop one track. It is ducked under the voice.

    uv run python audio.py demo     build/audio-demo/: every cue and the bed of a few chapters, to listen to
    uv run python audio.py test     the self-check: ducking depth, cue spacing, no cue on speech

build.py mixes each chapter with mix() and sets the loudness (-14 LUFS) when it encodes.
"""

import sys
from pathlib import Path

import numpy as np
import soundfile as sf

SR = 48000
HOP = 480  # 10 ms: the voice's envelope resolution
BUILD = Path(__file__).resolve().parent / "build"

# ---- Tones ------------------------------------------------------------------------------------------------


def hz(midi):
    return 440.0 * 2 ** ((midi - 69) / 12)


def wave(kind, f, n, duty=0.25):
    """Band-limited waveforms (additive, harmonics under 9 kHz): the chip sound without aliasing hiss."""
    t = np.arange(n) / SR
    f = np.broadcast_to(np.asarray(f, dtype=float), (n,)) if np.ndim(f) else np.full(n, float(f))
    ph = 2 * np.pi * np.cumsum(f) / SR
    top = int(9000 / max(f.max(), 1))
    if kind == "sine":
        return np.sin(ph)
    if kind == "tri":
        return sum((-1) ** k * np.sin((2 * k + 1) * ph) / (2 * k + 1) ** 2 for k in range(min(4, top // 2 + 1))) * 0.81
    if kind == "square":
        return sum(np.sin((2 * k + 1) * ph) / (2 * k + 1) for k in range(max(1, min(6, top // 2)))) * 0.9
    if kind == "pulse":
        return sum(np.sin(np.pi * k * duty) * np.sin(k * ph) / k for k in range(1, max(2, min(10, top)))) * 1.1
    if kind == "noise":
        return np.random.default_rng(int(t.size + f[0])).uniform(-1, 1, n)
    raise ValueError(kind)


def env(n, attack=0.004, decay=None, release=0.02):
    """Attack ramp, exponential decay (seconds to 1/e), release ramp at the end."""
    t = np.arange(n) / SR
    e = np.minimum(1.0, t / max(attack, 1e-4))
    if decay:
        e *= np.exp(-t / decay)
    r = int(release * SR)
    if r and n > r:
        e[-r:] *= np.linspace(1, 0, r)
    return e


def note(kind, midi_or_hz, dur, amp=1.0, hz_given=False, **kw):
    n = int(dur * SR)
    f = midi_or_hz if hz_given else hz(midi_or_hz)
    return wave(kind, f, n, kw.pop("duty", 0.25)) * env(n, **kw) * amp


def seq(*parts):
    return np.concatenate(parts)


# ---- Cues -------------------------------------------------------------------------------------------------


def cue_bank():
    """Each cue kind as a mono sound, peaks near 1."""
    rng = np.random.default_rng(7)
    click = lambda f: note("sine", f, 0.012, decay=0.004, hz_given=True)
    gap = lambda s: np.zeros(int(s * SR))
    bank = {
        "type": seq(*(np.concatenate([click(2000 + rng.integers(-300, 300)), gap(0.03)]) for _ in range(4))) * 0.5,
        "frame": seq(note("square", 79, 0.035, decay=0.03), note("square", 86, 0.05, decay=0.04)) * 0.55,
        "real": seq(*(note("pulse", int(rng.integers(84, 96)), 0.018, decay=0.01) for _ in range(7))) * 0.4,
        "big": (note("sine", np.geomspace(120, 50, int(0.18 * SR)), 0.18, hz_given=True, decay=0.08)
                + note("noise", 1, 0.18, hz_given=True, decay=0.03) * 0.25),
        "shot": note("noise", 1, 0.06, hz_given=True, decay=0.012) * 0.6 + np.pad(click(3000), (0, int(0.048 * SR))) * 0.4,
        "zoom": note("sine", np.geomspace(420, 760, int(0.12 * SR)), 0.12, hz_given=True, attack=0.03, decay=0.08) * 0.45,
        "mark": note("tri", 88, 0.18, decay=0.07) * 0.7,
        "ok": seq(note("square", 84, 0.06, decay=0.05), note("square", 88, 0.06, decay=0.05), note("square", 91, 0.12, decay=0.08)) * 0.55,
        "fail": seq(note("square", np.full(int(0.12 * SR), hz(69)) * (1 + 0.01 * np.sin(np.arange(int(0.12 * SR)) / SR * 2 * np.pi * 30)),
                         0.12, hz_given=True, decay=0.1), note("square", 65, 0.2, decay=0.12)) * 0.6,
        "transition": (seq(*(note("pulse", m, 0.045, decay=0.04) for m in (60, 64, 67, 72, 76, 79, 84)))
                       + np.pad(note("noise", 1, 0.32, hz_given=True, decay=0.12) * 0.15, (0, 0))[: int(0.315 * SR)]) * 0.6,
        "title": (seq(*(note("pulse", m, 0.09, decay=0.12, duty=0.125) for m in (62, 69, 74, 78, 81, 86)))
                  + note("tri", 50, 0.54, decay=0.4)[: int(0.54 * SR)] * 0.5) * 0.6,
    }
    return {k: v / max(1e-9, np.abs(v).max()) for k, v in bank.items()}


# How often each kind may sound (seconds), and whether it matters enough to sound below the voice when it must.
SPACING = {"type": 1.4, "frame": 0.7, "real": 0.9, "zoom": 1.2, "shot": 0.6, "mark": 0.6, "big": 0.5,
           "ok": 0.4, "fail": 0.4, "transition": 0.0, "title": 0.0}
MUST = {"ok", "fail", "transition", "title", "big"}
LEVEL = {"type": 0.10, "frame": 0.12, "real": 0.08, "zoom": 0.10, "shot": 0.12, "mark": 0.14, "big": 0.20,
         "ok": 0.18, "fail": 0.18, "transition": 0.22, "title": 0.25}  # peak, relative to the voice's peak


def free(loud, at, span):
    """No speech peak in the span HOPs from at seconds."""
    i = int(at * SR / HOP)
    return not loud[i: i + span].any()


def place(cues, loud, shift=0.45):
    """
    The cues that will sound: (time, kind, gain). loud is a boolean per HOP: the voice is near a peak there.
    A cue moves forward into a gap (up to shift s); if there is none it is dropped, or for a cue that must
    sound, played 15 dB lower. Kinds keep their spacing; no two cues start within 0.12 s.
    """
    out, last = [], {}
    bank = cue_bank()
    for c in sorted(cues, key=lambda c: c["t"]):
        k, t = c["k"], c["t"]
        if k not in bank or t - last.get(k, -99) < SPACING[k] or (out and t - out[-1][0] < 0.12):
            continue
        span = max(1, int(min(len(bank[k]) / SR, 0.2) * SR / HOP))
        at = next((t + d / 100 for d in range(0, int(shift * 100) + 1, 2) if free(loud, t + d / 100, span)), None)
        if at is None and k not in MUST:
            continue
        gain = 1.0 if at is not None else 10 ** (-15 / 20)
        at = t if at is None else at
        out.append((at, k, gain))
        last[k] = at
    return out


# ---- Music ------------------------------------------------------------------------------------------------

SCALES = {"major": [0, 2, 4, 5, 7, 9, 11], "minor": [0, 2, 3, 5, 7, 8, 10], "dorian": [0, 2, 3, 5, 7, 9, 10],
          "mixolydian": [0, 2, 4, 5, 7, 9, 10]}
PROGRESSIONS = {
    "major": [[0, 5, 3, 4], [0, 3, 5, 4], [5, 3, 0, 4], [0, 4, 5, 3], [3, 4, 2, 5]],
    "minor": [[0, 5, 2, 6], [0, 3, 6, 2], [0, 6, 5, 6], [5, 6, 0, 0]],
    "dorian": [[0, 3, 0, 3], [0, 6, 3, 0], [0, 1, 3, 4]],
    "mixolydian": [[0, 6, 3, 0], [0, 3, 6, 3]],
}
ARPS = [[0, 1, 2, 1], [0, 2, 1, 2], [0, 1, 2, 3], [2, 1, 0, 1], [0, 2, 3, 2], [0, 0, 2, 1]]


def mood(chapter):
    """Per part: the cold open drives, part 1 is calm and curious, part 2 brighter, the recipes warm."""
    rng = np.random.default_rng(1000 + chapter)
    if chapter == 0:
        return {"bpm": 100, "root": 50, "mode": "dorian", "sixteenths": True, "rng": rng}
    if chapter <= 10:
        return {"bpm": int(rng.integers(80, 90)), "root": int(rng.choice([45, 47, 50, 52, 43])),
                "mode": str(rng.choice(["minor", "dorian"])), "sixteenths": False, "rng": rng}
    if chapter <= 31:
        return {"bpm": int(rng.integers(90, 100)), "root": int(rng.choice([48, 50, 53, 55, 45])),
                "mode": str(rng.choice(["major", "mixolydian", "major"])), "sixteenths": bool(rng.random() < 0.4), "rng": rng}
    return {"bpm": 86, "root": 48, "mode": "major", "sixteenths": False, "rng": rng}


def bed(chapter, n):
    """The chapter's music, n samples, stereo, peak 1: pad, bass, arpeggio (with an echo) and, in some sections, a hat."""
    m = mood(chapter)
    rng, bpm, root, mode = m["rng"], m["bpm"], m["root"], m["mode"]
    scale = SCALES[mode]
    prog = PROGRESSIONS[mode][int(rng.integers(len(PROGRESSIONS[mode])))]
    beat = 60 / bpm
    bar = 4 * beat
    seconds = n / SR
    out = np.zeros((n + int(2 * bar * SR), 2))

    def deg(d, octave=0):
        return root + 12 * (octave + d // 7) + scale[d % 7]

    def add(sig, at, pan=0.0):
        i = int(at * SR)
        j = min(len(out), i + len(sig))
        if i >= len(out) or j <= i:
            return
        out[i:j, 0] += sig[: j - i] * (1 - pan) / 2
        out[i:j, 1] += sig[: j - i] * (1 + pan) / 2

    bars = int(seconds / bar) + 1
    for b in range(bars):
        section = b // 8
        srng = np.random.default_rng(chapter * 101 + section)
        layers = {"pad": True, "bass": section % 4 != 3, "arp": section % 3 != 2 or chapter == 0, "hat": srng.random() < 0.35}
        arp = ARPS[int(srng.integers(len(ARPS)))]
        lift = int(srng.choice([1, 1, 2]))  # the arpeggio's octave
        d = prog[(b // 2) % len(prog)]
        chord = [deg(d), deg(d + 2), deg(d + 4), deg(d + 6)]
        t0 = b * bar
        if b % 2 == 0 and layers["pad"]:
            # Each chord rings on past the next one's start, so the pad never dips between chords.
            for k, mnote in enumerate(chord[:3]):
                p = note("tri", mnote + 12, 2 * bar + 0.9, attack=0.5, release=0.9) * 0.16
                add(p, t0, pan=[-0.5, 0.0, 0.5][k])
                add(note("tri", mnote + 12.07, 2 * bar + 0.9, attack=0.6, release=0.9) * 0.06, t0, pan=[0.5, -0.3, -0.5][k])
        if layers["bass"]:
            for k in (0, 2):
                add(note("tri", chord[0] - 12, beat * 1.6, decay=0.5, release=0.08) * 0.42, t0 + k * beat)
        if layers["arp"]:
            step = beat / (4 if m["sixteenths"] else 2)
            for k in range(int(bar / step)):
                mnote = chord[arp[k % len(arp)]] + 12 * lift
                s = note("pulse", mnote, step * 0.9, decay=0.09, release=0.01, duty=0.25) * 0.11
                for echo, g in ((0, 1.0), (0.75 * beat, 0.35), (1.5 * beat, 0.14)):
                    add(s * g, t0 + k * step + echo, pan=0.35 if (k // 2) % 2 else -0.35)
        if layers["hat"]:
            for k in range(4):
                add(note("noise", 1, 0.03, hz_given=True, decay=0.008) * 0.05, t0 + k * beat + beat / 2, pan=0.2)
    out = out[:n]
    fade = int(min(1.5, seconds / 4) * SR)
    out[:fade] *= np.linspace(0, 1, fade)[:, None]
    out[-fade:] *= np.linspace(1, 0, fade)[:, None]
    return out / max(1e-9, np.abs(out).max())


# ---- Mix --------------------------------------------------------------------------------------------------


def envelope(voice):
    """The voice's RMS per HOP."""
    k = len(voice) // HOP
    return np.sqrt((voice[: k * HOP].reshape(k, HOP) ** 2).mean(1) + 1e-12)


def duck(rms, speech, depth_db=10.0, attack=3, release=45):
    """Sidechain gain per HOP: down depth_db while the voice speaks, back over release hops (450 ms)."""
    active = rms > 0.12 * speech
    g, cur = np.ones(len(rms)), 1.0
    low = 10 ** (-depth_db / 20)
    for i, a in enumerate(active):
        want = low if a else 1.0
        cur += (want - cur) / (attack if want < cur else release)
        g[i] = cur
    return g


def mix(voice, cues, chapter, bed_db=-17.0):
    """
    The chapter's sound, stereo float: the voice in the middle, the music bed bed_db below the voice's level
    while it pauses and about 10 dB lower again under speech, and the placed cues. Loudness is set later.
    """
    rms = envelope(voice)
    speech = np.percentile(rms[rms > rms.max() * 0.05], 75) if (rms > rms.max() * 0.05).any() else 1.0
    loud = rms > 0.45 * speech
    music = bed(chapter, len(voice))
    g = np.repeat(duck(rms, speech), HOP)
    g = np.pad(g, (0, len(voice) - len(g)), mode="edge")
    music *= (g * speech * 10 ** (bed_db / 20) / max(1e-9, np.sqrt((music**2).mean())))[:, None]
    out = np.repeat(voice[:, None], 2, axis=1) + music
    bank = cue_bank()
    peak = np.abs(voice).max()
    placed = place(cues, loud)
    for at, k, gain in placed:
        s = bank[k] * LEVEL[k] * peak * gain
        i = int(at * SR)
        j = min(len(out), i + len(s))
        if j > i:
            out[i:j] += (s[: j - i] * 0.71)[:, None]
    return out, placed


# ---- Self-check and demo ------------------------------------------------------------------------------------


def test():
    """Ducking reaches its depth under speech, cues keep their spacing and stay off speech peaks."""
    voice = np.zeros(10 * SR)
    t = np.arange(SR * 2) / SR
    voice[3 * SR: 5 * SR] = 0.3 * np.sin(2 * np.pi * 200 * t)  # 2 s of "speech"
    rms = envelope(voice)
    g = duck(rms, np.percentile(rms[rms > 0.01], 75))
    assert g[int(4.5 * SR / HOP)] < 10 ** (-8 / 20), "music is not ducked under speech"
    assert g[int(9.5 * SR / HOP)] > 0.95, "music does not come back after speech"
    loud = rms > 0.1
    cues = [{"t": 1.0, "k": "type"}, {"t": 1.5, "k": "type"}, {"t": 3.5, "k": "frame"}, {"t": 4.0, "k": "ok"}, {"t": 6.0, "k": "mark"}]
    placed = place(cues, loud)
    kinds = [k for _, k, _ in placed]
    assert kinds.count("type") == 1, "type cues ignore their spacing"
    assert "frame" not in kinds, "a cue was kept on speech"
    ok = next(p for p in placed if p[1] == "ok")
    assert ok[2] < 0.2, "a cue that must sound on speech is not played far below it"
    out, _ = mix(voice, cues, 3)
    assert np.isfinite(out).all() and out.shape[1] == 2
    assert bed(5, 1781759).shape == (1781759, 2), "the bed is not exactly as long as the voice"
    print("audio.py: self-check passed")


def demo():
    d = BUILD / "audio-demo"
    d.mkdir(parents=True, exist_ok=True)
    for k, v in cue_bank().items():
        sf.write(d / f"cue-{k}.wav", v * 0.5, SR)
    for ch in (0, 3, 17, 32):
        sf.write(d / f"bed-{ch:02d}.wav", bed(ch, 40 * SR) * 0.5, SR)
    print(f"wrote {d}")


if __name__ == "__main__":
    {"test": test, "demo": demo}[sys.argv[1] if len(sys.argv) > 1 else "test"]()
