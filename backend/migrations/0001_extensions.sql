-- 0001: extensiones requeridas.
-- postgis  → todas las coordenadas como geometry(Point, 4326) (ADR-09).
-- timescaledb → hypertable de telemetría + continuous aggregate (ADR-02).
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS timescaledb;
