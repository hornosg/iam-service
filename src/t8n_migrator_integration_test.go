//go:build integration

// Tests de contrato de ACC-E02 T8n — "Quién corre las migraciones cuando el
// servicio ya no es superusuario" (L3).
//
// Contrato (épica, verbatim en lo esencial):
//   * RunMigrations deja de correr sobre el pool de aplicación y abre su
//     PROPIA conexión con DB_MIGRATE_USER / DB_MIGRATE_PASSWORD (rol dedicado
//     account_migrator, DDL sobre iam_db, SIN uso en runtime), que se CIERRA
//     al terminar de migrar.
//   * El guard de T6 (assertNoRLSBypass) corre TAMBIÉN sobre esa conexión y
//     NO se relaja: un DB_MIGRATE_USER SUPERUSER/BYPASSRLS migraría con la
//     RLS inerte y se rechaza igual que en runtime.
//   * Negativa del "hecho cuando": el rol de runtime sigue SIN poder migrar —
//     TRUNCATE schema_migrations como account_app → permission denied.
//
// Se testea contra las funciones REALES de main() (setupMigratorDatabase,
// assertNoRLSBypass) y RunMigrations de go-shared, sobre un Postgres efímero
// con las migraciones reales 001→024. La parte "docker compose up -d deja el
// servicio sirviendo" es verificación en vivo del EXEC; acá se cubre su núcleo
// de DB: la secuencia de arranque migra, estampa y cierra sin tocar
// ALLOW_SUPERUSER_DB.
//
// Para correr:
//   cd platform/iam-service && GOWORK=off go test -tags=integration ./src/... -count=1 -v -run TestT8n
package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	sharedmigrate "github.com/hornosg/go-shared/migrate"

	iamroot "github.com/hornosg/iam-service"
)

const (
	t8nMigratorPassword = "t8n_test_migrator"
	t8nAppPassword      = "t8n_test_app"
	t8nProbePassword    = "t8n_test_probe"
)

// t8nLab es un lab efímero: Postgres limpio + conexión de administración
// (postgres superuser) + host/puerto publicados para conectar con roles.
type t8nLab struct {
	superDB *sql.DB
	host    string
	port    string
}

func startT8nLab(t *testing.T) *t8nLab {
	t.Helper()
	ctx := context.Background()

	pg, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("iam_db"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = pg.Terminate(ctx)
	})

	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	superDB, err := sql.Open("postgres", connStr)
	require.NoError(t, err)
	require.NoError(t, superDB.PingContext(ctx))
	t.Cleanup(func() { _ = superDB.Close() })

	host, err := pg.Host(ctx)
	require.NoError(t, err)
	mappedPort, err := pg.MappedPort(ctx, "5432")
	require.NoError(t, err)

	return &t8nLab{superDB: superDB, host: host, port: mappedPort.Port()}
}

// bootstrapMigrator replica scripts/bootstrap_migrator.sh: aplica 024 con un
// rol que ya puede crear roles (postgres) y fija la password out-of-band.
// Es el bootstrap documentado de la épica — la circularidad de que 024 crea
// al rol que corre 024 — jamás estampa schema_migrations.
func (l *t8nLab) bootstrapMigrator(t *testing.T) {
	t.Helper()

	up, err := iamroot.MigrationsFS.ReadFile("migrations/024_create_migrator_role.up.sql")
	require.NoError(t, err)
	_, err = l.superDB.Exec(string(up))
	require.NoError(t, err)

	_, err = l.superDB.Exec(fmt.Sprintf("ALTER ROLE account_migrator WITH PASSWORD '%s'", t8nMigratorPassword))
	require.NoError(t, err)
}

// openMigratorDB apunta las env de T8n al lab efímero y abre la conexión
// dedicada con setupMigratorDatabase — la MISMA función que corre main() al
// arrancar, con su MISMA fuente de credencial (DB_MIGRATE_USER/PASSWORD).
func (l *t8nLab) openMigratorDB(t *testing.T) *sql.DB {
	t.Helper()

	t.Setenv("DB_HOST", l.host)
	t.Setenv("DB_PORT", l.port)
	t.Setenv("DB_NAME", "iam_db")
	t.Setenv("DB_SSLMODE", "disable")
	t.Setenv("DB_MIGRATE_USER", "account_migrator")
	t.Setenv("DB_MIGRATE_PASSWORD", t8nMigratorPassword)
	// El contrato exige que esto funcione SIN el escape hatch de T6.
	t.Setenv("ALLOW_SUPERUSER_DB", "false")

	db, err := setupMigratorDatabase()
	require.NoError(t, err)
	return db
}

