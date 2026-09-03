-- Migration: Denormalizar refresh_tokens.tenant_id + escape de presentación y mantenimiento
-- Description: ACC-E02 T8e. Repara el camino por el que el tenant llega a las
--   policies de refresh_tokens/revoked_tokens para los caminos de sesión:
--   refresh, logout/revoke-all y el gate de revocación.
--
-- === Por qué se denormaliza tenant_id (decisión de diseño de T8e) ===
-- El endpoint POST /auth/refresh NO recibe tenant por ninguna vía: el tenant
-- sólo es conocible DESPUÉS de leer la fila del refresh token, pero la policy
-- de 019 (subselect `user_id IN (SELECT id FROM users WHERE tenant_id =
-- app.tenant_id)`) exige app.tenant_id ANTES de poder leerla. El lookup
-- pre-resolución necesita un escape de presentación (ver abajo) que lea la fila
-- por el secreto presentado; para resolver el tenant de esa fila SIN abrir una
-- segunda query contra users hubo que descartar dos alternativas:
--
--   (a) JOIN contra users dentro del escape: requeriría una policy extra en
--       `users` que subseleccione refresh_tokens. Eso cierra un ciclo
--       users→refresh_tokens→users entre policies (tenant_isolation de
--       refresh_tokens ya subselecciona users) → Postgres aborta con
--       "infinite recursion detected in policy".
--   (b) Exigir Bearer aunque esté expirado y usar su claim de tenant: cambia el
--       contrato del endpoint para los 8 servicios consumidores y el flutter-app
--       que hoy mandan sólo el refresh token. Blast radius innecesario.
--
-- La denormalización era la alternativa que T1 ya declaró válida ("policy
-- subselect O denormalizar tenant_id → T4"); T4 eligió la subselect, y T8e la
-- revisa con este motivo concreto: sin ella el escape de presentación no puede
-- resolver el tenant sin recursión. users no cambia de tenant (no existe el
-- caso de uso), así que la copia no agrega un riesgo de drift: la policy
-- WITH CHECK fuerza tenant_id = app.tenant_id en toda escritura de la app.
--
-- === Escape de presentación (pre-auth de la credencial) ===
-- app.refresh_digest = sha256(token presentado) en hex, fijado por el repo SOLO
-- dentro de la transacción de resolución del refresh. La policy
-- refresh_token_presentation (FOR SELECT) deja ver únicamente la fila cuyo
-- hash coincide. Es el mismo encuadre que users_login_lookup (T2) para el
-- login: la credencial presentada ES la credencial pre-auth, y no hay tenant
-- conocido hasta resolverla. Se compara por DIGEST, no por el token crudo, para
-- que el secreto no aparezca en pg_stat_activity/pg_stat_statements. Blast
-- radius del escape: con la credencial de DB de account_app + el digest de un
-- token se lee ESA fila (que da tenant_id); adivinar el digest exige poseer el
-- token (256 bits). Un escape FOR SELECT no cubre INSERT/UPDATE/DELETE: la
-- escritura sigue gated por tenant_isolation.
--
-- === Elección del hash del digest (resuelta en el gate L4 de T8e-1, 2026-09-01) ===
-- SHA-256, no MD5. El borrador usaba md5(token): contra preimagen (la amenaza
-- real del escape) MD5 resiste para un token aleatorio de 256 bits, pero es un
-- hash en deprecación (colisiones rotas) aplicado a un digest de credencial en
-- un esquema L4. sha256() es built-in de Postgres >=11 (el lab corre pg15), sin
-- extensión, misma comparación por hex y mismo GUC: el cambio no tiene costo y
-- evita ratificar un hash deprecado donde migrar una policy después es más
-- caro que elegir bien. Se evaluó HMAC y se descartó: contra preimagen no
-- agrega nada (quien no posee el token no computa el digest) y agrega gestión
-- de secreto. El repo debe fijar el GUC con sha256 hex (64 chars), NUNCA md5.
--
-- === Escape de mantenimiento en revoked_tokens ===
-- CleanupExpiredRevocations corre en una goroutine con ctx SIN tenant: la policy
-- de 019 (subselect por tenant) le niega todo y la limpieza nunca borra nada
-- (defecto (5) del checkpoint de T8). La policy revocation_maintenance (FOR
-- DELETE) abre el borrado sólo cuando la app fija app.token_maintenance = 'on',
-- algo que hace únicamente el path de limpieza (nunca un request). Es un escape
-- gated por la app, igual criterio que app.is_system_admin en 019. No es
-- escalada: sólo borra filas con expires_at < NOW(), es decir tokens de acceso
-- ya vencidos — borrarlas no re-valida ningún token vivo.
--
-- Prohibido por contrato de T4: ninguna policy con USING (true).
--
-- Idempotente (requisito de la épica): ADD COLUMN IF NOT EXISTS, SET NOT NULL
-- (no-op si ya lo está), DROP POLICY IF EXISTS antes de cada CREATE POLICY,
-- CREATE INDEX IF NOT EXISTS. Re-aplicarla no falla.

-- ============ refresh_tokens ============
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS tenant_id UUID;

-- Backfill: resuelve el tenant desde users. ON DELETE CASCADE garantiza que
-- todo refresh_token tiene usuario vivo; el DELETE defensivo cubre bases donde
-- el FK no exista (idempotencia sin depender de ella).
UPDATE refresh_tokens rt
   SET tenant_id = u.tenant_id
  FROM users u
 WHERE rt.user_id = u.id
   AND rt.tenant_id IS NULL;

DELETE FROM refresh_tokens WHERE tenant_id IS NULL;

ALTER TABLE refresh_tokens ALTER COLUMN tenant_id SET NOT NULL;

COMMENT ON COLUMN refresh_tokens.tenant_id IS 'Tenant dueño del token (denormalizado de users en ACC-E02 T8e): la policy lo compara con app.tenant_id sin subselect, y permite el escape de presentación pre-auth sin recursión RLS';

DROP POLICY IF EXISTS tenant_isolation ON refresh_tokens;
CREATE POLICY tenant_isolation ON refresh_tokens
  USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

DROP POLICY IF EXISTS refresh_token_presentation ON refresh_tokens;
-- Hardening (condición vinculante (ii) del gate de T8e-1, 2026-09-01): las
-- policies de Postgres son OR-permissivas. El escape de presentación es FOR
-- SELECT sin filtro de tenant, así que una sesión que llevara app.tenant_id Y
-- un digest podría leer la fila de un token de otro tenant. El escape sólo es
-- legítimo en estado pre-auth (no hay tenant conocido); se blinda exigiendo
-- que app.tenant_id esté SIN fijar — estructuralmente inutilizable para
-- cualquier código futuro que corra bajo la RLS de un tenant.
CREATE POLICY refresh_token_presentation ON refresh_tokens
  FOR SELECT
  USING (
    NULLIF(current_setting('app.tenant_id', true), '') IS NULL
    AND encode(sha256(convert_to(token, 'UTF8')), 'hex')
      = NULLIF(current_setting('app.refresh_digest', true), '')
  );

-- ============ revoked_tokens ============
-- La policy tenant_isolation de 019 (subselect por user_id) se conserva: logout
-- y revoke-all corren con app.tenant_id ya conocido de los claims verificados.
-- Sólo se agrega el escape de mantenimiento para la limpieza periódica.
DROP POLICY IF EXISTS revocation_maintenance ON revoked_tokens;
CREATE POLICY revocation_maintenance ON revoked_tokens
  FOR DELETE
  USING (NULLIF(current_setting('app.token_maintenance', true), '') = 'on');