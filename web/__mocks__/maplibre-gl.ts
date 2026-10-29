import { vi } from 'vitest'

// Mock de maplibre-gl para tests sin WebGL
const createMockMap = () => ({
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
})

// Export para uso directo
export const mockMap = createMockMap()

// Mock del módulo maplibre-gl
export const Map = vi.fn().mockImplementation(() => createMockMap())
export const Marker = vi.fn().mockImplementation(() => ({
  setLngLat: vi.fn().mockReturnThis(),
  addTo: vi.fn().mockReturnThis(),
  remove: vi.fn(),
  getElement: vi.fn().mockReturnValue(document.createElement('div')),
}))
export const LngLat = vi.fn().mockImplementation((lng: number, lat: number) => ({ lng, lat }))
export const NavigationControl = vi.fn().mockImplementation(() => ({}))

// Default export para compatibilidad con import * as
export default {
  Map,
  Marker,
  LngLat,
  NavigationControl,
}