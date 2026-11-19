import { describe, expect, it } from 'vitest'
import { classifyDiff } from './diff'

describe('classifyDiff', () => {
  it('classifies headers, hunks, additions, removals and context', () => {
    const text = [
      'diff --git a/hosts/web.nix b/hosts/web.nix',
      'index 1111111..2222222 100644',
      '--- a/hosts/web.nix',
      '+++ b/hosts/web.nix',
      '@@ -1,4 +1,4 @@',
      ' {',
      '-  services.nginx.enable = false;',
      '+  services.nginx.enable = true;',
      '--- not a header, a removed line',
      '+++ not a header, an added line',
      '',
      '\\ No newline at end of file',
      'diff --git a/new.nix b/new.nix',
      'new file mode 100644',
      '--- /dev/null',
      '+++ b/new.nix',
      '@@ -0,0 +1 @@',
      '+{ }',
      '',
    ].join('\n')
    expect(classifyDiff(text).map((l) => l.kind)).toEqual([
      'file', 'meta', 'meta', 'meta', 'hunk', 'ctx', 'del', 'add', 'del', 'add', 'ctx', 'meta',
      'file', 'meta', 'meta', 'meta', 'hunk', 'add',
    ])
  })

  it('returns nothing for an empty diff', () => {
    expect(classifyDiff('')).toEqual([])
  })
})
