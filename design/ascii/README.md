# Cellwork: the text-mode look of nixos-manager

A design system and a working prototype for redrawing the whole console in characters: every
frame, icon, meter and animation is text on one grid.

- `prototype/index.html` opens from disk in a browser. No build, no server, no network.
- `prototype/#/system` is the living style guide: palette with measured contrast, type, frames,
  glyph icons, every component in every state, and each animation with a replay button.
- `tools/check.py` fails if a glyph is missing from the shipped font or a text colour drops under
  4.5:1, and writes the contrast table below.
- `tools/capture.mjs` takes the screenshots and GIFs (`node tools/capture.mjs tools/shots.json out`,
  `tools/gifs.json` for the animations).

Keys in the prototype: `Ctrl K` palette, `g` then a letter to go to a view, `/` search,
`t` switch theme, `Esc` close. `?still` in the address freezes all motion; `?theme=print` starts
in the light theme.

## Idea

The console already describes machines as text: Nix files, store paths, hashes, logs. Cellwork
makes the interface out of the same material.

Three rules, and nothing on screen breaks them:

1. **The cell.** One font, one size. A cell is `1ch` wide and one row (`--lh`, 17 px) tall, and
   every size, gap and position is a whole number of cells.
2. **Glyphs only.** Lines, corners, icons, meters, shadows and spinners are characters. There is
   no `border`, `border-radius`, `box-shadow` or SVG icon. Colour fills whole cells (hover,
   selection, chips, diff lines), the way a terminal paints them.
3. **One clock.** Every animation steps on the same 80 ms beat (12.5 fps), so the screen moves
   as one device instead of a page of unrelated easings.

What makes it this console's own, and not a generic terminal skin:

- **Realisation.** Text does not fade in; it is *realised*, the way Nix realises a store path
  from its hash. Unresolved characters show in Nix's base-32 alphabet
  (`0123456789abcdfghijklmnpqrsvwxyz`) and settle left to right. Every glyph is one cell wide, so
  nothing reflows while it happens.
- **λ is the prompt.** The palette, search fields and the top bar's title all start at `λ ›`.
- **The snowflake is drawn, not embedded.** Six lambdas shaded in `█▓▒░` and the logo's two
  blues, turning on the clock (boot), and a small line-glyph one idling in the rail.
- **Frame weight is rank.** Round, square, double, heavy and dashed frames each mean one thing.
- **The pixel face.** Page titles, machine names and big numbers are set in a 5-pixel face
  printed with half blocks, three rows tall.
- **Two themes from the same tokens.** Night (default) uses bone white and the two blues of the
  Nix logo on near-black. Print is the same console on greenbar paper, for daylight.

The Instagram reference in the issue could not be opened from here (it returns a login wall), so
none of this is derived from it; it follows the written brief only.

## Palette

Token names are the ones `web/src/app.css` already uses, so components keep working; five are
new (`--shade`, `--accent-fill`, `--glow`, `--bars`, `--tick`) plus the cell (`--fs`, `--lh`).
The values live in `prototype/css/tokens.css`.

Contrast below is computed by `tools/check.py`: on `--bg`, and the lowest value over every
background of the theme (raised, input, hover, active, and the greenbar tint in Print). Text needs
4.5:1, glyph strokes 3:1.

