// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest'
import { sanitizeHTML } from './lsp.svelte'

describe('sanitizeHTML', () => {
  it('keeps plain formatting, text and safe links', () => {
    const out = sanitizeHTML('<p>Use <code class="tok">services.x</code> &amp; <a href="https://search.nixos.org/options">docs</a></p><pre>a &lt; b</pre>')
    expect(out).toBe('<p>Use <code class="tok">services.x</code> &amp; <a href="https://search.nixos.org/options">docs</a></p><pre>a &lt; b</pre>')
  })

  it('drops scripts, handlers, foreign content and unsafe link targets', () => {
    const out = sanitizeHTML(
      '<img src=x onerror="alert(1)"><script>alert(2)</script><style>*{}</style>' +
        '<svg><a href="javascript:alert(3)">s</a></svg><math><mi>m</mi></math>' +
        '<a href="javascript:alert(4)">j</a><a href="java&#10;script:alert(5)">n</a><a href="data:text/html,x">d</a>' +
        '<p onclick="x" style="color:red" title="t">kept</p>',
    )
    expect(out).not.toMatch(/script|onerror|onclick|alert|style|svg|math|data:|<img/i)
    expect(out).toContain('<a>j</a>')
    expect(out).toContain('<p>kept</p>')
  })

  it('unwraps unknown tags but keeps their text', () => {
    expect(sanitizeHTML('<details><summary>More</summary>text</details>')).toBe('Moretext')
  })
})
