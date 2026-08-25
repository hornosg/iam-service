-- Migration: RLS fail-closed en las tablas con tenant conocido (down)
-- Description: ACC-E02 T4 — revert RLS + policies en orden inverso al up
--   (patrón 005_rls_tenant.down.sql de tenant-service).

DROP POLICY IF EXISTS tenant_isolation ON revoked_tokens;
ALTER TABLE revoked_tokens NO FORCE ROW LEVEL SECURITY;
ALTER TABLE revoked_tokens DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON refresh_tokens;
ALTER TABLE refresh_tokens NO FORCE ROW LEVEL SECURITY;
ALTER TABLE refresh_tokens DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON tenants;
ALTER TABLE tenants NO FORCE ROW LEVEL SECURITY;
ALTER TABLE tenants DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON users;
DROP POLICY IF EXISTS users_login_lookup ON users;
ALTER TABLE users NO FORCE ROW LEVEL SECURITY;
ALTER TABLE users DISABLE ROW LEVEL SECURITY;
