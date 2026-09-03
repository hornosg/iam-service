// Package fakedb ofrece un *sql.DB respaldado por un driver falso construido
// sobre database/sql/driver (stdlib), para poder afirmar sobre las sentencias
// SQL que un repositorio emite sin levantar Postgres ni agregar una dependencia
// de mocking. Introducido en ACC-E02 T5 para verificar que el envoltorio RLS
// realmente emite SET LOCAL app.tenant_id.
package fakedb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
)

// Conn registra lo ejecutado contra la base falsa.
type Conn struct {
	mu         sync.Mutex
	statements []string
	Committed  bool
	RolledBack bool
	// ExecErr, si no es nil, hace fallar toda ejecución.
	ExecErr error
	// ZeroRows, si es true, hace que toda ejecución devuelva 0 filas
	// afectadas (T8f: para simular un DELETE que no tocó nada). El default
	// es 1 fila, que es lo que la mayoría de los tests quieren afirmar.
	ZeroRows bool
}

// Statements devuelve una copia de las sentencias ejecutadas, en orden.
func (c *Conn) Statements() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.statements))
	copy(out, c.statements)
	return out
}

func (c *Conn) record(q string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statements = append(c.statements, q)
}

func (c *Conn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.record(query)
	if c.ExecErr != nil {
		return nil, c.ExecErr
	}
	if c.ZeroRows {
		return driver.RowsAffected(0), nil
	}
	return driver.RowsAffected(1), nil
}

func (c *Conn) Begin() (driver.Tx, error)           { return &fakeTx{conn: c}, nil }
func (c *Conn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("fakedb: Prepare no soportado") }
func (c *Conn) Close() error                        { return nil }

type fakeTx struct{ conn *Conn }

func (t *fakeTx) Commit() error   { t.conn.Committed = true; return nil }
func (t *fakeTx) Rollback() error { t.conn.RolledBack = true; return nil }

type fakeDriver struct{ conn *Conn }

func (d *fakeDriver) Open(string) (driver.Conn, error) { return d.conn, nil }

var registrados sync.Map

// New devuelve un *sql.DB respaldado por un Conn inspeccionable. El driver se
// registra una vez por nombre de test; el pool queda en una sola conexión para
// que el Conn devuelto sea siempre el que se inspecciona.
func New(t *testing.T) (*sql.DB, *Conn) {
	t.Helper()
	conn := &Conn{}
	name := fmt.Sprintf("fakedb-%s", t.Name())
	if _, yaEstaba := registrados.LoadOrStore(name, conn); !yaEstaba {
		sql.Register(name, &fakeDriver{conn: conn})
	} else {
		t.Fatalf("fakedb: el driver %q ya fue registrado; usá un nombre de test único", name)
	}
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatalf("fakedb: abrir: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db, conn
}

// Rows es un driver.Rows vacío: basta para que database/sql entregue
// sql.ErrNoRows en el Scan, que es lo que los tests necesitan para afirmar
// sobre la sentencia emitida sin tener datos.
type Rows struct{}

func (Rows) Columns() []string { return nil }
func (Rows) Close() error      { return nil }
func (Rows) Next([]driver.Value) error {
	return io.EOF
}

// QueryContext permite afirmar sobre las sentencias SELECT que emiten los
// métodos de lectura (GetRefreshToken, IsTokenRevoked...). Devuelve siempre un
// result set vacío.
func (c *Conn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.record(query)
	if c.ExecErr != nil {
		return nil, c.ExecErr
	}
	return Rows{}, nil
}
