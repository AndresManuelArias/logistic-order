-- 0004: telemetría (TimescaleDB + PostGIS).
-- points  → hypertable de alta frecuencia (RF-03).
-- stops   → continuous aggregate derivado de points (detección de paradas).
-- stop_intervals → vista que agrupa buckets consecutivos en intervalos reales.

CREATE TABLE telemetry.points (
    time        TIMESTAMPTZ NOT NULL,
    vehicle_id  UUID NOT NULL,
    device_seq  BIGINT NOT NULL,
    order_id    UUID,
    location    geometry(Point, 4326) NOT NULL,
    speed_kph   DOUBLE PRECISION,
    accuracy_m  DOUBLE PRECISION,
    heading     DOUBLE PRECISION,
    source      app.source_enum NOT NULL DEFAULT 'DEVICE',
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (time, vehicle_id, device_seq),
    CONSTRAINT points_location_valid CHECK (ST_IsValid(location))
);

-- Hypertable particionada por tiempo (chunk diario).
SELECT create_hypertable('telemetry.points', 'time',
                         chunk_time_interval => INTERVAL '1 day');

CREATE INDEX idx_points_vehicle_time ON telemetry.points (vehicle_id, time DESC);
CREATE INDEX idx_points_order ON telemetry.points (order_id) WHERE order_id IS NOT NULL;
CREATE INDEX idx_points_location ON telemetry.points USING GIST (location);

-- Retención: 90 días (RNF-03; configurable por entorno en F6 si hace falta).
SELECT add_retention_policy('telemetry.points', INTERVAL '90 days');

-- ---------------------------------------------------------------------------
-- Continuous aggregate: bucket de 1 min con estado de movimiento.
-- La detección se deriva de LOS PUNTOS (mismo vehículo, velocidad media
-- del bucket). El radio real de 30 m se aplica en stop_intervals/consultas.
-- Refresco automático cada 1 min (sin cron externo).
-- ---------------------------------------------------------------------------
CREATE MATERIALIZED VIEW telemetry.stops
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 minute', time) AS bucket_start,
    vehicle_id,
    first(location, time)  AS location,
    first(order_id, time)  AS order_id,
    avg(speed_kph)         AS avg_speed_kph,
    (avg(speed_kph) < 2.0) AS is_stopped,   -- NULL si no hay velocidad: no cuenta como parada
    count(*)               AS sample_count
FROM telemetry.points
GROUP BY bucket_start, vehicle_id
WITH NO DATA;

-- Política de refresco del continuous aggregate.
SELECT add_continuous_aggregate_policy('telemetry.stops',
    start_offset         => INTERVAL '3 hours',
    end_offset           => INTERVAL '1 minute',
    schedule_interval    => INTERVAL '1 minute');

-- Intervalos de parada reales: agrupa buckets consecutivos con is_stopped.
-- Un hueco > 1 min sin buckets corta la parada (no se cuenta como detenido).
CREATE VIEW telemetry.stop_intervals AS
WITH s AS (
    SELECT
        vehicle_id,
        bucket_start,
        location,
        order_id,
        is_stopped,
        SUM(CASE WHEN is_stopped THEN 0 ELSE 1 END)
            OVER (PARTITION BY vehicle_id ORDER BY bucket_start) AS seg
    FROM telemetry.stops
),
g AS (
    SELECT
        vehicle_id,
        seg,
        MIN(bucket_start) AS stop_start,
        MAX(bucket_start) AS stop_end,
        first(location, bucket_start) AS location,
        first(order_id, bucket_start) AS order_id
    FROM s
    WHERE is_stopped
    GROUP BY vehicle_id, seg
)
SELECT
    vehicle_id,
    stop_start,
    stop_end + INTERVAL '1 minute' AS stop_end,
    (EXTRACT(EPOCH FROM (stop_end + INTERVAL '1 minute' - stop_start)))::INTEGER AS duration_seconds,
    location,
    order_id
FROM g;
