import { describe, expect, it } from 'vitest'
import type { HostModule } from './api'
import { machinesWith, moduleNameOk, moduleOn, moduleSource, offValue, optionNameOk, parseValue, settingValue, usesModules, valueText } from './modules'
import { host } from './testmodel'

const mf = 'hosts/pc/managed.json'
const mod = (enable: boolean, files: string[]): HostModule => ({ name: 'media', enable, files, options: {} })

describe('names', () => {
  it('takes what a folder and a Nix attribute can both be', () => {
    for (const ok of ['media', 'desk_top-2', '_x']) expect(moduleNameOk(ok)).toBe(true)
    for (const bad of ['', '1st', '-x', 'a.b', 'a b', '../x', 'x'.repeat(64)]) expect(moduleNameOk(bad)).toBe(false)
    expect(optionNameOk('port')).toBe(true)
    expect(optionNameOk('enable')).toBe(false)
  })
})

describe('moduleSource', () => {
  it('tells the machine, every machine, hand-written Nix and the default apart', () => {
    expect(moduleSource([mf], 'media', mf)).toEqual({ kind: 'here' })
    expect(moduleSource(['modules/managed.json'], 'media', mf)).toEqual({ kind: 'shared' })
    expect(moduleSource(['hosts/pc/default.nix'], 'media', mf)).toEqual({ kind: 'code', file: 'hosts/pc/default.nix' })
    // At its default, the module's own file declares the value.
    expect(moduleSource(['modules/fleet/media/managed.json'], 'media', mf)).toEqual({ kind: 'default' })
    expect(moduleSource([], 'media', null)).toEqual({ kind: 'default' })
  })
})

describe('moduleOn and offValue', () => {
  it('lets a value set here win, at once', () => {
    expect(moduleOn(mod(false, []), true, 'media', mf)).toBe(true)
    expect(moduleOn(mod(true, ['hosts/pc/default.nix']), false, 'media', mf)).toBe(false)
  })
  it('treats one turned on only here and reset since as off', () => {
    expect(moduleOn(mod(true, [mf]), undefined, 'media', mf)).toBe(false)
    expect(moduleOn(mod(true, ['modules/managed.json']), undefined, 'media', mf)).toBe(true)
    expect(moduleOn(undefined, undefined, 'media', mf)).toBe(false)
  })
  it('turns off what another file turns on with false, else resets', () => {
    expect(offValue(mod(true, ['modules/managed.json']), 'media', mf)).toBe(false)
    expect(offValue(mod(true, ['hosts/pc/default.nix']), 'media', mf)).toBe(false)
    expect(offValue(mod(true, [mf]), 'media', mf)).toBeUndefined()
    expect(offValue(mod(false, ['modules/fleet/media/managed.json']), 'media', mf)).toBeUndefined()
  })
})

describe('values in forms', () => {
  it('parses by type', () => {
    expect(parseValue('bool', 'true')).toEqual({ value: true })
    expect(parseValue('bool', 'yes')).toHaveProperty('problem')
    expect(parseValue('port', '8096')).toEqual({ value: 8096 })
    expect(parseValue('port', '70000')).toHaveProperty('problem')
    expect(parseValue('int', '-3')).toEqual({ value: -3 })
    expect(parseValue('int', '1.5')).toHaveProperty('problem')
    expect(parseValue('str', ' a b ')).toEqual({ value: ' a b ' })
    expect(parseValue('listOf str', 'a\n\n b \n')).toEqual({ value: ['a', 'b'] })
    expect(parseValue('anything', '{"a": [1]}')).toEqual({ value: { a: [1] } })
    expect(parseValue('anything', '{')).toHaveProperty('problem')
  })
  it('shows a value as its form takes it back', () => {
    for (const [type, v] of [
      ['bool', false],
      ['port', 8096],
      ['str', 'x y'],
      ['listOf str', ['a', 'b']],
      ['anything', { a: 1 }],
    ] as const) {
      expect(parseValue(type, valueText(type, v))).toEqual({ value: v })
    }
    expect(valueText('str', undefined)).toBe('')
    expect(valueText('str', null)).toBe('')
    // A null default (type anything) stays null when its option is edited.
    expect(parseValue('anything', valueText('anything', null))).toEqual({ value: null })
  })
  it('takes a setting as JSON, or else as text', () => {
    expect(settingValue('[ "@option:port@" ]')).toEqual({ value: ['@option:port@'] })
    expect(settingValue('@option:port@')).toEqual({ value: '@option:port@' })
    expect(settingValue('Europe/Madrid')).toEqual({ value: 'Europe/Madrid' })
    expect(settingValue('8080')).toEqual({ value: 8080 })
    expect(settingValue('[1,')).toHaveProperty('problem')
    expect(settingValue(' ')).toHaveProperty('problem')
  })
})

describe('machines', () => {
  it('knows which apply modules and which have one on', () => {
    expect(usesModules(host('a', { managedFile: 'hosts/a/managed.json', managedVersion: 7 }))).toBe(true)
    expect(usesModules(host('a', { managedFile: 'hosts/a/managed.json', managedVersion: 6 }))).toBe(false)
    expect(usesModules(host('a', { managedFile: null, managedVersion: 7 }))).toBe(false)
    const hosts = [host('a', { modules: [{ ...mod(true, []), name: 'media' }] }), host('b', { modules: [mod(false, [])] }), host('c')]
    expect(machinesWith(hosts, 'media')).toEqual(['a'])
  })
})
