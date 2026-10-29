import * as maplibregl from 'maplibre-gl'

export const MAP_STYLE_URL = 'https://tiles.openfreemap.org/styles/liberty'
export const MAP_WORKER_URL = '/maplibre-gl-worker.mjs'

export function createTrackingMap(container: HTMLElement): maplibregl.Map {
  maplibregl.setWorkerUrl(MAP_WORKER_URL)

  return new maplibregl.Map({
    container,
    style: MAP_STYLE_URL,
    center: [-74.08, 4.61],
    zoom: 12,
  })
}
