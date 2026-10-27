/
├── cmd/
│   └── server/
│       └── main.go                 # Punto de entrada de la aplicación (inicia el servidor, DB y rutas)
│
├── internal/                       # Código privado de la aplicación
│   │
│   ├── shared/                     # Componentes transversales a todo el monolito
│   │   ├── database/               # Conexión a TimescaleDB/PostgreSQL y migraciones
│   │   ├── middleware/             # Middlewares globales (JWT, RBAC, Logger, Recovery)
│   │   └── websocket/              # Hub central de WebSockets / SSE para tiempo real
│   │
│   ├── auth/                       # Módulo de Autenticación y Control de Accesos (RBAC)
│   │   ├── domain/                 # Entidades (User, Role) e interfaces de repositorio
│   │   ├── usecase/                # Lógica de negocio (Login, Registro, Validación de Token)
│   │   └── delivery/               # Handlers HTTP (Controllers)
│   │
│   ├── orders/                     # Módulo de Órdenes y Logística (B2B y B2C)
│   │   ├── domain/                 # Entidades (Order, Item, Status) y reglas de negocio
│   │   ├── usecase/                # Casos de uso (Crear orden, cambiar estado, asignar flota)
│   │   └── delivery/               # Handlers HTTP
│   │
│   ├── telemetry/                  # Módulo de Telemetría (Ingesta de GPS y Alta Concurrencia)
│   │   ├── domain/                 # Entidades (Coordinate, Vehicle, Route)
│   │   ├── usecase/                # Procesamiento de coordenadas y filtrado por rol
│   │   └── delivery/               # Handlers HTTP y Endpoints de WebSockets
│   │
│   └── aiagent/                    # Módulo del Agente de IA Operativo
│       ├── domain/                 # Definición de herramientas y prompts del agente
│       ├── usecase/                # Lógica del agente (LangChain/Go) para traducir NL a consultas
│       └── delivery/               # Handlers HTTP para el chat de la IA
│
├── pkg/                            # Librerías reutilizables externas o utilidades genéricas
│   └── logger/                     # Logger estructurado (ej. zap o slog)
│
├── .env.example                    # Variables de entorno de ejemplo
├── Dockerfile                      # Dockerfile optimizado para Go
├── docker-compose.yml              # Orquestación local (App + TimescaleDB/PostgreSQL)
└── go.mod                          # Dependencias de Go