-- Migration: Add slug to roles + seed cashier/supervisor (RBAC en JWT)
-- Description: El slug es el identificador de autorización estable que viaja en el
-- claim `roles` del JWT (independiente del Name visible y del RoleType grueso).
-- Aditiva y retrocompatible: la columna es nullable y el seed es idempotente.

-- 1. Columna slug (nullable para no romper filas existentes)
ALTER TABLE roles ADD COLUMN IF NOT EXISTS slug VARCHAR(50);

-- 2. Backfill de los roles de sistema existentes
UPDATE roles SET slug = 'system_admin' WHERE type = 'SYSTEM_ADMIN' AND slug IS NULL;
UPDATE roles SET slug = 'tenant_admin' WHERE type = 'TENANT_ADMIN' AND slug IS NULL;
UPDATE roles SET slug = 'user'         WHERE type = 'USER'         AND slug IS NULL;
UPDATE roles SET slug = 'read_only'    WHERE type = 'READ_ONLY'    AND slug IS NULL;

-- 3. Unicidad de slug por tenant.
--    OJO (condición @architect A1 / @security O4): en un UNIQUE normal Postgres trata
--    dos NULL como distintos, así que dos roles de SISTEMA (tenant_id IS NULL) con el
--    mismo slug podrían coexistir. Se usan DOS índices parciales para garantizar
--    unicidad real en ambos lados:
CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_slug_system
    ON roles(slug) WHERE tenant_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_slug_tenant
    ON roles(slug, tenant_id) WHERE tenant_id IS NOT NULL;

-- 4. Seed de roles de caja como roles de SISTEMA (tenant_id NULL), reutilizables por
--    todos los tenants. Idempotente vía el índice parcial de sistema sobre slug.
--    El permiso `sales:cash_session:approve_review` SOLO lo tiene supervisor:
--    materializa la separación de funciones (un cajero no aprueba su propio descuadre).
INSERT INTO roles (name, description, type, tenant_id, slug, permissions, is_active)
VALUES
  ('Cashier', 'Operador de caja POS', 'CUSTOM', NULL, 'cashier',
   ARRAY['sales:pos:sell',
         'sales:cash_session:open',
         'sales:cash_session:close',
         'sales:cash_session:read',
         'sales:cash_movement:create'],
   true)
ON CONFLICT DO NOTHING;

INSERT INTO roles (name, description, type, tenant_id, slug, permissions, is_active)
VALUES
  ('Supervisor', 'Supervisor de caja y arqueos', 'CUSTOM', NULL, 'supervisor',
   ARRAY['sales:pos:sell',
         'sales:cash_session:open',
         'sales:cash_session:close',
         'sales:cash_session:read',
         'sales:cash_movement:create',
         'sales:cash_session:approve_review'],
   true)
ON CONFLICT DO NOTHING;
