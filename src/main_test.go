package main

import (
	"errors"
	"os"
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

	mock.ExpectQuery("SELECT rolsuper OR rolbypassrls").
		WillReturnRows(sqlmock.NewRows([]string{"privileged"}).AddRow(true))

	err = assertNoRLSBypass(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SUPERUSER o BYPASSRLS")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAssertNoRLSBypass_AceptaRolNoBypass cubre el happy path: un rol
// NOBYPASSRLS (account_app / iam_login) pasa el guard sin error.
func TestAssertNoRLSBypass_AceptaRolNoBypass(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT rolsuper OR rolbypassrls").
		WillReturnRows(sqlmock.NewRows([]string{"privileged"}).AddRow(false))

	require.NoError(t, assertNoRLSBypass(db))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAssertNoRLSBypass_ErrorDeQuerySePropaga asegura que un fallo al leer
// pg_roles no se trague: sin saber el rol, no es seguro arrancar.
func TestAssertNoRLSBypass_ErrorDeQuerySePropaga(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT rolsuper OR rolbypassrls").
		WillReturnError(errors.New("pq: permiso denegado"))

	err = assertNoRLSBypass(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no se pudo verificar los privilegios")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAssertNoRLSBypass_AllowSuperuserOmiteChequeo verifica el escape hatch
// para tareas administrativas locales. No debe usarse en producción.
func TestAssertNoRLSBypass_AllowSuperuserOmiteChequeo(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	t.Setenv("ALLOW_SUPERUSER_DB", "true")
	defer os.Unsetenv("ALLOW_SUPERUSER_DB")

	// No se espera ninguna query; el chequeo se omite.
	require.NoError(t, assertNoRLSBypass(db))
	assert.NoError(t, mock.ExpectationsWereMet())
}
