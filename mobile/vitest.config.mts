import { defineConfig } from 'vitest/config'

// La app usa Metro para compilar UI y módulos nativos. Vitest sólo ejecuta
// lógica pura (dominio + aplicación) con adaptadores en memoria: no importa
// `react-native` ni `@nozbe/watermelondb` para evitar dependencias nativas.
export default defineConfig({
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
    reporters: 'default',
  },
})
