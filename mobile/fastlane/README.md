fastlane documentation
----

# Installation

Make sure you have the latest version of the Xcode command line tools installed:

```sh
xcode-select --install
```

For _fastlane_ installation instructions, see [Installing _fastlane_](https://docs.fastlane.tools/#installing-fastlane)

# Available Actions

## Android

### android test

```sh
[bundle exec] fastlane android test
```

Typecheck y pruebas unitarias de la app móvil

### android build

```sh
[bundle exec] fastlane android build
```

Bundle JS de producción con Expo (no requiere Android SDK)

### android ci

```sh
[bundle exec] fastlane android ci
```

Lane de CI: validación + bundle JS

### android ios

```sh
[bundle exec] fastlane android ios
```

Build nativo iOS (solo macOS con Xcode)

----

This README.md is auto-generated and will be re-generated every time [_fastlane_](https://fastlane.tools) is run.

More information about _fastlane_ can be found on [fastlane.tools](https://fastlane.tools).

The documentation of _fastlane_ can be found on [docs.fastlane.tools](https://docs.fastlane.tools).
