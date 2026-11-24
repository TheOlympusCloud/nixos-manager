import { describe, expect, it } from 'vitest'
import { homeLink, link, parseHash } from './router.svelte'

describe('routes', () => {
  it('parses views, parts and the query', () => {
    expect(parseHash('')).toEqual({ view: 'fleet', parts: [], query: {} })
    expect(parseHash('#/machines/workstation/packages')).toEqual({ view: 'machines', parts: ['workstation', 'packages'], query: {} })
    expect(parseHash('#/code/hosts/pi/default.nix').parts.join('/')).toBe('hosts/pi/default.nix')
    expect(parseHash('#/options/pi?q=services.openssh').query).toEqual({ q: 'services.openssh' })
    expect(parseHash('#/nonsense/x')).toEqual({ view: 'fleet', parts: [], query: {} })
  })

  it('round-trips links with odd characters', () => {
    const h = link('code', ['hosts', 'a b', 'x#y.nix'], { q: 'a&b', empty: '' })
    expect(h).toBe('#/code/hosts/a%20b/x%23y.nix?q=a%26b')
    expect(parseHash(h)).toEqual({ view: 'code', parts: ['hosts', 'a b', 'x#y.nix'], query: { q: 'a&b' } })
  })
  it('opens a standalone home among the machines, by its edit target', () => {
    expect(parseHash(homeLink('pablo@laptop'))).toEqual({ view: 'machines', parts: ['home:pablo@laptop'], query: {} })
  })
})
