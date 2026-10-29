# Análisis de cumplimiento — Prueba Técnica Senior Fullstack

**Documento evaluado:** `prueba-tecnica-senior-fullstack-nivel-ejecutivo.md`
**Repositorio:** Tiendas-Track (monolito modular Go + portal Next.js + app Expo)
**Fecha del análisis:** 2026-10-29
**Último commit observado:** `77f3f8b add run dev web`
**Alcance:** contraste requisito por requisito entre el enunciado (§4, §6 y §7) y el estado real del repositorio.

---

## 0. Método y límites del análisis

Este documento se basa en:

- Inspección del código y la configuración versionados (`backend/`, `web/`, `mobile/`, `docker-compose.yml`, `.github/workflows/`, `scripts/`, `backend/api/openapi.yaml`).
- Lectura de la documentación del proyecto (`README.md`, `doc/plan_desarrollo.md`, `web/README.md`, `mobile/README.md`, ADR en `doc/arquitectura/`).
- Historial de Git (`git log`, `git status`).

**Límites explícitos:**

- No se re-ejecutaron en esta revisión las suites ni los scripts e2e. Las cifras de pruebas que se citan provienen de la documentación del propio repositorio (`doc/plan_desarrollo.md`, `web/README.md`, `mobile/README.md`).
- No hay emulador, Android SDK ni Xcode en el entorno, por lo que no se validó compilación nativa, GPS real, permisos, cámara ni consumo de batería.
- El video de sustentación no está en el repositorio: no es verificable.
- No hay remoto Git configurado (`git remote -v` vacío), por lo que no se puede verificar la publicación en GitHub/GitLab.
- Al cierre de este análisis el árbol de trabajo no está limpio: hay cambios modificados en `web/` y el enunciado sin seguimiento.

**Criterio de clasificación:**

| Símbolo | Significado |
|---|---|
| ✅ **Cumplido** | Implementado y con evidencia verificable en código/configuración; validaciones registradas cuando aplica. |
| 🟡 **Parcial** | Existe una implementación real, pero no cubre el requisito literal o le faltan validaciones/partes. |
| 🟠 **Solo diseño** | Hay decisión, ADR o documento, pero no código ejecutable. |
| ❌ **No implementado** | No existe implementación ni diseño operativo. |
| ⚪ **No verificable** | El artefacto está fuera del repositorio o depende de un entorno no disponible. |

---

## 1. Veredicto ejecutivo

El repositorio contiene un **MVP funcional y coherente del núcleo operativo**: autenticación con RBAC, ciclo de vida de órdenes, ingesta de telemetría idempotente, persistencia temporal/geoespacial, portal multi-rol con tiempo real y una app móvil offline-first con cola de sincronización. La calidad de la implementación y la documentación es alta para las fases F1–F4.

Sin embargo, **no cubre los diferenciales que la prueba evalúa explícitamente**: agente de IA operativo, bus de eventos distribuido, pruebas de caos/carga, IaC, CI/CD móvil y la auditoría del uso de IA. El proyecto está, por su propio plan, en **F4 de 6 fases**; F5 (IA + resiliencia transversal) y F6 (carga, IaC, CI/CD y auditoría) están pendientes.

| Dimensión del enunciado | Nivel observado | Diagnóstico en una línea |
|---|---|---|
| **A. Ingesta orientada a eventos** | 🟡 Medio | Persistencia especializada sólida; el bus es en memoria y la ingesta no es asíncrona; el circuit breaker se aplica a la BD, no entre servicios. |
| **B. Desarrollo agéntico (IA)** | ❌ Nulo | No hay agente ni endpoint; solo un placeholder de UI y un diseño en F5/ADR-07. |
| **C. Portal corporativo** | 🟡 Alto | SPA, mapa y WebSocket funcionan; el chat de IA no es funcional y no hay módulo explícito de alertas en tiempo real. |
| **D. Ecosistema móvil** | 🟡 Alto / ❌ en CI-CD | Offline-first implementado y probado en lógica; CI/CD móvil inexistente y validación nativa pendiente. |
| **E. Infraestructura, caos y testing** | 🟡 Bajo | Docker Compose cumple; no hay k6, inyección de caos ni IaC. |
| **Entregable 1. Repositorio Git** | 🟡 Parcial | Historial estructurado por fases, pero sin remoto y con cambios sin commitear al momento del análisis. |
| **Entregable 2. README con ejecución e IaC** | 🟡 Parcial | Ejecución documentada y detallada; IaC ausente. |
| **Entregable 3. Auditoría de IA** | ❌ Nulo | No existe la sección exigida en el README. |
| **Entregable 4. Video de sustentación** | ⚪ No verificable | No está en el repositorio. |

