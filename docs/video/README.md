# Video tutorial sources

The narrated tutorial *Nix, NixOS and nixos-manager*, built from [`script.md`](script.md). It is drawn the way the
console is: text on one grid of cells, the console's own stylesheet, font and frames, and motion stepping on its
80 ms clock (`web/src/lib/ascii.ts`), 1920 x 1080 at 25 fps. Console captures sit in the same frames. The voice
is a local Kokoro model; the sound effects and music are synthesised in code ([`CREDITS.md`](CREDITS.md)).
Nothing here calls a paid service. Rendered files go to `build/` and stay out of git.

| File | What it does |
|---|---|
| `script.md` | The script: narration, visuals and production notes, scene by scene |
| `narrate.py` | Voice-over: one WAV per scene with word timings (Kokoro-82M, voice `af_heart`), after the script's pronunciation table |
| `capture.py` | Console screenshots: a demo server on `scripts/dev/fake-nix` with the fixture fleet, driven by Playwright |
| `film/` | The stage (`film.ts`) and every scene (`part*.ts`), written against `web/src/lib/ascii.ts` and the intro's scenes |
| `film.py` | Renders scenes in Chromium, one image per tick that changed, encoded with the voice; writes each scene's sound cues |
| `audio.py` | The 8-bit sound effects and the music bed per chapter, and the mix: music ducked under the voice, cues kept off speech |
| `build.py` | Mixes and cuts one MP4 per chapter (-14 LUFS) and the full video with chapter markers; fits it under a size, or splits it into parts |
| `CREDITS.md` | Every sound in the video, its source and its license |

## Build

Needs uv, ffmpeg and Chromium, and the web dependencies (`cd web && npm ci`) for the bundler. The demo server
also needs Go. The voice takes about an hour on a 16-core machine without a GPU; the render about an hour.

```sh
cd docs/video
uv sync                                    # Python 3.12, Kokoro, Playwright (CPU torch)
uv run python narrate.py --jobs 4          # build/audio/S*.wav + word timings

uv run python capture.py serve fresh &     # a new install, on 127.0.0.1:18339
uv run python capture.py serve fleet &     # the fixture fleet, as http://nixos-manager.home.lan:8080
uv run python capture.py shots fresh
uv run python capture.py shots fleet       # build/shots/*.png + the boxes scenes zoom to

uv run python film.py --check              # every scene's layout, in seconds
uv run python film.py --jobs 8             # build/scenes/S*.mkv, and build/steps/<scene>/ for review
uv run python audio.py test                # the mix's self-check (ducking, cue spacing, cues off speech)
uv run python build.py cut                 # build/out/NN-chapter.mp4 and the full cut, mixed at -14 LUFS
uv run python build.py fit 97              # the full cut re-encoded under 97 MiB (two passes), to attach it
uv run python build.py parts 95            # or: the full cut in parts under 95 MiB, each with its markers
```

`film.py` accepts scene ids or chapter numbers. `build.py cut` also writes `build/out/chapters.txt`, the chapter
list with start times (YouTube style). Change a scene's narration in `script.md` and run `narrate.py` again: only
that scene is re-voiced, and its animation follows the new timing when rendered again.

## The grid checks

Every element on the stage sits on whole cells, so `film.py` checks each scene on every tick: nothing may leave
the 145 x 40 grid, no two elements may cover the same cells unless one sits inside the other's frame, a frame's
title and tag must fit on its top line whole, and nothing may sit on the status line (the last row). Captures are
checked too: each is drawn at the size its view asks for, and a view may not zoom into a capture box of a few
pixels. And the picture may never stand still for more than 5 seconds (the status line does not count). A scene
that breaks a rule is listed and the run exits non-zero. `build/steps/<scene>/` keeps one frame per step of each
scene (each cue it waits for, each capture step) to look through.

## Motion and sound

Besides what each scene draws on its cues, the stage moves on its own, never over text the viewer reads:

- labels the narration names light up (the console's flash, then its selected-row colour); on a capture the
  button or heading named gets an outline, exact to the pixel like the console's highlights;
- arrows draw themselves in and now and then send a packet along; captions end in a blinking cursor;
- when the picture has stood still for 3.6 s, a capture steps in towards what is in focus (or back out), or a
  bright run goes once round the newest frame;
- the status line shows the chapter's progress and a meter that moves with the voice; scenes leave in a dither,
  chapter cards come and go in one.

The scenes log their events (a frame drawn, text typed, a success, a failure, a chapter card) and `audio.py` turns
them into short 8-bit cues: each kind has a minimum spacing, and a cue that would land on a speech peak moves into
the next pause or is dropped. Each chapter gets its own music bed, ducked about 25 dB under the voice while it
speaks. `build.py` mixes the three and sets the chapter to -14 LUFS.

## What the captures show

The console in the captures is real; the fleet is not. `fleet` serves the repository's fixture fleet
(`internal/core/testdata/fleet`, without its broken `oldbox`), cloned from a local bare repository that a
stand-in `ssh` presents as `ssh://git@forgejo.home.lan:2222/fleet/nixos.git`. Its data lives in
`~/nixos-manager-demo`. `capture.py` writes a marker file there and refuses to start in, or reset, a folder
without it, so it never touches a folder it did not make. Delete the folder when done. Where fake Nix cannot
answer, the capture fills in:

- deployment reports, heartbeats and closures are posted to the console's own report API, as comin's
  report hook would, with each machine's token;
- a wrapper around fake Nix makes each machine's toplevel follow its managed files, so checks build what
  changed and trials can start;
- the per-machine model diff in Changes (S22.2), the scheduled update result (S26.2) and the installer's
  host key (S16.3) are answered in the browser with plausible data;
- the demo fixtures gain a few search results (ripgrep, ncdu, nodejs_24, ffmpeg) and the `nas` machine
  the New machine wizard creates.

Never point `capture.py` at a real console: the captures show host names, remote URLs and settings.
Keys and tokens on screen are placeholders. `capture.py shots` exits non-zero if a shot or step failed.
