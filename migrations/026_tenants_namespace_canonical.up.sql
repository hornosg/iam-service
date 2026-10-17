-- ACC-E04.T8: normalización del product-namespace de tenants (ADR-005 DD-3).
-- NO crea estructura: la columna namespace VARCHAR(50) NOT NULL, el índice
-- idx_tenants_namespace y tenants_namespace_slug_unique UNIQUE (namespace, slug)
-- existen desde 010 (que dropeó tenants_slug_key). Se conservan tal cual.
-- 'mc' fue un default de 010, no una decisión: el valor canónico es el slug del
-- producto y diverge a propósito del SERVICE_NAMESPACE='mc' del JWT (DD-3 H3).
-- La policy RLS de tenants (019) no cambia: aísla por PK/app.tenant_id.
-- Idempotente: UPDATE condicional + SET DEFAULT son no-op en un segundo pase.
-- Si ya conviviera ('mc', s) con ('mercado-cercano', s), el UPDATE viola la
-- unicidad y la migración aborta: fail-closed, no se fusionan tenants.

-- account_migrator es owner NOBYPASSRLS y tenants tiene FORCE RLS (019): sin GUC
-- el UPDATE ve 0 filas y la migración "pasaría" sin normalizar nada. Escape de 019
-- (app.is_system_admin) con is_local=true: vive sólo en la transacción implícita de
-- este archivo, no queda en la conexión.
SELECT set_config('app.is_system_admin', 'true', true);

UPDATE tenants SET namespace = 'mercado-cercano'
 WHERE namespace = 'mc';

ALTER TABLE tenants ALTER COLUMN namespace SET DEFAULT 'mercado-cercano';
