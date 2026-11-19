import { describe, expect, it } from 'vitest'
import { commitUrl, fileUrl, forgetHostKey, hookTarget, onRemote } from './forgejo'

const web = 'https://forgejo.lan/pablo/nixos-config'

describe('links to Forgejo', () => {
  it('builds commit and file pages', () => {
    expect(commitUrl(web, 'abc123')).toBe(`${web}/commit/abc123`)
    expect(fileUrl(web, 'main', 'hosts/pi/default.nix')).toBe(`${web}/src/branch/main/hosts/pi/default.nix`)
    expect(fileUrl(web, 'testing/pi', 'a b/#1.nix')).toBe(`${web}/src/branch/testing/pi/a%20b/%231.nix`)
  })
  it('gives nothing without a web address', () => {
    expect(commitUrl('', 'abc')).toBe('')
    expect(fileUrl('', 'main', 'flake.nix')).toBe('')
    expect(fileUrl(web, '', 'flake.nix')).toBe('')
  })
  it('knows which commits the remote has', () => {
    expect(onRemote(0, 'origin/main', 2)).toBe(false)
    expect(onRemote(1, 'origin/main', 2)).toBe(false)
    expect(onRemote(2, 'origin/main', 2)).toBe(true)
    expect(onRemote(0, 'origin/main', 0)).toBe(true)
    expect(onRemote(5, '', 0)).toBe(false)
  })
})

describe('hookTarget', () => {
  it('is the console address plus the hook path', () => {
    expect(hookTarget('http://nixmgr.lan:8080')).toEqual({ url: 'http://nixmgr.lan:8080/api/hooks/forgejo', warning: '' })
  })
  it('warns about addresses Forgejo cannot reach', () => {
    expect(hookTarget('http://127.0.0.1:8112').warning).not.toBe('')
    expect(hookTarget('http://localhost:8080').warning).not.toBe('')
  })
})

describe('forgetHostKey', () => {
  const kh = '/var/lib/nixos-manager/ssh/known_hosts'
  it('names the host the way known_hosts does', () => {
    expect(forgetHostKey('ssh://git@forgejo.lan:2222/p/f.git', kh)).toBe(`ssh-keygen -R '[forgejo.lan]:2222' -f '${kh}'`)
    expect(forgetHostKey('ssh://git@forgejo.lan/p/f.git', kh)).toBe(`ssh-keygen -R 'forgejo.lan' -f '${kh}'`)
    expect(forgetHostKey('git@forgejo.lan:p/f.git', kh)).toBe(`ssh-keygen -R 'forgejo.lan' -f '${kh}'`)
  })
  it('gives nothing for other remotes', () => {
    expect(forgetHostKey('https://forgejo.lan/p/f.git', kh)).toBe('')
    expect(forgetHostKey('git@forgejo.lan:p/f.git', '')).toBe('')
  })
})
