//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	sharedmigrate "github.com/hornosg/go-shared/migrate"
	"github.com/hornosg/iam-service"
)

// expectedCounts refleja el seed idempotente de las migraciones 005, 006 y 011.
// Si cambia el seed, este test debe actualizarse.
var expectedCounts = map[string]int{
	"roles":          6, // 4 sistema de 005 + cashier + supervisor de 011
	"plans":          4, // seed de 004
	"tenants":        2, // system + demo de 006
	"users":          3, // admin system + admin demo + compat de 006
	"refresh_tokens": 0,
	"revoked_tokens": 0,
}

func TestMigrationsIdempotentOnCleanDatabase(t *testing.T) {
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
	if err != nil {
		t.Fatalf("error starting postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("warn: failed to terminate postgres container: %v", err)
		}
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("error getting connection string: %v", err)
	}

	runAndCheck := func(round int) {
		db, err := sql.Open("postgres", connStr)
		if err != nil {
			t.Fatalf("round %d: error opening database: %v", round, err)
		}
		defer db.Close()

		if err := db.PingContext(ctx); err != nil {
			t.Fatalf("round %d: error pinging database: %v", round, err)
		}

		if err := sharedmigrate.RunMigrations(db, iam.MigrationsFS, "iam_db"); err != nil {
			t.Fatalf("round %d: RunMigrations failed: %v", round, err)
		}

		for table, expected := range expectedCounts {
			var count int
			query := "SELECT COUNT(*) FROM " + table
			if err := db.QueryRow(query).Scan(&count); err != nil {
				t.Fatalf("round %d: counting %s: %v", round, table, err)
			}
			if count != expected {
				t.Fatalf("round %d: %s expected %d rows, got %d", round, table, expected, count)
			}
			t.Logf("round %d: %s = %d", round, table, count)
		}

		// Verificar que las tablas muertas no existen después de la migración 020.
		deadTables := []string{"roles_dup_archive", "roles_bkp_e23_20260617", "users_role_bkp_e23_20260617"}
		for _, dead := range deadTables {
			var exists bool
			if err := db.QueryRow(
				"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)",
				dead,
			).Scan(&exists); err != nil {
				t.Fatalf("round %d: checking %s existence: %v", round, dead, err)
			}
			if exists {
				t.Fatalf("round %d: dead table %s should not exist", round, dead)
			}
			t.Logf("round %d: %s absent", round, dead)
		}
	}

	// Primera corrida sobre base limpia.
	runAndCheck(1)

	// Resetear el esquema para simular una base limpia nueva. Los roles globales
	// (iam_login, account_app) persisten, lo cual es realista y prueba la
	// idempotencia de CREATE ROLE IF NOT EXISTS.
	resetDB, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatalf("error opening reset connection: %v", err)
	}
	if _, err := resetDB.Exec(`
		DROP SCHEMA public CASCADE;
		CREATE SCHEMA public;
		GRANT ALL ON SCHEMA public TO postgres;
		GRANT ALL ON SCHEMA public TO public;
	`); err != nil {
		resetDB.Close()
		t.Fatalf("error resetting schema: %v", err)
	}
	if err := resetDB.Close(); err != nil {
		t.Fatalf("error closing reset connection: %v", err)
	}

	// Segunda corrida sobre base limpia.
	runAndCheck(2)
}
