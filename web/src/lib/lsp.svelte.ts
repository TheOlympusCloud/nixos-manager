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

/**
 * Hover docs are Markdown rendered to HTML; option descriptions come from flake inputs,
 * so strip anything that could run script before it reaches the page.
 */
export function sanitizeHTML(html: string): string {
  const doc = new DOMParser().parseFromString(html, 'text/html')
  doc.querySelectorAll('script, style, iframe, object, embed, link, meta, form, base').forEach((el) => el.remove())
  for (const el of doc.body.querySelectorAll('*'))
    for (const a of [...el.attributes])
      if (a.name.startsWith('on') || /^\s*(javascript|data|vbscript):/i.test(a.value)) el.removeAttribute(a.name)
  return doc.body.innerHTML
}

export const lspState = $state({ status: 'off' as 'off' | 'connecting' | 'ready' | 'down' })

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
  sock.addEventListener('open', () => {
    c.connect(socketTransport(sock))
    c.initializing.then(
      () => {
        retries = 0
        lspState.status = 'ready'
      },
      () => {},
    )
  })
  sock.addEventListener('close', () => {
    if (socket !== sock) return
    socket = null
    c.disconnect()
    lspState.status = 'down'
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
