-- ACC-E04.T2: rollback explícito de la tabla nueva (destruye su config).
-- Sólo ejecutar en rollback autorizado, con backup si contiene datos.
-- Sin CASCADE: dependencias futuras deben impedir un borrado silencioso.
DROP TABLE IF EXISTS tenant_config;