**Conclusión anticipada:** el sistema demuestra capacidad de construcción fullstack y diseño offline-first, pero **no demuestra todavía** los aspectos que la prueba pondera como “nivel ejecutivo”: orquestación de IA, auditoría crítica del código generado, resiliencia distribuida y automatización de infraestructura/despliegue.

---

## 2. Matriz de cumplimiento — Bloque A: Ingesta orientada a eventos

| # | Requisito literal | Estado | Evidencia | Brecha / observación |
|---|---|---|---|---|
| A.1 | Flujo de ingesta **asíncrona** con bus de eventos (Kafka, RabbitMQ) para procesamiento desacoplado | 🟡 **Parcial** | `backend/internal/shared/eventbus/bus.go`: pub/sub en memoria con `Go channels`, buffer 256 y descarte contabilizado. `backend/internal/telemetry/usecase/telemetry.go`: `Ingest`/`IngestBulk` **persisten primero** en PostgreSQL y después publican el evento; el bus alimenta el fan-out de WebSocket (`shared/websocket/hub.go:218`). | No hay broker externo, ni tópicos persistentes, ni consumidores independientes, ni reintentos/replay. La ingesta sigue el camino síncrono HTTP → validación → INSERT → publicación en memoria. El desacoplamiento existe **dentro** del proceso, no como pipeline asíncrono. La decisión está justificada en `doc/plan_desarrollo.md:76-77` (ADR-01) por la ventana de 8–12 h y el entorno local, pero el criterio de evaluación §6 pide “uso correcto de eventos (Kafka)”. |
| A.2 | Persistencia especializada para miles de dispositivos, justificada (series de tiempo) | ✅ **Cumplido** | `backend/migrations/0004_telemetry.sql`: `create_hypertable('telemetry.points', 'time')`, `add_retention_policy(... '90 days')`, agregado continuo `telemetry.stops` con política de refresco. `0001_extensions.sql`: PostGIS; coordenadas `geometry(Point,4326)` con índice GiST. Justificación en `doc/plan_desarrollo.md` (ADR-02, ADR-09) y `README.md` §Base de datos. | No se usa Cassandra ni Druid, pero el enunciado los presenta como ejemplos y permite elegir stack. La justificación de TimescaleDB está documentada. Falta evidencia de particionado/compresión o prueba de volumen real. |
| A.3 | **Circuit Breakers** en la comunicación entre microservicios | 🟡 **Parcial** | `backend/internal/shared/resilience/breaker.go` implementa `gobreaker` con estados y error tipado. Se aplica a lecturas de BD en `telemetry/usecase/fleet.go:50` y `telemetry/usecase/telemetry.go:227,250`; el handler traduce a HTTP en `telemetry/delivery/http.go:295`. | No hay microservicios ni llamadas entre servicios que proteger. El breaker protege la BD, no la comunicación distribuida. Es una aplicación válida del patrón, pero **no cumple el requisito literal**. F5 propone extenderlo al proveedor LLM (`doc/plan_desarrollo.md:362`). |

**Lectura arquitectónica:** el monolito modular es una decisión defendible y está documentada (ADR-01), pero el diseño de microservicios con Kafka existe solo como “estado futuro” en `doc/arquitectura/microservicios/`. Para la evaluación, conviene explicar con claridad qué se desacopló realmente y qué no.

---

## 3. Matriz de cumplimiento — Bloque B: Desarrollo agéntico (IA)

