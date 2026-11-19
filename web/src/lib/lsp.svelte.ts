import {
  LSPClient,
  findReferencesKeymap,
  hoverTooltips,
  jumpToDefinitionKeymap,
  renameKeymap,
  serverCompletion,
  serverDiagnostics,
  signatureHelp,
  type Transport,
} from '@codemirror/lsp-client'
import { keymap } from '@codemirror/view'

type SocketLike = Pick<WebSocket, 'send' | 'readyState' | 'addEventListener'>

/** One JSON-RPC message per WebSocket text frame, no Content-Length headers. */
export function socketTransport(sock: SocketLike): Transport {
  const handlers = new Set<(msg: string) => void>()
  sock.addEventListener('message', (e: MessageEvent) => {
    if (typeof e.data === 'string') for (const h of handlers) h(e.data)
  })
  return {
    send(msg) {
      if (sock.readyState !== 1) throw new Error('Language server is not connected') // 1 = OPEN
      sock.send(msg)
    },
    subscribe: (h) => void handlers.add(h),
    unsubscribe: (h) => void handlers.delete(h),
  }
}

const KEEP = new Set(['p', 'br', 'hr', 'pre', 'code', 'em', 'strong', 'b', 'i', 'del', 'sub', 'sup', 'a', 'ul', 'ol', 'li',
  'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'blockquote', 'table', 'thead', 'tbody', 'tr', 'th', 'td', 'span', 'div'])
const DROP = new Set(['script', 'style', 'template', 'svg', 'math', 'iframe', 'object', 'embed', 'noscript', 'textarea', 'title', 'select', 'img', 'video', 'audio', 'form', 'input', 'button', 'link', 'meta', 'base'])

/**
 * Hover docs are Markdown rendered to HTML, and option descriptions come from flake inputs.
 * Rebuild them from an allowlist: plain formatting tags, text, class names and http(s) links only.
 */
export function sanitizeHTML(html: string): string {
  const out = document.createElement('div')
  const copy = (from: Node, to: Node) => {
    for (const n of from.childNodes) {
      if (n.nodeType === Node.TEXT_NODE) {
        to.appendChild(document.createTextNode(n.textContent ?? ''))
        continue
      }
      if (n.nodeType !== Node.ELEMENT_NODE) continue
      const el = n as Element
      const tag = el.localName
      if (DROP.has(tag) || el.namespaceURI !== 'http://www.w3.org/1999/xhtml') continue
      if (!KEEP.has(tag)) {
        copy(el, to) // unknown wrapper: keep its text
        continue
      }
      const clean = document.createElement(tag)
      const cls = el.getAttribute('class')
      if (cls) clean.setAttribute('class', cls)
      const href = el.getAttribute('href')?.trim() ?? ''
      if (tag === 'a' && /^https?:\/\//i.test(href)) clean.setAttribute('href', href)
      copy(el, clean)
      to.appendChild(clean)
    }
  }
  copy(new DOMParser().parseFromString(html, 'text/html').body, out)
  return out.innerHTML
}

/**
 * Stands in for nixd when the server cannot start it: it answers `initialize` with no
 * capabilities, so the editor never asks for completion or hover and nothing times out.
 */
export function silentTransport(): Transport {
  const handlers = new Set<(msg: string) => void>()
  return {
    send(msg) {
      const m = JSON.parse(msg)
      if (m.id === undefined || !m.method) return
      const reply = JSON.stringify({ jsonrpc: '2.0', id: m.id, result: m.method === 'initialize' ? { capabilities: {} } : null })
      queueMicrotask(() => handlers.forEach((h) => h(reply)))
    },
    subscribe: (h) => void handlers.add(h),
    unsubscribe: (h) => void handlers.delete(h),
  }
}

export const lspState = $state({ status: 'off' as 'off' | 'connecting' | 'ready' | 'down' | 'missing', reason: '' })

let client: LSPClient | null = null
let retries = 0
let retryTimer: ReturnType<typeof setTimeout> | undefined
let socket: WebSocket | null = null

function connect() {
  clearTimeout(retryTimer)
  if (!client || (socket && socket.readyState <= 1)) return
  const c = client
  const sock = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/api/lsp`)
  socket = sock
  lspState.status = 'connecting'
  let linked = false
  let wait: ReturnType<typeof setTimeout> | undefined
  sock.addEventListener('open', () => {
    // The server closes the socket at once when nixd cannot start: wait a moment before
    // talking, so no request is left waiting for a server that never came.
    wait = setTimeout(() => {
      if (socket !== sock) return
      linked = true
      c.connect(socketTransport(sock))
      c.initializing.then(
        () => {
          retries = 0
          Object.assign(lspState, { status: 'ready', reason: '' })
        },
        () => {},
      )
    }, 400)
  })
  sock.addEventListener('close', (e) => {
    clearTimeout(wait)
    if (socket !== sock) return
    socket = null
    if (linked) c.disconnect()
    if (/cannot start nixd/.test(e.reason)) {
      // Retrying cannot help until nixd is installed; stop, say why, and keep the editor quiet.
      Object.assign(lspState, { status: 'missing', reason: 'nixd is not installed on the server (or cannot start), so the editor has no completion, hover help or diagnostics. Install nixd, or use the container image, and reload.' })
      c.connect(silentTransport())
      return
    }
    Object.assign(lspState, { status: 'down', reason: e.reason || 'The connection to the language server closed; retrying.' })
    retryTimer = setTimeout(connect, Math.min(30000, 1000 * 2 ** retries++))
  })
}

/** The shared client; created and connected the first time a Nix file opens. */
export function lspClient(root: string): LSPClient {
  if (!client) {
    client = new LSPClient({
      rootUri: 'file://' + root,
      timeout: 10000,
      sanitizeHTML,
      // Shift-Alt-F is ours (nixfmt through the API), so the client's format keymap is left out.
      extensions: [
        serverCompletion(),
        hoverTooltips(),
        signatureHelp(),
        serverDiagnostics(),
        keymap.of([...renameKeymap, ...jumpToDefinitionKeymap, ...findReferencesKeymap]),
      ],
    })
    connect()
  }
  return client
}

/** Try again now, e.g. right after logging in. */
export function reconnectLsp() {
  retries = 0
  connect()
}

export const documentUri = (root: string, path: string) => `file://${root}/${path}`
