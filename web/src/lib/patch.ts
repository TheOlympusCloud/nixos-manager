// A git diff split into files and hunks, without the plumbing lines (index, modes, ---/+++).

export type FileStatus = 'added' | 'deleted' | 'modified' | 'renamed'
export type PatchLine = { kind: 'add' | 'del' | 'ctx' | 'note'; text: string; old?: number; new?: number }
export type Hunk = { start: number; context: string; lines: PatchLine[] }
export type PatchFile = { path: string; from?: string; status: FileStatus; binary: boolean; hunks: Hunk[]; adds: number; dels: number }

export const statusWord: Record<FileStatus, string> = { added: 'New', deleted: 'Deleted', modified: 'Edited', renamed: 'Renamed' }

/** `"a/x y"` or `a/x` → `x` (git quotes paths with unusual characters). */
function clean(p: string): string {
  let s = p.trim()
  if (s.startsWith('"') && s.endsWith('"')) s = s.slice(1, -1).replace(/\\(.)/g, '$1')
  return s.replace(/^[ab]\//, '')
}

export function parsePatch(text: string): PatchFile[] {
  const files: PatchFile[] = []
  let f: PatchFile | null = null
  let h: Hunk | null = null
  let oldNo = 0
  let newNo = 0
  for (const line of text.split('\n')) {
    if (line.startsWith('diff --git ')) {
      const m = / b\/(.*)$/.exec(line) ?? / "b\/(.*)"$/.exec(line)
      f = { path: m ? clean(m[1]) : line.slice(11), status: 'modified', binary: false, hunks: [], adds: 0, dels: 0 }
      files.push(f)
      h = null
      continue
    }
    if (!f) continue
    if (!h || !/^[-+ \\]/.test(line)) {
      // Header lines of the file (before the first hunk, or between hunks).
      if (line.startsWith('new file mode')) f.status = 'added'
      else if (line.startsWith('deleted file mode')) f.status = 'deleted'
      else if (line.startsWith('rename from ')) ((f.from = clean(line.slice(12))), (f.status = 'renamed'))
      else if (line.startsWith('rename to ')) f.path = clean(line.slice(10))
      else if (line.startsWith('+++ ') && line !== '+++ /dev/null') f.path = clean(line.slice(4))
      else if (line.startsWith('Binary files') || line === 'GIT binary patch') f.binary = true
      else if (line.startsWith('@@')) {
        const m = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@ ?(.*)$/.exec(line)
        oldNo = m ? +m[1] : 0
        newNo = m ? +m[2] : 0
        h = { start: newNo, context: m?.[3] ?? '', lines: [] }
        f.hunks.push(h)
      }
      continue
    }
    const c = line[0]
    if (c === '+') {
      h.lines.push({ kind: 'add', text: line.slice(1), new: newNo++ })
      f.adds++
    } else if (c === '-') {
      h.lines.push({ kind: 'del', text: line.slice(1), old: oldNo++ })
      f.dels++
    } else if (c === ' ') h.lines.push({ kind: 'ctx', text: line.slice(1), old: oldNo++, new: newNo++ })
    else h.lines.push({ kind: 'note', text: line.slice(1).trim() })
  }
  return files
}

export const patchLines = (f: PatchFile) => f.hunks.reduce((n, h) => n + h.lines.length, 0)
