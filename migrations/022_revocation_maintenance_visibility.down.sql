-- Revierte la reparación del escape de mantenimiento (T8h): restaura el estado
-- de la 021 — una sola policy FOR DELETE sin rama de visibilidad. Tras el
-- down, la limpieza vuelve a borrar 0 filas en sesiones sin tenant (el defecto
-- que motivó la 022).
DROP POLICY IF EXISTS revocation_maintenance ON revoked_tokens;
DROP POLICY IF EXISTS revocation_maintenance_visibility ON revoked_tokens;
CREATE POLICY revocation_maintenance ON revoked_tokens
  FOR DELETE
  USING (NULLIF(current_setting('app.token_maintenance', true), '') = 'on');