# Video tutorial sources

The narrated tutorial *Nix, NixOS and nixos-manager*, built from [`script.md`](script.md): Manim animations,
captures of the real console running on fake Nix, and a local Kokoro voice. Nothing here calls a paid
service. Rendered files go to `build/` and stay out of git.

| File | What it does |
|---|---|
| `script.md` | The script: narration, visuals and production notes, scene by scene |
| `narrate.py` | Voice-over: one WAV per scene with word timings (Kokoro-82M, voice `af_heart`), after the script's pronunciation table |
| `capture.py` | Console screenshots: a demo server on `scripts/dev/fake-nix` with the fixture fleet, driven by Playwright |
| `scenes/` | One Manim class per scene (`S1_1` is scene S1.1), timed to the words of its narration |
| `build.py` | Renders scenes and cuts one MP4 per chapter plus the full video with chapter markers |

## Build

Needs uv, ffmpeg, Chromium, and Go and npm for the demo server. About 45 minutes of video; on a 16-core
machine without a GPU, expect about an hour for the voice and two for the render.

```sh
cd docs/video
uv sync                                    # Python 3.12, Kokoro, Manim, Playwright (CPU torch)
uv run python narrate.py --jobs 4          # build/audio/S*.wav + word timings

uv run python capture.py serve fresh &     # a new install, on 127.0.0.1:18339
uv run python capture.py serve fleet &     # the fixture fleet, as http://nixos-manager.home.lan:8080
uv run python capture.py shots fresh
uv run python capture.py shots fleet       # build/shots/*.png + the boxes scenes zoom to

uv run python build.py render --jobs 6     # build/media/…/S*.mp4 (ids or chapter numbers to redo a part)
uv run python build.py cut                 # build/out/NN-chapter.mp4 and the full cut
```

`build.py cut` also writes `build/out/chapters.txt`, the chapter list with start times (YouTube style).
Change a scene's narration in `script.md` and run `narrate.py` again: only that scene is re-voiced, and its
animation follows the new timing when rendered again.

## What the captures show

The console in the captures is real; the fleet is not. `fleet` serves the repository's fixture fleet
(`internal/core/testdata/fleet`, without its broken `oldbox`), cloned from a local bare repository that a
stand-in `ssh` presents as `ssh://git@forgejo.home.lan:2222/fleet/nixos.git`. Its data lives in
`~/nixos-manager`, so check logs show a plain path. Where fake Nix cannot answer, the capture fills in:

- deployment reports, heartbeats and closures are posted to the console's own report API, as comin's
  report hook would, with each machine's token;
- the per-machine model diff in Changes (S22.2), the scheduled update result (S26.2) and the installer's
  host key (S16.3) are answered in the browser with plausible data;
- the demo fixtures gain a few search results (ripgrep, ncdu, nodejs_24, ffmpeg) and the `nas` machine
  the New machine wizard creates.

Never point `capture.py` at a real console: the captures show host names, remote URLs and settings.
Keys and tokens on screen are placeholders.

## Machine pictures

`scenes/kit.py` draws each machine as a vector icon. Put `homeserver.png`, `pi.png`, `workstation.png`,
`laptop.png` and `gitserver.png` (square, transparent) in `assets/` and render again to use pictures
instead.
