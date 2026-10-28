// WatermelonDB usa decoradores legacy (`@field`, `@date`, `@relation`).
// El plugin debe ejecutarse antes que los de class properties de babel-preset-expo.
module.exports = function (api) {
  api.cache(true)
  return {
    presets: [['babel-preset-expo', { jsxRuntime: 'automatic' }]],
    plugins: [['@babel/plugin-proposal-decorators', { legacy: true }]],
  }
}
