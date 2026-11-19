import { describe, expect, it } from 'vitest'
import { parsePatch, patchLines } from './patch'

const text = `diff --git a/README.md b/README.md
new file mode 100644
index 0000000..e2c07b6
--- /dev/null
+++ b/README.md
@@ -0,0 +1,2 @@
+# Fleet
+
diff --git a/hosts/pi/default.nix b/hosts/pi/default.nix
index 1111111..2222222 100644
--- a/hosts/pi/default.nix
+++ b/hosts/pi/default.nix
@@ -10,4 +10,4 @@ in
 {
--- not a header
+  services.jellyfin.enable = true;
 }
\\ No newline at end of file
diff --git a/old.nix b/new.nix
similarity index 90%
rename from old.nix
rename to new.nix
diff --git a/gone.nix b/gone.nix
deleted file mode 100644
index 3333333..0000000
--- a/gone.nix
+++ /dev/null
@@ -1 +0,0 @@
-x
diff --git a/logo.png b/logo.png
Binary files a/logo.png and b/logo.png differ
`

describe('parsePatch', () => {
  const files = parsePatch(text)
  it('splits files and drops the plumbing lines', () => {
    expect(files.map((f) => [f.path, f.status])).toEqual([
      ['README.md', 'added'],
      ['hosts/pi/default.nix', 'modified'],
      ['new.nix', 'renamed'],
      ['gone.nix', 'deleted'],
      ['logo.png', 'modified'],
    ])
    expect(files[2].from).toBe('old.nix')
    expect(files[4].binary).toBe(true)
    expect(files[0].hunks[0].lines.map((l) => l.text)).toEqual(['# Fleet', ''])
  })

  it('numbers lines and counts additions and removals', () => {
    const pi = files[1]
    expect(pi.hunks[0].start).toBe(10)
    expect(pi.hunks[0].context).toBe('in')
    expect(pi.hunks[0].lines).toEqual([
      { kind: 'ctx', text: '{', old: 10, new: 10 },
      { kind: 'del', text: '-- not a header', old: 11 },
      { kind: 'add', text: '  services.jellyfin.enable = true;', new: 11 },
      { kind: 'ctx', text: '}', old: 12, new: 12 },
      { kind: 'note', text: 'No newline at end of file' },
    ])
    expect([pi.adds, pi.dels, patchLines(pi)]).toEqual([1, 1, 5])
  })

  it('handles empty input', () => {
    expect(parsePatch('')).toEqual([])
  })
})
