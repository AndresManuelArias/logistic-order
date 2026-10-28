import { fileURLToPath } from 'node:url'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// Pruebas de las librerías compartidas y de los componentes del portal.
// El entorno jsdom permite montar componentes React sin navegador. El alias
// replica el `paths` de apps/portal/tsconfig.json.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./apps/portal/src', import.meta.url)),
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./vitest.setup.ts'],
    include: ['packages/**/*.test.{ts,tsx}', 'apps/**/*.test.{ts,tsx}'],
    restoreMocks: true,
    clearMocks: true,
  },
})
