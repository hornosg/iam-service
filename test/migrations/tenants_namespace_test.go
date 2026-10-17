//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"testing"
	"time"

	sharedmigrate "github.com/hornosg/go-shared/migrate"
	iam "github.com/hornosg/iam-service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TEST-ACC-E04-T8: normalización del product-namespace (ADR-005 DD-3). 026 no crea
// estructura: la columna y tenants_namespace_slug_unique vienen de 010. Lo que se
// prueba es el valor canónico y que la unicidad por producto sigue siendo el control.
func TestTenantsNamespaceMigration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pg, err := postgres.Run(ctx, "pgvector/pgvector:pg15",
		postgres.WithDatabase("iam_db"), postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pg.Terminate(context.Background())) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	super, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = super.Close() })
	exec := func(q string, args ...any) error {
		_, err := super.ExecContext(ctx, q, args...)
		return err
	}
	file := func(name string) string {
		t.Helper()
		b, err := iam.MigrationsFS.ReadFile("migrations/" + name)
		require.NoError(t, err)
		return string(b)
	}
	count := func(q string) int {
		t.Helper()
		var n int
		require.NoError(t, super.QueryRowContext(ctx, q).Scan(&n))
		return n
	}

	// Mismo camino que el arranque: bootstrap de 024 y RunMigrations como account_migrator.
	require.NoError(t, exec(file("024_create_migrator_role.up.sql")))
	require.NoError(t, exec(`ALTER ROLE account_migrator PASSWORD 'test-only-migration'`))
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.User = url.UserPassword("account_migrator", "test-only-migration")
	migrator, err := sql.Open("postgres", u.String())
	require.NoError(t, err)
	migrator.SetMaxOpenConns(10)
	t.Cleanup(func() { _ = migrator.Close() })
	require.NoError(t, sharedmigrate.RunMigrations(migrator, iam.MigrationsFS, "iam_db"))

	// (a) aplicada, sin NULL ni residuo del default de 010.
	require.Equal(t, 1, count(`SELECT count(*) FROM schema_migrations WHERE version=26 AND NOT dirty`))
	require.Equal(t, 0, count(`SELECT count(*) FROM tenants WHERE namespace IS NULL`))
	require.Equal(t, 0, count(`SELECT count(*) FROM tenants WHERE namespace = 'mc'`))
	require.Greater(t, count(`SELECT count(*) FROM tenants WHERE namespace = 'mercado-cercano'`), 0,
		"el seed de 006 debe quedar en el namespace canónico")
	var def string
	require.NoError(t, super.QueryRowContext(ctx,
		`SELECT column_default FROM information_schema.columns WHERE table_name='tenants' AND column_name='namespace'`).Scan(&def))
	require.Contains(t, def, "'mercado-cercano'")

	// Estructura de 010 intacta: la constraint objetivo es la existente, sin UNIQUE(slug) global.
	require.Equal(t, 1, count(`SELECT count(*) FROM pg_constraint WHERE conrelid='tenants'::regclass AND conname='tenants_namespace_slug_unique' AND contype='u'`))
	require.Equal(t, 0, count(`SELECT count(*) FROM pg_constraint WHERE conrelid='tenants'::regclass AND conname='tenants_slug_key'`))
	// RLS de tenants sin cambios (019).
	require.Equal(t, 1, count(`SELECT count(*) FROM pg_class WHERE oid='tenants'::regclass AND relrowsecurity AND relforcerowsecurity`))
	require.Equal(t, 1, count(`SELECT count(*) FROM pg_policies WHERE tablename='tenants' AND policyname='tenant_isolation'`))

	var currentRole string
	var superuser, bypassRLS bool
	require.NoError(t, migrator.QueryRowContext(ctx,
		`SELECT current_user, rolsuper, rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&currentRole, &superuser, &bypassRLS))
	require.Equal(t, "account_migrator", currentRole)
	require.False(t, superuser)
	require.False(t, bypassRLS)
	migrateSQL := func(name string) error {
		t.Helper()
		_, err := migrator.ExecContext(ctx, file(name))
		return err
	}

	// (b) idempotencia: un segundo pase del SQL sobre la base migrada no falla ni cambia nada.
	require.NoError(t, migrateSQL("026_tenants_namespace_canonical.up.sql"))
	require.Equal(t, 0, count(`SELECT count(*) FROM tenants WHERE namespace = 'mc'`))

	// El escape de 026 es local a la transacción del archivo: no queda en la conexión.
	conn, err := migrator.Conn(ctx)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, file("026_tenants_namespace_canonical.up.sql"))
	require.NoError(t, err)
	var leaked sql.NullString
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT NULLIF(current_setting('app.is_system_admin', true), '')`).Scan(&leaked))
	require.False(t, leaked.Valid, "app.is_system_admin no debe sobrevivir a la migración: %q", leaked.String)
	require.NoError(t, conn.Close())

	insert := func(namespace, slug string) error {
		return exec(`INSERT INTO tenants (name, slug, type, owner_id, namespace)
			VALUES ($1, $2, 'BUSINESS', gen_random_uuid(), $3)`, slug+"@"+namespace, slug, namespace)
	}

	// (c) positiva: mismo slug en dos productos coexiste.
	require.NoError(t, insert("mercado-cercano", "t8-shared-slug"))
	require.NoError(t, insert("whatsapp-agent", "t8-shared-slug"))
	require.Equal(t, 2, count(`SELECT count(*) FROM tenants WHERE slug='t8-shared-slug'`))

	// (d) negativa: el mismo (namespace, slug) choca con la constraint de unicidad por producto.
	err = insert("mercado-cercano", "t8-shared-slug")
	var pqErr *pq.Error
	require.True(t, errors.As(err, &pqErr), "un (namespace, slug) duplicado debe fallar: %v", err)
	require.Equal(t, pq.ErrorCode("23505"), pqErr.Code)
	require.Equal(t, "tenants_namespace_slug_unique", pqErr.Constraint)

	// Default: un tenant creado sin namespace cae en el canónico, no en 'mc'.
	require.NoError(t, exec(`INSERT INTO tenants (name, slug, type, owner_id) VALUES ('t8 default', 't8-default', 'BUSINESS', gen_random_uuid())`))
	require.Equal(t, 1, count(`SELECT count(*) FROM tenants WHERE slug='t8-default' AND namespace='mercado-cercano'`))

	// Fail-closed: si conviviera ('mc', s) con ('mercado-cercano', s), 026 aborta en vez de fusionar.
	require.NoError(t, insert("mc", "t8-shared-slug"))
	err = migrateSQL("026_tenants_namespace_canonical.up.sql")
	require.True(t, errors.As(err, &pqErr), "la normalización no debe fusionar tenants: %v", err)
	require.Equal(t, pq.ErrorCode("23505"), pqErr.Code)
	require.NoError(t, exec(`DELETE FROM tenants WHERE namespace='mc' AND slug='t8-shared-slug'`))

	// Sentinel completo de otro producto: no cambia ni al bajar ni al subir 026.
	whatsappSnapshot := func() string {
		t.Helper()
		var row string
		require.NoError(t, super.QueryRowContext(ctx,
			`SELECT row_to_json(t)::text FROM tenants t WHERE namespace='whatsapp-agent' AND slug='t8-shared-slug'`).Scan(&row))
		return row
	}
	whatsappBefore := whatsappSnapshot()
	canonicalBefore := count(`SELECT count(*) FROM tenants WHERE namespace='mercado-cercano'`)

	// Down/up del SQL real como account_migrator NOBYPASSRLS (no superusuario).
	// Sólo PG efímero: no se revierte la migración aplicada en el lab.
	require.NoError(t, migrateSQL("026_tenants_namespace_canonical.down.sql"))
	require.Equal(t, whatsappBefore, whatsappSnapshot())
	require.Equal(t, canonicalBefore, count(`SELECT count(*) FROM tenants WHERE namespace='mc'`))
	require.Equal(t, 0, count(`SELECT count(*) FROM tenants WHERE namespace = 'mercado-cercano'`))
	require.NoError(t, super.QueryRowContext(ctx,
		`SELECT column_default FROM information_schema.columns WHERE table_name='tenants' AND column_name='namespace'`).Scan(&def))
	require.Contains(t, def, "'mc'")
	require.NoError(t, migrateSQL("026_tenants_namespace_canonical.up.sql"))
	require.Equal(t, 0, count(`SELECT count(*) FROM tenants WHERE namespace = 'mc'`))
	require.Equal(t, canonicalBefore, count(`SELECT count(*) FROM tenants WHERE namespace='mercado-cercano'`))
	require.Equal(t, whatsappBefore, whatsappSnapshot())
}
