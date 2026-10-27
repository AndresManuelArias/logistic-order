-- 0002: esquemas lógicos.
-- app       → datos relacionales del negocio.
-- telemetry → series de tiempo ( TimescaleDB) y control de migraciones.
CREATE SCHEMA IF NOT EXISTS app;
CREATE SCHEMA IF NOT EXISTS telemetry;
