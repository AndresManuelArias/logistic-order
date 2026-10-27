src/
├── app/                              # Configuración global de la app
│   ├── navigation/                   # Configuración de React Navigation (Stacks, Tabs)
│   ├── providers/                    # Context Providers globales (Auth, Network, Theme)
│   └── App.tsx                       # Punto de entrada raíz
│
├── core/                             # Utilidades transversales y clientes base
│   ├── api/                          # Cliente HTTP configurado (Axios / Fetch e interceptores)
│   ├── database/                     # Inicialización y esquemas globales de WatermelonDB / SQLite
│   ├── network/                      # Detector de conectividad (NetInfo para Offline-First)
│   └── storage/                      # Manejo seguro de tokens (SecureStore / MMKV)
│
└── modules/                          # Módulos de negocio (DDD)
    │
    ├── auth/                         # Módulo de Autenticación del Conductor
    │   ├── domain/                   # (Clean: Capa más interna)
    │   │   ├── entities/             # Driver.ts (Modelo de dominio puro)
    │   │   └── repositories/         # IAuthRepository.ts (Contratos/Interfaces)
    │   ├── data/                     # (Clean: Capa de infraestructura y fuentes de datos)
    │   │   ├── datasources/          # AuthApiDataSource.ts, AuthLocalDataSource.ts
    │   │   ├── models/               # AuthResponseDTO.ts
    │   │   └── repositories/         # AuthRepositoryImpl.ts (Implementa la interfaz del dominio)
    │   └── presentation/             # (Clean: Capa de interfaz de usuario)
    │       ├── screens/              # LoginScreen.tsx
    │       ├── components/           # LoginForm.tsx
    │       └── hooks/                # useAuth.ts (Lógica de vista / Zustand o Context)
    │
    ├── orders/                       # Módulo de Órdenes y Entregas del Conductor
    │   ├── domain/
    │   │   ├── entities/             # Order.ts, DeliveryItem.ts
    │   │   └── repositories/         # IOrderRepository.ts
    │   ├── data/
    │   │   ├── datasources/          # OrderApiDataSource.ts, OrderLocalDataSource.ts (SQLite)
    │   │   ├── models/               # OrderDTO.ts, OrderSchema.ts (WatermelonDB)
    │   │   ├── mappers/              # OrderMapper.ts (Transforma DTOs a Entidades de Dominio)
    │   │   └── repositories/         # OrderRepositoryImpl.ts
    │   └── presentation/
    │       ├── screens/              # OrderListScreen.tsx, OrderDetailScreen.tsx
    │       ├── components/           # OrderCard.tsx, DeliveryProofModal.tsx
    │       └── hooks/                # useOrders.ts
    │
    └── tracking/                     # Módulo Core de Telemetría (GPS & Bulk Sync)
        ├── domain/
        │   ├── entities/             # GPSCoordinate.ts
        │   └── repositories/         # ITelemetryRepository.ts
        ├── data/
        │   ├── datasources/          # BackgroundGeolocationDataSource.ts, TelemetryLocalDataSource.ts
        │   ├── queue/                # SyncQueueManager.ts (Cola local para sincronización en bloque)
        │   └── repositories/         # TelemetryRepositoryImpl.ts
        └── presentation/
            ├── components/           # NetworkStatusBar.tsx (Indica si hay red o datos pendientes de sync)
            └── hooks/                # useBackgroundTracking.ts (Controla el servicio GPS en segundo plano)
    ```

---

### ¿Cómo se aplican los principios en esta estructura?

1.  **Independencia del Framework (`domain/`):** 
    Las carpetas `domain` dentro de cada módulo contienen TypeScript puro. No conocen nada de React Native, componentes visuales, bases de datos ni librerías externas. Esto garantiza que las reglas de negocio (por ejemplo, validar si una orden puede marcarse como entregada) sean completamente testeadas de forma aislada.
2.  **Patrón Repository y Mappers (`data/`):** 
    La capa de datos implementa las interfaces definidas en el dominio. Gracias a los *Mappers*, los datos crudos que vienen de la API o de la base de datos local (SQLite/WatermelonDB) se convierten en entidades limpias que la interfaz de usuario puede consumir sin preocuparse por la fuente de los datos.
3.  **Estrategia Offline-First (`tracking/data/queue/`):** 
    El módulo de telemetría cuenta con un administrador de cola (`SyncQueueManager`). Cuando el conductor pasa por una zona sin cobertura celular, las coordenadas GPS capturadas se guardan en la base de datos local y, tan pronto el módulo `core/network` detecta el retorno de internet, la cola dispara una sincronización en bloque (*Bulk Sync*) hacia el backend en Go.