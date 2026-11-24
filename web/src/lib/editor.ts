import { basicSetup } from 'codemirror'
import { EditorState, Prec, type Extension } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { lintGutter } from '@codemirror/lint'
import { tags as t } from '@lezer/highlight'
import { nix } from '@replit/codemirror-lang-nix'
import { documentUri, lspClient } from './lsp.svelte'

export const isNix = (path: string) => path.endsWith('.nix')

// Code in cells: the console's own inks on its grid. One font size, one row height, no rounded or shadowed
// boxes; tooltips, panels and gutters are filled cells, the cursor a block in --accent, the selection reverse video.
const theme = EditorView.theme({
  '&': { height: '100%', color: 'var(--fg)', backgroundColor: 'var(--bg)' },
  '.cm-scroller': { fontFamily: 'var(--mono)', lineHeight: 'var(--lh)' },
  // The console reverse-videos :focus-visible, which a focused contenteditable always matches: not here.
  '.cm-content': { padding: '0', color: 'var(--fg)', backgroundColor: 'transparent', caretColor: 'var(--accent)' },
  '.cm-line': { padding: '0 1ch' },
  // A still block (no blink: motion is the console's one clock). Difference keeps the letter under it legible.
  '.cm-cursorLayer': { mixBlendMode: 'difference' },
  '&.cm-focused > .cm-scroller > .cm-cursorLayer': { animation: 'none' },
  '.cm-cursor, .cm-dropCursor': { width: '1ch', marginLeft: '0', borderLeft: '0', backgroundColor: 'var(--accent)' },
  '.cm-selectionBackground': { background: 'var(--bg-active)' },
  '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground': { background: 'var(--accent)' },
  '&.cm-focused .cm-line ::selection, &.cm-focused .cm-line::selection': { color: 'var(--on-accent)' },
  '.cm-gutters': { color: 'var(--fg-faint)', backgroundColor: 'var(--bg)', border: '0' },
  '.cm-lineNumbers .cm-gutterElement': { minWidth: '4ch', padding: '0 1ch 0 1ch' },
  '.cm-foldGutter .cm-gutterElement': { padding: '0 1ch 0 0', color: 'var(--fg-muted)' },
  '.cm-activeLine': { backgroundColor: 'transparent' },
  '.cm-activeLineGutter': { color: 'var(--fg-strong)', backgroundColor: 'transparent' },
  '.cm-searchMatch': { backgroundColor: 'color-mix(in srgb, var(--mark) 30%, transparent)' },
  '.cm-searchMatch.cm-searchMatch-selected': { color: 'var(--on-accent)', backgroundColor: 'var(--mark)' },
  '.cm-selectionMatch': { backgroundColor: 'var(--bg-active)' },
  '&.cm-focused .cm-matchingBracket': { color: 'var(--fg-strong)', backgroundColor: 'var(--bg-active)', outline: '0' },
  '&.cm-focused .cm-nonmatchingBracket': { color: 'var(--red)', backgroundColor: 'color-mix(in srgb, var(--red) 16%, transparent)', outline: '0' },
  '.cm-foldPlaceholder': { margin: '0', padding: '0 1ch', color: 'var(--fg-muted)', backgroundColor: 'var(--bg-hover)', border: '0', borderRadius: '0' },
  // Tooltips, hover help and completion: a raised fill of whole cells.
  '.cm-tooltip': { color: 'var(--fg)', backgroundColor: 'var(--bg-raised)', border: '0', borderRadius: '0', boxShadow: 'none' },
  '.cm-tooltip-section:not(:first-child)': { borderTop: '0' },
  '.cm-tooltip.cm-tooltip-autocomplete > ul': { fontFamily: 'var(--mono)', maxHeight: 'calc(var(--lh) * 10)' },
  '.cm-tooltip.cm-tooltip-autocomplete > ul > li': { padding: '0 1ch', lineHeight: 'var(--lh)' },
  '.cm-tooltip-autocomplete ul li[aria-selected]': { color: 'var(--on-accent)', backgroundColor: 'var(--accent)' },
  '.cm-completionIcon': { width: '2ch', padding: '0', fontSize: 'inherit', opacity: '1' },
  '.cm-completionDetail': { color: 'var(--fg-muted)', fontStyle: 'normal' },
  '.cm-completionMatchedText': { color: 'var(--accent-strong)', textDecoration: 'none', fontWeight: '700' },
  '.cm-completionInfo': { padding: '0 1ch' },
  '.cm-lsp-documentation': { padding: '0 1ch' },
  '.cm-lsp-signature-tooltip': { padding: '0 1ch', borderRadius: '0' },
  '.cm-lsp-signature-tooltip .cm-lsp-documentation': { fontSize: 'inherit' },
  // Diagnostics: a glyph and a word, and a fill under the range instead of a wavy line.
  '.cm-diagnostic': { padding: '0 1ch', marginLeft: '0' },
  '.cm-diagnostic-error': { borderLeft: '0', color: 'var(--red)' },
  '.cm-diagnostic-warning': { borderLeft: '0', color: 'var(--yellow)' },
  '.cm-diagnostic-info, .cm-diagnostic-hint': { borderLeft: '0', color: 'var(--blue)' },
  '.cm-diagnostic-error::before': { content: '"✖ "' },
  '.cm-diagnostic-warning::before': { content: '"▲ "' },
  '.cm-diagnostic-info::before, .cm-diagnostic-hint::before': { content: '"◆ "' },
  '.cm-diagnosticSource': { fontSize: 'inherit', color: 'var(--fg-muted)', opacity: '1' },
  '.cm-diagnosticAction': { padding: '0 1ch', marginLeft: '1ch', color: 'var(--on-accent)', backgroundColor: 'var(--accent)', borderRadius: '0' },
  '.cm-lintRange-error': { backgroundImage: 'none', backgroundColor: 'color-mix(in srgb, var(--red) 22%, transparent)' },
  '.cm-lintRange-warning': { backgroundImage: 'none', backgroundColor: 'color-mix(in srgb, var(--yellow) 22%, transparent)' },
  '.cm-lintRange-info, .cm-lintRange-hint': { backgroundImage: 'none', backgroundColor: 'color-mix(in srgb, var(--blue) 18%, transparent)' },
  '.cm-lintRange': { paddingBottom: '0' },
  '.cm-lintPoint::after': { content: 'none' },
  '.cm-gutter-lint': { width: '2ch' },
  '.cm-gutter-lint .cm-gutterElement': { padding: '0' },
  '.cm-lint-marker': { width: '1ch', height: 'var(--lh)', content: 'normal' },
  '.cm-lint-marker-error::before': { content: '"✖"', color: 'var(--red)' },
  '.cm-lint-marker-warning::before': { content: '"▲"', color: 'var(--yellow)' },
  '.cm-lint-marker-info::before': { content: '"◆"', color: 'var(--blue)' },
  // Search and rename panels: a raised band; their buttons and fields are the console's own.
  '.cm-panels': { color: 'var(--fg)', backgroundColor: 'var(--bg-raised)' },
  '.cm-panels.cm-panels-top': { borderBottom: '0' },
  '.cm-panels.cm-panels-bottom': { borderTop: '0' },
  '.cm-panel.cm-search, .cm-panel.cm-lsp-rename-panel': { padding: '0 1ch' },
  '.cm-panel.cm-search input, .cm-panel.cm-search button, .cm-panel.cm-search label': { margin: '0 1ch 0 0' },
  '.cm-panel.cm-search label, .cm-panel.cm-lsp-rename-panel label': { fontSize: 'inherit' },
  '.cm-button': { padding: '0', fontSize: 'inherit', backgroundImage: 'none', border: '0', borderRadius: '0' },
  '.cm-textfield': { width: 'auto', padding: '0 1ch', fontSize: 'inherit', color: 'var(--fg-strong)', backgroundColor: 'var(--bg-input)', border: '0' },
  '.cm-textfield:focus': { backgroundColor: 'var(--bg-active)' },
  // Compare (MergeView): whole-cell washes, as in the diffs.
  '&.cm-merge-a .cm-changedLine, .cm-deletedChunk': { backgroundColor: 'color-mix(in srgb, var(--red) 13%, transparent)' },
  '&.cm-merge-b .cm-changedLine, .cm-inlineChangedLine': { backgroundColor: 'color-mix(in srgb, var(--green) 13%, transparent)' },
  '&.cm-merge-a .cm-changedText, .cm-deletedChunk .cm-deletedText': { background: 'color-mix(in srgb, var(--red) 30%, transparent)' },
  '&.cm-merge-b .cm-changedText': { background: 'color-mix(in srgb, var(--green) 30%, transparent)' },
  // The change gutter is a sign column, as in a diff: − on the old side, + on the new one.
  '.cm-changeGutter': { width: '1ch', paddingLeft: '0' },
  '&.cm-merge-a .cm-changedLineGutter, .cm-deletedLineGutter': { color: 'var(--red)', background: 'none' },
  '&.cm-merge-b .cm-changedLineGutter': { color: 'var(--green)', background: 'none' },
  '.cm-inlineChangedLineGutter': { color: 'var(--purple)', background: 'none' },
  '&.cm-merge-a .cm-changedLineGutter::before, .cm-deletedLineGutter::before': { content: '"−"' },
  '&.cm-merge-b .cm-changedLineGutter::before': { content: '"+"' },
  '.cm-inlineChangedLineGutter::before': { content: '"±"' },
  '.cm-collapsedLines': { padding: '0 1ch', color: 'var(--fg-muted)', background: 'var(--bg-hover)' },
  '.cm-collapsedLines::before': { content: '"┄ "', marginInlineEnd: '0' },
  '.cm-collapsedLines::after': { content: '" ┄"', marginInlineStart: '0' },
})

