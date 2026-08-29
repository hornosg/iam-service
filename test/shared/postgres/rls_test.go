package postgres_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sharedpostgres "iam/src/shared/postgres"
)

// --- driver falso -----------------------------------------------------------
// Implementado sobre database/sql/driver (stdlib) para no agregar una
// dependencia de mocking sólo por estos tests. Registra las sentencias que
// recibe para poder afirmar sobre el SET LOCAL exacto que se emite.

type fakeConn struct {
	mu         sync.Mutex
	statements []string
	committed  bool
	rolledBack bool
	execErr    error
}

func (c *fakeConn) record(q string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statements = append(c.statements, q)
}

func (c *fakeConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.record(query)
	if c.execErr != nil {
		return nil, c.execErr
	}
	return driver.RowsAffected(1), nil
}

func (c *fakeConn) Begin() (driver.Tx, error)                   { return &fakeTx{conn: c}, nil }
func (c *fakeConn) Prepare(string) (driver.Stmt, error)         { return nil, errors.New("no usado") }
func (c *fakeConn) Close() error                                { return nil }

type fakeTx struct{ conn *fakeConn }

func (t *fakeTx) Commit() error   { t.conn.committed = true; return nil }
func (t *fakeTx) Rollback() error { t.conn.rolledBack = true; return nil }

type fakeDriver struct{ conn *fakeConn }

func (d *fakeDriver) Open(string) (driver.Conn, error) { return d.conn, nil }

var registerOnce sync.Map

// nuevaDB registra un driver único por test y devuelve el *sql.DB junto con la
// conexión falsa para inspeccionar lo ejecutado.
func nuevaDB(t *testing.T) (*sql.DB, *fakeConn) {
	t.Helper()
	conn := &fakeConn{}
	name := fmt.Sprintf("fake-%s", t.Name())
	if _, cargado := registerOnce.LoadOrStore(name, true); !cargado {
		sql.Register(name, &fakeDriver{conn: conn})
	}
	db, err := sql.Open(name, "")
	require.NoError(t, err)
	// Una sola conexión: garantiza que el driver devuelva SIEMPRE este fakeConn.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db, conn
}

// --- tests ------------------------------------------------------------------

// La GUC debe seguir siendo la que fija la migración 019 de ACC-E02 T4. Si
// alguien la renombra de un lado y no del otro, la RLS deja de aislar en
// silencio: este test es el candado.
func TestTenantVar_CoincideConLaMigracion019(t *testing.T) {
	assert.Equal(t, "app.tenant_id", sharedpostgres.TenantVar)
}

func TestWithRLSInTransaction_EmiteSetLocalConElTenantYCommitea(t *testing.T) {
	db, conn := nuevaDB(t)
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
	require.NotEmpty(t, conn.statements)
	assert.Equal(t,
		"SET LOCAL app.tenant_id = '22222222-2222-2222-2222-222222222222'",
		conn.statements[0],
		"el tenant debe ir citado como literal en el SET LOCAL")
	assert.True(t, conn.committed)
	assert.False(t, conn.rolledBack)
}

// uuid.Nil es el valor cero: si pasara, el SET LOCAL fijaría un tenant
// "todo ceros" y la policy dejaría de discriminar. Debe fallar ANTES de
// abrir la transacción.
func TestWithRLSInTransaction_RechazaTenantNil(t *testing.T) {
	db, conn := nuevaDB(t)

	err := sharedpostgres.WithRLSInTransaction(context.Background(), db, uuid.Nil,
		func(context.Context, *sql.Tx) error {
			t.Fatal("fn no debe ejecutarse con tenant nil")
			return nil
		})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant_id requerido")
	assert.Empty(t, conn.statements, "no debe tocar la base")
	assert.False(t, conn.committed)
}

func TestWithRLSInTransaction_PropagaElErrorDeFnYNoCommitea(t *testing.T) {
	db, conn := nuevaDB(t)
	fallo := errors.New("insert falló")

	err := sharedpostgres.WithRLSInTransaction(context.Background(), db, uuid.New(),
		func(context.Context, *sql.Tx) error { return fallo })

	require.ErrorIs(t, err, fallo)
	assert.False(t, conn.committed, "un error de fn no debe commitear")
	assert.True(t, conn.rolledBack)
}

func TestWithRLSInTransaction_FallaSiNoPuedeFijarElTenant(t *testing.T) {
	db, conn := nuevaDB(t)
	conn.execErr = io.ErrUnexpectedEOF

	err := sharedpostgres.WithRLSInTransaction(context.Background(), db, uuid.New(),
		func(context.Context, *sql.Tx) error {
			t.Fatal("fn no debe correr si el SET LOCAL falló")
			return nil
		})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "set tenant_id")
	assert.False(t, conn.committed)
}