| # | Requisito literal | Estado | Evidencia | Brecha / observación |
|---|---|---|---|---|
| B.1 | Agente operativo integrado en el backend (LangChain, Semantic Kernel, etc.) que consuma el estado de los vehículos | ❌ **No implementado** | No existe módulo `internal/aiagent`, ni dependencias de LLM en `backend/go.mod`, ni endpoint de IA en `backend/api/openapi.yaml`. El diseño está en `doc/arquitectura/monolito_modular/arquitectura_monolito_modular.md` (módulo `internal/aiagent`) y las tareas en `doc/plan_desarrollo.md:354-363` (F5). | No hay catálogo de herramientas, prompt de sistema, integración con proveedor, ni auditoría de consultas. |
| B.2 | Responder consultas en lenguaje natural sobre el estado actual (ej. vehículos detenidos > 20 min en zonas críticas) | ❌ **No implementado** | `web/apps/portal/src/components/ai-assistant.tsx` es un panel provisional: llama a `api.queryAi(text)`, espera un 404 y muestra que la función pertenece a F5. No inventa respuestas. | No hay backend que resuelva la consulta. La UI por sí sola no satisface el requisito. |
| B.3 | (Derivado) Trazabilidad/auditoría de las consultas del agente | ❌ **No implementado** | Solo está previsto en el plan: “Endpoint `POST /ai/query` con auditoría de cada consulta (pregunta, herramienta, params, latencia)” (`doc/plan_desarrollo.md:361`). | Sin implementación. |

**Impacto:** es la brecha más visible frente al título de la prueba (“Telemetría y Desarrollo Agéntico Extremo”) y frente a la dimensión “Arquitectura agéntica” de §6.

---

## 4. Matriz de cumplimiento — Bloque C: Portal corporativo

| # | Requisito literal | Estado | Evidencia | Brecha / observación |
|---|---|---|---|---|
| C.1 | SPA que consuma datos procesados | ✅ **Cumplido** | `web/apps/portal` con Next.js 16 + React 19 (App Router), paneles `/proveedor`, `/tienda`, `/conductor` y vista pública `/seguimiento/[orderId]`. Cliente tipado generado desde OpenAPI (`web/packages/tracking-core/src/api`). | — |
| C.2 | Tiempo real vía WebSocket o SSE | ✅ **Cumplido** | `web/packages/tracking-core/src/hooks/use-tracking.ts`: suscripción WS con canales, backoff, dedupe y descarte de eventos obsoletos. Backend `shared/websocket/hub.go` con autorización por canal y revalidación de concesiones. Evidencia registrada: e2e F3 63/63 y latencia WS de 4,3 ms (`doc/plan_desarrollo.md:275`). | Solo WebSocket; no hay fallback SSE implementado (ADR-03 lo menciona como fallback). No es un requisito si WS funciona. |
| C.3 | Mostrar **mapa** | ✅ **Cumplido** | `web/packages/tracking-ui/src/components/tracking-map.tsx`: mapa SVG accesible con traza y selección de vehículo; integrado en `provider-dashboard.tsx`. | Es un mapa esquemático, no un proveedor de mapas real (Mapbox/Leaflet). El enunciado no exige proveedor, pero conviene declararlo. |
| C.4 | **Alertas en tiempo real** | ❌ **No implementado (explícito)** | No existe centro de alertas, feed de notificaciones ni reglas de alerta. Las únicas coincidencias de “alert” son `role="alert"` de formularios y mensajes de error. | El dashboard muestra estado de flota, pero no alertas proactivas (detenciones prolongadas, desvíos, zonas críticas, retrasos). Es un requisito literal de §4.C. |
| C.5 | **Chat con la IA** | 🟡 **Parcial** | `ai-assistant.tsx` renderiza historial, input, estados de carga y error; se integra en el portal. | El backend no expone `POST /api/v1/ai/query`; la UI queda inoperativa por diseño (404 controlado). No cumple funcionalmente. |
| C.6 | Multi-rol y aislamiento de datos | ✅ **Cumplido** | Paneles por rol, guardas (`require-role.tsx`), BFF con cookies `httpOnly`, RBAC server-side con scope. Evidencia: e2e F2 72/72 y F3 63/63; 58 pruebas web. | — |

---

## 5. Matriz de cumplimiento — Bloque D: Ecosistema móvil

