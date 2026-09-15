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
	"strings"
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
	// FailQueryContains, si no está vacío, acota ExecErr a las sentencias que
	// lo contienen: sirve para fallar la DML sin que el SET LOCAL de la
	// transacción RLS —que corre antes— se coma el error.
	FailQueryContains string
	// ZeroRows, si es true, hace que toda ejecución devuelva 0 filas
	// afectadas (T8f: para simular un DELETE que no tocó nada). El default
	// es 1 fila, que es lo que la mayoría de los tests quieren afirmar.
	ZeroRows bool
	// results es la cola FIFO de result sets programados con EnqueueResult:
	// cada QueryContext/QueryRowContext consume el siguiente; si la cola está
	// vacía, entrega el result set vacío de siempre (sql.ErrNoRows en el
	// Scan). Se encola ANTES de correr el método bajo test.
	results []*ResultSet
}

// ResultSet es un result set programado: los nombres de columna (deben
// coincidir en cantidad con los valores de cada fila) y las filas. Los
// driver.Value admitidos son los del stdlib: string, int64, bool, time.Time,
// []byte y nil.
//
// IterErr, si no es nil, es el error que la iteración reporta cuando se
// agotan las filas — en vez de un EOF limpio. Reproduce el fallo a mitad del
// recorrido (la conexión se corta después de la primera fila): el consumidor
// de rows debe terminar en error y NO devolver una lista parcial como éxito.
type ResultSet struct {
	Columns []string
	Rows    [][]driver.Value
	IterErr error
}

// EnqueueResult programa el próximo result set que los SELECT de la base
// falsa van a entregar, en orden FIFO.
func (c *Conn) EnqueueResult(rs *ResultSet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results = append(c.results, rs)
}

// dequeueResult saca el siguiente result set programado, o nil si no hay.
func (c *Conn) dequeueResult() *ResultSet {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.results) == 0 {
		return nil
	}
	rs := c.results[0]
	c.results = c.results[1:]
	return rs
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

// debeFallar resuelve si una sentencia concreta falla según ExecErr y su
// acotación opcional por FailQueryContains.
func (c *Conn) debeFallar(query string) bool {
	if c.ExecErr == nil {
		return false
	}
	return c.FailQueryContains == "" || strings.Contains(query, c.FailQueryContains)
}

func (c *Conn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.record(query)
	if c.debeFallar(query) {
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

// QueryContext entrega el result set programado con EnqueueResult (FIFO), o
// uno vacío si no hay ninguno. Con datos alcanza para cubrir los caminos de
// Scan de los métodos de lectura (T2 de ACC-E08: cubrir los scans del repositorio).
func (c *Conn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.record(query)
	if c.debeFallar(query) {
		return nil, c.ExecErr
	}
	if rs := c.dequeueResult(); rs != nil {
		return &dataRows{rs: rs}, nil
	}
	return Rows{}, nil
}

// dataRows adapta un ResultSet a driver.Rows: Columns reporta los nombres
// programados (o columnas sintéticas c0..cN según la primera fila) y Next
// recorre las filas en orden.
type dataRows struct {
	rs *ResultSet
	i  int
}

func (r *dataRows) Columns() []string {
	if len(r.rs.Columns) > 0 {
		return r.rs.Columns
	}
	if len(r.rs.Rows) > 0 {
		names := make([]string, len(r.rs.Rows[0]))
		for i := range names {
			names[i] = fmt.Sprintf("c%d", i)
		}
		return names
	}
	return nil
}

func (r *dataRows) Close() error { return nil }

func (r *dataRows) Next(dest []driver.Value) error {
	if r.i >= len(r.rs.Rows) {
		if r.rs.IterErr != nil {
			return r.rs.IterErr
		}
		return io.EOF
	}
	row := r.rs.Rows[r.i]
	r.i++
	copy(dest, row)
	return nil
}
