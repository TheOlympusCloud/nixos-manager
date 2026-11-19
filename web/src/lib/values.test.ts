import { describe, expect, it } from 'vitest'
import type { HostValue } from './api'
import { copyable, editingPreview, effective, managedFeatures, managedOptions, showValue, sourceLabel, sourceOf } from './values'

const hv = (over: Partial<HostValue> = {}): HostValue => ({ host: 'a', exists: true, value: 1, files: [], type: 'integer', ...over })
const mf = 'hosts/a/managed.json'

describe('sourceOf', () => {
  it('tells set here, in code and default apart', () => {
    expect(sourceOf([mf], mf)).toEqual({ kind: 'here' })
    expect(sourceOf(['modules/managed.nix'], mf)).toEqual({ kind: 'here' })
    expect(sourceOf(['modules/common.nix', 'hosts/a/default.nix'], mf)).toEqual({ kind: 'code', file: 'modules/common.nix', files: ['modules/common.nix', 'hosts/a/default.nix'] })
    expect(sourceOf(['nixos'], mf)).toEqual({ kind: 'default' })
    expect(sourceOf([], null)).toEqual({ kind: 'default' })
    // A Code only machine never has values set here.
    expect(sourceOf(['modules/managed.nix'], null).kind).toBe('code')
    expect(sourceLabel(sourceOf(['hosts/a/default.nix'], mf))).toBe('In code · hosts/a/default.nix')
  })
})

describe('effective', () => {
  it('prefers the managed value, which wins at priority 90', () => {
    expect(effective(hv({ value: 1, files: ['x.nix'] }), 2, mf, true)).toEqual({ state: 'value', value: 2, source: { kind: 'here' }, pending: false })
    expect(effective(undefined, 'z', mf, true)).toMatchObject({ value: 'z' })
  })
  it('marks a value reset here but still in the evaluation as pending', () => {
    expect(effective(hv({ files: [mf] }), undefined, mf, true)).toMatchObject({ pending: true })
    expect(effective(hv({ files: [mf] }), undefined, mf, false)).toMatchObject({ pending: false })
  })
  it('reports missing options, errors and unknown values', () => {
    expect(effective(undefined, undefined, mf, true)).toBeNull()
    expect(effective(hv({ exists: false }), undefined, mf, true)).toEqual({ state: 'missing' })
    expect(effective(hv({ error: 'boom' }), undefined, mf, true)).toEqual({ state: 'error', error: 'boom' })
  })
})

describe('showValue and copyable', () => {
  it('writes values as short Nix', () => {
    expect(showValue('Europe/Madrid')).toBe('"Europe/Madrid"')
    expect(showValue([22, 80])).toBe('[ 22 80 ]')
    expect(showValue([])).toBe('[ ]')
    expect(showValue({ a: true, 'b.c': 'x' })).toBe('{ a = true; "b.c" = "x"; }')
    expect(showValue(null)).toBe('null')
    expect(showValue(undefined)).toBe('—')
    expect(showValue('«firefox-140»')).toBe('«firefox-140»')
  })
  it('refuses nulls and package or function markers', () => {
    expect(copyable([1, 'a', { b: true }])).toBe(true)
    expect(copyable(null)).toBe(false)
    expect(copyable(['«derivation»'])).toBe(false)
    expect(copyable({ f: '«function»' })).toBe(false)
  })
})

describe('managedOptions and managedFeatures', () => {
  const settings = {
    services: { openssh: { enable: true, settings: { PermitRootLogin: 'no' } }, desktopManager: { plasma6: { enable: false } } },
    networking: { firewall: { allowedTCPPorts: [80] } },
    users: { users: { 'a.b': { isNormalUser: true } } },
    empty: {},
  }
  it('flattens settings into option names', () => {
    expect(managedOptions(settings).map((o) => o.name)).toEqual([
      'services.openssh.enable',
      'services.openssh.settings.PermitRootLogin',
      'services.desktopManager.plasma6.enable',
      'networking.firewall.allowedTCPPorts',
      'users.users."a.b".isNormalUser',
      'empty',
    ])
    expect(managedOptions(undefined)).toEqual([])
  })
  it('lists the enable switches as features', () => {
    expect(managedFeatures(settings)).toEqual([
      { group: 'services', name: 'openssh', on: true },
      { group: 'services', name: 'desktopManager.plasma6', on: false },
    ])
  })
})

describe('editingPreview', () => {
  const flake = ['{', '  outputs = { nixpkgs, ... }: {', '    nixosConfigurations.pi = nixpkgs.lib.nixosSystem {', '      modules = [', '        ./hosts/pi', '        ./modules/common.nix', '      ];', '    };', '    # ./hosts/pie is a comment', '  };', '}'].join('\n')
  it('adds the line after the machine import, at its indentation', () => {
    expect(editingPreview(flake, 'pi')).toEqual({
      before: ['  outputs = { nixpkgs, ... }: {', '    nixosConfigurations.pi = nixpkgs.lib.nixosSystem {', '      modules = [', '        ./hosts/pi'],
      line: '        (import ./modules/managed.nix ./hosts/pi/managed.json)',
      after: ['        ./modules/common.nix', '      ];'],
      at: 6,
    })
  })
  it('does not match other machines or comments', () => {
    expect(editingPreview(flake, 'pie')).toBeNull()
    expect(editingPreview('modules = [ ./hosts/pi/default.nix ];', 'pi')?.at).toBe(2)
    expect(editingPreview('mkHost = name: ./hosts/${name};', 'pi')).toBeNull()
  })
})
