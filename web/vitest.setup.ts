import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// Limpia el DOM entre pruebas para evitar fugas de componentes montados.
afterEach(() => {
  cleanup()
})
