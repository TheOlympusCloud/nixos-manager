// @vitest-environment happy-dom
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'

let ascii: typeof import('./ascii')

beforeAll(async () => {
  vi.useFakeTimers() // the clock's interval must not tick on its own: each test steps it
  // happy-dom lays nothing out; sweep only repaints text that is on screen.
  HTMLElement.prototype.getClientRects = () => [{}] as unknown as DOMRectList
  ascii = await import('./ascii')
})
afterAll(() => vi.useRealTimers())

function mount(html: string) {
  const root = document.createElement('div')
  root.innerHTML = html
  document.body.append(root)
  return root
}

describe('sweep', () => {
  it('repaints text through dots and base-32 back to itself', () => {
    const text = mount('<p>hello world</p>').querySelector('p')!.firstChild as Text
    ascii.sweep(text.parentElement!.parentElement!)
    expect(text.data).toBe('····· ·····')
    for (let i = 0; i < 3; i++) ascii.step()
    expect(text.data).toBe('hello world')
  })

  it('lets text written meanwhile win, as Svelte writes it', () => {
    const root = mount('<p>3 machines</p>')
    const text = root.querySelector('p')!.firstChild as Text
    ascii.sweep(root)
    ascii.step()
    text.data = '4 machines'
    for (let i = 0; i < 5; i++) ascii.step()
    expect(text.data).toBe('4 machines')
  })

  it('leaves live regions alone', () => {
    const root = mount('<p role="status">Saving…</p><p aria-live="polite">2 errors</p>')
    ascii.sweep(root)
    expect([...root.querySelectorAll('p')].map((p) => p.textContent)).toEqual(['Saving…', '2 errors'])
  })
})
