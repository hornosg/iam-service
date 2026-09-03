-- Migration: Reparar el escape de mantenimiento de revoked_tokens (ACC-E02 T8h)
-- Description: La policy revocation_maintenance de la 021 (FOR DELETE) NUNCA
--   podía borrar: para DELETE, Postgres exige que la fila sea visible por el
--   grupo de policies aplicables a SELECT (FOR SELECT / FOR ALL, OR entre sí)
--   Y eliminable por el grupo aplicable a DELETE (FOR DELETE / FOR ALL, OR
--   entre sí). Los dos grupos se ANDean. Esta migración agrega la rama de
--   visibilidad que faltaba.
--
-- === Diagnóstico (T8h, reproducido en 11 probes contra pg15 — la imagen del
-- === lab — y verificado también en pg16) ===
--
-- El hallazgo que abrió esta tarea: con app.token_maintenance='on' y SIN
-- app.tenant_id, `DELETE FROM revoked_tokens WHERE expires_at < NOW()` borra
-- 0 filas. Tres veredictos previos lo explicaron como "el subselect de la
-- policy FOR ALL anda ANDeando", contradiciendo la semántica OR de policies
-- permisivas. El EXPLAIN resuelve la contradicción: el subselect aparece DOS
-- veces en el filtro del DELETE:
--
--   Filter: ( (hashed SubPlan 1) OR (token_maintenance = 'on')   <- grupo delete
--             AND (hashed SubPlan 2)                             <- grupo visibilidad
--             AND (expires_at < now()) )
--
-- La combinación es (visibilidad) AND (borrado), NO un OR global:
--
--   * visibilidad: USING de las policies permissivas aplicables a SELECT
--     (FOR SELECT / FOR ALL). Si no hay ninguna → `false` (probe: una tabla
--     con SOLO una policy FOR DELETE USING (true) borra 0 filas).
--   * borrado: USING de las aplicables a DELETE (FOR DELETE / FOR ALL), OR
--     entre sí. Sin ninguna → también false.
--
-- En revoked_tokens el grupo de visibilidad sólo tiene a tenant_isolation
-- (FOR ALL de 019), cuyo USING es `user_id IN (SELECT id FROM users WHERE
-- tenant_id = app.tenant_id)` — y users está a su vez bajo RLS FORCE con
-- policies que exigen app.tenant_id (o ser iam_login). En una sesión de
-- mantenimiento SIN tenant el subselect devuelve 0 usuarios → visibilidad
-- false para TODAS las filas → el OR del grupo de borrado nunca se evalúa a
-- favor → DELETE 0, pase lo que pase con el GUC. Por eso "con tenant del
-- dueño sí borra" (la visibilidad pasa) y por eso el escape era
-- estructuralmente inalcanzable: no era un GUC mal fijado ni una policy
-- RESTRICTIVE (pg_policy.polpermissive = 't' en ambas), sino la mitad
-- faltante del AND.
--
-- === La corrección ===
--
-- Se agrega la rama de visibilidad como policy FOR SELECT gated igual que la
-- de borrado, y se acota el escape entero con dos condiciones defensivas que
-- el WHERE de la app no garantizaba por sí solo:
--
--   * app.tenant_id IS NULL — el escape sólo es legítimo en estado sin tenant
--     (la goroutine de limpieza); estructuralmente inutilizable por código
--     futuro que corra bajo la RLS de un tenant. Mismo encuadre que el
--     hardening (ii) de refresh_token_presentation (gate de T8e-1).
--   * expires_at < NOW() — el escape sólo ve/borra revocaciones vencidas,
--     cuyos JTIs ya no cubren ningún token vivo. Aunque un código futuro
--     fijara el GUC de más, no puede leer ni borrar revocaciones vivas.
--
-- El INSERT/UPDATE de revoked_tokens sigue gated por tenant_isolation
-- (WITH CHECK de 019): las dos policies nuevas son FOR SELECT y FOR DELETE,
-- nada más.
--
-- Prohibido por contrato de T4: ninguna policy con USING (true).
--
-- Idempotente (requisito de la épica): DROP POLICY IF EXISTS antes de cada
-- CREATE POLICY. Re-aplicarla no falla. NO se retoca la 021 (artefacto firmado
-- por el gate L4 de T8e-2); esta migración corrige por encima, dejando el
-- diagnóstico en el historial.

-- Rama de visibilidad: sin ella el grupo de visibilidad del DELETE es false
-- en toda sesión sin tenant y ninguna policy de borrado alcanza.
DROP POLICY IF EXISTS revocation_maintenance_visibility ON revoked_tokens;
CREATE POLICY revocation_maintenance_visibility ON revoked_tokens
  FOR SELECT
  USING (
    NULLIF(current_setting('app.tenant_id', true), '') IS NULL
    AND NULLIF(current_setting('app.token_maintenance', true), '') = 'on'
    AND expires_at < NOW()
  );

DROP POLICY IF EXISTS revocation_maintenance ON revoked_tokens;
CREATE POLICY revocation_maintenance ON revoked_tokens
  FOR DELETE
  USING (
    NULLIF(current_setting('app.tenant_id', true), '') IS NULL
    AND NULLIF(current_setting('app.token_maintenance', true), '') = 'on'
    AND expires_at < NOW()
  );