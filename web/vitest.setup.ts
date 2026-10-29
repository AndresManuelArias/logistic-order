import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// Mock de maplibre-gl antes de que se importe el componente
vi.mock('maplibre-gl', () => {
  const mockMap = {
    addControl: vi.fn(),
    on: vi.fn(),
    remove: vi.fn(),
    getStyle: vi.fn(() => ({ layers: [] })),
    getSource: vi.fn(() => null),
    removeSource: vi.fn(),
    removeLayer: vi.fn(),
    addSource: vi.fn(),
    addLayer: vi.fn(),
    fitBounds: vi.fn(),
    setStyle: vi.fn(),
  }

  return {
    Map: vi.fn(() => mockMap),
    Marker: vi.fn(() => ({
      setLngLat: vi.fn().mockReturnThis(),
      addTo: vi.fn().mockReturnThis(),
      remove: vi.fn(),
      getElement: vi.fn().mockReturnValue(document.createElement('div')),
    })),
    LngLat: vi.fn((lng: number, lat: number) => ({ lng, lat })),
    NavigationControl: vi.fn(() => ({})),
  }
})

// Limpia el DOM entre pruebas para evitar fugas de componentes montados.
afterEach(() => {
  cleanup()
})