<!-- contrast:begin -->
| Theme | Token | Value | On `--bg` | Lowest | On |
|---|---|---|---|---|---|
| Night | `fg` | `#d3cfc3` | 12.5:1 | 9.6:1 | `bg-active` |
| Night | `fg-strong` | `#f6f2e6` | 17.4:1 | 13.3:1 | `bg-active` |
| Night | `fg-muted` | `#a6abbb` | 8.5:1 | 6.5:1 | `bg-active` |
| Night | `fg-faint` | `#8f95ab` | 6.5:1 | 5.0:1 | `bg-active` |
| Night | `accent` | `#7ebae4` | 9.3:1 | 7.1:1 | `bg-active` |
| Night | `accent-strong` | `#a5d2f2` | 12.1:1 | 9.3:1 | `bg-active` |
| Night | `green` | `#8dd28a` | 10.8:1 | 8.3:1 | `bg-active` |
| Night | `yellow` | `#f0b950` | 10.9:1 | 8.3:1 | `bg-active` |
| Night | `red` | `#ff7468` | 7.4:1 | 5.6:1 | `bg-active` |
| Night | `purple` | `#b9a0f5` | 8.7:1 | 6.7:1 | `bg-active` |
| Night | `cyan` | `#6fd2c6` | 10.8:1 | 8.3:1 | `bg-active` |
| Night | `g-virtualisation` | `#f08fb6` | 8.6:1 | 6.6:1 | `bg-active` |
| Night | `g-hardware` | `#f09a5e` | 8.8:1 | 6.7:1 | `bg-active` |
| Night | `reverse video on accent` | `#7ebae4` | 9.6:1 | 9.6:1 | `accent` |
| Night | `reverse video on accent-strong` | `#a5d2f2` | 12.5:1 | 12.5:1 | `accent-strong` |
| Night | `reverse video on red` | `#ff7468` | 7.6:1 | 7.6:1 | `red` |
| Night | `reverse video on fg-strong` | `#f6f2e6` | 17.4:1 | 17.4:1 | `fg-strong` |
| Night | `line-strong (stroke)` | `#6b78a0` | 4.5:1 | 4.5:1 | `bg` |
| Print | `fg` | `#23272e` | 13.2:1 | 10.6:1 | `bg-active` |
| Print | `fg-strong` | `#0e1114` | 16.7:1 | 13.4:1 | `bg-active` |
| Print | `fg-muted` | `#4a4f55` | 7.3:1 | 5.8:1 | `bg-active` |
| Print | `fg-faint` | `#5a5f63` | 5.7:1 | 4.6:1 | `bg-active` |
| Print | `accent` | `#264f9c` | 6.9:1 | 5.5:1 | `bg-active` |
| Print | `accent-strong` | `#1c3d7c` | 9.2:1 | 7.4:1 | `bg-active` |
| Print | `green` | `#1f6338` | 6.4:1 | 5.1:1 | `bg-active` |
| Print | `yellow` | `#6e4f00` | 6.7:1 | 5.3:1 | `bg-active` |
| Print | `red` | `#a32a22` | 6.4:1 | 5.1:1 | `bg-active` |
| Print | `purple` | `#5f3fa0` | 6.9:1 | 5.5:1 | `bg-active` |
| Print | `cyan` | `#0d5c64` | 6.8:1 | 5.4:1 | `bg-active` |
| Print | `g-virtualisation` | `#91305f` | 6.6:1 | 5.3:1 | `bg-active` |
| Print | `g-hardware` | `#84400f` | 6.8:1 | 5.4:1 | `bg-active` |
| Print | `reverse video on accent` | `#264f9c` | 7.4:1 | 7.4:1 | `accent` |
| Print | `reverse video on accent-strong` | `#1c3d7c` | 9.8:1 | 9.8:1 | `accent-strong` |
| Print | `reverse video on red` | `#a32a22` | 6.8:1 | 6.8:1 | `red` |
| Print | `reverse video on fg-strong` | `#0e1114` | 16.7:1 | 16.7:1 | `fg-strong` |
| Print | `line-strong (stroke)` | `#666c62` | 4.8:1 | 4.8:1 | `bg` |
<!-- contrast:end -->

Rules of use:

- **Accent** means "you are here" or "you can act": focus, selection, links, the primary button.
  It is never decoration.
- **Green, yellow, red** are states only: ok / set here / added, warning / behind, failed /
  removed. A state always carries a glyph too (`●` `▲` `✖`), never colour alone.
- **Group colours** (services cyan, programs purple, virtualisation pink, hardware orange) tint
  chips and section headings, as today.
- **Selection is reverse video**: accent background, `--on-accent` text. Hover on a row is a
  `--bg-hover` fill; hover on a control is reverse video.

## Type

| Voice | Where | How |
|---|---|---|
| Pixel face | page titles, machine names, tile numbers, intro headlines | `A.big(text)`: 5 pixel rows in 3 text rows of `▀▄█`, uppercase, with `--glow` at night |
| Section heading | `h2` | bold, uppercase, after an accent `▌` |
| Strong | `h3`, values, names, table keys | bold, `--fg-strong` |
| Body | everything else | 400, `--fg`, 14 px on a 17 px row |
| Muted / faint | labels / hints, placeholders | `--fg-muted` / `--fg-faint` |

There is one font size. Hierarchy comes from the voices above, from space (blank rows) and from
frames, never from a second size.

**Font.** The prototype ships DejaVu Sans Mono (regular and bold), subset to the ranges the
console uses, 73 KB each as WOFF2, self-hosted, which the CSP requires. It is the reference
because its box-drawing glyphs join vertically at a 17 px row for a 14 px size (line-height
1.214) and it covers every glyph below; `tools/check.py` proves the second part. It has no
braille, so no spinner or chart uses braille. The current Atkinson Hyperlegible Mono has not been
checked for box drawing and blocks; a replacement must pass `tools/check.py` and render the
"Coverage test" panel of `#/system` with no ragged line. Changing the face is a one-line change
in `tokens.css`.

