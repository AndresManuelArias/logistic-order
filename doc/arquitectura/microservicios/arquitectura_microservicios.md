A continuación, te presento el diseño de la arquitectura orientada a microservicios para el sistema de telemetría y trazabilidad multi-rol (**Tiendas-Track**). Este diseño cumple con los requerimientos de la prueba técnica (alta concurrencia, eventos, persistencia especializada) y sirve como un módulo desacoplado integrable en tu plataforma de tiendas.

### Diagrama de Arquitectura (PlantUML)

Puedes copiar este código y visualizarlo en cualquier visor de PlantUML (como PlantText, el plugin de VS Code o Mermaid/PlantUML online):

```plantuml
@startuml
skinparam componentStyle rectangle
skinparam backgroundColor #F9FBFD
skinparam DefaultFontName Arial

title Arquitectura de Microservicios - Tiendas-Track (Telemetría & Logística)

' --- CAPA DE CLIENTES ---
package "Capa de Clientes (React & React Native)" #LightCyan {
    [Portal Web Multi-Rol\n(React / Next.js)] as WebPortal
    [App Conductor\n(React Native + WatermelonDB)] as MobileApp
}

' --- CAPA DE ENTRADA Y SEGURIDAD ---
package "Capa de Entrada" #LightYellow {
    [API Gateway / BFF\n(Go - JWT & RBAC)] as Gateway
}

' --- CAPA DE MICROSERVICIOS ---
package "Microservicios Core (Golang / Clean Architecture)" #LightGreen {
    [Servicio de Órdenes y Trazabilidad\n(Go)] as OrderService
    [Servicio de Telemetría\n(Go - Alta Concurrencia)] as TelemetryService
    [Agente Operativo IA\n(LangChain / Go)] as AIAgent
}

' --- CAPA DE MENSAJERÍA ---
package "Capa de Eventos" #MistyRose {
    queue "Apache Kafka\n(Stream de Coordenadas)" as Kafka
}

' --- CAPA DE PERSISTENCIA ---
package "Capa de Datos" #Lavender {
    database "TimescaleDB\n(Telemetría / Series de Tiempo)" as Timescale
    database "PostgreSQL\n(Usuarios, Tiendas, Órdenes)" as RelDB
}

' --- CONEXIONES ---
WebPortal --> Gateway : HTTPS / REST / WebSockets
MobileApp --> Gateway : HTTPS / REST (Sincronización Offline-First)

Gateway --> OrderService : gRPC / REST
Gateway --> TelemetryService : gRPC / REST
Gateway --> AIAgent : gRPC / REST

TelemetryService --> Kafka : Publica Coordenadas (GPS)
Kafka --> OrderService : Consume Eventos de Ruta
OrderService --> RelDB : CRUD Órdenes / Roles
TelemetryService --> Timescale : Escritura Masiva (Series de Tiempo)

AIAgent --> Timescale : Consulta de Tiempos y Estado de Flotas
AIAgent --> RelDB : Consulta Metadatos de Tiendas/Pedidos

' Notificaciones en tiempo real al WebPortal
TelemetryService ..> Gateway : WebSockets / SSE (Push de Ubicación)
Gateway ..> WebPortal : Stream de Ubicación Filtrado por Rol

@enduml

```

---

### Descripción de Componentes y Tecnologías Utilizadas

* **1. Capa de Frontend y Mobile (React / React Native):**
* **Portal Web (`@tiendas/tracking-ui`):** Diseñado como una librería modular de React para que cualquier otro sistema de la plataforma lo importe. Renderiza mapas interactivos y se conecta a WebSockets filtrando la información según el rol (`Proveedor`, `Tendero`, `Cliente`).
* **App Móvil del Conductor:** Desarrollada en **React Native** con persistencia local basada en **SQLite/WatermelonDB** para garantizar el funcionamiento *Offline-First* (captura coordenadas sin internet y hace *Bulk Sync* al reconectar).




* **2. API Gateway / BFF (Backend For Frontend):**
* Construido en **Golang**. Se encarga de la validación de tokens JWT, control de acceso basado en roles (**RBAC**) para aislar la visualización de flotas, y manejo de WebSockets/SSE para despachar la ubicación en tiempo real al navegador.




* **3. Microservicios Backend (Go - Clean Architecture):**
* **Servicio de Telemetría:** Optimizado en Go para la ingesta masiva y de alta concurrencia de eventos de geolocalización provenientes de los dispositivos o la app móvil.
* **Servicio de Órdenes y Trazabilidad:** Administra el ciclo de vida de los pedidos (del proveedor al tendero, y del tendero al cliente final) asociándolos a flotas de transporte específicas.
* **Agente Operativo de IA:** Integrado en el ecosistema para procesar lenguaje natural (ej. consultar por chat qué vehículos llevan más de 20 minutos detenidos), conectándose directamente a las bases de datos analíticas.




* **4. Bus de Eventos y Persistencia:**
* **Apache Kafka:** Desacopla la ingesta masiva de coordenadas GPS para evitar cuellos de botella en la base de datos.


* **TimescaleDB:** Base de datos relacional optimizada para series de tiempo, ideal para almacenar millones de registros de telemetría vehicular de alta frecuencia con consultas espaciales eficientes.


* **PostgreSQL:** Almacena los datos relacionales tradicionales del negocio (usuarios, tiendas de barrio, catálogos de productos y asignación de órdenes).