| # | Requisito literal | Estado | Evidencia | Brecha / observación |
|---|---|---|---|---|
| D.1 | App móvil (React Native u otro) que envíe coordenadas | ✅ **Cumplido (lógica)** | `mobile/` con Expo SDK 54 + RN 0.81. Captura GPS en segundo plano (`expo-location` + `expo-task-manager`, foreground service Android), políticas 15 s/25 m y filtro de precisión (`mobile/src/config/policies.ts`, `infrastructure/device/location-task.ts`). | No validado en dispositivo/emulador real. |
| D.2 | Persistencia local si se pierde la red (SQLite, WatermelonDB) | ✅ **Cumplido** | WatermelonDB/SQLite (`mobile/src/infrastructure/persistence/watermelon/*`), cola write-ahead de puntos y transiciones, aislamiento por `owner_id`. | — |
| D.3 | Sincronización en bloque al reconectar | ✅ **Cumplido** | `application/sync/point-sync-service.ts`, `sync-coordinator.ts`, `backoff.ts`; detección NetInfo; lote con `Idempotency-Key`; cuarentena sin borrar datos. Idempotencia validada contra backend real: misma clave → `already_processed: true`; misma carga con clave nueva → `duplicates: 2` (`doc/plan_desarrollo.md:339`). | La sincronización de transiciones se reconcilia por `GET`; correcto, pero no hay prueba E2E en dispositivo. |
| D.4 | **CI/CD móvil** (Fastlane, GitHub Actions) | ❌ **No implementado** | `.github/workflows/ci.yml` solo tiene jobs `backend` y `compose`. No existe `fastlane/`, ni workflow de build Android/iOS, ni job de la app móvil. | Brecha literal. No hay firma, artefacto APK/IPA, distribución ni automatización de versionado. |
| D.5 | (Derivado) Prueba de entrega (POD) | 🟡 **Parcial** | POD local implementado (`domain/delivery-proof.ts`, `proof-file.store.ts`, UI en `OrderDetailScreen.tsx`). El backend no expone endpoint de POD, y la UI lo declara. | No es requisito del enunciado, pero limita el valor end-to-end de la app. |

**Validaciones registradas en el repo (no re-ejecutadas aquí):** 99/99 pruebas Vitest, `tsc --noEmit`, `expo install --check`, `expo export --platform android` y `expo config` en verde (`mobile/README.md:258-277`, `doc/plan_desarrollo.md:339`).

---

## 6. Matriz de cumplimiento — Bloque E: Infraestructura, caos y testing

| # | Requisito literal | Estado | Evidencia | Brecha / observación |
|---|---|---|---|---|
| E.1 | Script de carga (k6, JMeter) con cientos de vehículos | ❌ **No implementado** | No hay archivos k6/JMeter ni scripts de carga en el repositorio. `doc/plan_desarrollo.md:370` lo deja para F6. | Brecha literal. |
| E.2 | Inyectar **10 % de peticiones duplicadas** y **5 % de errores** | ❌ **No implementado** | No existe escenario de caos. El plan de F6 menciona “5% duplicados, errores inyectados” (`doc/plan_desarrollo.md:370`), lo que además **no coincide** con el 10 % de duplicados del enunciado. | Brecha literal y desalineación de porcentajes con el enunciado. |
| E.3 | Docker Compose para ejecución local orquestada | ✅ **Cumplido** | `docker-compose.yml`: `db` (TimescaleDB-HA + PostGIS), `backend`, `web`, perfil `seed`, healthchecks y volúmenes. CI valida `docker compose config -q`. README documenta `make dev`, `make up`, `make db-up`, `make seed`. | — |
| E.4 | Scripts de IaC (Terraform, AWS CDK) | ❌ **No implementado** | No hay archivos `.tf`, `.tfvars`, `cdk.json` ni equivalentes. `doc/plan_desarrollo.md:371` lo deja para F6. | Brecha literal. El README solo documenta ejecución local. |
| E.5 | (Derivado) CI de calidad | 🟡 **Parcial** | `.github/workflows/ci.yml`: `gofmt`, `go vet`, `go test -race`, `go build` y `docker compose config -q`. | No hay jobs para web (lint/test/build), móvil (typecheck/test/bundle), lint de OpenAPI/Spectral ni pruebas de contrato. |

---

## 7. Entregables de la sección 7

