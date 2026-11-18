import { describe, expect, it, vi } from 'vitest'
import { documentUri, socketTransport } from './lsp.svelte'

class FakeSocket extends EventTarget {
  readyState = 1
  sent: string[] = []
  send(msg: string) {
    this.sent.push(msg)
  }
}

describe('socketTransport', () => {
  it('passes one JSON-RPC message per text frame both ways', () => {
    const sock = new FakeSocket()
    const t = socketTransport(sock as unknown as WebSocket)
    const handler = vi.fn()
    t.subscribe(handler)
    sock.dispatchEvent(new MessageEvent('message', { data: '{"jsonrpc":"2.0","id":1,"result":null}' }))
    expect(handler).toHaveBeenCalledWith('{"jsonrpc":"2.0","id":1,"result":null}')

    t.send('{"jsonrpc":"2.0","method":"initialized","params":{}}')
    expect(sock.sent).toEqual(['{"jsonrpc":"2.0","method":"initialized","params":{}}'])

    t.unsubscribe(handler)
    sock.dispatchEvent(new MessageEvent('message', { data: '{}' }))
    expect(handler).toHaveBeenCalledOnce()
  })

  it('throws when the socket is not open, so the client sees the broken connection', () => {
    const sock = new FakeSocket()
    sock.readyState = 3
    expect(() => socketTransport(sock as unknown as WebSocket).send('{}')).toThrow(/not connected/)
  })

  it('builds document URIs from the working copy root', () => {
    expect(documentUri('/srv/nixos-config', 'hosts/web/default.nix')).toBe('file:///srv/nixos-config/hosts/web/default.nix')
  })
})
