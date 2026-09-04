package postgres

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAssertNoRLSBypass_RechazaRolPrivilegiado cubre el caso crítico de T6:
// si el rol conectado es SUPERUSER o BYPASSRLS, el boot debe abortar con un
// error explícito. Con un rol privilegiado FORCE RLS no se aplica.
func TestAssertNoRLSBypass_RechazaRolPrivilegiado(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("WITH RECURSIVE memberships").
		WillReturnRows(sqlmock.NewRows([]string{"privileged"}).AddRow(true))

	err = AssertNoRLSBypass(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SUPERUSER o BYPASSRLS")
	assert.Contains(t, err.Error(), "membresía")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAssertNoRLSBypass_AceptaRolNoBypass cubre el happy path: un rol
// NOBYPASSRLS (account_app / iam_login / account_migrator) y sin membresías
// en roles privilegiados pasa el guard sin error.
func TestAssertNoRLSBypass_AceptaRolNoBypass(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("WITH RECURSIVE memberships").
		WillReturnRows(sqlmock.NewRows([]string{"privileged"}).AddRow(false))

	require.NoError(t, AssertNoRLSBypass(db))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAssertNoRLSBypass_ErrorDeQuerySePropaga asegura que un fallo al leer
// pg_roles/pg_auth_members no se trague: sin saber el rol, no es seguro arrancar.
func TestAssertNoRLSBypass_ErrorDeQuerySePropaga(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("WITH RECURSIVE memberships").
		WillReturnError(errors.New("pq: permiso denegado"))

	err = AssertNoRLSBypass(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no se pudo verificar los privilegios")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAssertNoRLSBypass_EscotillaOmiteChequeoEnLab verifica el escape hatch
// para tareas administrativas locales: con el marcador explícito de entorno no
// productivo, el chequeo se omite (T8o residual 1: antes la escotilla andaba
// en cualquier entorno y sólo un comentario la separaba de prod).
func TestAssertNoRLSBypass_EscotillaOmiteChequeoEnLab(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	t.Setenv("ALLOW_SUPERUSER_DB", "true")
	t.Setenv("ENVIRONMENT", "lab")

	// No se espera ninguna query; el chequeo se omite.
	require.NoError(t, AssertNoRLSBypass(db))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAssertNoRLSBypass_EscotillaAbortaEnProduccion es la prueba negativa del
// criterio de T8o: con la escotilla puesta y un ENVIRONMENT de producción el
// servicio NO arranca y el error lo dice. Hasta esta tarea arrancaba igual.
func TestAssertNoRLSBypass_EscotillaAbortaEnProduccion(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	t.Setenv("ALLOW_SUPERUSER_DB", "true")
	t.Setenv("ENVIRONMENT", "production")

	err = AssertNoRLSBypass(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marcador explícito de no-producción")
	assert.Contains(t, err.Error(), "production")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAssertNoRLSBypass_EscotillaAbortaSinMarcador: sin ENVIRONMENT no hay
// marcador explícito de no-producción, y la condición de T8o es el marcador,
// no la ausencia de uno que diga "production". Fail-closed.
func TestAssertNoRLSBypass_EscotillaAbortaSinMarcador(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	t.Setenv("ALLOW_SUPERUSER_DB", "true")
	// Vacío explícito: no se puede "des-setear" con t.Setenv, y con "" el
	// resultado es el mismo que sin la variable (fallback de env.Get).
	t.Setenv("ENVIRONMENT", "")

	err = AssertNoRLSBypass(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no está seteado")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestIsNonProductionEnvironment fija la lista de entornos que habilitan la
// escotilla. Cualquier valor fuera de {local, lab, test} —incluido vacío—
// cuenta como producción para el guard.
func TestIsNonProductionEnvironment(t *testing.T) {
	for _, env := range []string{"local", "lab", "test"} {
		require.True(t, isNonProductionEnvironment(env), "ENVIRONMENT=%s debe habilitar la escotilla", env)
	}
	for _, env := range []string{"", "production", "prod", "prd", "staging", "PRODUCTION", "Lab"} {
		require.False(t, isNonProductionEnvironment(env), "ENVIRONMENT=%q NO debe habilitar la escotilla", env)
	}
}