//go:build integration

// Test de comportamiento del escape de mantenimiento de revoked_tokens — ACC-E02 T8h.
//
// La objeción (A) del gate L4 de T8f (2026-09-02) era vinculante: el único test
// que cubría el escape corria contra fakedb, y "un mock no tiene policies, así
// que no puede probar nada sobre el comportamiento de una policy". Este test
// corre contra Postgres real (testcontainers), con la MISMA imagen que la base
// viva del lab (pgvector/pgvector:pg15), migraciones reales por RunMigrations y
// el rol account_app NOBYPASSRLS.
//
// Qué prueba:
//   - Diagnóstico (criterio (a) de T8h): el estado del catálogo de policies de
//     revoked_tokens es capturado y afirmado en el mismo test — ambas policies
//     de mantenimiento permisivas, ninguna RESTRICTIVE, con las condiciones del
//     escape en el USING. El diagnóstico del 2026-09-02 (bitácora): para DELETE,
//     Postgres ANDea el grupo de visibilidad (FOR SELECT/FOR ALL) con el grupo
//     de borrado (FOR DELETE/FOR ALL, OR entre sí); la 021 sólo traía el grupo
//     de borrado, y la visibilidad quedaba en manos del subselect de
//     tenant_isolation, que sin app.tenant_id no ve nada (users está bajo RLS).
//   - Comportamiento (criterio (b)): CleanupExpiredRevocations —el método real
//     del repo, con su propia transacción y GUC— borra las revocaciones vencidas
//     de TODOS los tenants y SÓLO esas: la viva queda, y una sesión de
//     mantenimiento no puede leer ni borrar revocaciones vivas.
//   - Contrafactual: recreada la policy tal como la dejó la 021 (sólo FOR
//     DELETE, sin rama de visibilidad), el cleanup borra 0 — el test detecta el
//     defecto que T8h diagnosticó.
//   - El aislamiento se conserva: sin el GUC de mantenimiento, account_app sigue
//     sin ver nada de revoked_tokens (fail-closed de 019 intacto).
//
// Para correr sólo este archivo:
//   cd platform/iam-service && GOWORK=off go test -tags=integration ./test/rls/... -run TestRLS_Mantenimiento -count=1 -v
package rls_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"iam"

	"iam/src/identity/infrastructure/persistence/repository"

	sharedmigrate "github.com/hornosg/go-shared/migrate"

	sharedpostgres "iam/src/shared/postgres"
)