const inks = HighlightStyle.define([
  { tag: [t.keyword, t.controlKeyword, t.definitionKeyword, t.moduleKeyword, t.operatorKeyword], color: 'var(--purple)' },
  { tag: [t.propertyName, t.attributeName, t.definition(t.propertyName)], color: 'var(--accent)' },
  { tag: [t.string, t.special(t.string), t.regexp], color: 'var(--green)' },
  { tag: [t.number, t.bool, t.null, t.atom, t.constant(t.name)], color: 'var(--yellow)' },
  { tag: [t.url, t.link, t.special(t.name)], color: 'var(--g-virtualisation)' },
  { tag: [t.function(t.variableName), t.function(t.propertyName), t.typeName, t.className], color: 'var(--cyan)' },
  { tag: [t.comment, t.lineComment, t.blockComment, t.docComment], color: 'var(--fg-faint)' },
  { tag: [t.operator, t.punctuation, t.bracket, t.separator], color: 'var(--fg-muted)' },
  { tag: t.invalid, color: 'var(--red)' },
  { tag: t.heading, fontWeight: '700', color: 'var(--fg-strong)' },
  { tag: t.emphasis, color: 'var(--fg-strong)' },
  { tag: t.strong, fontWeight: '700' },
])

/** Highlighting and theme shared by editors and read-only viewers. */
export const baseExtensions = (path: string): Extension[] => [basicSetup, Prec.high(theme), syntaxHighlighting(inks), isNix(path) ? nix() : []]

export const readOnly: Extension = [EditorState.readOnly.of(true), EditorView.editable.of(false)]

export function createEditor(opts: {
  parent: HTMLElement
  path: string
  content: string
  root: string
  onSave: () => void
  onFormat: () => void
  onChange: () => void
}): EditorView {
  const extensions: Extension[] = [
    Prec.highest(
      keymap.of([
        { key: 'Mod-s', run: () => (opts.onSave(), true), preventDefault: true },
        { key: 'Shift-Alt-f', run: () => (opts.onFormat(), true), preventDefault: true },
      ]),
    ),
    baseExtensions(opts.path),
    EditorView.updateListener.of((u) => u.docChanged && opts.onChange()),
  ]
  if (isNix(opts.path)) extensions.push(lintGutter(), lspClient(opts.root).plugin(documentUri(opts.root, opts.path), 'nix'))
  return new EditorView({ parent: opts.parent, doc: opts.content, extensions })
}
