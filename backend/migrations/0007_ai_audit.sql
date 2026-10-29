-- 0007: auditoría de consultas del agente IA (RF-10).
-- La tabla base se creó en 0003; aquí se añaden los campos necesarios para
-- registrar el resultado completo sin guardar secretos ni prompts de sistema.

ALTER TABLE app.ai_query_log
    ADD COLUMN IF NOT EXISTS status     VARCHAR(16) NOT NULL DEFAULT 'OK',
    ADD COLUMN IF NOT EXISTS error_code VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS answer     TEXT        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS model      VARCHAR(80) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS tools      JSONB       NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE app.ai_query_log
    ADD CONSTRAINT ai_query_log_status_check CHECK (status IN ('OK', 'ERROR'));

CREATE INDEX IF NOT EXISTS idx_ai_query_log_user_time
    ON app.ai_query_log (user_id, occurred_at DESC);

COMMENT ON COLUMN app.ai_query_log.tools IS
    'Invocaciones de herramientas: [{name, arguments, ok, error, duration_ms}]';
COMMENT ON COLUMN app.ai_query_log.status IS
    'OK si la consulta terminó con respuesta; ERROR si falló (ver error_code).';
