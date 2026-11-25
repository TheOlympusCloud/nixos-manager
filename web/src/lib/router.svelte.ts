// Hash routes, so reload and back work: #/fleet, #/machines/pi/packages, #/code/hosts/pi/default.nix, ...

import { HOME } from './packages'

export const views = ['fleet', 'machines', 'packages', 'services', 'options', 'health', 'deployments', 'dependencies', 'inputs', 'changes', 'history', 'code', 'settings', 'intro'] as const
export type View = (typeof views)[number]
export type Route = { view: View; parts: string[]; query: Record<string, string> }

export function parseHash(hash: string): Route {
  const [path, qs = ''] = hash.replace(/^#\/?/, '').split('?', 2)
  const [first, ...rest] = path.split('/').filter(Boolean)
  const decode = (s: string) => {
    try {
      return decodeURIComponent(s)
    } catch {
      return s
    }
  }
  const view = (views as readonly string[]).includes(first) ? (first as View) : 'fleet'
  return { view, parts: view === first ? rest.map(decode) : [], query: Object.fromEntries(new URLSearchParams(qs)) }
}

/** Build a hash from a view, path parts and query (empty values dropped). */
export function link(view: View, parts: string[] = [], query: Record<string, string | undefined> = {}): string {
  const q = new URLSearchParams(Object.entries(query).filter((e): e is [string, string] => !!e[1])).toString()
  return `#/${[view, ...parts.map(encodeURIComponent)].join('/')}${q ? '?' + q : ''}`
}

export const route = $state<Route>(parseHash(typeof location === 'undefined' ? '' : location.hash))

if (typeof window !== 'undefined') window.addEventListener('hashchange', () => Object.assign(route, parseHash(location.hash)))

export function go(hash: string) {
  if (location.hash !== hash) location.hash = hash
}
/** Change the URL without a new history entry (e.g. the active file in Code). */
export function replace(hash: string) {
  if (location.hash === hash) return
  history.replaceState(history.state, '', hash)
  Object.assign(route, parseHash(hash))
}

export const codeLink = (path: string) => link('code', path.split('/'))
export const machineLink = (host: string, tab?: string) => link('machines', tab ? [host, tab] : [host])
/** A standalone home's page, among the machines: #/machines/home:pablo@laptop. */
export const homeLink = (name: string) => link('machines', [HOME + name])
