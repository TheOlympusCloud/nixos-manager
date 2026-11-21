import { basicSetup } from 'codemirror'
import { EditorState, Prec, type Extension } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { lintGutter } from '@codemirror/lint'
import { tags as t } from '@lezer/highlight'
import { nix } from '@replit/codemirror-lang-nix'
import { documentUri, lspClient } from './lsp.svelte'

export const isNix = (path: string) => path.endsWith('.nix')

// Code on paper: the app's own inks and surfaces, so the editor follows the theme tokens.
const theme = EditorView.theme({
  '&': { height: '100%', fontSize: '13.5px', color: 'var(--fg)', backgroundColor: 'var(--bg-raised)' },
  '.cm-scroller': { fontFamily: 'var(--mono)', lineHeight: '1.55' },
  '.cm-content': { caretColor: 'var(--fg-strong)' },
  '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--fg-strong)' },
  '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
    backgroundColor: 'color-mix(in srgb, var(--mark) 55%, transparent)',
  },
  '.cm-gutters': { backgroundColor: 'var(--bg-raised)', borderRight: '1px solid var(--border)', color: 'var(--fg-faint)' },
  '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: 'color-mix(in srgb, var(--bg-hover) 55%, transparent)' },
  '.cm-searchMatch': { backgroundColor: 'color-mix(in srgb, var(--mark) 45%, transparent)', outline: '1px solid color-mix(in srgb, var(--yellow) 40%, transparent)' },
  '.cm-searchMatch.cm-searchMatch-selected': { backgroundColor: 'var(--mark)' },
  '.cm-selectionMatch': { backgroundColor: 'color-mix(in srgb, var(--accent) 12%, transparent)' },
  '&.cm-focused .cm-matchingBracket': { backgroundColor: 'color-mix(in srgb, var(--accent) 16%, transparent)', outline: '1px solid color-mix(in srgb, var(--accent) 40%, transparent)' },
  '&.cm-focused .cm-nonmatchingBracket': { backgroundColor: 'color-mix(in srgb, var(--red) 16%, transparent)' },
  '.cm-foldPlaceholder': { color: 'var(--fg-muted)', backgroundColor: 'var(--bg-hover)', border: '1px solid var(--line)' },
  '.cm-tooltip': { color: 'var(--fg)', backgroundColor: 'var(--bg-raised)', border: '1px solid var(--line)', borderRadius: 'var(--r)', boxShadow: 'var(--shadow-2)' },
  '.cm-tooltip-autocomplete > ul > li[aria-selected]': { color: 'var(--fg-strong)', backgroundColor: 'color-mix(in srgb, var(--accent) 14%, var(--bg-raised))' },
  '.cm-panels': { color: 'var(--fg)', backgroundColor: 'var(--bg-side)' },
  '.cm-panels.cm-panels-top': { borderBottom: '1px solid var(--line)' },
  '.cm-panels.cm-panels-bottom': { borderTop: '1px solid var(--line)' },
})

const inks = HighlightStyle.define([
  { tag: [t.keyword, t.controlKeyword, t.definitionKeyword, t.moduleKeyword, t.operatorKeyword], color: 'var(--purple)' },
  { tag: [t.propertyName, t.attributeName, t.definition(t.propertyName)], color: 'var(--accent)' },
  { tag: [t.string, t.special(t.string), t.regexp], color: 'var(--green)' },
  { tag: [t.number, t.bool, t.null, t.atom, t.constant(t.name)], color: 'var(--g-hardware)' },
  { tag: [t.url, t.link, t.special(t.name)], color: 'var(--g-virtualisation)' },
  { tag: [t.function(t.variableName), t.function(t.propertyName), t.typeName, t.className], color: 'var(--cyan)' },
  { tag: [t.comment, t.lineComment, t.blockComment, t.docComment], color: 'var(--fg-faint)', fontStyle: 'italic' },
  { tag: [t.operator, t.punctuation, t.bracket, t.separator], color: 'var(--fg-muted)' },
  { tag: t.invalid, color: 'var(--red)' },
  { tag: t.heading, fontWeight: '700', color: 'var(--fg-strong)' },
  { tag: t.emphasis, fontStyle: 'italic' },
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
