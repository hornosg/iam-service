-- Migration: rollback del rol de migraciones (account_migrator)
-- Description: revierte 024 — devuelve la base y sus objetos a postgres y dropea
-- el rol. Idempotente: si el rol no existe, no hace nada.
--
-- ⚠ REQUIERE un rol con SET ROLE sobre postgres (en el lab: el superuser, y en
-- general quien haya hecho el bootstrap). REASSIGN OWNED exige poder SET ROLE a
-- AMBOS roles: account_migrator no es member de postgres, así que el propio rol
-- que crea esta migración no puede ejecutar su down — es consistente con el
-- bootstrap de 024: esta familia de migraciones se administra con un rol que ya
-- pueda crear roles, no con el rol que crea.

DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'account_migrator') THEN
    -- Objetos del schema public de vuelta a postgres (tablas, vistas, secuencias,
    -- funciones: todo lo que transfirió el up).
    REASSIGN OWNED BY account_migrator TO postgres;
    -- La base en sí (REASSIGN OWNED no la alcanza: las bases no son objetos "owned"
    -- dentro de otra base).
    ALTER DATABASE iam_db OWNER TO postgres;
    -- Limpia los grants recibidos por el rol (CONNECT y cualquier ACL residual).
    DROP OWNED BY account_migrator;
    DROP ROLE account_migrator;
  END IF;
END
$$;