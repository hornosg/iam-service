-- Migration: RLS fail-closed en las tablas con tenant conocido
-- Description: ACC-E02 T4. ENABLE + FORCE ROW LEVEL SECURITY + policies de
--   aislamiento en users, tenants, refresh_tokens y revoked_tokens. plans y roles
--   quedan SIN RLS (catálogo global — motivo abajo).
--
-- Idempotente: cada CREATE POLICY va precedido de DROP POLICY IF EXISTS, y
-- ENABLE/FORCE ROW LEVEL SECURITY son no-ops si ya están activos. Re-aplicarla
-- sobre una base ya migrada no falla. (Sin los DROP, un segundo pase abortaba con
-- `policy "tenant_isolation" for table "users" already exists` → RunMigrations
-- devuelve error → `log.Fatalf` en src/main.go y el servicio NO arranca.)
--
-- Role-agnostic (convención del lab, ver 018): los grants viven en el DDL de los
-- roles (017 iam_login, 018 account_app); esta migración NO grantea nada. En
-- particular, nada a iam_login — carry-forward vinculante de T2: la RLS es sobre
-- account_app. Un grant extra a iam_login rompería el down de 017 (DROP ROLE
-- falla con objetos dependientes) y ensancharía un rol que por diseño no filtra
-- por tenant.
--
-- === T1-D3 — contrato de GUC de sesión ===
-- Las policies comparan contra el GUC de sesión `app.tenant_id`, fijado por
-- request con `SET LOCAL` (patrón PLAT-E29 / tenant-service). El plumbing en
-- src/ NO existe hoy (verificado 2026-08-12: grep vacío de SET LOCAL /
-- current_setting / app.tenant_id); T5 lo implementa. Se usa missing_ok=true
-- (segundo arg de current_setting): mientras el GUC no se fije, la expresión
-- devuelve NULL → 0 filas → fail-closed, no error. iam_login no lleva GUC (no
-- hay tenant pre-auth): su acceso a `users` lo da la policy users_login_lookup
-- (FOR SELECT, role-gated), no el GUC.
--
-- Además de `app.tenant_id` se introduce `app.is_system_admin` (bool): el branch
-- que T1 delegó a T4 para los handlers cross-tenant de `system:admin` sobre
-- `tenants` (GET /tenants list, plan/features admin, PUT/DELETE /tenants/:id
-- cross) y para el INSERT de provision (POST /tenants, scope S2S
-- tenant:provision o system:admin). La app lo fija con SET LOCAL en T5
-- únicamente tras pasar el gate de scope (authorize.go); sin él, NULL →
-- fail-closed. No es USING(true): el escape es deliberado y gated por la app.
--
-- NULLIF(current_setting(...), '') en todas las policies: un GUC custom nunca
-- seteado devuelve NULL (missing_ok), pero un RESET devuelve '' (default de
-- placeholder) y `''::uuid` erraría en vez de fallar cerrado. El NULLIF unifica
-- ambos estados a NULL → 0 filas → fail-closed. Verificado en vivo 2026-08-13:
-- `RESET app.tenant_id; SELECT count(*) FROM users` como account_app sin NULLIF
-- erró con "invalid input syntax for type uuid"; con NULLIF devuelve 0.
--
-- === Por qué plans y roles NO llevan RLS (T1) ===
-- * plans: catálogo global sin tenant_id. Ponerle RLS por reflejo —"toda tabla
--   lleva aislamiento"— rompería la lectura del catálogo (login/refresh leen el
--   tier) sin proteger nada: no hay dato de un tenant que filtrar. SELECT para
--   cualquier account_app autenticado; escritura gated system:admin en app.
-- * roles: ídem (T1-D5, T10). Reclasificado a catálogo global: tenant_id
--   dropeado en 016; las 6 filas vivas tenían tenant_id NULL — nunca existió un
--   rol de tenant. El gate de escritura es system:admin en app (T10).
--
-- Prohibido por contrato de T4: ninguna policy con USING (true).

-- ============ users ============
-- Split rol de login + RLS (T1): iam_login resuelve la credencial por email SIN
-- filtro de tenant (fase pre-auth del login); account_app corre todo lo demás
-- con RLS por tenant.
--   * users_login_lookup: escapa la RLS de filas SOLO para SELECT y SOLO para
--     iam_login — el rol que por diseño lee las 8 columnas de credencial (grant
--     de 017) de todos los tenants. Es un escape role-gated, no USING(true):
--     un hipotético grant de UPDATE a iam_login NO lo cubre (FOR SELECT) →
--     fail-closed de todos modos.
--   * tenant_isolation: aislamiento real de account_app: ve (y escribe) sólo
--     filas de su tenant de sesión.
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS users_login_lookup ON users;
CREATE POLICY users_login_lookup ON users
  FOR SELECT
  USING (current_user = 'iam_login');

DROP POLICY IF EXISTS tenant_isolation ON users;
CREATE POLICY tenant_isolation ON users
  USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- ============ tenants ============
-- RLS por caso de uso (T1): el scoped (GET/PUT/DELETE /tenants/:id) filtra por
-- `id = app.tenant_id`; el branch cross-tenant de system:admin y el INSERT de
-- provision (POST /tenants) se materializan via `app.is_system_admin`
-- (delegación de T1 a T4). tenants no tiene tenant_id: la PK `id` es el tenant.
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON tenants;
CREATE POLICY tenant_isolation ON tenants
  USING (
    id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
    OR NULLIF(current_setting('app.is_system_admin', true), '')::bool
  )
  WITH CHECK (
    id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
    OR NULLIF(current_setting('app.is_system_admin', true), '')::bool
  );

-- ============ refresh_tokens ============
-- Sin tenant_id (sólo user_id): la policy resuelve el tenant por subquery
-- `user_id IN (SELECT id FROM users WHERE tenant_id = app.tenant_id)` — la
-- opción "resolver" de T1 (vs denormalizar tenant_id). La subquery corre con
-- RLS del invocador (security invoker): account_app sólo ve usuarios de su
-- tenant de sesión, y de ahí sus tokens.
ALTER TABLE refresh_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE refresh_tokens FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON refresh_tokens;
CREATE POLICY tenant_isolation ON refresh_tokens
  USING (
    user_id IN (SELECT id FROM users WHERE tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
  )
  WITH CHECK (
    user_id IN (SELECT id FROM users WHERE tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
  );

-- ============ revoked_tokens ============
-- ídem refresh_tokens (logout/revoke-all insertan el jti del usuario; el
-- validate lee filtrado por tenant de sesión).
ALTER TABLE revoked_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE revoked_tokens FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON revoked_tokens;
CREATE POLICY tenant_isolation ON revoked_tokens
  USING (
    user_id IN (SELECT id FROM users WHERE tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
  )
  WITH CHECK (
    user_id IN (SELECT id FROM users WHERE tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
  );
