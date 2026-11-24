#!/usr/bin/env python3
"""Checks the two promises the prototype makes, and fails if either is broken:

1. Every non-ASCII character the prototype can print is in the shipped font (regular and bold),
   so no glyph ever comes from a fallback font of another width.
2. Every text colour holds 4.5:1 on every background of its theme, and reverse video does too.

It also rewrites the contrast table in ../README.md. Needs Python 3 and fontTools (with brotli).

    python3 tools/check.py

With --app it checks the console itself instead: the font in web/src/fonts, every character the
code in web/src can print (comments and tests left out), and the tokens in web/src/app.css. The
README is left alone.

    python3 tools/check.py --app
"""
import glob
import os
import re
import sys

from fontTools.ttLib import TTFont

HERE = os.path.dirname(os.path.abspath(__file__))
PROTO = os.path.join(HERE, '..', 'prototype')
README = os.path.join(HERE, '..', 'README.md')
APP = '--app' in sys.argv[1:]
WEB = os.path.join(HERE, '..', '..', '..', 'web', 'src')
failed = False


def fail(message):
    global failed
    failed = True
    print('FAIL', message)


# ---- 1. Glyph coverage ---------------------------------------------------------------------------
FONTS = os.path.join(WEB, 'fonts') if APP else os.path.join(PROTO, 'fonts')
fonts = {name: set(TTFont(os.path.join(FONTS, name)).getBestCmap()) for name in ('DejaVuSansMono.subset.woff2', 'DejaVuSansMono-Bold.subset.woff2')}
if APP:
    sources = [p for p in glob.glob(os.path.join(WEB, '**', '*'), recursive=True) if re.search(r'\.(svelte|ts|css|html)$', p) and not p.endswith('.test.ts')]
else:
    sources = glob.glob(os.path.join(PROTO, 'js', '*.js')) + glob.glob(os.path.join(PROTO, 'css', '*.css')) + [os.path.join(PROTO, 'index.html')]


def printable(text):
    """The text without its comments: a comment never reaches the screen."""
    if not APP:
        return text
    text = re.sub(r'/\*.*?\*/|<!--.*?-->', '', text, flags=re.S)
    return re.sub(r'(^|\s)//.*$', r'\1', text, flags=re.M)


used = {}
for path in sources:
    with open(path, encoding='utf8') as f:
        for ch in printable(f.read()):
            if ord(ch) > 126:
                used.setdefault(ch, set()).add(os.path.relpath(path, WEB) if APP else os.path.basename(path))
for name, cmap in fonts.items():
    missing = sorted(ch for ch in used if ord(ch) not in cmap)
    if missing:
        fail(f'{name} lacks {" ".join(f"{c} U+{ord(c):04X} ({", ".join(sorted(used[c]))})" for c in missing)}')
print(f'glyphs: {len(used)} non-ASCII characters used, all present in {len(fonts)} font files' if not failed else 'glyphs: see above')

# ---- 2. Contrast ---------------------------------------------------------------------------------
with open(os.path.join(WEB, 'app.css') if APP else os.path.join(PROTO, 'css', 'tokens.css'), encoding='utf8') as f:
    css = f.read()
blocks = {'Night': re.search(r':root \{(.*?)\n\}', css, re.S).group(1), 'Print': re.search(r"\[data-theme='print'\] \{(.*?)\n\}", css, re.S).group(1)}


def tokens(block):
    return {m.group(1): m.group(2) for m in re.finditer(r'--([\w-]+):\s*(#[0-9a-fA-F]{6})\b', block)}


def rgb(hex_):
    return tuple(int(hex_[i:i + 2], 16) for i in (1, 3, 5))


def lum(c):
    def ch(v):
        v /= 255
        return v / 12.92 if v <= 0.03928 else ((v + 0.055) / 1.055) ** 2.4
    r, g, b = (ch(v) for v in c)
    return 0.2126 * r + 0.7152 * g + 0.0722 * b


def ratio(a, b):
    hi, lo = sorted((lum(a), lum(b)), reverse=True)
    return (hi + 0.05) / (lo + 0.05)


def over(top, alpha, base):
    return tuple(round(t * alpha + b * (1 - alpha)) for t, b in zip(top, base))


TEXT = ['fg', 'fg-strong', 'fg-muted', 'fg-faint', 'accent', 'accent-strong', 'green', 'yellow', 'red', 'purple', 'cyan', 'g-virtualisation', 'g-hardware']
GROUNDS = ['bg', 'bg-raised', 'bg-input', 'bg-hover', 'bg-active']
rows = []
for theme, block in blocks.items():
    t = tokens(block)
    grounds = {g: rgb(t[g]) for g in GROUNDS}
    if theme == 'Print':
        grounds['greenbar'] = over((48, 128, 78), 0.075, rgb(t['bg']))
    for name in TEXT:
        on_bg = ratio(rgb(t[name]), grounds['bg'])
        worst_ground, worst = min(((g, ratio(rgb(t[name]), c)) for g, c in grounds.items()), key=lambda x: x[1])
        if worst < 4.5:
            fail(f'{theme} --{name} {t[name]} is {worst:.2f}:1 on {worst_ground}')
        rows.append((theme, name, t[name], on_bg, worst, worst_ground))
    for fill in ('accent', 'accent-strong', 'red', 'fg-strong'):
        r = ratio(rgb(t['on-accent' if fill != 'fg-strong' else 'bg']), rgb(t[fill]))
        if r < 4.5:
            fail(f'{theme} reverse video on --{fill} is {r:.2f}:1')
        rows.append((theme, f'reverse video on {fill}', t[fill], r, r, fill))
    for stroke in ('line-strong',):
        r = ratio(rgb(t[stroke]), grounds['bg'])
        if r < 3:
            fail(f'{theme} --{stroke} is {r:.2f}:1 on bg (strokes need 3:1)')
        rows.append((theme, f'{stroke} (stroke)', t[stroke], r, r, 'bg'))

table = ['| Theme | Token | Value | On `--bg` | Lowest | On |', '|---|---|---|---|---|---|']
table += [f'| {th} | `{n}` | `{v}` | {a:.1f}:1 | {w:.1f}:1 | `{g}` |' for th, n, v, a, w, g in rows]
print(f'contrast: {len(rows)} pairs checked, lowest {min(r[4] for r in rows):.2f}:1')
if APP:
    sys.exit(1 if failed else 0)
with open(README, encoding='utf8') as f:
    doc = f.read()
doc = re.sub(r'(<!-- contrast:begin -->\n).*?(<!-- contrast:end -->)', lambda m: m.group(1) + '\n'.join(table) + '\n' + m.group(2), doc, flags=re.S)
with open(README, 'w', encoding='utf8') as f:
    f.write(doc)
sys.exit(1 if failed else 0)
