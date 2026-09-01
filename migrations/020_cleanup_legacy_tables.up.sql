-- Migration: Limpieza heredada de MC-E27
-- Description: ACC-E02 T7. Drop de tablas muertas del incidente MC-E27 y sink
--   de auditoría que ya no se usan. Idempotente: DROP TABLE IF EXISTS.
--
-- Tablas afectadas:
--   * roles_dup_archive      — sink de auditoría creado por la migración 011
--     (dedup defensivo de roles de sistema). No existe en el vivo actual
--     (verificado 2026-08-13, iam_db v12), pero la migración 011 la crea;
--     se droppea para no dejar tablas huérfanas.
--   * roles_bkp_e23_20260617 / users_role_bkp_e23_20260617 — cruft vivo del
--     incidente MC-E27, no creado por migraciones. T1-D6 confirma que deben
--     desaparecer.
--
-- No se toca schema_migrations: es bookkeeping de golang-migrate.

DROP TABLE IF EXISTS roles_dup_archive;
DROP TABLE IF EXISTS roles_bkp_e23_20260617;
DROP TABLE IF EXISTS users_role_bkp_e23_20260617;
