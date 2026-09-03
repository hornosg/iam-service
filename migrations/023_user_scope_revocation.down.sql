-- Revierte la marca de alcance user de revoke-all (T8i): vuelve revoked_tokens
-- al shape de 009/019/021/022 (sólo marcas por JTI). Tras el down,
-- RevokeAllUserTokens no puede insertar (columna scope inexistente) — el
-- endpoint queda en el estado roto que motivó la 023.
DROP INDEX IF EXISTS idx_revoked_tokens_user_scope;
ALTER TABLE revoked_tokens DROP CONSTRAINT IF EXISTS revoked_tokens_scope_check;
ALTER TABLE revoked_tokens DROP COLUMN IF EXISTS scope;