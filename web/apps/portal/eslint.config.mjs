import { defineConfig } from 'eslint/config'
import nextVitals from 'eslint-config-next/core-web-vitals'
import nextTypeScript from 'eslint-config-next/typescript'

// Configuración plana de Next 16 (eslint-config-next ya exporta arrays).
export default defineConfig([
  ...nextVitals,
  ...nextTypeScript,
  {
    ignores: ['.next/**', 'node_modules/**', 'next-env.d.ts', 'next.config.ts'],
  },
  {
    rules: {
      '@typescript-eslint/no-explicit-any': 'error',
      'react-hooks/exhaustive-deps': 'warn',
    },
  },
])
