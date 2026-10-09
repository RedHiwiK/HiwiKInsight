import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// Build output goes to dist/app and is embedded into the server binary by Go's web.Dist (go:embed)
export default defineConfig({
  base: '/dashboard/',
  plugins: [react(), tailwindcss()],
  build: { outDir: 'dist/app', emptyOutDir: true, chunkSizeWarningLimit: 1200 },
  server: {
    // Local development: run the server on :8090 first (e.g. the demo with -listen :8090), then pnpm dev
    proxy: { '/v1': 'http://127.0.0.1:8090' },
  },
})
