import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig } from 'vite'

// The Go binary embeds web/dist; only dist/app is ours to empty so dist/.keep survives.
export default defineConfig({
  plugins: [svelte()],
  base: '/',
  // The graph layout engine (ELK, ~1.5 MB) and CodeMirror load on demand in their own chunks; fine for a LAN app.
  build: { outDir: 'dist/app', chunkSizeWarningLimit: 1600 },
  server: {
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080', ws: true },
      '/mcp': { target: 'http://127.0.0.1:8080' }, // so the endpoint shown on the Tokens page works in dev too
    },
  },
})
