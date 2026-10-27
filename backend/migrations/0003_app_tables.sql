-- 0003: tablas del negocio (esquema app).
-- Todas las columnas geográficas usan PostGIS geometry(Point, 4326) (ADR-09).

CREATE TYPE app.role_enum AS ENUM ('PROVIDER', 'STORE', 'DRIVER');
CREATE TYPE app.user_status_enum AS ENUM ('ACTIVE', 'SUSPENDED');
CREATE TYPE app.vehicle_status_enum AS ENUM ('ACTIVE', 'INACTIVE');
CREATE TYPE app.order_status AS ENUM ('CREATED', 'IN_TRANSIT', 'DELIVERED', 'CANCELLED');
CREATE TYPE app.source_enum AS ENUM ('DEVICE', 'BULK_SYNC');

-- Usuarios (RF-01): un rol por usuario.
CREATE TABLE app.users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         VARCHAR(255) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    full_name     VARCHAR(120) NOT NULL DEFAULT '',
    role          app.role_enum NOT NULL,
    status        app.user_status_enum NOT NULL DEFAULT 'ACTIVE',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Tiendas de barrio (Tendero, RF-02): georreferenciadas con PostGIS.
CREATE TABLE app.stores (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          VARCHAR(120) NOT NULL,
    owner_user_id UUID NOT NULL REFERENCES app.users(id),
    address       TEXT NOT NULL DEFAULT '',
    location      geometry(Point, 4326) NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT stores_location_valid CHECK (ST_IsValid(location))
);
CREATE INDEX idx_stores_location ON app.stores USING GIST (location);

-- Vehículos de la flota B2B (Proveedor).
CREATE TABLE app.vehicles (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plate               VARCHAR(10) NOT NULL UNIQUE,
    provider_id         UUID NOT NULL REFERENCES app.users(id),
    status              app.vehicle_status_enum NOT NULL DEFAULT 'ACTIVE',
    last_known_location geometry(Point, 4326),
    last_reported_at    TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Conductores (Driver): usuario + vehículo asignado.
CREATE TABLE app.drivers (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES app.users(id),
    vehicle_id     UUID NOT NULL REFERENCES app.vehicles(id),
    license_number VARCHAR(30) NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, vehicle_id)
);

-- Órdenes (RF-05, RF-06): origin/destination en PostGIS (ADR-09).
CREATE TABLE app.orders (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_ref        VARCHAR(64) NOT NULL UNIQUE,
    store_id            UUID NOT NULL REFERENCES app.stores(id),
    customer_id         UUID REFERENCES app.users(id),
    status              app.order_status NOT NULL DEFAULT 'CREATED',
    assigned_vehicle_id UUID REFERENCES app.vehicles(id),
    assigned_driver_id  UUID REFERENCES app.drivers(id),
    origin              geometry(Point, 4326) NOT NULL,
    destination         geometry(Point, 4326) NOT NULL,
    destination_address TEXT NOT NULL DEFAULT '',
    distance_meters     DOUBLE PRECISION,
    placed_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    picked_up_at        TIMESTAMPTZ,
    delivered_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT orders_origin_valid CHECK (ST_IsValid(origin)),
    CONSTRAINT orders_destination_valid CHECK (ST_IsValid(destination))
);
CREATE INDEX idx_orders_destination ON app.orders USING GIST (destination);
CREATE INDEX idx_orders_origin ON app.orders USING GIST (origin);
CREATE INDEX idx_orders_store_status ON app.orders (store_id, status);

-- Eventos de ciclo de vida (append-only: sin UPDATE ni DELETE).
CREATE TABLE app.order_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id      UUID NOT NULL REFERENCES app.orders(id),
    status        app.order_status NOT NULL,
    actor_user_id UUID REFERENCES app.users(id),
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    location      geometry(Point, 4326),
    payload       JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX idx_order_events_order ON app.order_events (order_id, occurred_at DESC);

CREATE OR REPLACE FUNCTION app.block_order_events_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'order_events es append-only: % no permitido', TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_order_events_append_only
    BEFORE UPDATE OR DELETE ON app.order_events
    FOR EACH ROW EXECUTE FUNCTION app.block_order_events_mutation();

-- Tokens de invitado (RF-02, Cliente Final): acceso temporal a una orden.
CREATE TABLE app.guest_tokens (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id   UUID NOT NULL REFERENCES app.orders(id),
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked    BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_guest_tokens_order ON app.guest_tokens (order_id);

-- Control de idempotencia del Bulk Sync offline (RF-09).
CREATE TABLE app.sync_receipts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key TEXT NOT NULL UNIQUE,
    device_id       VARCHAR(64) NOT NULL DEFAULT '',
    driver_id       UUID REFERENCES app.users(id),
    points_count    INTEGER NOT NULL DEFAULT 0,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Auditoría de consultas al agente IA (RF-10).
CREATE TABLE app.ai_query_log (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES app.users(id),
    question   TEXT NOT NULL,
    tool       VARCHAR(64) NOT NULL DEFAULT '',
    params     JSONB NOT NULL DEFAULT '{}'::jsonb,
    latency_ms INTEGER,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
