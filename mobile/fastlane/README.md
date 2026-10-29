# Fastlane — app móvil

Lanes disponibles:

| Lane | Entorno | Qué hace |
|---|---|---|
| `test` | Linux/macOS | `npm run typecheck` + `npm run test`. |
| `build` | Linux/macOS | `test` + `npx expo export --platform android` (bundle JS). |
| `ci` | Linux/macOS | Igual que `build`; es la lane que ejecuta GitHub Actions. |
| `ios` | **solo macOS + Xcode** | `expo prebuild` + `build_app` con firma. |

## Uso

```bash
cd mobile
bundle install
bundle exec fastlane ci
```

## Limitaciones

- El entorno de CI (Ubuntu) no tiene Xcode ni Android SDK: no se genera un
  binario nativo. `build`/`ci` validan TypeScript, pruebas y que Metro puede
  producir el bundle JS de producción, que es lo verificable sin SDK.
- La lane `ios` falla de forma explícita en Linux y no incluye credenciales de
  firma. Para publicar en App Store/Play Store hacen falta macOS, certificados,
  perfiles de aprovisionamiento y secretos del repositorio (no versionados).
- `Gemfile.lock` se genera con `bundle install`; commitearlo cuando se fije la
  versión de Fastlane para builds reproducibles.
