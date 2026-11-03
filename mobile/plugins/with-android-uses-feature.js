// Declara la cámara como hardware opcional en el manifest de Android.
//
// La app usa la cámara sólo para la foto de entrega (POD) y siempre permite
// elegir de la galería, así que funciona en dispositivos sin cámara. Sin
// <uses-feature ... required="false">, `lint` falla con
// PermissionImpliesUnsupportedChromeOsHardware y Play/ChromeOS asumen que el
// hardware es obligatorio.
//
// `android/` se genera con `expo prebuild`, por eso el cambio vive en un config
// plugin y no en el manifest a mano.
const { withAndroidManifest } = require('@expo/config-plugins')

const CAMERA_FEATURE = 'android.hardware.camera'

module.exports = function withAndroidUsesFeature(config) {
  return withAndroidManifest(config, (config) => {
    const manifest = config.modResults.manifest
    const features = (manifest['uses-feature'] = manifest['uses-feature'] ?? [])

    const exists = features.some((feature) => feature?.$?.['android:name'] === CAMERA_FEATURE)
    if (!exists) {
      features.push({ $: { 'android:name': CAMERA_FEATURE, 'android:required': 'false' } })
    }

    return config
  })
}
