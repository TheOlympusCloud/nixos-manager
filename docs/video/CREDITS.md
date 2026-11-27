# Audio credits

Every sound in the video, where it comes from, and its license. Nothing here is recorded, sampled or bought, so
no Content ID reference can match it.

| Sound | Source | License |
|---|---|---|
| Narration | Spoken by the [Kokoro-82M](https://huggingface.co/hexgrad/Kokoro-82M) text-to-speech model, voice `af_heart`, run locally by `narrate.py` from the text of `script.md` | Model and voice: Apache-2.0. The speech is generated output, not a recording |
| Sound effects | Synthesised by `audio.py` (`cue_bank`): sine, triangle, pulse and noise tones with envelopes, made for this video | Original, part of this repository |
| Music | Composed by `audio.py` (`bed`): a chiptune bed per chapter from code-chosen keys, chords and arpeggios, made for this video | Original, part of this repository |

`espeak-ng` (GPL-3.0) only turns the script's words into phonemes for Kokoro; none of its audio is used.

Rebuilding the video regenerates all of the above from the sources (`narrate.py`, `audio.py`); there are no
audio files in the repository.
