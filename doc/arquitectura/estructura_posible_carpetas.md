tracking-service/
├── cmd/
│   └── server/
│       └── main.go                # Punto de entrada de la aplicación (Inyección de dependencias)
├── internal/                      # Código privado de la aplicación (no importable por otros módulos)
│   ├── domain/                    # Capa 1: Entidades de Negocio y Reglas Puras (Sin dependencias externas)
│   │   ├── vehicle.go             # Entidad vehículo, coordenadas, estados
│   │   ├── order.go               # Entidad orden, estados de entrega
│   │   └── errors.go              # Errores de dominio personalizados
│   │
│   ├── usecase/                   # Capa 2: Casos de Uso / Lógica de Aplicación
│   │   ├── telemetry_interactor.go# Lógica de procesamiento de coordenadas
│   │   ├── order_interactor.go    # Lógica de asignación de flotas y trazabilidad
│   │   └── interfaces.go          # Puertos (Interfaces de Repositorios y Brokers)
│   │
│   ├── repository/                # Capa 3: Adaptadores de Base de Datos (Infraestructura)
│   │   ├── postgres/              # Conexión y repositorios para PostgreSQL (Órdenes, Usuarios)
│   │   │   ├── db.go
│   │   │   └── order_repository.go
│   │   └── timescaledb/           # Conexión y repositorios para TimescaleDB (Series de tiempo GPS)
│   │       ├── db.go
│   │       └── telemetry_repository.go
│   │
│   ├── broker/                    # Capa 4: Adaptadores de Mensajería
│   │   └── kafka/                 # Productores y Consumidores de Apache Kafka
│   │       ├── producer.go
│   │       └── consumer.go
│   │
│   └── delivery/                  # Capa 5: Capa de Presentación / Entrada (Adaptadores Primarios)
│       └── http/                  # API REST / WebSockets / gRPC
│           ├── handler/           # Handlers HTTP (Gin, Fiber o net/http)
│           │   ├── telemetry_handler.go
│           │   └── order_handler.go
│           ├── websocket/         # Servidor WS/SSE para despacho en tiempo real al Frontend
│           │   └── hub.go
│           ├── middleware/        # Autenticación JWT y control de roles (RBAC: Proveedor, Tendero, Cliente)
│           │   └── rbac.go
│           └── router.go          # Configuración de rutas
│
├── pkg/                           # Código reutilizable que podría exponerse externamente si es necesario
│   └── logger/                    # Logger estructurado (Zap / Logrus)
├── config/                        # Carga de variables de entorno y configuración
│   └── config.go
├── docker-compose.yml             # Orquestación local (Kafka, TimescaleDB, Zookeeper, etc.)
├── Dockerfile                     # Multi-stage build para Go
├── go.mod
└── go.sum