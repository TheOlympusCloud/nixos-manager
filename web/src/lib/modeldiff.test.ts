import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from './api'
import { managedSettings } from './modeldiff'

vi.mock('./fleet.svelte', () => ({ waitJob: vi.fn() })) // its store needs a browser

const managed = (settings: object) => JSON.stringify({ settings })

describe('managedSettings', () => {
  afterEach(() => vi.restoreAllMocks())

  it('compares the settings of each side', async () => {
    vi.spyOn(api, 'fileAt').mockResolvedValue(managed({ services: { openssh: { enable: true } } }))
    vi.spyOn(api, 'file').mockResolvedValue({ content: managed({ services: { openssh: { enable: false } } }) } as Awaited<ReturnType<typeof api.file>>)
    const got = await managedSettings(['hosts/pi/managed.json'], 'HEAD', null)
    expect(Object.keys(got)).toEqual(['pi'])
  })

  it('names the package changes next to the settings', async () => {
    vi.spyOn(api, 'fileAt').mockResolvedValue(JSON.stringify({ settings: {}, removedPackages: ['perl'] }))
    const now = { settings: {}, removedPackages: ['perl', 'nano'], packageOverrides: { htop: { override: { withVimKeys: true } } }, home: { pablo: { packages: ['fd'] } } }
    vi.spyOn(api, 'file').mockResolvedValue({ content: JSON.stringify(now) } as Awaited<ReturnType<typeof api.file>>)
    const got = await managedSettings(['hosts/pi/managed.json'], 'HEAD', null)
    expect(got.pi.map((c) => c.path.join('.'))).toEqual(['home.pablo.packages', 'packageOverrides.htop.override.withVimKeys', 'removedPackages'])
  })

  it('takes a file that is not there (404) as empty', async () => {
    vi.spyOn(api, 'fileAt').mockRejectedValue(new ApiError(404, 'hosts/pi/managed.json did not exist'))
    vi.spyOn(api, 'file').mockResolvedValue({ content: managed({ time: { timeZone: 'UTC' } }) } as Awaited<ReturnType<typeof api.file>>)
    const got = await managedSettings(['hosts/pi/managed.json'], 'HEAD', null)
    expect(got.pi?.length).toBe(1)
  })

  it('shows no settings for a file it could not read, rather than every setting as added or removed', async () => {
    vi.spyOn(api, 'fileAt').mockRejectedValue(new ApiError(500, 'git failed'))
    vi.spyOn(api, 'file').mockResolvedValue({ content: managed({ time: { timeZone: 'UTC' } }) } as Awaited<ReturnType<typeof api.file>>)
    expect(await managedSettings(['hosts/pi/managed.json'], 'HEAD', null)).toEqual({})
    vi.spyOn(api, 'fileAt').mockResolvedValue('{ "settings": ') // saved half way
    expect(await managedSettings(['hosts/pi/managed.json'], 'HEAD', null)).toEqual({})
  })
})
