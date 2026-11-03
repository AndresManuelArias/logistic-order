import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'

// Mock de maplibre-gl antes de que se importe el componente.
// Los constructores se declaran como funciones normales (no flecha) porque el
// componente los invoca con `new`; una arrow function no es construible.
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
    easeTo: vi.fn(),
    setStyle: vi.fn(),
    setWorkerUrl: vi.fn(),
  }

  function MockMap() {
    return mockMap
  }

  function MockMarker() {
    return {
      setLngLat: vi.fn().mockReturnThis(),
      addTo: vi.fn().mockReturnThis(),
      remove: vi.fn(),
      getElement: vi.fn().mockReturnValue(document.createElement('div')),
    }
  }

  function MockLngLat(lng: number, lat: number) {
    return { lng, lat }
  }

  function MockNavigationControl() {
    return {}
  }

  return {
    Map: vi.fn(MockMap),
    setWorkerUrl: vi.fn(),
    Marker: vi.fn(MockMarker),
    LngLat: vi.fn(MockLngLat),
    NavigationControl: vi.fn(MockNavigationControl),
  }
})

// Limpia el DOM entre pruebas para evitar fugas de componentes montados.
afterEach(() => {
  cleanup()
})
