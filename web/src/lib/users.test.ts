import { describe, expect, it } from 'vitest'
import { keySummary, parseSSHKey, userNameProblem } from './users'

// A well-formed ed25519 public key: its own type, then 32 bytes.
const blob = (type: string, n = 32) => {
  const len = (k: number) => String.fromCharCode(0, 0, k >> 8, k & 255)
  return btoa(len(type.length) + type + len(n) + 'k'.repeat(n))
}
const ed = `ssh-ed25519 ${blob('ssh-ed25519')} me@laptop`

describe('parseSSHKey', () => {
  it('accepts public keys and keeps the comment', () => {
    expect(parseSSHKey(ed)).toEqual({ type: 'ssh-ed25519', data: blob('ssh-ed25519'), comment: 'me@laptop' })
    expect(parseSSHKey(`  ssh-rsa ${blob('ssh-rsa', 256)}  `)).toMatchObject({ type: 'ssh-rsa', comment: '' })
  })
  it('explains what is wrong', () => {
    expect(parseSSHKey('')).toHaveProperty('problem')
    expect((parseSSHKey(`ssh-dss ${blob('ssh-dss')}`) as { problem: string }).problem).toMatch(/DSA/)
    expect((parseSSHKey('-----BEGIN OPENSSH PRIVATE KEY-----') as { problem: string }).problem).toMatch(/private key/)
    expect((parseSSHKey('ssh-ed25519') as { problem: string }).problem).toMatch(/missing/)
    expect((parseSSHKey('ssh-ed25519 not*base64') as { problem: string }).problem).toMatch(/not valid/)
    expect((parseSSHKey(`ssh-ed25519 ${blob('ssh-rsa')}`) as { problem: string }).problem).toBe('The key data is for ssh-rsa, not ssh-ed25519.')
  })
  it('summarises keys for display', () => {
    expect(keySummary(ed)).toEqual({ type: 'ed25519', comment: 'me@laptop', tail: blob('ssh-ed25519').slice(-10) })
    expect(keySummary('ecdsa-sha2-nistp256 AAAA x').type).toBe('ecdsa nistp256')
  })
})

describe('userNameProblem', () => {
  it('checks the form, system accounts and duplicates', () => {
    expect(userNameProblem('', [])).toBe('')
    expect(userNameProblem('alice', ['pablo'])).toBe('')
    expect(userNameProblem('Alice', [])).toMatch(/Lowercase/)
    expect(userNameProblem('1a', [])).toMatch(/Lowercase/)
    expect(userNameProblem('root', [])).toMatch(/system account/)
    expect(userNameProblem('pablo', ['pablo'])).toMatch(/already exists/)
    expect(userNameProblem('a'.repeat(33), [])).toMatch(/32/)
  })
})