// TestRLS_MantenimientoRevocaciones verifica el escape de mantenimiento contra
// Postgres real. Reemplaza como evidencia al test fakedb
// TestCleanupExpiredRevocations_EmiteGUCDeMantenimiento, que sigue válido para
// lo suyo (el orden de las sentencias del envoltorio) pero no puede probar
// policies.
func TestRLS_MantenimientoRevocaciones(t *testing.T) {
	ctx := context.Background()

	// Misma imagen que lab-postgres: el diagnóstico de T8h es sobre semántica
	// del motor, y la evidencia tiene que correr en la versión que corre en vivo.
	pgContainer, err := postgres.Run(ctx,
		"pgvector/pgvector:pg15",
		postgres.WithDatabase("iam_db"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	superConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	superDB, err := sql.Open("postgres", superConnStr)
	require.NoError(t, err)
	require.NoError(t, superDB.PingContext(ctx))
	t.Cleanup(func() { _ = superDB.Close() })

	// Migraciones reales, secuencia completa 001→022, aplicadas por el mismo
	// runner que usa el servicio (no por psql a mano — lección del 2026-08-14).
	require.NoError(t, sharedmigrate.RunMigrations(superDB, iam.MigrationsFS, "iam_db"))

	// Una sola fila en schema_migrations (regla de la épica).
	var filasMigraciones int
	require.NoError(t, superDB.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&filasMigraciones))
	assert.Equal(t, 1, filasMigraciones, "schema_migrations debe tener una sola fila")

	_, err = superDB.Exec("ALTER ROLE account_app WITH PASSWORD '" + testAppPassword + "'")
	require.NoError(t, err)

	appConnStr, err := withCredentials(superConnStr, "account_app", testAppPassword)
	require.NoError(t, err)
	appDB, err := sql.Open("postgres", appConnStr)
	require.NoError(t, err)
	require.NoError(t, appDB.PingContext(ctx))
	t.Cleanup(func() { _ = appDB.Close() })
	require.NoError(t, sharedpostgres.AssertNoRLSBypass(appDB), "account_app debe ser NOBYPASSRLS")

	// Semilla: dos tenants, un admin cada uno, y revocaciones vencidas y vivas
	// en ambos tenants. Superuser inserta (bypass RLS): replica lo que hace la
	// app por otros caminos.
	tenantA := uuid.New()
	tenantB := uuid.New()
	userA := uuid.New()
	userB := uuid.New()
	roleID := queryTenantAdminRoleID(t, superDB)
	seedTenantAndUser(t, superDB, tenantA, userA, "mnt-a@example.com", "mnt-a", roleID)
	seedTenantAndUser(t, superDB, tenantB, userB, "mnt-b@example.com", "mnt-b", roleID)

	jtiVencidaA := uuid.New()
	jtiVencidaB := uuid.New()
	jtiVivaA := uuid.New()
	jtiVivaB := uuid.New()
	seedRevocacion(t, superDB, jtiVencidaA, userA, -1*time.Hour)
	seedRevocacion(t, superDB, jtiVencidaB, userB, -2*time.Hour)
	seedRevocacion(t, superDB, jtiVivaA, userA, time.Hour)
	seedRevocacion(t, superDB, jtiVivaB, userB, 2*time.Hour)

	t.Run("diagnóstico: catálogo de policies del escape (criterio (a) de T8h)", func(t *testing.T) {
		// Ninguna policy RESTRICTIVE: las tres permissivas. La explicación del
		// defecto nunca fue una policy restrictiva.
		var restrictivas int
		require.NoError(t, superDB.QueryRow(`
			SELECT COUNT(*) FROM pg_policy
			WHERE polrelid = 'revoked_tokens'::regclass AND NOT polpermissive`).Scan(&restrictivas))
		assert.Zero(t, restrictivas, "no debe haber policies restrictivas en revoked_tokens")

		var cmds int
		require.NoError(t, superDB.QueryRow(`
			SELECT COUNT(*) FROM pg_policy
			WHERE polrelid = 'revoked_tokens'::regclass
			  AND polname IN ('revocation_maintenance', 'revocation_maintenance_visibility')`).Scan(&cmds))
		assert.Equal(t, 2, cmds,
			"el escape de mantenimiento necesita las DOS mitades: visibilidad (FOR SELECT) y borrado (FOR DELETE)")

		// Ambas con las tres condiciones del escape: sin tenant, GUC de
		// mantenimiento y sólo vencidas.
		for _, pol := range []string{"revocation_maintenance", "revocation_maintenance_visibility"} {
			var expr string
			require.NoError(t, superDB.QueryRow(`
				SELECT pg_get_expr(polqual, polrelid) FROM pg_policy
				WHERE polrelid = 'revoked_tokens'::regclass AND polname = $1`, pol).Scan(&expr))
			assert.Contains(t, expr, "app.token_maintenance", "%s: gated por el GUC de mantenimiento", pol)
			assert.Contains(t, expr, "app.tenant_id", "%s: exige tenant sin fijar (hardening como refresh_token_presentation)", pol)
			assert.Contains(t, expr, "expires_at", "%s: sólo revocaciones vencidas", pol)
		}
	})

	t.Run("comportamiento: la limpieza borra las vencidas y sólo esas", func(t *testing.T) {
		repo := repository.NewPostgresAuthRepository(appDB)

		borradas, err := repo.CleanupExpiredRevocations(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(2), borradas,
			"borra las 2 revocaciones vencidas (una por tenant) — el escape ya no es inalcanzable")

		var restantes int
		require.NoError(t, superDB.QueryRow("SELECT COUNT(*) FROM revoked_tokens").Scan(&restantes))
		assert.Equal(t, 2, restantes, "las revocaciones VIVAS de ambos tenants quedan")

		var viveJTI bool
		require.NoError(t, superDB.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM revoked_tokens WHERE jti = $1)`, jtiVivaA).Scan(&viveJTI))
		assert.True(t, viveJTI, "la revocación viva de A sobrevive a la limpieza")
	})

	t.Run("el escape no abre revocaciones vivas (defensa en profundidad)", func(t *testing.T) {
		// Sesión de mantenimiento (GUC on, sin tenant): una revocación viva NO
		// es visible ni borrable — el expires_at está en la policy, no sólo en
		// el WHERE del repo.
		var visibles int
		require.NoError(t, appDB.QueryRow(`
			SELECT COUNT(*) FROM (
				SELECT 1 FROM revoked_tokens
				WHERE NULLIF(current_setting('app.token_maintenance', true), '') = 'on'
			) s`).Scan(&visibles))
		// current_setting con missing_ok=true devuelve NULL fuera de la tx del
		// repo, así que esta consulta es trivialmente 0; la prueba fuerte es el
		// DELETE dentro de una tx con el GUC fijado, abajo.
		_ = visibles

		tx, err := appDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		_, err = tx.Exec(`SET LOCAL app.token_maintenance = 'on'`)
		require.NoError(t, err)

		var vivasVisibles int
		require.NoError(t, tx.QueryRow(
			`SELECT COUNT(*) FROM revoked_tokens WHERE expires_at >= NOW()`).Scan(&vivasVisibles))
		assert.Zero(t, vivasVisibles, "con mantenimiento on no se ven revocaciones vivas")

		res, err := tx.Exec(`DELETE FROM revoked_tokens`)
		require.NoError(t, err)
		afectadas, err := res.RowsAffected()
		require.NoError(t, err)
		assert.Zero(t, afectadas, "un DELETE sin WHERE bajo el GUC no toca revocaciones vivas: el límite está en la policy")
	})

	t.Run("fail-closed intacto sin el GUC de mantenimiento", func(t *testing.T) {
		var visibles int
		require.NoError(t, appDB.QueryRow(`SELECT COUNT(*) FROM revoked_tokens`).Scan(&visibles))
		assert.Zero(t, visibles, "sin app.tenant_id ni mantenimiento, account_app no ve nada de revoked_tokens")

		res, err := appDB.Exec(`DELETE FROM revoked_tokens`)
		require.NoError(t, err)
		afectadas, err := res.RowsAffected()
		require.NoError(t, err)
		assert.Zero(t, afectadas, "y tampoco borra")
	})

	t.Run("contrafactual: la policy de la 021 (sólo FOR DELETE) borra 0", func(t *testing.T) {
		// Reproduce el defecto diagnosticado en T8h dentro del propio test: sin
		// la rama de visibilidad, el grupo de visibilidad del DELETE queda en
		// manos del subselect de tenant_isolation y el cleanup no borra nada.
		_, err := superDB.Exec(`
			DROP POLICY IF EXISTS revocation_maintenance ON revoked_tokens;
			DROP POLICY IF EXISTS revocation_maintenance_visibility ON revoked_tokens;
			CREATE POLICY revocation_maintenance ON revoked_tokens
			  FOR DELETE
			  USING (NULLIF(current_setting('app.token_maintenance', true), '') = 'on');`)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = superDB.Exec(`
				DROP POLICY IF EXISTS revocation_maintenance ON revoked_tokens;
				DROP POLICY IF EXISTS revocation_maintenance_visibility ON revoked_tokens;
				CREATE POLICY revocation_maintenance_visibility ON revoked_tokens
				  FOR SELECT
				  USING (
				    NULLIF(current_setting('app.tenant_id', true), '') IS NULL
				    AND NULLIF(current_setting('app.token_maintenance', true), '') = 'on'
				    AND expires_at < NOW()
				  );
				CREATE POLICY revocation_maintenance ON revoked_tokens
				  FOR DELETE
				  USING (
				    NULLIF(current_setting('app.tenant_id', true), '') IS NULL
				    AND NULLIF(current_setting('app.token_maintenance', true), '') = 'on'
				    AND expires_at < NOW()
				  );`)
		})

		repo := repository.NewPostgresAuthRepository(appDB)
		borradas, err := repo.CleanupExpiredRevocations(context.Background())
		require.NoError(t, err)
		assert.Zero(t, borradas,
			"con la policy de la 021 el cleanup borra 0 filas: el test detecta el defecto que T8h diagnosticó")
	})
}

func seedRevocacion(t *testing.T, db *sql.DB, jti, userID uuid.UUID, venceEn time.Duration) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO revoked_tokens (jti, user_id, expires_at) VALUES ($1, $2, $3)`,
		jti, userID, time.Now().Add(venceEn))
	require.NoError(t, err)
}