## Glyph language

**Frames.** Drawn by `A.scan()` into a `<pre class="fr" aria-hidden="true">` laid over any
element with `data-frame`. Title, right-hand note and foot sit *on* the frame line
(`data-title`, `data-tag`, `data-foot`); `data-tone` colours it; `data-shadow` adds a `░` shadow.

| Frame | Glyphs | Means |
|---|---|---|
| round | `╭─╮ │ ╰─╯` | a card or panel |
| square | `┌─┐ │ └─┘` | data: tables, diffs, menus, editor |
| double | `╔═╗ ║ ╚═╝` | a modal dialog; one on screen at a time, always with a `░` shadow |
| heavy | `┏━┓ ┃ ┗━┛` | hovered, selected, or a drawer |
| dash | `┌┄┐ ┆ └┄┘` | not there yet: an add form, an empty slot, a disabled button |

**Rules.** `────` separates, `┄┄┄┄` separates weakly, `━━━` under a tab marks the active one,
`▌` starts a section heading.

**Status.** `●` ok, `◐◓◑◒` working (spins), `▲` warning, `✖` failed, `○` silent or unknown,
`◆` suggestion. Source of a value: `✎ Set here`, `λ In code · file`, `· Default`.

**Controls.**

| Control | Rest | Hover / focus | On / active | Busy | Disabled |
|---|---|---|---|---|---|
| Button | `[ Push ]` | reverse video; focus `▸ Push ◂` | pressed `  [Push]  ` | `[ ┘ Pushing… ]` | `┆ Push ┆` faint |
| Primary | `▐ Commit ▌` reverse | brighter fill | | | as button |
| Danger | `[! Revert ]` red | red reverse | | | |
| Checkbox | `[ ]` | row fill | `[x]` | | faint |
| Radio | `( )` | row fill | `(•)` | | faint |
| Switch | `○──` | row fill | `──●` green | `─◐─` | faint |
| Segmented | `‹ a │ b ›` | cell fill | pick in reverse video | | |
| Field | filled cells after a label or `λ ›` | brighter fill, block caret | | | |
| Tab | muted word | strong | bold on `━━━` | | |
| Row | text | `--bg-hover` fill | reverse video | | |

**Meters.** `█████▍░░░░` (eighth-blocks for the fraction), sparkline `▁▂▃▄▅▆▇█`, skeleton
`░░░▒▓▒░░░`, spinners `┤┘┴└├┌┬┐` (tee), `◜◝◞◟` (arc) and `◐◓◑◒` (dot), steps `✓ Name ──── Role ──── Platform`.

**Icons.** The 61 names of `Icon.svelte` map to one or two cells each (`U.icons` in
`prototype/js/ui.js`, shown in `#/system`): fleet `◈`, machines `▣`, packages `◫`, services `≋`,
options `≔`, health `♥`, deployments `⇉`, inputs `↧`, changes `±`, history `↺`, code `λ`,
settings `⚙`, branch `⌥`, pull `⇣`, push `⇡`, search `⌕`, key `¤`, mark `❄`, and so on.

## Layout

- **Shell.** A framed rail (26 cells) with the views and their `g` shortcuts, a framed top bar,
  the view, and a one-row **mode line** at the bottom: repository, branch, health counts, a
  ticker of what the server last did, key hints, clock. The mode line is new; everything else is
  where it was.
- **Pages.** Pixel title, one muted line, tools at the right; then tiles, sections under `▌`
  headings, tables.
- **Fleet.** Nodes are framed elements placed by cell on a dot grid; wires are box-drawing text
  routed at right angles that join themselves where they meet (`A.Buf.path`).
- **Overlays.** The scrim is a `░` dither over a dimmed screen. Dialog, palette, package search
  and wizard are double frames; menus are square; the drawer and the inspector are heavy.

## Motion

All of it is text, steps on the clock, and stops on its last frame under
`prefers-reduced-motion` (or `?still`).

