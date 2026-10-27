A continuación, te presento el **Diagrama de Casos de Uso** en PlantUML para el sistema **Tiendas-Track** (Logística, Trazabilidad y Telemetría Multi-Rol).

Siguiendo las **buenas prácticas de UML**:

1. **Delimitación clara del sistema:** Se utiliza un rectángulo (`rectangle`) para definir los límites de la plataforma.
2. **Definición precisa de actores:** Se identifican los roles del ecosistema (Proveedor, Tendero, Cliente Final, Conductor) y el Agente de IA.
3. **Nombrado correcto de casos de uso:** Se estructuran con verbos en infinitivo + objeto (ej. *Monitorear Flota*, *Enviar Telemetría*).
4. **Relaciones lógicas:** Uso de asociaciones directas y relaciones de extensión/inclusión (`<<include>>` / `<<extend>>`) donde aplique.

### Código PlantUML

Puedes copiar este código y visualizarlo en cualquier renderizador de PlantUML (como PlantText, VS Code, etc.):

```plantuml
@startuml
left to right direction
skinparam packageStyle rectangle
skinparam backgroundColor #F9FBFD
skinparam DefaultFontName Arial

title Diagrama de Casos de Uso - Tiendas-Track (Telemetría Multi-Rol)

' --- ACTORES ---
actor "Proveedor B2B" as Proveedor
actor "Tendero (Comercio)" as Tendero
actor "Cliente Final" as Cliente
actor "Conductor / Repartidor" as Conductor
actor "Agente de IA" as IA

' --- DELIMITACIÓN DEL SISTEMA ---
rectangle "Sistema Tiendas-Track (Monolito / Portal)" {

    ' Casos de uso de Autenticación y Seguridad
    usecase "Autenticar Usuario y\nAlicar Permisos (RBAC)" as UC_Auth

    ' Casos de uso de Proveedor
    usecase "Monitorear Flota Global\nde Proveedor" as UC_ProvFleet

    ' Casos de uso de Tendero
    usecase "Monitorear Flotas de Entrada\ny Salida (Dual)" as UC_StoreLogistics

    ' Casos de uso de Cliente Final
    usecase "Visualizar Ubicación de\nOrden Específica" as UC_ClientTrack

    ' Casos de uso de Conductor (Móvil)
    usecase "Enviar Coordenadas GPS\nen Tiempo Real" as UC_SendGPS
    usecase "Sincronización Bloqueada\n(Offline-First)" as UC_SyncOffline

    ' Casos de uso de Inteligencia Artificial
    usecase "Consultar Estado de Flotas\nmediante Lenguaje Natural" as UC_AIAgent
    usecase "Traducir Pregunta a\nConsulta Analítica" as UC_ProcessNLP
}

' --- ASOCIACIONES (ACTORES -> CASOS DE USO) ---

' Proveedor
Proveedor --> UC_Auth
Proveedor --> UC_ProvFleet
Proveedor --> UC_AIAgent

' Tendero
Tendero --> UC_Auth
Tendero --> UC_StoreLogistics
Tendero --> UC_AIAgent

' Cliente Final
Cliente --> UC_ClientTrack

' Conductor
Conductor --> UC_Auth
Conductor --> UC_SendGPS

' Agente de IA (Actor de apoyo / sistema externo integrado)
IA --> UC_ProcessNLP

' --- RELACIONES INTERNAS (EXTEND / INCLUDE) ---

' La sincronización offline extiende el envío de GPS cuando se recupera la red
UC_SendGPS ..> UC_SyncOffline : <<extend>>

' Consultar con IA requiere procesar lenguaje natural internamente
UC_AIAgent ..> UC_ProcessNLP : <<include>>

@enduml

```

---

### Descripción de los Elementos Clave:

1. **Actores y Aislamiento (RBAC):**
* **Proveedor:** Accede a la vista global de su flota de distribución hacia las tiendas.
* **Tendero:** Posee una vista dual (ve la flota que le surte y la flota que reparte a sus clientes).
* **Cliente Final:** Cuenta con un caso de uso acotado exclusivamente a rastrear la orden única que le pertenece.
* **Conductor:** Interactúa desde la aplicación móvil ejecutando el flujo *Offline-First*.




2. **Relaciones de Dependencia (`<<extend>>` y `<<include>>`):**
* El envío de GPS del conductor (`UC_SendGPS`) puede disparar de forma opcional la sincronización masiva si estuvo offline (`UC_SyncOffline`).


* La consulta al agente de IA (`UC_AIAgent`) incluye obligatoriamente el procesamiento de lenguaje natural (`UC_ProcessNLP`) para consultar las bases de datos de series de tiempo.