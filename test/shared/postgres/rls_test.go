package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sharedpostgres "iam/src/shared/postgres"
	"iam/test/testsupport/fakedb"
)

// --- tests ------------------------------------------------------------------

// La GUC debe seguir siendo la que fija la migración 019 de ACC-E02 T4. Si
// alguien la renombra de un lado y no del otro, la RLS deja de aislar en
// silencio: este test es el candado.
func TestTenantVar_CoincideConLaMigracion019(t *testing.T) {
	assert.Equal(t, "app.tenant_id", sharedpostgres.TenantVar)
}

func TestWithRLSInTransaction_EmiteSetLocalConElTenantYCommitea(t *testing.T) {
	db, conn := fakedb.New(t)
	tenantID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	llamado := false
	err := sharedpostgres.WithRLSInTransaction(context.Background(), db, tenantID,
		func(ctx context.Context, tx *sql.Tx) error {
			llamado = true
			_, e := tx.ExecContext(ctx, "SELECT 1")
			return e
		})

	require.NoError(t, err)
	assert.True(t, llamado, "fn debe ejecutarse dentro de la transacción")
	require.NotEmpty(t, conn.Statements())
	assert.Equal(t,
		"SET LOCAL app.tenant_id = '22222222-2222-2222-2222-222222222222'",
		conn.Statements()[0],
		"el tenant debe ir citado como literal en el SET LOCAL")
	assert.True(t, conn.Committed)
	assert.False(t, conn.RolledBack)
}

// uuid.Nil es el valor cero: si pasara, el SET LOCAL fijaría un tenant
// "todo ceros" y la policy dejaría de discriminar. Debe fallar ANTES de
// abrir la transacción.
func TestWithRLSInTransaction_RechazaTenantNil(t *testing.T) {
	db, conn := fakedb.New(t)

	err := sharedpostgres.WithRLSInTransaction(context.Background(), db, uuid.Nil,
		func(context.Context, *sql.Tx) error {
			t.Fatal("fn no debe ejecutarse con tenant nil")
			return nil
		})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant_id requerido")
	assert.Empty(t, conn.Statements(), "no debe tocar la base")
	assert.False(t, conn.Committed)
}

func TestWithRLSInTransaction_PropagaElErrorDeFnYNoCommitea(t *testing.T) {
	db, conn := fakedb.New(t)
	fallo := errors.New("insert falló")

	err := sharedpostgres.WithRLSInTransaction(context.Background(), db, uuid.New(),
		func(context.Context, *sql.Tx) error { return fallo })

	require.ErrorIs(t, err, fallo)
	assert.False(t, conn.Committed, "un error de fn no debe commitear")
	assert.True(t, conn.RolledBack)
}

func TestWithRLSInTransaction_FallaSiNoPuedeFijarElTenant(t *testing.T) {
	db, conn := fakedb.New(t)
	conn.ExecErr = io.ErrUnexpectedEOF

	err := sharedpostgres.WithRLSInTransaction(context.Background(), db, uuid.New(),
		func(context.Context, *sql.Tx) error {
			t.Fatal("fn no debe correr si el SET LOCAL falló")
			return nil
		})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "set tenant_id")
	assert.False(t, conn.Committed)
}
