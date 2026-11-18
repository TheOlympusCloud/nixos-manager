import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api, errorMessage, onUnauthorized, request, withQuery } from './api'

function mockFetch(status: number, body = '', statusText = '') {
  const fn = vi.fn(async (_url: string, _init?: RequestInit) => new Response(status === 204 ? null : body, { status, statusText }))
  vi.stubGlobal('fetch', fn)
  return fn
}

afterEach(() => {
  vi.unstubAllGlobals()
  onUnauthorized(() => {})
})

describe('request', () => {
  it('sends JSON content type and a JSON body on every non-GET request', async () => {
    const fetch = mockFetch(204)
    await request('POST', '/api/logout')
    const init = fetch.mock.calls[0][1]!
    expect(init.method).toBe('POST')
    expect(init.headers).toEqual({ 'Content-Type': 'application/json' })
    expect(init.body).toBe('{}')

    await api.revokeToken('t/1')
    expect(fetch.mock.calls[1][0]).toBe('/api/tokens/t%2F1')
    expect(fetch.mock.calls[1][1]!.method).toBe('DELETE')
    expect(fetch.mock.calls[1][1]!.body).toBe('{}')
  })

  it('sends no body on GET and parses JSON', async () => {
    const fetch = mockFetch(200, '{"hash":"abc"}')
    expect(await request('GET', '/api/x')).toEqual({ hash: 'abc' })
    expect(fetch.mock.calls[0][1]!.body).toBeUndefined()
    expect(fetch.mock.calls[0][1]!.headers).toBeUndefined()
  })

  it('returns undefined for 204', async () => {
    mockFetch(204)
    expect(await request('POST', '/api/stage', { paths: [] })).toBeUndefined()
  })

  it('signals 401 so the login screen shows, and still throws', async () => {
    const seen = vi.fn()
    onUnauthorized(seen)
    mockFetch(401, '{"error":"not logged in"}')
    await expect(request('GET', '/api/status')).rejects.toMatchObject({ status: 401, message: 'not logged in' })
    expect(seen).toHaveBeenCalledOnce()
  })

  it('does not signal 401 for a wrong password at login', async () => {
    const seen = vi.fn()
    onUnauthorized(seen)
    mockFetch(401, '{"error":"wrong password"}')
    await expect(api.login('nope')).rejects.toBeInstanceOf(ApiError)
    expect(seen).not.toHaveBeenCalled()
  })

  it('uses the JSON error message', async () => {
    mockFetch(409, '{"error":"file changed on disk"}')
    await expect(api.saveFile('a.nix', 'x', 'h')).rejects.toMatchObject({ status: 409, message: 'file changed on disk' })
  })

  it('turns network failures into an ApiError', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => Promise.reject(new TypeError('Failed to fetch'))))
    await expect(api.status()).rejects.toMatchObject({ status: 0, message: 'Cannot reach the server' })
  })
})

describe('errorMessage', () => {
  it('falls back to the body text, then to the status line', async () => {
    expect(await errorMessage(new Response('plain failure\n', { status: 500 }))).toBe('plain failure')
    expect(await errorMessage(new Response('{"other":1}', { status: 500 }))).toBe('{"other":1}')
    expect(await errorMessage(new Response('', { status: 502, statusText: 'Bad Gateway' }))).toBe('502 Bad Gateway')
  })
})

describe('withQuery', () => {
  it('encodes values and skips empty ones', () => {
    expect(withQuery('/api/diff', { staged: true, path: 'hosts/a b.nix' })).toBe('/api/diff?staged=true&path=hosts%2Fa+b.nix')
    expect(withQuery('/api/log', { path: undefined, limit: undefined })).toBe('/api/log')
    expect(withQuery('/api/diff', { staged: false, path: '' })).toBe('/api/diff?staged=false')
  })
})