| Paper console | Cellwork | Function |
|---|---|---|
| view `fade-in`, `rise`, `.stagger` | **Repaint sweep**: dots, then base-32, then the text, top to bottom, about 0.5 s | `A.sweep(root)` |
| `pop-in`, `dialog-in`, `pal-in`, `drop` | **Stroke-in**: the frame is drawn from its top-left corner in 5 ticks | `A.scan(root, true)` |
| crumb `slide-in`, toast `fly` | **Typed**: 2 to 3 characters per tick; a toast's bottom edge is a fuse that burns down to its dismissal | `A.type` |
| `countUp` | **Odometer** in the pixel face | `A.run` + `A.big` |
| `spin`, boot `turn` | **Tee spinner** `┤┘┴└├┌┬┐`; the boot snowflake is a shaded field of blocks that turns | `.spin`, `snowflake()` |
| `ping`, `pulse`, boot `halo` | **Dot cycle** `·∙•●`; a ring of dots leaves the boot mark | `.spin[data-k]` |
| `shimmer` | **Dither wave** `░▒▓▒░` | `.skel` |
| `sheen` | **Leading edge** `▓▒░` through the filled part of a meter | `.bar[data-live]` |
| tab ink slide | **Heavy rule** slides under the tabs, cell by cell | `A.ink` |
| `.lift` hover | **Frame weight**: round to heavy, in accent | `.lift` |
| `edge-flow` | **Packets** `•∙∙∙` run the wires; hovered wires light up and carry trains | `fleet.js` |
| `bump` | **Flash**: reverse video for 2 ticks | `.flash` |
| Inspector / Drawer `fly` | heavy frame stroked in, content swept | `U.drawer` |
| Intro: WebGL planet, tiles, arcs, orbits, pulses, starfield | **Text scenes**: a lambert-shaded sphere of package dots on a luminance ramp, packages that scatter and lock back, great-circle arcs with packets, orbits with machine labels, commit pulses, twinkling stars and meteors | `intro.js` |
| Intro: SVG blueprint | the same five acts drafted in box drawing: draw, scan, fail, patch, stamp | `blueprint()` |

## Accessibility

- Text colours hold 4.5:1 on every background in both themes (table above; `#/system` measures
  them live).
- Frames, dithers and scenes are `aria-hidden` decoration over real elements: buttons are
  `<button>`, switches are checkboxes with `role="switch"`, dialogs are `role="dialog"`.
- State is never colour alone: each has its glyph and its word.
- Focus is unmistakable: reverse video, plus `▸ ◂` on buttons.
- Reduced motion freezes every animation on its final frame; nothing depends on having seen it.

## Screens in the prototype

Intro (9 chapters), login, boot, fleet (hover, inspector, search), machines, machine page (all 6
tabs, broken and unknown machine), packages and services matrices, options, health, deployments,
inputs (with a running job and the scheduled update), changes, history, code, settings, and the
overlays: palette, package search, new-machine wizard (4 steps), dialogs (confirm, danger,
prompt), toasts (ok, error, info, busy), service catalog drawer, branch / user / model menus.
`#/states` has loading, error, empty and all-clear.

## Bringing it into the app

The prototype is plain CSS and classic scripts so each piece can be lifted on its own.

1. **Tokens.** Replace the `:root` block of `web/src/app.css` with `tokens.css` (same names).
   Self-host the font as today's fonts are. `index.html` already says `color-scheme: dark`.
2. **Engine.** `prototype/js/ascii.js` has no dependencies and is CSP-safe (no inline script, no
   `blob:`). It can become `web/src/lib/ascii.ts` nearly as is; `A.scan` maps to a Svelte action
   (`use:frame`), `A.sweep` runs where `.content > *` fades in today, and the clock should be the
   only `setInterval`.
3. **Components.** `system.css` restyles by element and class (`.btn`, `.chip`, `.tabs`, `.rows`,
   `th`); `Switch.svelte`, `Dialog`, `Toasts`, `Drawer`, `Icon` (glyph map instead of paths) and
   `SourceTag` are the ones to rewrite.
4. **Third-party canvases.** CodeMirror only needs a theme (it is already a monospace grid).
   Svelte Flow keeps doing pan, zoom and ELK layout: nodes take the frame action; edges can stay
   SVG step paths with a dotted stroke at first, and move to the text router (`A.Buf.path`) once
   positions are snapped to cells. Zoom should step the font size rather than scale.
5. **Intro.** `three`, `lenis` and the four `.webp` screenshots can go; `intro.js` replaces
   `intro/three/*`, `Blueprint.svelte` and `Showcase.svelte`, keeping `story.ts`'s chapters.

Known limits of the prototype: it is drawn for 1280 px and wider (the narrow-window rail collapse
is not drawn), the fleet layout is fixed rather than computed, and forms do not validate.
