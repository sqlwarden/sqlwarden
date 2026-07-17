import { defineConfig, loadEnv } from 'vite'
import { devtools } from '@tanstack/devtools-vite'
import tsconfigPaths from 'vite-tsconfig-paths'

import { tanstackRouter } from '@tanstack/router-plugin/vite'

import viteReact from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const communitySource = fileURLToPath(new URL('./src', import.meta.url))
const communityStyles = fileURLToPath(new URL('./src/styles.css', import.meta.url))

function distributionTailwindSource(distributionBuild: string | undefined) {
  if (!distributionBuild) return false
  const sourceDirectory = dirname(distributionBuild).replaceAll('\\', '/')
  return {
    name: 'sqlwarden-distribution-tailwind-source',
    enforce: 'pre' as const,
    transform(source: string, id: string) {
      if (id !== communityStyles) return null
      return `${source}\n@source ${JSON.stringify(sourceDirectory)};`
    },
  }
}

const config = defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const distributionBuild = env.SQLWARDEN_FRONTEND_DISTRIBUTION
  const outputDirectory = env.SQLWARDEN_FRONTEND_OUT_DIR || '../assets/static'
  return {
    plugins: [
      devtools(),
      tsconfigPaths({ projects: ['./tsconfig.json'] }),
      distributionTailwindSource(distributionBuild),
      tailwindcss(),
      tanstackRouter({ target: 'react', autoCodeSplitting: true }),
      viteReact(),
    ],
    resolve: {
      dedupe: ['react', 'react-dom', '@tanstack/react-query', '@tanstack/react-router'],
      alias: {
        ...(distributionBuild ? { '#/distribution/build': distributionBuild } : {}),
        '#': communitySource,
      },
    },
    build: { outDir: outputDirectory, emptyOutDir: true },
    server: {
      proxy: { '/api': { target: 'http://localhost:6020', changeOrigin: true } },
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
  }
})

export default config