// connectAs abre una conexión ad-hoc a iam_db con un rol arbitrario del lab.
func (l *t8nLab) connectAs(t *testing.T, user, password string) *sql.DB {
	t.Helper()

	db, err := sql.Open("postgres", fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=iam_db sslmode=disable",
		l.host, l.port, user, password,
	))
	require.NoError(t, err)
	require.NoError(t, db.Ping())
	return db
}

// assertStampedVersion verifica el sello de la historia de migraciones:
// max(version) = esperado en la ÚNICA fila de schema_migrations (SetVersion
// hace DELETE+INSERT; estampar a mano dejó tres filas y el servicio a un
// restart de no arrancar).
func (l *t8nLab) assertStampedVersion(t *testing.T, expected int) {
	t.Helper()

	var maxVersion, rowCount int
	require.NoError(t, l.superDB.QueryRow(
		`SELECT max(version), count(*) FROM schema_migrations`,
	).Scan(&maxVersion, &rowCount))
	assert.Equal(t, expected, maxVersion, "la versión nueva debe quedar estampada en schema_migrations")
	assert.Equal(t, 1, rowCount, "schema_migrations debe tener UNA sola fila (driver SetVersion = DELETE+INSERT)")
}

// TestT8n_ArranqueUnicoConMigracionPendiente cubre el "hecho cuando" positivo:
// con TODA la migración pendiente (base limpia) y SIN tocar
// ALLOW_SUPERUSER_DB, la secuencia de arranque de main() aplicada con el rol
// dedicado — setupMigratorDatabase → guard de T6 → RunMigrations → Close —
// migra 001→024, estampa v24 y no deja conexiones de migrator abiertas.
// Es el Flow B de la épica: base limpia migrada POR account_migrator.
func TestT8n_ArranqueUnicoConMigracionPendiente(t *testing.T) {
	l := startT8nLab(t)
	l.bootstrapMigrator(t)

	migrateDB := l.openMigratorDB(t)

	// La conexión de migraciones es del rol dedicado — NO del pool de
	// aplicación (account_app), que es lo que el contrato manda dejar de usar.
	var currentUser string
	require.NoError(t, migrateDB.QueryRow(`SELECT current_user`).Scan(&currentUser))
	assert.Equal(t, "account_migrator", currentUser)

	// Guard de T6 sobre la conexión de migraciones: tercera conexión abierta
	// → tercer guard. account_migrator es NOBYPASSRLS → pasa.
	require.NoError(t, assertNoRLSBypass(migrateDB))

	// Flow B: migrar la base limpia completa como account_migrator. Requiere
	// CREATEROLE (017/018 crean iam_login/account_app) y ownership de la base.
	require.NoError(t, sharedmigrate.RunMigrations(migrateDB, iamroot.MigrationsFS, "iam_db"))

	l.assertStampedVersion(t, 24)

	// FORCE RLS ata al owner: account_migrator puede reescribir el esquema
	// (es su trabajo) pero sin el GUC app.tenant_id ve 0 filas de tenant.
	var migratorSees, superSees int
	require.NoError(t, migrateDB.QueryRow(`SELECT count(*) FROM users`).Scan(&migratorSees))
	require.NoError(t, l.superDB.QueryRow(`SELECT count(*) FROM users`).Scan(&superSees))
	assert.Equal(t, 0, migratorSees, "FORCE ROW LEVEL SECURITY debe aplicar también al owner (migrator)")
	assert.Greater(t, superSees, 0, "sanity: el seed de 006 dejó filas en users")

	// La conexión se cierra al terminar de migrar (decisión del owner): sin
	// uso en runtime → 0 conexiones de account_migrator en pg_stat_activity.
	require.NoError(t, migrateDB.Close())
	assert.Eventually(t, func() bool {
		var open int
		if err := l.superDB.QueryRow(
			`SELECT count(*) FROM pg_stat_activity WHERE usename = 'account_migrator'`,
		).Scan(&open); err != nil {
			return false
		}
		return open == 0
	}, 5*time.Second, 100*time.Millisecond,
		"la conexión de migraciones debe quedar cerrada tras migrar (no es un pool de servicio)")

	// Steady-state del segundo arranque: sin migraciones pendientes, mismo
	// sello, UNA fila.
	secondBoot := l.openMigratorDB(t)
	defer secondBoot.Close()
	require.NoError(t, assertNoRLSBypass(secondBoot))
	require.NoError(t, sharedmigrate.RunMigrations(secondBoot, iamroot.MigrationsFS, "iam_db"))
	l.assertStampedVersion(t, 24)
}