| # | Entregable | Estado | Evidencia | Brecha |
|---|---|---|---|---|
| 1 | **Repositorio Git con historial de commits estructurado** | 🟡 **Parcial** | 20 commits visibles con secuencia por fases (`init F1` → `terminate fase 1` → `init f2` → `terminate f2` → `init f4` → `develoment mobile in fase F4` → `add run dev web`). 302 archivos versionados. | No hay remoto configurado, por lo que no se verifica la publicación pública exigida. El árbol tiene cambios sin commitear (`web/package-lock.json`, `web/package.json`, `web/packages/tracking-ui/*`) y el enunciado sin seguimiento. Hay un typo en el mensaje `develoment`. No se observa una convención estricta (Conventional Commits) ni tags. |
| 2 | **README con instrucciones de ejecución e IaC** | 🟡 **Parcial** | `README.md` documenta stack, requisitos, inicio rápido, auth/RBAC, telemetría, WS, portal, app móvil, seed, comandos, estructura, base de datos, estado de fases y problemas comunes. | **No incluye IaC** porque no existe. La parte de ejecución está bien cubierta. |
| 3 | **Auditoría de IA en el README** (mínimo dos decisiones donde el IDE agéntico sugirió algo deficiente/inseguro/no escalable y cómo se corrigió) | ❌ **No implementado** | `README.md` no contiene sección de auditoría de IA. `doc/plan_desarrollo.md:461` mantiene el checklist sin marcar: “El README incluye decisiones de arquitectura y auditoría del uso de la IA”. | Brecha **fundamental** según el enunciado. No basta con decir que se usó IA: hay que documentar casos concretos, el defecto sugerido, el criterio aplicado y el resultado. |
| 4 | **Video de sustentación (5–10 min, YouTube no listado)** | ⚪ **No verificable** | No hay video ni enlace en el repositorio. | Debe aportarse fuera del repo. El enunciado también pide 2 minutos sobre configuración del entorno agéntico (`.cursorrules`, prompts, contexto). |

---

## 8. Contraste con los criterios de evaluación (§6)

| Dimensión | Expectativa | Estado real | Comentario |
|---|---|---|---|
| **Arquitectura agéntica** | Auditar y refactorizar código generado por IA hacia Clean Architecture y resiliencia | 🟡 **Parcial, no evidenciado** | El código muestra Clean Architecture por módulos (domain/application/infrastructure/delivery), interfaces, tests y ADR. Hay señales de corrección real (p. ej. el DTO `TokenPair` que filtraba `PasswordHash`, documentado en `doc/plan_desarrollo.md:338`). Pero **no hay auditoría escrita** que conecte esos episodios con el uso de IA, que es lo que se evalúa. |
| **Manejo de datos** | Uso correcto de eventos (Kafka) y justificación de BD para alta frecuencia | 🟡 **Parcial** | TimescaleDB + PostGIS está bien justificado e implementado (hypertable, retención, agregados). Kafka no existe; el bus en memoria está justificado por tiempo/entorno, pero no demuestra ingesta desacoplada ni escalabilidad horizontal. |
| **Mobile & Edge** | Offline-first sólido + automatización de despliegues | 🟡 **Parcial** | Offline-first es la parte más fuerte del repo: WatermelonDB, cola write-ahead, idempotencia, backoff, reconciliación y 99 pruebas. CI/CD móvil es inexistente. |

---

## 9. Fortalezas verificables (lo que sí está bien)

1. **Núcleo backend coherente y probado.** Monolito modular con separación por capas, RBAC server-side, JWT con refresh rotativo, máquina de estados transaccional y auditoría de transiciones en `app.order_events`. Evidencia: e2e F2 72/72 y F3 63/63.
2. **Persistencia adecuada al dominio.** TimescaleDB con hypertable, retención de 90 días, agregado continuo de paradas y PostGIS para coordenadas; migraciones versionadas y testeadas.
3. **Ingesta idempotente real.** `Idempotency-Key` + `sync_receipts` + PK compuesta; validado contra el backend con duplicados reales. Es exactamente el tipo de garantía que un sistema móvil offline-first necesita.
4. **Tiempo real con control de acceso.** WebSocket con autorización por canal, revalidación de concesiones, cierre por expiración de token y por consumidor lento; no es un “broadcast abierto”.
5. **Portal multi-rol con BFF seguro.** Access token en memoria, refresh en cookie `httpOnly`, proxy same-origin, guardas por rol y cliente tipado desde OpenAPI.
6. **App móvil con arquitectura limpia y offline-first maduro.** Puertos/adaptadores, cola write-ahead, backoff con jitter, cuarentena, SecureStore, POD local y UI completa.
7. **Documentación operativa por encima de la media.** README con comandos, credenciales demo, ejemplos curl, estado de fases, problemas comunes y ADR.

---

## 10. Brechas priorizadas y plan de cierre

La prioridad se basa en el impacto sobre la evaluación y en la dependencia entre brechas. No se asignan horas porque no se dispone de una estimación validada.

