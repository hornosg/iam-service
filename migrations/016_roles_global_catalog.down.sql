-- Migration: rollback de roles como catálogo global — restaura tenant_id
-- Description: revierte 016 restaurando la columna `tenant_id` (nullable) y los
-- índices/constraints originales, incluidos los dos índices parciales de `slug`
-- de la migración 011.

-- 1. Quitar la unicidad global introducida por 016.
DROP INDEX IF EXISTS idx_roles_slug;
ALTER TABLE roles DROP CONSTRAINT IF EXISTS roles_name_unique;

-- 2. Restaurar la columna nullable.
ALTER TABLE roles ADD COLUMN IF NOT EXISTS tenant_id UUID;

-- 3. Restaurar constraint e índice sobre tenant_id.
ALTER TABLE roles ADD CONSTRAINT roles_name_tenant_unique UNIQUE (name, tenant_id);
CREATE INDEX IF NOT EXISTS idx_roles_tenant_id ON roles(tenant_id);

-- 4. Restaurar los índices parciales de slug de la migración 011.
CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_slug_system
    ON roles(slug) WHERE tenant_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_slug_tenant
    ON roles(slug, tenant_id) WHERE tenant_id IS NOT NULL;