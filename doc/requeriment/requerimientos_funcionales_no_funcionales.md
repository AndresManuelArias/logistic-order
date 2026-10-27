A continuación, te presento el listado completo y estructurado de **Requerimientos Funcionales (RF)** y **Requerimientos No Funcionales (RNF)** para el desarrollo del sistema **Tiendas-Track**.

Este conjunto consolida tanto los estándares de élite exigidos en la prueba técnica como las necesidades del producto enfocado en tiendas de barrio (multi-rol, offline-first y trazabilidad en tiempo real).

---

## 1. Requerimientos Funcionales (RF)

### 1.1 Autenticación, Roles y Seguridad (RBAC)

* **RF-01 (Autenticación Única):** El sistema debe permitir el inicio de sesión para diferentes tipos de usuarios mediante credenciales seguras y generación de tokens JWT.
* **RF-02 (Control de Acceso Basado en Roles):** El sistema debe restringir y filtrar la información visualizada según el rol del usuario autenticado:
* *Proveedor:* Acceso a la visualización global de toda su flota de transporte B2B.
* *Tendero:* Acceso a una vista dual (flotas de entrada de sus proveedores y flotas de salida hacia sus clientes finales).
* *Cliente Final:* Acceso restringido y temporal (mediante enlace o token de invitado) exclusivamente al vehículo de su orden particular.



### 1.2 Módulo de Telemetría y Tiempo Real

* **RF-03 (Ingesta de Coordenadas):** El sistema debe recibir y procesar flujos continuos de coordenadas geográficas (latitud, longitud, velocidad, timestamp) enviados por los dispositivos de los vehículos o la app móvil.
* **RF-04 (Transmisión en Tiempo Real):** El sistema debe despachar la ubicación actualizada de los vehículos hacia el portal web utilizando canales bidireccionales (WebSockets o Server-Sent Events).



### 1.3 Módulo de Órdenes y Logística

* **RF-05 (Gestión de Ciclo de Vida de Órdenes):** El sistema debe registrar, actualizar y consultar el estado de los pedidos (Creado, En Tránsito, Entregado, Cancelado).
* **RF-06 (Asociación de Flota):** El sistema debe permitir vincular una orden de compra o entrega a un vehículo y conductor específico.

### 1.4 Aplicación Móvil del Conductor (Offline-First)

* **RF-07 (Captura GPS en Campo):** La app móvil debe capturar automáticamente la geolocalización del conductor durante la ruta de entrega.
* **RF-08 (Persistencia Local sin Conexión):** Si el dispositivo pierde la conexión a internet, la app debe almacenar localmente (SQLite/WatermelonDB) las coordenadas y estados de entrega.


* **RF-09 (Sincronización en Bloque / Bulk Sync):** Tan pronto la app detecte el restablecimiento de la red, debe sincronizar automáticamente en bloque los datos acumulados hacia el backend.



### 1.5 Módulo de Agente Operativo de IA

* **RF-10 (Consultas en Lenguaje Natural):** El sistema debe incluir un agente de IA integrado capaz de interpretar preguntas del usuario (ej. *“¿Qué vehículos llevan detenidos más de 20 minutos?”*) y traducirlas en consultas analíticas sobre el estado de la flota.



---

## 2. Requerimientos No Funcionales (RNF)

### 2.1 Arquitectura y Diseño de Software

* **RNF-01 (Clean Architecture & DDD):** El código backend (Golang) y frontend (React / React Native) debe estructurarse bajo principios de arquitectura limpia y diseño guiado por el dominio, separando estrictamente la lógica de negocio de la infraestructura.
* **RNF-02 (Modularidad y Reutilización):** El frontend debe construirse bajo un esquema de monorepo o librería (`@tiendas/tracking-core` y `@tiendas/tracking-ui`) para permitir su integración limpia en otros sistemas de la compañía.

### 2.2 Rendimiento, Concurrencia y Datos

* **RNF-03 (Persistencia de Alta Frecuencia):** El almacenamiento de telemetría debe delegarse en una base de datos optimizada para series de tiempo (TimescaleDB) capaz de soportar miles de inserciones concurrentes de dispositivos GPS sin degradación.


* **RNF-04 (Baja Latencia en WebSockets):** La entrega de actualizaciones de ubicación a través de WebSockets no debe superar un retraso perceptible (< 500 ms) bajo condiciones normales de red.

### 2.3 Resiliencia y Tolerancia a Fallos

* **RNF-05 (Circuit Breakers):** La comunicación interna y externa del sistema debe implementar patrones de *Circuit Breaker* para evitar efectos en cascada ante caídas de bases de datos o servicios externos.



### 2.4 Pruebas de Carga y Caos (Requisito de Prueba Técnica)

* **RNF-06 (Simulación de Concurrencia):** El sistema debe evaluarse mediante scripts de pruebas de carga (ej. k6) capaces de simular cientos de vehículos enviando telemetría simultánea, inyectando un porcentaje controlado de peticiones duplicadas y errores.



### 2.5 Infraestructura y Despliegue (IaC)

* **RNF-07 (Contenedorización):** Todo el entorno de desarrollo y ejecución debe estar orquestado mediante Docker Compose para garantizar portabilidad local inmediata.
* **RNF-08 (Infraestructura como Código):** Se deben proveer scripts de IaC (Terraform o AWS CDK) para la provisión automatizada de recursos en la nube.


* **RNF-09 (CI/CD Móvil):** La aplicación móvil debe incluir pipelines de automatización de despliegue y empaquetado (Fastlane / GitHub Actions).