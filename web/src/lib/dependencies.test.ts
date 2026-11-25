import { describe, expect, it } from 'vitest'
import type { DepNode, DepRow, MachineDeps } from './api'
import { children, filterRows, findNodes, label, stateMessage, usedBy, why } from './dependencies'

const node = (c: string, name: string, version: string | undefined, refs: number[], via: number, narSize = 1): DepNode => ({
  path: `/nix/store/${c.repeat(32)}-${name}${version ? '-' + version : ''}`,
  name,
  version,
  narSize,
  refs,
  via,
})

// system → hello, bash; bash → glibc; hello → glibc, openssl
const d: MachineDeps = {
  host: 'pi',
  state: 'ready',
  size: 0,
  root: 0,
  nodes: [
    node('a', 'nixos-system-pi', undefined, [2, 1], -1),
    node('b', 'hello', '2.12.2', [3, 4], 0),
    node('c', 'bash', '5.2p37', [3], 0),
    node('d', 'glibc', '2.40-66', [], 2),
    node('f', 'openssl', '3.0.15', [], 1),
  ],
}

describe('dependency tree', () => {
  it('names an entry without the store and the hash', () => {
    expect(label(d.nodes[3])).toBe('glibc-2.40-66')
    expect(label(d.nodes[0])).toBe('nixos-system-pi')
  })

  it('lists what an entry uses, by name', () => {
    expect(children(d, 0)).toEqual([2, 1])
    expect(children(d, 1)).toEqual([3, 4])
    expect(children(d, 3)).toEqual([])
  })

  it('lists what uses each entry', () => {
    const by = usedBy(d)
    expect(by[3]).toEqual([2, 1])
    expect(by[0]).toEqual([])
  })

  it('explains why an entry is there with a chain from the system', () => {
    expect(why(d, 3)).toEqual([0, 2, 3])
    expect(why(d, 0)).toEqual([0])
  })

  it('finds entries by name, exact names first', () => {
    const big: MachineDeps = { ...d, nodes: [...d.nodes, node('g', 'glibc-locales', '2.40-66', [], 3)] }
    expect(findNodes(big, 'GLIBC')).toEqual([3, 5])
    expect(findNodes(big, 'glibc', 1)).toEqual([3])
    expect(findNodes(big, '')).toEqual([])
  })
})

describe('fleet table', () => {
  const rows: DepRow[] = [
    { name: 'bash', versions: [['5.2p37'], ['5.3p3']], differs: true },
    { name: 'glibc', versions: [['2.40-66'], ['2.40-66']], differs: false },
  ]
  it('filters by name and by differing versions', () => {
    expect(filterRows(rows, '', false)).toHaveLength(2)
    expect(filterRows(rows, 'GLI', false).map((r) => r.name)).toEqual(['glibc'])
    expect(filterRows(rows, '', true).map((r) => r.name)).toEqual(['bash'])
  })
})

describe('stateMessage', () => {
  it('says what to do for each state without a tree', () => {
    expect(stateMessage({ state: 'ready' }, 'pi')).toBe('')
    expect(stateMessage({ state: 'none' }, 'pi')).toContain('Deployments')
    expect(stateMessage({ state: 'shared' }, 'pi')).toContain('own token')
    expect(stateMessage({ state: 'old' }, 'pi')).toContain('report-deployments.nix')
    expect(stateMessage({ state: 'waiting' }, 'pi')).toContain('15 minutes')
    expect(stateMessage({ state: 'refused', message: 'it has more than 100000 paths' }, 'pi')).toContain('it has more than 100000 paths')
    expect(stateMessage({ state: 'error' }, 'pi')).toContain('could not read')
  })
})