### P0 — Sin esto, la prueba no se considera completa

| Brecha | Entregable mínimo | Criterio de aceptación | Evidencia a adjuntar |
|---|---|---|---|
| **Agente IA operativo (B)** | Módulo `internal/aiagent` + `POST /api/v1/ai/query` + catálogo de herramientas (`get_stopped_vehicles`, `get_order_status`, `get_fleet_eta`, `get_late_deliveries`) + proveedor LLM configurable por env + prompt con reglas de no invención + auditoría de consultas. Actualizar `openapi.yaml`, conectar `ai-assistant.tsx` y añadir pruebas. | “¿Qué vehículos llevan más de 20 minutos detenidos?” devuelve datos reales citando la herramienta usada. Sin API key → error controlado (503/422), no 500. Cada consulta queda auditada (pregunta, herramienta, parámetros, latencia, resultado). | Test de integración con proveedor mockeado + prueba manual documentada + entrada OpenAPI + captura del chat respondiendo. |
| **Auditoría de IA en README (Entregable 3)** | Sección nueva con **al menos dos** casos concretos: propuesta inicial del IDE agéntico, por qué era deficiente/insegura/no escalable, criterio aplicado y refactor final. | Casos verificables contra el código (antes/después) y con enlace a commits o archivos. | Dos o tres casos: p. ej. fuga de `PasswordHash` en DTO, bus en memoria vs. Kafka, o breaker en lecturas de BD. |
| **k6 con caos (E.1/E.2)** | Script k6 con 100+ vehículos virtuales, **10 % duplicados** y **5 % errores**, umbrales p95 de ingesta y WS, y reporte de resultados. | El script corre contra Docker Compose, respeta los porcentajes del enunciado y publica resultados (p95, error rate, duplicados). | Script + salida de ejecución + conclusión (cuello de botella y decisión). |
| **CI/CD móvil (D.4)** | Workflow de GitHub Actions para `typecheck`, tests, bundle y build Android (idealmente con Fastlane); documentar firma y artefacto. | Pipeline verde que produce un APK (o AAB) descargable como artefacto; iOS puede quedar documentado si no hay macOS. | Workflow + artefacto + instrucciones en README. |
| **IaC (E.4)** | Terraform o AWS CDK mínimo: red, base de datos, servicio de contenedores, secretos y variables. | `terraform plan` (o `cdk synth`) sin errores y README con el procedimiento. | Código IaC + salida de `plan`/`synth` + diagrama de despliegue. |

### P1 — Cierra requisitos literales y mejora la demostración

| Brecha | Entregable mínimo | Criterio de aceptación |
|---|---|---|
| **Alertas en tiempo real (C.4)** | Reglas de detención prolongada/desvío/retraso + feed de alertas en el portal, alimentado por WebSocket o por consulta periódica. | Una detención simulada genera una alerta visible en el dashboard sin recargar. |
| **Kafka o alternativa justificada (A.1)** | Opción 1: añadir Redpanda/Kafka al Compose, publicar telemetría a un tópico y consumirla para persistencia/fan-out. Opción 2: si se mantiene el bus en memoria, escribir un ADR con análisis explícito de límites y un plan de extracción. | Opción 1: evidencia de productor/consumidor y replay. Opción 2: ADR aprobado y honesto sobre lo que no se cumple. |
| **Circuit Breaker en la dimensión correcta (A.3)** | Aplicar breaker a la llamada al LLM y a cualquier integración HTTP externa; documentar que no hay microservicios. | Fallo del proveedor LLM → respuesta controlada sin degradar el resto del backend. |
| **Video de sustentación (Entregable 4)** | Video de 5–10 min con arquitectura, demo y 2 min de entorno agéntico. | Enlace no listado en el README. |

### P2 — Calidad y consistencia

| Brecha | Acción |
|---|---|
| **Git** | Configurar remoto público, commitear cambios pendientes, adoptar Conventional Commits y tags por fase; corregir el typo `develoment` en el historial visible (o dejarlo documentado). |
| **CI web** | Añadir job de `npm ci`, lint, test, typecheck y build del portal. |
| **Contrato** | Añadir lint de OpenAPI (Spectral) y verificación de que los tipos generados están sincronizados (`npm run check:api` ya existe en móvil; replicarlo en web). |
| **Validación móvil nativa** | Probar en emulador/dispositivo: permisos, GPS en segundo plano, cámara, batería y sync real. Documentar resultados. |
| **POD end-to-end** | Endpoint de prueba de entrega en backend y subida de evidencia; hoy es solo local. |
| **Observabilidad** | Verificar métricas/trazas expuestas y documentar dashboards o al menos consultas de ejemplo. |

