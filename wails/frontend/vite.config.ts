import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'

// In `wails dev`, Wails proxies unknown requests to the Vite dev server, and
// only falls back to our Go AssetServer Handler if Vite returns 404 or 405
// (see pkg/assetserver/assethandler_external.go). Vite's default SPA behavior
// is to serve index.html for any path, which would make `<img src="/files?…">`
// receive HTML instead of bytes. This plugin short-circuits `/files` so Wails'
// proxy fallback hands the request to our handler as it does in production.
const dukto404Files = {
  name: 'dukto-404-go-routes',
  configureServer(server: {
    middlewares: {
      use: (fn: (req: { url?: string }, res: { statusCode: number; end: () => void }, next: () => void) => void) => void
    }
  }) {
    server.middlewares.use((req, res, next) => {
      if (req.url && req.url.startsWith('/files')) {
        res.statusCode = 404
        res.end()
        return
      }
      next()
    })
  },
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte(), dukto404Files],
})
