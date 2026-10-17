//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"net/url"
	"testing"
	"time"

	gmigrate "github.com/golang-migrate/migrate/v4"
	pgdriver "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	sharedmigrate "github.com/hornosg/go-shared/migrate"
	iam "github.com/hornosg/iam-service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TEST-ACC-E04-T2: SQL real, PG15 como el lab, identidades de conexión reales.
// No usa Skip, SET ROLE ni superusuario para probar aislamiento.
func TestTenantConfigMigration(t *testing.T) {
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
	open := func(role string) *sql.DB {
		t.Helper()
		u, err := url.Parse(dsn)
		require.NoError(t, err)
		u.User = url.UserPassword(role, "test-only-migration")
		db, err := sql.Open("postgres", u.String())
		require.NoError(t, err)
		// RunMigrations reserva conexiones del driver hasta cerrar el pool.
		if role == "account_migrator" {
			db.SetMaxOpenConns(10)
		} else {
			db.SetMaxOpenConns(1)
		}
		t.Cleanup(func() { _ = db.Close() })
		require.NoError(t, db.PingContext(ctx))
		var user string
		var privileged bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT current_user, rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&user, &privileged))
		require.Equal(t, role, user)
		require.False(t, privileged)
		return db
	}
	super, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = super.Close() })
	exec := func(db *sql.DB, q string, args ...any) {
		t.Helper()
		_, err := db.ExecContext(ctx, q, args...)
		require.NoError(t, err)
	}
	file := func(name string) string {
		t.Helper()
		b, err := iam.MigrationsFS.ReadFile("migrations/" + name)
		require.NoError(t, err)
		return string(b)
	}
	// Bootstrap privilegiado documentado de 024; la app nunca obtiene DDL.
	exec(super, file("024_create_migrator_role.up.sql"))
	exec(super, `ALTER ROLE account_migrator PASSWORD 'test-only-migration'`)
	migrator := open("account_migrator")
	require.NoError(t, sharedmigrate.RunMigrations(migrator, iam.MigrationsFS, "iam_db"))
	checkVersion := func() {
		t.Helper()
		var total, latest int
		require.NoError(t, migrator.QueryRowContext(ctx, `SELECT count(*), count(*) FILTER (WHERE version=26 AND NOT dirty) FROM schema_migrations`).Scan(&total, &latest))
		require.Equal(t, 1, total)
		require.Equal(t, 1, latest)
	}
	checkVersion()
	exec(super, `ALTER ROLE account_app PASSWORD 'test-only-migration'; ALTER ROLE iam_login PASSWORD 'test-only-migration'`)
	app, login := open("account_app"), open("iam_login")

	t.Run("catalogo_y_privilegios", func(t *testing.T) {
		var enabled, forced bool
		var owner string
		require.NoError(t, migrator.QueryRowContext(ctx, `SELECT relrowsecurity,relforcerowsecurity,pg_get_userbyid(relowner) FROM pg_class WHERE oid='tenant_config'::regclass`).Scan(&enabled, &forced, &owner))
		require.True(t, enabled)
		require.True(t, forced)
		require.Equal(t, "account_migrator", owner)
		var policies, columns, constraints, indexes, comments int
		require.NoError(t, migrator.QueryRowContext(ctx, `SELECT count(*) FROM pg_policies WHERE tablename='tenant_config' AND policyname='tenant_isolation' AND cmd='ALL' AND qual=with_check`).Scan(&policies))
		require.Equal(t, 1, policies)
		require.NoError(t, migrator.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name='tenant_config' AND is_nullable='NO' AND ((column_name IN ('id','tenant_id') AND data_type='uuid') OR (column_name='config_key' AND character_maximum_length=100) OR (column_name='config_value' AND character_maximum_length=255) OR (column_name IN ('created_at','updated_at') AND data_type='timestamp without time zone' AND column_default IS NOT NULL))`).Scan(&columns))
		require.Equal(t, 6, columns)
		require.NoError(t, migrator.QueryRowContext(ctx, `SELECT count(*) FROM pg_constraint WHERE conrelid='tenant_config'::regclass AND conname='uq_tenant_config' AND pg_get_constraintdef(oid)='UNIQUE (tenant_id, config_key)'`).Scan(&constraints))
		require.Equal(t, 1, constraints)
		require.NoError(t, migrator.QueryRowContext(ctx, `SELECT count(*) FROM pg_indexes WHERE tablename='tenant_config' AND indexname IN ('idx_tenant_config_tenant','idx_tenant_config_key')`).Scan(&indexes))
		require.Equal(t, 2, indexes)
		require.NoError(t, migrator.QueryRowContext(ctx, `SELECT count(*) FROM pg_description WHERE objoid='tenant_config'::regclass`).Scan(&comments))
		require.Equal(t, 7, comments)
		var dml, ddl bool
		require.NoError(t, app.QueryRowContext(ctx, `SELECT has_table_privilege(current_user,'tenant_config','SELECT,INSERT,UPDATE,DELETE'), has_schema_privilege(current_user,'public','CREATE')`).Scan(&dml, &ddl))
		require.True(t, dml)
		require.False(t, ddl)
		_, err := login.ExecContext(ctx, `SELECT * FROM tenant_config`)
		var pe *pq.Error
		require.ErrorAs(t, err, &pe)
		require.Equal(t, pq.ErrorCode("42501"), pe.Code)
	})
	a, b := uuid.New(), uuid.New()
	exec(super, `INSERT INTO tenant_config(tenant_id,config_key,config_value) VALUES($1,'catalog.stock_policy','A'),($2,'catalog.stock_policy','B')`, a, b)
	count := func(db *sql.DB, expected int) {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM tenant_config`).Scan(&n))
		require.Equal(t, expected, n)
	}
	t.Run("ausencia_reset_y_owner_force", func(t *testing.T) {
		count(app, 0)
		count(migrator, 0)
		exec(app, `RESET app.tenant_id`)
		count(app, 0)
		// El escape de tenants NO habilita config cross-tenant.
		exec(app, `SET app.is_system_admin='true'`)
		count(app, 0)
		exec(app, `RESET app.is_system_admin`)
	})
	for _, tenant := range []uuid.UUID{a, b} {
		t.Run("aislamiento_"+tenant.String(), func(t *testing.T) {
			tx, err := app.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer tx.Rollback()
			_, err = tx.ExecContext(ctx, `SELECT set_config('app.tenant_id',$1,true)`, tenant.String())
			require.NoError(t, err)
			var id uuid.UUID
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT tenant_id FROM tenant_config`).Scan(&id))
			require.Equal(t, tenant, id)
			other := a
			if tenant == a {
				other = b
			}
			var n int
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM tenant_config WHERE tenant_id=$1`, other).Scan(&n))
			require.Zero(t, n)
			for _, q := range []string{`UPDATE tenant_config SET config_value='intruso' WHERE tenant_id=$1`, `DELETE FROM tenant_config WHERE tenant_id=$1`} {
				result, err := tx.ExecContext(ctx, q, other)
				require.NoError(t, err)
				n, err := result.RowsAffected()
				require.NoError(t, err)
				require.Zero(t, n)
			}
			// WITH CHECK rechaza cambiar el tenant incluso de una fila visible.
			_, err = tx.ExecContext(ctx, `UPDATE tenant_config SET tenant_id=$1 WHERE tenant_id=$2`, other, tenant)
			var pe *pq.Error
			require.ErrorAs(t, err, &pe)
			require.Equal(t, pq.ErrorCode("42501"), pe.Code)
			require.NoError(t, tx.Rollback())
			count(app, 0)
		})
	}
	t.Run("dml_local_unicidad_y_pool", func(t *testing.T) {
		tx, err := app.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer tx.Rollback()
		_, err = tx.ExecContext(ctx, `SELECT set_config('app.tenant_id',$1,true)`, a.String())
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `INSERT INTO tenant_config(tenant_id,config_key,config_value) VALUES($1,'probe.key','original')`, a)
		require.NoError(t, err)
		r, err := tx.ExecContext(ctx, `UPDATE tenant_config SET config_value='nuevo' WHERE config_key='probe.key'`)
		require.NoError(t, err)
		n, err := r.RowsAffected()
		require.NoError(t, err)
		require.EqualValues(t, 1, n)
		r, err = tx.ExecContext(ctx, `DELETE FROM tenant_config WHERE config_key='probe.key'`)
		require.NoError(t, err)
		n, err = r.RowsAffected()
		require.NoError(t, err)
		require.EqualValues(t, 1, n)
		require.NoError(t, tx.Commit())
		count(app, 0)
		// INSERT ajeno denegado, no sólo SELECT filtrado.
		tx, err = app.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer tx.Rollback()
		_, err = tx.ExecContext(ctx, `SELECT set_config('app.tenant_id',$1,true)`, a.String())
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `INSERT INTO tenant_config(tenant_id,config_key,config_value) VALUES($1,'bad.key','x')`, b)
		var pe *pq.Error
		require.ErrorAs(t, err, &pe)
		require.Equal(t, pq.ErrorCode("42501"), pe.Code)
		require.NoError(t, tx.Rollback())
		count(app, 0)
		_, err = super.ExecContext(ctx, `INSERT INTO tenant_config(tenant_id,config_key,config_value) VALUES($1,'catalog.stock_policy','duplicado')`, a)
		require.ErrorAs(t, err, &pe)
		require.Equal(t, pq.ErrorCode("23505"), pe.Code)
	})
	t.Run("reaplicar_preserva_filas_y_restaura_grants", func(t *testing.T) {
		exec(migrator, file("025_create_tenant_config.up.sql"))
		require.NoError(t, sharedmigrate.RunMigrations(migrator, iam.MigrationsFS, "iam_db"))
		checkVersion()
		count(super, 2)
		exec(migrator, `REVOKE ALL ON tenant_config FROM account_app`)
		exec(migrator, file("018_create_app_role.up.sql"))
		count(app, 0) // Sin grant habría permission denied, no cero filas.
	})
	t.Run("rollback_y_upgrade_24_a_head", func(t *testing.T) {
		// Fixture efímera: golang-migrate baja a 24 (026 y 025), nunca se estampa versión a mano.
		source, err := iofs.New(iam.MigrationsFS, "migrations")
		require.NoError(t, err)
		driver, err := pgdriver.WithInstance(migrator, &pgdriver.Config{DatabaseName: "iam_db"})
		require.NoError(t, err)
		m, err := gmigrate.NewWithInstance("iofs", source, "iam_db", driver)
		require.NoError(t, err)
		require.NoError(t, m.Steps(-2))
		var absent bool
		require.NoError(t, migrator.QueryRowContext(ctx, `SELECT to_regclass('public.tenant_config') IS NULL`).Scan(&absent))
		require.True(t, absent)
		exec(migrator, file("025_create_tenant_config.down.sql")) // Down también idempotente.
		require.NoError(t, sharedmigrate.RunMigrations(migrator, iam.MigrationsFS, "iam_db"))
		checkVersion()
		count(app, 0)
		_, err = login.ExecContext(ctx, `SELECT * FROM tenant_config`)
		require.Error(t, err)
	})
}