---

## 11. Riesgos y interpretaciones que conviene declarar en la sustentación

1. **Monolito vs. microservicios.** El enunciado pide circuit breakers “entre microservicios”, pero también permite elegir stack. La decisión de monolito modular está justificada (ADR-01). El riesgo es que el evaluador lo lea como ausencia de resiliencia distribuida. Mitigación: explicar qué se protegió (BD, y próximamente LLM), qué se desacopló (bus interno) y qué haría falta para extraer telemetría a un servicio.
2. **Bus en memoria vs. Kafka.** El bus actual no es un broker: no persiste, no hay replay ni consumidores independientes. Si no se añade Kafka, la justificación debe ser explícita y acompañada de una demostración de que el fan-out y la ingesta cumplen los objetivos de latencia. No conviene presentarlo como “equivalente a Kafka”.
3. **“Tiempo real” del portal.** El WebSocket funciona y tiene evidencia de latencia, pero las alertas proactivas no existen. Conviene no afirmar que el dashboard “alerta en tiempo real” sin implementar C.4.
4. **Cifras de pruebas.** Las cifras 72/72, 63/63, 58 y 99 provienen de la documentación y de ejecuciones previas. En la sustentación deben reproducirse o adjuntarse como evidencia, no solo citarse.
5. **Video y repositorio público.** Son entregables externos al código. Sin ellos, la evaluación queda incompleta aunque el MVP funcione.

---

## 12. Checklist de cierre

- [ ] Agente IA implementado y endpoint `POST /api/v1/ai/query` documentado en OpenAPI.
- [ ] Consultas del agente auditadas (pregunta, herramienta, parámetros, latencia, resultado).
- [ ] Chat del portal conectado y probado contra el backend real.
- [ ] Alertas en tiempo real implementadas y demostradas.
- [ ] Script k6 con 100+ vehículos, 10 % duplicados y 5 % errores, con umbrales y resultados.
- [ ] Decisión sobre Kafka/Redpanda tomada e implementada o justificada en ADR.
- [ ] Circuit breaker aplicado a integraciones externas/LLM.
- [ ] IaC (Terraform o CDK) con `plan`/`synth` exitoso y documentado.
- [ ] Workflow móvil con build de Android y artefacto descargable.
- [ ] CI con jobs de web y validación de contrato.
- [ ] README con sección de auditoría de IA (mínimo dos casos concretos).
- [ ] Video de 5–10 min publicado y enlazado.
- [ ] Repositorio público con historial limpio y cambios commiteados.

---

## 13. Conclusión

El repositorio demuestra un **trabajo de ingeniería real y de buena calidad en el núcleo del producto**: la arquitectura modular, la persistencia temporal/geoespacial, la idempotencia de la ingesta, el tiempo real autorizado y la app offline-first son sólidos y están documentados. Es un MVP que funciona y que resiste una revisión técnica seria.

Frente al enunciado, sin embargo, el cumplimiento es **incompleto y desigual**:

- **Lo logrado:** persistencia especializada, Docker Compose, portal SPA con mapa y WebSocket, app móvil offline-first con sincronización idempotente, RBAC y documentación operativa.
- **Lo parcial:** bus de eventos (en memoria), circuit breaker (aplicado a BD), chat de IA (solo UI), CI (solo backend), repositorio (sin remoto ni árbol limpio) y README (sin IaC).
- **Lo no logrado:** agente IA operativo, auditoría de IA, alertas en tiempo real, k6/caos, IaC y CI/CD móvil.
- **Lo no verificable:** video de sustentación y validación nativa del móvil.

La prioridad para cerrar la brecha no es añadir más funcionalidades CRUD, sino **completar F5 y F6**: un agente IA con herramientas y auditoría, la auditoría escrita del uso de IA, el escenario de carga/caos, la IaC y el pipeline móvil. Con esos entregables, el repositorio pasaría de “MVP bien construido” a una respuesta completa al nivel ejecutivo que la prueba solicita.
