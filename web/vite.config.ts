import tailwindcss from '@tailwindcss/vite'
import { tanstackStart } from '@tanstack/react-start/plugin/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import type { Plugin } from 'vite'

const canonicalDevOrigin = 'http://localhost:3000'

const canonicalDevOriginPlugin: Plugin = {
  name: 'orca-canonical-dev-origin',
  configureServer(server) {
    server.middlewares.use((request, response, next) => {
      const host = request.headers.host ?? ''
      const acceptsHTML = request.headers.accept?.includes('text/html') ?? false
      if (!host.endsWith(':5173') || !acceptsHTML) {
        next()
        return
      }

      const requested = new URL(request.url ?? '/', 'http://vite.local')
      const destination = new URL(canonicalDevOrigin)
      destination.pathname = requested.pathname
      destination.search = requested.search
      response.statusCode = 307
      response.setHeader('Location', destination.toString())
      response.end()
    })
  },
}

export default defineConfig({
  plugins: [canonicalDevOriginPlugin, tanstackStart(), tailwindcss(), react()],
})
