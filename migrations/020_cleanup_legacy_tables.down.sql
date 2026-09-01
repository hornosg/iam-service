-- Migration: rollback de limpieza heredada de MC-E27
-- Description: ACC-E02 T7. Las tres tablas eran huérfanas o de auditoría sin
--   esquema estable en migraciones; no se recrean estructuras que no tienen
--   definición canónica. El down es no-op salvo notificación.
--
-- roles_dup_archive se recrea automáticamente (vacía) si se vuelve a aplicar la
-- migración 011, así que no hace falta recrearla aquí. Las dos tablas de backup
-- del incidente MC-E27 no tienen DDL canónico: recrearlas sin su esquema real
-- sería fabricar datos.

DO $$
BEGIN
    RAISE NOTICE 'down no-op: tablas huérfanas de MC-E27 no se recrean (sin DDL canónico)';
END $$;
