-- 0005: refresh tokens rotativos (RF-01, ADR-04).
-- Se almacena el hash SHA-256 del token, nunca el token en claro.
CREATE TABLE app.refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES app.users(id),
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked     BOOLEAN NOT NULL DEFAULT false,
    replaced_by UUID REFERENCES app.refresh_tokens(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_user ON app.refresh_tokens (user_id);

-- Los tokens de invitado se rellenan en F2; se añade índice para búsquedas por orden.
CREATE INDEX idx_guest_tokens_expires ON app.guest_tokens (expires_at);