// TestT8n_Negativas cubre las negativas del contrato:
//
//  1. La del "hecho cuando", textual: el rol de runtime sigue SIN poder
//     migrar — TRUNCATE schema_migrations como account_app → permission
//     denied. Si pudiera, el aislamiento que la épica construyó estaría
//     deshecho: comprometer la credencial de runtime entregaría la historia
//     de migraciones. SELECT sigue funcionando: es el grant correcto de 018
//     (leer la versión al arrancar).
//  2. La del guard: DB_MIGRATE_USER apuntando a un rol BYPASSRLS
//     (no-superuser, el caso sutil) o al superuser se rechaza — migrar con
//     la RLS inerte no se permite ni en el rol dedicado.
func TestT8n_Negativas(t *testing.T) {
	t.Setenv("ALLOW_SUPERUSER_DB", "false")

	l := startT8nLab(t)
	l.bootstrapMigrator(t)

	migrateDB := l.openMigratorDB(t)
	require.NoError(t, sharedmigrate.RunMigrations(migrateDB, iamroot.MigrationsFS, "iam_db"))
	require.NoError(t, migrateDB.Close())

	// Password del rol de runtime out-of-band, como en el lab (018 jamás la
	// versiona: quedaría en git history).
	_, err := l.superDB.Exec(fmt.Sprintf("ALTER ROLE account_app WITH PASSWORD '%s'", t8nAppPassword))
	require.NoError(t, err)
	appDB := l.connectAs(t, "account_app", t8nAppPassword)
	defer appDB.Close()

	// Sanity del grant correcto: account_app puede LEER la versión.
	var version int
	require.NoError(t, appDB.QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&version))
	assert.Equal(t, 24, version)

	// NEGATIVA del contrato: TRUNCATE → permission denied.
	_, err = appDB.Exec(`TRUNCATE schema_migrations`)
	require.Error(t, err, "account_app NO debe poder TRUNCATE schema_migrations")
	assert.Contains(t, strings.ToLower(err.Error()), "permission denied")

	// Ni INSERT ni UPDATE: reescribir la historia de migraciones no es del
	// rol de runtime.
	_, err = appDB.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (999, false)`)
	require.Error(t, err, "account_app NO debe poder INSERT en schema_migrations")
	assert.Contains(t, strings.ToLower(err.Error()), "permission denied")

	_, err = appDB.Exec(`UPDATE schema_migrations SET version = 0`)
	require.Error(t, err, "account_app NO debe poder UPDATE schema_migrations")
	assert.Contains(t, strings.ToLower(err.Error()), "permission denied")

	// La historia quedó intacta tras los intentos.
	l.assertStampedVersion(t, 24)

	// Guard de T6 sobre la credencial de migraciones — caso sutil: rol
	// NOSUPERUSER pero BYPASSRLS. rolsuper=false solo engañaría a un chequeo
	// ingenuo; el guard real consulta rolsuper OR rolbypassrls.
	_, err = l.superDB.Exec(fmt.Sprintf(
		"CREATE ROLE t8n_bypass_probe LOGIN NOSUPERUSER BYPASSRLS PASSWORD '%s'", t8nProbePassword,
	))
	require.NoError(t, err)
	probeDB := l.connectAs(t, "t8n_bypass_probe", t8nProbePassword)
	defer probeDB.Close()

	err = assertNoRLSBypass(probeDB)
	require.Error(t, err, "el guard debe rechazar un DB_MIGRATE_USER con BYPASSRLS aunque no sea superuser")
	assert.Contains(t, err.Error(), "SUPERUSER o BYPASSRLS")

	// Y el superuser del lab tampoco pasaría el guard como migrator.
	err = assertNoRLSBypass(l.superDB)
	require.Error(t, err, "el guard debe rechazar un DB_MIGRATE_USER superuser")
	assert.Contains(t, err.Error(), "SUPERUSER o BYPASSRLS")
}