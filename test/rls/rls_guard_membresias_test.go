//go:build integration

// Test del guard de arranque contra Postgres real (ACC-E02 T8o, residual 2 del GO
// de @dev-security sobre el cierre de la épica): un rol sin BYPASSRLS que sea
// MIEMBRO de otro que sí lo tenga NO puede pasar el guard. Los atributos no se
// heredan por membresía, pero SET ROLE al rol privilegiado le da BYPASSRLS dentro
// de la sesión — la membresía es una escalada de privilegios real.
//
// Ejerce el guard REAL (sharedpostgres.AssertNoRLSBypass, el mismo que corre en el
// boot), no una copia: hasta T8o este paquete tenía su propia réplica del guard, y
// la evidencia contra una réplica no prueba el arranque.
//
// Controles positivos en cada paso (el guard pasa antes de otorgar y vuelve a
// pasar tras revocar): sin ellos, un guard que falla siempre sería
// indistinguible de uno que detecta la membresía — mismo modo de fallo que la
// lección de T8c para la lectura.
//
// Para correr sólo este archivo:
//   cd platform/iam-service && GOWORK=off go test -tags=integration ./test/rls/... -run TestRLS_GuardMembresias -count=1 -v
package rls_test

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/hornosg/iam-service"

	sharedmigrate "github.com/hornosg/go-shared/migrate"

	sharedpostgres "github.com/hornosg/iam-service/src/shared/postgres"
)

func TestRLS_GuardMembresias(t *testing.T) {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
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

	// Migraciones reales: la 018 crea account_app (NOSUPERUSER NOBYPASSRLS).
	require.NoError(t, sharedmigrate.RunMigrations(superDB, iam.MigrationsFS, "iam_db"))

	_, err = superDB.Exec("ALTER ROLE account_app WITH PASSWORD '" + testAppPassword + "'")
	require.NoError(t, err)

	appConnStr, err := withCredentials(superConnStr, "account_app", testAppPassword)
	require.NoError(t, err)
	appDB, err := sql.Open("postgres", appConnStr)
	require.NoError(t, err)
	require.NoError(t, appDB.PingContext(ctx))
	t.Cleanup(func() { _ = appDB.Close() })

	// El guard lee ALLOW_SUPERUSER_DB del entorno: fijarlo en "false" explícito
	// para que la escotilla del host no convierta el test en un no-op.
	t.Setenv("ALLOW_SUPERUSER_DB", "false")

	// Roles desechables del probe: uno con BYPASSRLS y uno intermediario para la
	// cadena transitiva. El contenedor es efímero, no puede quedar basura.
	_, err = superDB.Exec(`CREATE ROLE t8o_bypass_probe WITH BYPASSRLS`)
	require.NoError(t, err)
	_, err = superDB.Exec(`CREATE ROLE t8o_mid`)
	require.NoError(t, err)

	t.Run("sin membresías el guard pasa (control positivo)", func(t *testing.T) {
		require.NoError(t, sharedpostgres.AssertNoRLSBypass(appDB))
	})

	_, err = superDB.Exec(`GRANT t8o_bypass_probe TO account_app`)
	require.NoError(t, err)

	t.Run("membresía directa en un rol con BYPASSRLS hace FALLAR el guard", func(t *testing.T) {
		err := sharedpostgres.AssertNoRLSBypass(appDB)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SUPERUSER o BYPASSRLS")
		assert.Contains(t, err.Error(), "membresía")
	})

	_, err = superDB.Exec(`REVOKE t8o_bypass_probe FROM account_app`)
	require.NoError(t, err)

	t.Run("tras revocar, el guard vuelve a pasar", func(t *testing.T) {
		require.NoError(t, sharedpostgres.AssertNoRLSBypass(appDB))
	})

	// Membresía en un rol SIN privilegios no debe trip el guard: la detección es
	// sobre el atributo del rol miembro, no sobre "tiene alguna membresía".
	_, err = superDB.Exec(`GRANT t8o_mid TO account_app`)
	require.NoError(t, err)

	t.Run("membresía en un rol SIN BYPASSRLS no trip el guard", func(t *testing.T) {
		require.NoError(t, sharedpostgres.AssertNoRLSBypass(appDB))
	})

	// Cadena transitiva: account_app ∈ t8o_mid ∈ t8o_bypass_probe. El CTE
	// recursivo del guard la tiene que alcanzar.
	_, err = superDB.Exec(`GRANT t8o_bypass_probe TO t8o_mid`)
	require.NoError(t, err)

	t.Run("membresía transitiva (account_app ∈ mid ∈ bypass) también la detecta", func(t *testing.T) {
		err := sharedpostgres.AssertNoRLSBypass(appDB)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "membresía")
	})
}