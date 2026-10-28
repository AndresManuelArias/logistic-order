import { registerLocationTask, setLocationTaskHandler } from '../infrastructure/device/location-task'
import { getServices } from './services'

// Efectos de módulo: deben ejecutarse al cargar el bundle, antes de montar
// React, para que el sistema operativo encuentre la tarea registrada incluso en
// un arranque headless (app cerrada + actualización de ubicación).
registerLocationTask()

setLocationTaskHandler(async (samples) => {
  const services = await getServices()
  await services.handleBackgroundSamples(samples)
})
