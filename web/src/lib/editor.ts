import { basicSetup } from 'codemirror'
import { EditorState, Prec, type Extension } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { lintGutter } from '@codemirror/lint'
import { oneDark } from '@codemirror/theme-one-dark'
import { nix } from '@replit/codemirror-lang-nix'
import { documentUri, lspClient } from './lsp.svelte'

export const isNix = (path: string) => path.endsWith('.nix')

// One Dark's syntax colours on the app's own surfaces.
const theme = EditorView.theme(
  {
    '&': { height: '100%', fontSize: '13.5px', backgroundColor: 'var(--bg)' },
    '.cm-scroller': { fontFamily: 'var(--mono)', lineHeight: '1.55' },
    '.cm-gutters': { backgroundColor: 'var(--bg)', borderRight: '1px solid var(--border)', color: 'var(--fg-faint)' },
    '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: 'color-mix(in srgb, var(--bg-hover) 70%, transparent)' },
  },
  { dark: true },
)

/** Highlighting and theme shared by editors and read-only viewers. */
export const baseExtensions = (path: string): Extension[] => [basicSetup, oneDark, Prec.high(theme), isNix(path) ? nix() : []]

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
