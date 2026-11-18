export type DiffKind = 'file' | 'meta' | 'hunk' | 'add' | 'del' | 'ctx'
export type DiffLine = { kind: DiffKind; text: string }

/**
 * Classify unified-diff lines for coloring. Header lines ("--- a/x") only count as
 * headers outside a hunk, so a removed line that starts with "--" stays a removal.
 */
export function classifyDiff(text: string): DiffLine[] {
  const lines = text.split('\n')
  if (lines.at(-1) === '') lines.pop()
  let inHunk = false
  return lines.map((line) => {
    let kind: DiffKind
    if (line.startsWith('diff ')) {
      kind = 'file'
      inHunk = false
    } else if (line.startsWith('@@')) {
      kind = 'hunk'
      inHunk = true
    } else if (!inHunk) kind = 'meta'
    else if (line.startsWith('+')) kind = 'add'
    else if (line.startsWith('-')) kind = 'del'
    else if (line.startsWith(' ') || line === '') kind = 'ctx'
    else kind = 'meta' // "\ No newline at end of file"
    return { kind, text: line }
  })
}
