-- 018_create_app_role.up.sql — ACC-E02 T3 (rol de aplicación account_app)
--
-- Crea el rol de aplicación de iam-service, la identidad de runtime que reemplaza al
-- superusuario (T5) y bajo la cual corre todo caso de uso con tenant conocido (RLS en T4).
-- Patrón copiado de sales-service/migrations/025_create_app_role.up.sql (PLAT-E25 T3):
-- DO block con guard IF NOT EXISTS, NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS.
--
-- account_app es el complemento de iam_login (017): mientras iam_login resuelve la
-- credencial por email SIN filtro de tenant (fase pre-auth del login), account_app es el
-- rol con RLS de la fase post-auth y de todo el resto. Dos roles de base, no uno —
-- la regla de la épica: el aislamiento se justifica por caso de uso, no por barrido de
-- tablas (ver ACC-E02-aislamiento-por-caso-de-uso.md).
--
-- Grants acá, policies en 019: convención del lab (tenant-service migrations/roles/
-- tenant_app.sql y sales 025) — el GRANT del rol de aplicación vive en el DDL del rol;
-- las migraciones RLS numeradas quedan role-agnostic (ENABLE/FORCE/POLICY, sin grants).
-- La matriz de grants materializa la tabla "Destino por tabla" de T1 (ACC-E02 T1):
--   * users, tenants, refresh_tokens, revoked_tokens → DML; el aislamiento real lo dan
--     las policies de 019 (FORCE RLS), no el grant.
--   * plans, roles → catálogo global sin RLS (decisión T1-D5); el gate de escritura es
--     la app (scope system:admin, T10), no la DB.
--   * schema_migrations → SELECT: RunMigrations (go-shared/migrate) lee la versión al
--     arrancar; patrón idéntico a tenant_app.
--
-- Seguridad (mismo patrón que 017/sales 025): esta migración NO fija password — un
-- literal acá quedaría en git history para siempre. El rol se crea sin password (login
-- deshabilitado hasta setearla); la password real se fija out-of-band vía
-- `ALTER ROLE account_app PASSWORD '...'` corrido a mano contra lab-postgres, nunca
-- versionada.
--
-- Idempotente: seguro de re-aplicar (guard IF NOT EXISTS en el DO block; GRANT es
-- siempre idempotente en Postgres).

DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'account_app') THEN
    CREATE ROLE account_app LOGIN
      NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
  END IF;
END
$$;

GRANT CONNECT ON DATABASE iam_db TO account_app;
GRANT USAGE ON SCHEMA public TO account_app;

-- DML sobre las tablas de la fase post-auth y del resto de casos de uso. El aislamiento
-- cross-tenant lo ejercen las policies de 019 (FORCE RLS), no el grant.
GRANT SELECT, INSERT, UPDATE, DELETE ON
  users,
  tenants,
  refresh_tokens,
  revoked_tokens,
  plans,
  roles
TO account_app;

-- RunMigrations (go-shared/migrate) consulta schema_migrations al arrancar.
GRANT SELECT ON schema_migrations TO account_app;
