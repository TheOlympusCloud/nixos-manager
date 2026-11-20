import { describe, expect, it } from 'vitest'
import { forUsers, secretNameProblem, secretPath, takesSecretFile } from './secrets'

describe('secret names', () => {
  it('takes what the server takes', () => {
    for (const ok of ['', 'wifi', 'comin-token', 'api.key_2', 'A'.repeat(100)]) expect(secretNameProblem(ok)).toBe('')
    for (const bad of ['-x', 'a/b', '../x', 'with space', 'A'.repeat(101), 'sops', 'pass_unencrypted', 'é']) expect(secretNameProblem(bad)).not.toBe('')
  })
})

describe('options that take a secret file', () => {
  it('are file paths given as text', () => {
    expect(takesSecretFile(['services', 'tailscale', 'authKeyFile'], 'null or absolute path')).toBe(true)
    expect(takesSecretFile(['users', 'users', 'pablo', 'hashedPasswordFile'], 'null or string')).toBe(true)
    expect(takesSecretFile(['services', 'x', 'environmentFile'], 'null or path')).toBe(true)
    expect(takesSecretFile(['services', 'x', 'token_file'], 'string')).toBe(true)
    expect(takesSecretFile(['services', 'x', 'environmentFiles'], 'list of absolute path')).toBe(false)
    expect(takesSecretFile(['services', 'x', 'dataDir'], 'path')).toBe(false)
    expect(takesSecretFile(['services', 'x', 'configFile'], 'attribute set of anything')).toBe(false)
  })
  it('puts a password hash where it is there before users are made', () => {
    expect(forUsers(['users', 'users', 'pablo', 'hashedPasswordFile'])).toBe(true)
    expect(forUsers(['users', 'users', 'pablo', 'passwordFile'])).toBe(false)
    expect(forUsers(['services', 'x', 'hashedPasswordFile'])).toBe(false)
    expect(secretPath('pw', true)).toBe('/run/secrets-for-users/pw')
    expect(secretPath('wifi', false)).toBe('/run/secrets/wifi')
  })
})
