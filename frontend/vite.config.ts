import path from 'node:path'
import { defineConfig } from 'vite'
import { devtools } from '@tanstack/devtools-vite'
import tsconfigPaths from 'vite-tsconfig-paths'

import { tanstackRouter } from '@tanstack/router-plugin/vite'

import viteReact from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// The '@enterprise' alias is the frontend edition seam: community builds
// resolve it to the stub module (no enterprise code in the bundle),
// enterprise builds to the real module under the commercial license.
const enterpriseEdition = process.env.SQLWARDEN_EDITION === 'enterprise'

const config = defineConfig({
  resolve: {
    alias: {
      '@enterprise': path.resolve(
        import.meta.dirname,
        enterpriseEdition ? 'src/enterprise' : 'src/enterprise-stub',
      ),
    },
  },
  plugins: [
    devtools(),
    tsconfigPaths({ projects: ['./tsconfig.json'] }),
    tailwindcss(),
    tanstackRouter({ target: 'react', autoCodeSplitting: true }),
    viteReact(),
  ],
  build: {
    outDir: '../assets/static',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:6020',
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    restoreMocks: true,
    clearMocks: true,
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json-summary', 'html'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: [
        'src/**/*.test.{ts,tsx}',
        'src/test/**',
        'src/routeTree.gen.ts',
        'src/main.tsx',
        'src/components/ui/**',
      ],
    },
  },
})

export default config
