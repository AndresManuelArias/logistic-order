¡Es una estrategia brillante! Matar dos pájaros de un tiro: resolver una prueba técnica de nivel senior con estándares de élite y, al mismo tiempo, construir una pieza arquitectónica modular (un **Módulo de Logística, Trazabilidad y Telemetría en Tiempo Real**) que encaja perfectamente en el núcleo de tu plataforma POS para tenderos en Latinoamérica.



---

## 1. Alineación del Producto: El Módulo "Tiendas-Track" (Logística Multi-Rol)

Para que sirva tanto para la prueba como para tu ecosistema, el sistema debe concebirse como un **Microservicio de Logística Agnóstico** y una **Librería Frontend Plug-and-Play**.

### Gestión de Roles y Permisos (RBAC con Aislamiento de Datos)

El sistema debe filtrar la telemetría según quién esté consultando mediante tokens JWT enriquecidos con roles:

* **Proveedor:** Vista global de toda su flota de vehículos de distribución B2B hacia los tenderos.
* **Tendero (Tienda de Barrio):** Vista dual. Puede ver la flota que le trae mercancía desde sus proveedores y la flota (domiciliarios propios o aliados) que lleva los pedidos a sus clientes finales.
* **Cliente Final (Consumidor):** Acceso acotado y por tiempo limitado (mediante un token de invitado o enlace seguro) exclusivamente al vehículo que transporta su orden específica.

---

## 2. Arquitectura Backend (Golang + Microservicio Independiente)

Para cumplir con la prueba técnica y tu producto, el backend en Go debe seguir **Clean Architecture** (Domain, Use Cases, Interfaces, Infrastructure):

* **Ingesta de Alta Concurrencia:** Utilizar un bus de eventos (Kafka o una alternativa más ligera como RabbitMQ/NATS si el entorno local es limitado). Los dispositivos GPS de los vehículos o la app móvil del conductor envían coordenadas continuas.


* **Persistencia Temporal:** **TimescaleDB** (sobre PostgreSQL) para almacenar series de tiempo de geolocalización de manera eficiente.


* **Comunicación en Tiempo Real:** Servidor WebSocket o SSE (Server-Sent Events) en Go para despachar la posición exacta del vehículo hacia el frontend sin latencia.


* **Resiliencia:** Implementar *Circuit Breakers* (con librerías como `sony/gobreaker`) en la comunicación con bases de datos o servicios externos.



---

## 3. Librería Frontend y Mobile (React / React Native)

La clave para que sea "una pieza que se pueda integrar a cualquier otro desarrollo" es aislar los componentes visuales y la lógica de conexión.

* **Arquitectura de Librería:** Organizar el proyecto frontend como un monorepo (usando Turborepo o npm workspaces) donde exista un paquete `@tiendas/tracking-core` (Hooks de conexión WebSocket, manejo de estados de la orden) y `@tiendas/tracking-ui` (Componentes visuales de mapas con Leaflet o Mapbox, tarjetas de estado, barras de progreso del pedido).
* **Aplicación Móvil (Offline-First):** La app del domiciliario del tendero debe usar **WatermelonDB** o SQLite local. Si el repartidor entra a una zona de baja señal en la ciudad, las coordenadas GPS se guardan localmente y se sincronizan en bloque (Bulk Sync) tan pronto se recupera la conexión.


* **Frontend de Demostración:** Una SPA en React/Next.js que simule el portal del proveedor, el panel del tendero y la vista pública del cliente final usando la misma librería.

---

## 4. Agente de IA Integrado (El valor diferencial de la prueba)

La prueba exige un **Agente Operativo** conectado al backend. En lugar de consultar solo datos abstractos de flotas, adapta el agente para que responder preguntas de negocio de la tienda:

* *Ejemplo de prompt al agente:* "¿Qué pedidos del proveedor de abarrotes llevan más de 20 minutos detenidos cerca de la zona norte?"
* *Implementación:* El agente traduce lenguaje natural a consultas SQL optimizadas sobre TimescaleDB o llama a endpoints internos del microservicio para devolver métricas de entrega en tiempo real.

---

## 5. Estrategia de Ejecución en Tiempo Récord (8 - 12 Horas)

Dado que las pruebas de este nivel asumen el uso de **IDEs agénticos (Cursor, Windsurf, Copilot)**, te sugiero este plan de ataque:

1. **Hora 1-3 (Esqueleto y Contratos):** Usa la IA para generar la estructura limpia en Go (Clean Architecture), los contratos de API y el esquema inicial de TimescaleDB con Docker Compose.
2. **Hora 4-7 (Core de Telemetría y WebSockets):** Pídele a la IA implementar el flujo de ingesta de eventos y el canal WebSocket, asegurando que el filtrado por roles (Proveedor/Tendero/Cliente) quede listo en el middleware de autenticación.
3. **Hora 8-10 (Librería Frontend y App Móvil):** Genera los componentes de React para el mapa y la lógica Offline-First con SQLite/WatermelonDB para el móvil.
4. **Hora 11-12 (Pruebas de Caos, IaC y Documentación):** Escribe un script rápido en k6 para simular 100 vehículos enviando coordenadas concurrentes, prepara el Terraform/Docker Compose y documenta en el README las decisiones de arquitectura y la auditoría del uso de la IA (crucial para la evaluación).



¿Te gustaría que profundicemos en la estructura de carpetas para el microservicio en Go o en cómo diseñar el componente de React para que funcione como librería reutilizable?