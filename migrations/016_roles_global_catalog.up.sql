-- Migration: roles como catálogo global — dropeo de tenant_id
-- Description: ACC-E02 T10 (decisión del owner T1-D5, 2026-08-13).
--
-- `roles` se reclasifica como catálogo global (igual que `plans`): no hay roles
-- de tenant. Verificado contra el vivo el 2026-08-13: 6 filas, las 6 con
-- `tenant_id NULL` — nunca existió un rol de tenant. La capacidad estaba en el
-- esquema pero no en el producto.
--
-- Dropea `tenant_id` y todo lo que depende de ella, y reemplaza la unicidad
-- `(name, tenant_id)` por `UNIQUE (name)` (las 6 filas vivas no colisionan por
-- nombre). Los dos índices parciales de `slug` creados por la migración 011
-- (`idx_roles_slug_system` con `WHERE tenant_id IS NULL` y `idx_roles_slug_tenant`
-- con `WHERE tenant_id IS NOT NULL`) dependen de `tenant_id` en su predicado y
-- **deben** dropearse antes que la columna; se reemplazan por un único índice
-- único global sobre `slug` que preserva la unicidad de los slugs de sistema
-- (cashier, supervisor, system_admin, ...). `slug` sigue nullable: un CUSTOM
-- creado por API sin slug queda NULL y varios NULL coexisten (Postgres trata
-- NULL como distintos en UNIQUE).
--
-- Condición inseparable (T1-D5): las escrituras de /roles se mueven a
-- `system:admin` en la app (T10, src/main.go) — sin RLS, el gate de scope es la
-- única defensa de la tabla. Hoy estaba roto: `tenant:admin` podía crearse un
-- rol `SYSTEM_ADMIN` y mutar los roles de sistema existentes.

-- 1. Dropear índices parciales de slug que dependen de tenant_id en su predicado.
DROP INDEX IF EXISTS idx_roles_slug_tenant;
DROP INDEX IF EXISTS idx_roles_slug_system;

-- 2. Dropear constraint e índice sobre tenant_id.
ALTER TABLE roles DROP CONSTRAINT IF EXISTS roles_name_tenant_unique;
DROP INDEX IF EXISTS idx_roles_tenant_id;

-- 3. Dropear la columna.
ALTER TABLE roles DROP COLUMN IF EXISTS tenant_id;

-- 4. Unicidad global por nombre (reemplaza roles_name_tenant_unique).
ALTER TABLE roles ADD CONSTRAINT roles_name_unique UNIQUE (name);

-- 5. Unicidad global de slug (reemplaza los dos índices parciales de 011).
CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_slug ON roles(slug);