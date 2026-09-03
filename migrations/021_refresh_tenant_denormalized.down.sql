-- Migration: Revert denormalización de refresh_tokens.tenant_id (ACC-E02 T8e)
-- Description: Restaura el estado de 019: policies por subselect sobre users y
--   sin columna tenant_id en refresh_tokens. Los escapes de presentación y
--   mantenimiento introducidos acá se eliminan (no existían en 019).

-- ============ refresh_tokens ============
DROP POLICY IF EXISTS refresh_token_presentation ON refresh_tokens;

DROP POLICY IF EXISTS tenant_isolation ON refresh_tokens;
CREATE POLICY tenant_isolation ON refresh_tokens
  USING (
    user_id IN (SELECT id FROM users WHERE tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
  )
  WITH CHECK (
    user_id IN (SELECT id FROM users WHERE tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
  );

-- ============ revoked_tokens ============
DROP POLICY IF EXISTS revocation_maintenance ON revoked_tokens;

-- ============ columna denormalizada ============
ALTER TABLE refresh_tokens ALTER COLUMN tenant_id DROP NOT NULL;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS tenant_id;