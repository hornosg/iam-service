-- 018_create_app_role.down.sql — revierte 018_create_app_role.up.sql
--
-- Revoca los grants de account_app y dropea el rol. Idempotente: REVOKE es no-op si el
-- grant no existe; DROP ROLE IF EXISTS.
-- Nota: si 019 (RLS) ya corrió, las policies quedan intactas — son role-agnostic por
-- convención (el GRANT vive en el DDL del rol, no en la migración RLS).

REVOKE ALL ON
  users,
  tenants,
  refresh_tokens,
  revoked_tokens,
  plans,
  roles,
  schema_migrations
FROM account_app;

REVOKE USAGE ON SCHEMA public FROM account_app;
REVOKE CONNECT ON DATABASE iam_db FROM account_app;

DROP ROLE IF EXISTS account_app;
