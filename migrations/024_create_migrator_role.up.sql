-- Migration: crear el rol de migraciones account_migrator (ACC-E02 T8n)
-- Description: decisión del owner del 2026-09-03, destapada por el cutover de ACC-E02.
--
-- Hasta acá, RunMigrations corría sobre el pool de aplicación. Con T5, ese pool
-- conecta como account_app, que sólo tiene SELECT sobre schema_migrations (rol de
-- menor privilegio, correcto para runtime) — así que TODO arranque con una migración
-- pendiente fallaba con permission denied y el workaround era un baile manual de dos
-- arranques (DB_USER=postgres + ALLOW_SUPERUSER_DB=true, luego volver a los roles
-- acotados). Desde T8n las migraciones corren con un TERCER rol dedicado:
-- account_migrator, privilegio de DDL sobre iam_db y SIN uso en runtime. main.go abre
-- su propia conexión con DB_MIGRATE_USER / DB_MIGRATE_PASSWORD, migra y la cierra;
-- el guard de T6 (assertNoRLSBypass) corre también sobre esa conexión y NO se relaja.
--
-- === BOOTSTRAP (documentado, no implícito) ===
--
-- Esta migración CREA el rol con el que desde ahora corren las migraciones. En toda
-- base donde 024 esté pendiente, ese rol todavía no puede aplicar 024: hay que correrla
-- UNA VEZ con un rol que ya pueda crear roles. Ese bootstrap es
-- scripts/bootstrap_migrator.sh: aplica ESTE archivo con postgres y fija la password
-- out-of-band. Nunca estampar schema_migrations a mano al aplicar SQL con psql: el
-- driver go-shared/migrate mantiene UNA fila (SetVersion hace DELETE+INSERT) y
-- estampar a mano dejó tres filas y al servicio a un restart de no arrancar
-- (ver advertencia en la cabecera de la épica). El bootstrap NO toca
-- schema_migrations: el próximo arranque re-aplica 024 vía RunMigrations — todos los
-- statements de acá son no-ops por sus guards — y estampa v24 en la única fila.
--
-- === Atributos del rol ===
--
-- * CREATEROLE: la historia de migraciones de este servicio CREA roles (017/018
--   crean iam_login y account_app). En una base nueva migrada por account_migrator,
--   sin CREATEROLE la secuencia 001→024 se cae en 017. account_app/iam_login siguen
--   NOCREATEROLE.
--
--   CORRECCIÓN (2026-09-03, verificada contra lab-postgres 15.18): NO es cierto que en
--   pg15 CREATEROLE esté acotado a otorgar sólo memberships que el rol ya tiene — esa
--   restricción llegó en **pg16**. En pg15 un rol con CREATEROLE puede otorgar
--   CUALQUIER rol no-superusuario a quien quiera, incluido a sí mismo. Comprobado:
--   `psql -U account_migrator -c "GRANT account_app TO account_migrator"` → GRANT ROLE.
--   (La prueba se revirtió.)
--
--   Por qué se acepta igual: account_migrator ya es DUEÑO de las tablas —lo necesita
--   para ALTER/DROP, que no se puede otorgar— y un dueño siempre puede hacer
--   `ALTER TABLE ... NO FORCE ROW LEVEL SECURITY`. O sea que el camino vía CREATEROLE
--   no agrega poder que el rol no tenga ya. Lo que SÍ importa, y es lo que sostiene la
--   decisión, es que esta credencial **no se usa en runtime** y es distinta de las otras
--   dos. Si alguna vez se separa el owner del migrador, revisar esto de nuevo: ahí el
--   CREATEROLE sí sería una escalación real.
-- * NOBYPASSRLS (y NOBYPASSRLS del guard de T6): el ownership NO libera los datos.
--   users, tenants, refresh_tokens y revoked_tokens tienen FORCE ROW LEVEL SECURITY,
--   que aplica TAMBIÉN al owner: account_migrator puede reescribir el esquema (es su
--   trabajo) pero sin el GUC app.tenant_id ve 0 filas de cualquier tabla de tenant.
--   El plan y roles (catálogo global, sin RLS) sí le son legibles: son datos de
--   catálogo, no de tenant.
-- * Sin password acá, igual que 017/018: un literal en esta migración quedaría en
--   git history para siempre. Se fija out-of-band vía bootstrap_migrator.sh
--   (ALTER ROLE ... WITH PASSWORD), nunca versionado.
--
-- === Privilegios de DDL = ownership de la base y de sus objetos ===
--
-- ALTER/DROP TABLE requiere ser owner (no se puede granter), y en pg15 el schema
-- public ya no lo crea cualquiera: es propiedad de pg_database_owner, del que es
-- miembro implícito el owner de la base. Por eso esta migración cede la BASE y
-- transfiere los objetos públicos existentes; en una base limpia no hay nada que
-- transferir — todo lo que cree account_migrator le pertenecerá desde el vamos.
-- Los grants de account_app/iam_login (017/018) sobreviven al cambio de owner: los
-- ACL viven en el objeto, no en quien los otorgó.

DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'account_migrator') THEN
    CREATE ROLE account_migrator LOGIN
      NOSUPERUSER NOCREATEDB CREATEROLE NOBYPASSRLS;
  END IF;
END
$$;

GRANT CONNECT ON DATABASE iam_db TO account_migrator;

-- Dueño de la base → miembro implícito de pg_database_owner → dueño del schema
-- public (pg15) → CREATE de tablas/índices/policies en migraciones futuras.
ALTER DATABASE iam_db OWNER TO account_migrator;

-- Transferencia de los objetos existentes en public. En la base limpia es no-op
-- (account_migrator aún no corrió nada); en el vivo de hoy la ejecuta el bootstrap
-- con postgres. El re-run vía RunMigrations como account_migrator es no-op porque
-- ya es dueño. Los tipos compuestos de cada tabla siguen al owner de la tabla.
DO $$
DECLARE
  r record;
BEGIN
  FOR r IN
    SELECT tablename FROM pg_tables
     WHERE schemaname = 'public' AND tableowner <> 'account_migrator'
  LOOP
    EXECUTE format('ALTER TABLE public.%I OWNER TO account_migrator', r.tablename);
  END LOOP;

  FOR r IN
    SELECT sequencename FROM pg_sequences
     WHERE schemaname = 'public' AND sequenceowner <> 'account_migrator'
  LOOP
    EXECUTE format('ALTER SEQUENCE public.%I OWNER TO account_migrator', r.sequencename);
  END LOOP;

  FOR r IN
    SELECT schemaname, viewname, viewowner FROM pg_views
     WHERE schemaname = 'public' AND viewowner <> 'account_migrator'
  LOOP
    EXECUTE format('ALTER VIEW %I.%I OWNER TO account_migrator', r.schemaname, r.viewname);
  END LOOP;

  FOR r IN
    SELECT schemaname, matviewname, matviewowner FROM pg_matviews
     WHERE schemaname = 'public' AND matviewowner <> 'account_migrator'
  LOOP
    EXECUTE format('ALTER MATERIALIZED VIEW %I.%I OWNER TO account_migrator', r.schemaname, r.matviewname);
  END LOOP;

  -- Sin firma: regprocedure califica por search_path. Colisionar con pg_catalog
  -- exigiría un nombre+args idéntico a una función de sistema; no existe hoy y una
  -- colisión futura fallaría ruidoso en el bootstrap, no en silencio.
  FOR r IN
    SELECT p.oid::regprocedure AS sig
      FROM pg_proc p
      JOIN pg_namespace n ON n.oid = p.pronamespace
     WHERE n.nspname = 'public'
       AND pg_get_userbyid(p.proowner) <> 'account_migrator'
  LOOP
    EXECUTE format('ALTER FUNCTION %s OWNER TO account_migrator', r.sig);
  END LOOP;
END
$$;