-- Migration: crear el rol acotado del path de login (iam_login)
-- Description: ACC-E02 T2. Hereda 3 condiciones vinculantes del gate L4 de T1
--   (T1-D1, T1-D2, T1-D4 — ver ACC-E02-aislamiento-por-caso-de-uso.md § Gate L4 de T1).
--
-- El path de login (POST /auth/login) es el único caso de uso donde el tenant NO se
-- conoce hasta después de autenticar: el lookup de credencial se hace por email sin
-- filtro de tenant. De ahí un rol de base dedicado, separado del account_app con RLS.
--
-- iam_login lee password_hash de TODOS los usuarios de TODOS los tenants (sin filtro,
-- por diseño). Dos controles acotan ese blast radius:
--   * GRANT SELECT por columnas — nunca `GRANT SELECT ON users` entero (esta migración).
--   * pg_hba.conf acotado a la red de iam-service (T1-D4, scripts/pg_hba_iam_login.sh).
--
-- Sin password acá: un literal en esta migración quedaría en git history para siempre.
-- El rol se crea sin password (login deshabilitado hasta setearla) y la password real se
-- fija out-of-band vía `ALTER ROLE iam_login PASSWORD '...'` contra lab-postgres, nunca
-- versionada. Patrón tomado de sales-service/migrations/025_create_app_role.up.sql.

-- === T1-D2 — switch point y modelo de conexión (declaración; la implementa T5/T6) ===
--
-- El login NO es un solo rol: es un switch point de rol dentro de POST /auth/login.
--   Fase (a) pre-auth: FindUserByEmail / GetUserByFederatedID sobre `users` sin filtro
--            de tenant → pool iam_login.
--   Fase (b) post-auth: generateAccessToken lee tenants/roles/plans y generateRefreshToken
--            hace INSERT en refresh_tokens → pool account_app bajo RLS (acá el tenant YA
--            se conoce: user.TenantID). LinkFederatedID (UPDATE federated_id, login.go:187)
--            se mueve a esta fase bajo account_app con policy sobre la propia fila (T1-D1).
--
-- Modelo de conexión elegido: DOS pools *sql.DB con DSN distinto (iam_login / account_app),
-- NO `SET ROLE`/`RESET ROLE` sobre un solo pool. Razón de seguridad: un RESET ROLE olvidado
-- dejaría la próxima query corriendo con el rol equivocado — contaminación de pool. Dos
-- pools hacen el switch explícito y el rol por conexión inmutable. Determina a T5 (threat
-- model: sin SET ROLE no hay contaminación) y a T6 (assertNoRLSBypass corre en AMBOS pools;
-- el conteo de llamadas = cantidad de pools abiertos, no 1).
--   Implementación: T5 (docker-compose DB_USER + config de DB en src/) y refactor de
--   login.go (mover LinkFederatedID post-issue). T2 declara; no implementa el wiring.

-- === T1-D1 — iam_login es read-only sobre credenciales ===
--
-- Queda PROHIBIDO cualquier GRANT UPDATE en `users` a iam_login. Esta migración sólo
-- otorga SELECT (por columnas). LinkFederatedID se mueve post-auth bajo account_app
-- (ver T1-D2 arriba). Un UPDATE con iam_login sería una primitiva de account-takeover
-- cross-tenant: quien comprometa esa credencial de DB podría linkear el federated_id
-- de cualquier usuario de cualquier tenant a su Google identity y loguearse como víctima.

DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'iam_login') THEN
    CREATE ROLE iam_login LOGIN
      NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
  END IF;
END
$$;

-- CONNECT + USAGE: mínimos para alcanzar la tabla users. Sin grants sobre plans/tenants
-- (esos quedan denegados para iam_login por diseño — verificado en "Hecho cuando").
GRANT CONNECT ON DATABASE iam_db TO iam_login;
GRANT USAGE ON SCHEMA public TO iam_login;

-- GRANT SELECT SÓLO sobre las 8 columnas de credencial que listó T1
-- (postgres_auth_repository.go lee: id, email, password_hash, tenant_id, role_id,
--  status, provider, federated_id — sin timestamps). Nunca `GRANT SELECT ON users`.
-- created_at/updated_at quedan fuera: el path de credencial no las lee.
GRANT SELECT (id, email, password_hash, tenant_id, role_id, status, provider, federated_id)
  ON users TO iam_login;