-- ACC-E09.T2: anti-replay de client_assertion del token de servicio (ADR-006 «Emisión y renovación»).
-- Caso de uso: emisión client-credentials, previa a cualquier tenant — un emisor de servicio se
-- autentica, no un usuario. La tabla es global (sin tenant_id) y NO lleva RLS: no guarda datos de
-- ningún tenant, sólo jti opacos con vencimiento (justificación por caso de uso, ACC-E02).
-- account_app inserta (ON CONFLICT DO NOTHING) y purga vencidos; no hay UPDATE.
-- TTL de cada fila = exp de la aserción + 30 s de leeway.

CREATE TABLE IF NOT EXISTS s2s_assertion_jti (
    jti        TEXT PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_s2s_assertion_jti_expires_at ON s2s_assertion_jti (expires_at);

COMMENT ON TABLE s2s_assertion_jti IS 'ACC-E09 T2: jti de client_assertion ya consumidos (anti-replay); purga por expires_at';

GRANT SELECT, INSERT, DELETE ON s2s_assertion_jti TO account_app;
