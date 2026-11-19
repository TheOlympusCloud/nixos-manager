import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig } from 'vite'

// The Go binary embeds web/dist; only dist/app is ours to empty so dist/.keep survives.
export default defineConfig({
  plugins: [svelte()],
  base: '/',
  // The graph layout engine (ELK, ~1.5 MB) runs in its own worker file; CodeMirror and the intro's WebGL
  // scene (~600 kB) load on demand. Anything bigger than that in a chunk deserves a warning.
  build: { outDir: 'dist/app', chunkSizeWarningLimit: 650 },
  server: {
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080', ws: true },
      '/mcp': { target: 'http://127.0.0.1:8080' }, // so the endpoint shown on the Tokens page works in dev too
    },
  },
})
