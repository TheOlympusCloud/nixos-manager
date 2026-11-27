# Video tutorial sources

The narrated tutorial *Nix, NixOS and nixos-manager*, built from [`script.md`](script.md). It is drawn the way the
console is: text on one grid of cells, the console's own stylesheet, font and frames, and motion stepping on its
80 ms clock (`web/src/lib/ascii.ts`). Console captures sit in the same frames. The voice is a local Kokoro model.
Nothing here calls a paid service. Rendered files go to `build/` and stay out of git.

| File | What it does |
|---|---|
| `script.md` | The script: narration, visuals and production notes, scene by scene |
| `narrate.py` | Voice-over: one WAV per scene with word timings (Kokoro-82M, voice `af_heart`), after the script's pronunciation table |
| `capture.py` | Console screenshots: a demo server on `scripts/dev/fake-nix` with the fixture fleet, driven by Playwright |
| `film/` | The stage (`film.ts`) and every scene (`part*.ts`), written against `web/src/lib/ascii.ts` and the intro's scenes |
| `film.py` | Renders scenes in Chromium, one image per tick that changed, encoded with the voice |
| `build.py` | Cuts one MP4 per chapter and the full video with chapter markers; can split it into parts |

## Build

Needs uv, ffmpeg and Chromium, and the web dependencies (`cd web && npm ci`) for the bundler. The demo server
also needs Go. The voice takes about an hour on a 16-core machine without a GPU; the render about fifteen minutes.

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
uv run python build.py cut                 # build/out/NN-chapter.mp4 and the full cut
uv run python build.py parts 95            # the full cut in parts under 95 MiB, if it is larger
```

`film.py` accepts scene ids or chapter numbers. `build.py cut` also writes `build/out/chapters.txt`, the chapter
list with start times (YouTube style). Change a scene's narration in `script.md` and run `narrate.py` again: only
that scene is re-voiced, and its animation follows the new timing when rendered again.

## The grid checks

Every element on the stage sits on whole cells, so `film.py` checks each scene on every tick: nothing may leave
the 145 x 40 grid, no two elements may cover the same cells unless one sits inside the other's frame, and a
frame's title and tag must fit on its top line whole. Captures are checked too: each is drawn at the size its
view asks for, and a view may not zoom into a capture box of a few pixels. A scene that breaks a rule is listed
and the run exits non-zero. `build/steps/<scene>/` keeps one frame per step of
each scene (each cue it waits for) to look through.

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
