// Screenshots and GIFs of the prototype, through headless Chromium's DevTools protocol.
//   node capture.mjs <shots.json> <outdir>
// A shot is { name, hash, query?, w?, h?, wait?, run?, frames?, every? }:
//   wait    ms to let the page settle before a still (default 1500)
//   run     JavaScript evaluated once the page is up (open a dialog, hover a node…)
//   frames  record a GIF of that many frames instead; the page then runs on ?manual and this
//           script steps its clock `every` ticks (default 1) per frame, so the GIF is exact.
// Needs Node 22+ (built-in WebSocket), chromium and ffmpeg on PATH.
import { spawn, execFileSync } from 'node:child_process'
import { mkdirSync, readFileSync, writeFileSync, rmSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const [shotsFile, outDir] = process.argv.slice(2)
if (!shotsFile || !outDir) throw new Error('usage: node capture.mjs <shots.json> <outdir>')
const shots = JSON.parse(readFileSync(shotsFile, 'utf8'))
const page = pathToFileURL(resolve(dirname(fileURLToPath(import.meta.url)), '../prototype/index.html')).href
const port = 9300 + Math.floor(Math.random() * 500)
const profile = join(outDir, '.profile')
mkdirSync(outDir, { recursive: true })

const chrome = spawn('chromium', ['--headless=new', '--no-sandbox', '--hide-scrollbars', `--remote-debugging-port=${port}`, `--user-data-dir=${profile}`, 'about:blank'], { stdio: 'ignore' })
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

async function connect() {
  for (let i = 0; i < 50; i++) {
    try {
      const tabs = await (await fetch(`http://127.0.0.1:${port}/json`)).json()
      const tab = tabs.find((t) => t.type === 'page')
      if (tab) return new WebSocket(tab.webSocketDebuggerUrl)
    } catch {}
    await sleep(200)
  }
  throw new Error('chromium did not start')
}

try {
  const ws = await connect()
  await new Promise((r) => (ws.onopen = r))
  let id = 0
  let failed = false
  const waiting = new Map()
  ws.onmessage = (e) => {
    const m = JSON.parse(e.data)
    if (m.id && waiting.has(m.id)) waiting.get(m.id)(m), waiting.delete(m.id)
    // Page errors fail the run: a screenshot of a broken page is not a result.
    if (m.method === 'Runtime.exceptionThrown') (failed = true), console.error('PAGE ERROR', m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text)
    if (m.method === 'Runtime.consoleAPICalled' && m.params.type === 'error') (failed = true), console.error('CONSOLE', m.params.args.map((a) => a.value || a.description).join(' '))
  }
  // A page that never answers (an endless loop in its script) fails the run instead of stalling it.
  const send = (method, params = {}) =>
    new Promise((res, rej) => {
      const timer = setTimeout(() => rej(new Error(method + ': no answer in 30 s')), 30000)
      waiting.set(++id, (m) => (clearTimeout(timer), m.error ? rej(new Error(method + ': ' + m.error.message)) : res(m.result)))
      ws.send(JSON.stringify({ id, method, params }))
    })
  const evaluate = async (expression) => {
    const r = await send('Runtime.evaluate', { expression, awaitPromise: true })
    if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text)
    return r.result.value
  }
  const png = async (file) => writeFileSync(file, Buffer.from((await send('Page.captureScreenshot', { format: 'png' })).data, 'base64'))
  await send('Page.enable')
  await send('Runtime.enable')

  for (const s of shots) {
    const w = s.w || 1440
    const h = s.h || 900
    await send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: false })
    const query = [s.frames ? 'manual' : '', s.query || ''].filter(Boolean).join('&')
    await send('Page.navigate', { url: 'about:blank' })
    await send('Page.navigate', { url: `${page}${query ? '?' + query : ''}#/${s.hash || ''}` })
    await sleep(700)
    await evaluate('document.fonts.ready.then(() => 1)')
    if (!s.frames) {
      if (s.run) await evaluate(s.run)
      await sleep(s.wait ?? 1500)
      await png(join(outDir, s.name + '.png'))
      console.log('still', s.name)
      continue
    }
    const dir = join(outDir, '.frames-' + s.name)
    rmSync(dir, { recursive: true, force: true })
    mkdirSync(dir, { recursive: true })
    for (let f = 0; f < s.frames; f++) {
      if (s.run && f === (s.runAt || 0)) await evaluate(s.run)
      if (s.each) await evaluate(`(${s.each})(${f})`)
      for (let k = 0; k < (s.every || 1); k++) await evaluate('A.step()')
      await png(join(dir, String(f).padStart(4, '0') + '.png'))
    }
    const fps = s.fps || 12.5
    const scale = s.gifw ? `scale=${s.gifw}:-1:flags=lanczos,` : ''
    execFileSync('ffmpeg', ['-loglevel', 'error', '-y', '-framerate', String(fps), '-i', join(dir, '%04d.png'), '-vf', `${scale}split[a][b];[a]palettegen=max_colors=64[p];[b][p]paletteuse=dither=none`, join(outDir, s.name + '.gif')])
    rmSync(dir, { recursive: true, force: true })
    console.log('gif', s.name)
  }
  ws.close()
  if (failed) process.exitCode = 1
} finally {
  chrome.kill()
  await sleep(300)
  rmSync(profile, { recursive: true, force: true })
}
