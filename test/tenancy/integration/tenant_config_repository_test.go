//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	sharedmigrate "github.com/hornosg/go-shared/migrate"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	iam "github.com/hornosg/iam-service"
	"github.com/hornosg/iam-service/src/tenancy/application/usecase"
	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/src/tenancy/infrastructure/persistence/repository"
)

// TEST-ACC-E04-T3: aceptación de ADR-005 §Bootstrap atómico sobre PostgreSQL real (PG15 como
// el lab), migraciones reales y dos conexiones independientes `account_app` (NOBYPASSRLS). El
// orden se fuerza con barreras (hook previo al commit + bloqueo observado en pg_stat_activity),
// nunca con sleeps; todo espera con timeout.

const key = "catalog.stock_policy"

func TestTenantConfigRepository_BootstrapAtomico(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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
	open := func(role string, maxConns int) *sql.DB {
		t.Helper()
		u, err := url.Parse(dsn)
		require.NoError(t, err)
		u.User = url.UserPassword(role, "test-only-config")
		db, err := sql.Open("postgres", u.String())
		require.NoError(t, err)
		db.SetMaxOpenConns(maxConns)
		t.Cleanup(func() { _ = db.Close() })
		var user string
		var privileged bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT current_user, rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&user, &privileged))
		require.Equal(t, role, user)
		require.False(t, privileged)
		return db
	}
	up024, err := iam.MigrationsFS.ReadFile("migrations/024_create_migrator_role.up.sql")
	require.NoError(t, err)
	_, err = super.ExecContext(ctx, string(up024))
	require.NoError(t, err)
	_, err = super.ExecContext(ctx, `ALTER ROLE account_migrator PASSWORD 'test-only-config'`)
	require.NoError(t, err)
	require.NoError(t, sharedmigrate.RunMigrations(open("account_migrator", 10), iam.MigrationsFS, "iam_db"))
	_, err = super.ExecContext(ctx, `ALTER ROLE account_app PASSWORD 'test-only-config'`)
	require.NoError(t, err)

	// Una conexión física por pool: la sesión que fija el GUC es la misma que después se inspecciona.
	adminDB, bootDB := open("account_app", 1), open("account_app", 1)
	plainAdmin := repository.NewPostgresTenantConfigRepository(adminDB)
	plainBoot := repository.NewPostgresTenantConfigRepository(bootDB)

	await := func(t *testing.T, ch <-chan struct{}, what string) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(20 * time.Second):
			t.Fatalf("timeout esperando %s", what)
		}
	}
	// holder retiene la TX abierta tras el SQL y antes del commit hasta recibir el veredicto.
	type holder struct {
		hook    repository.BeforeCommitHook
		held    chan struct{}
		release chan error
	}
	newHolder := func() *holder {
		h := &holder{held: make(chan struct{}), release: make(chan error, 1)}
		var once sync.Once
		h.hook = func(ctx context.Context) error {
			once.Do(func() { close(h.held) })
			select {
			case err := <-h.release:
				return err
			case <-time.After(20 * time.Second):
				return errors.New("holder: timeout sin release")
			}
		}
		return h
	}
	// waitLockWait espera a que exactamente una sesión account_app quede bloqueada por un lock.
	waitLockWait := func(t *testing.T) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			var n int
			require.NoError(t, super.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE usename='account_app' AND wait_event_type='Lock'`).Scan(&n))
			if n == 1 {
				return
			}
			time.Sleep(10 * time.Millisecond) // intervalo de sondeo, no prueba de orden
		}
		t.Fatal("timeout: la segunda TX nunca quedó esperando el lock del índice único")
	}
	type result struct {
		n   int
		err error
	}
	row := func(t *testing.T, tenantID uuid.UUID, k string) *entity.TenantConfig {
		t.Helper()
		cfg, found, err := plainAdmin.GetByKey(ctx, tenantID, k)
		require.NoError(t, err)
		require.True(t, found, "fila %s/%s ausente", tenantID, k)
		return cfg
	}
	ts := func(tm time.Time) string { return tm.Round(time.Microsecond).Format("2006-01-02 15:04:05.000000") }
	countRows := func(t *testing.T, tenantID uuid.UUID) int {
		t.Helper()
		var n int
		require.NoError(t, superCount(ctx, super, tenantID, &n))
		return n
	}

	var tenantA uuid.UUID

	t.Run("A_admin_primero_bootstrap_pierde_sin_tocar_nada", func(t *testing.T) {
		tenantA = uuid.New()
		h := newHolder()
		admin := repository.NewPostgresTenantConfigRepositoryWithHook(adminDB, h.hook)
		adminCfg := entity.NewTenantConfig(tenantA, key, "IGNORE_STOCK")
		adminDone := make(chan error, 1)
		go func() { adminDone <- admin.Save(ctx, adminCfg) }()
		await(t, h.held, "TX admin retenida")

		bootDone := make(chan result, 1)
		go func() {
			n, err := usecase.NewBootstrapTenantConfigUseCase(plainBoot).Execute(ctx, tenantA)
			bootDone <- result{n, err}
		}()
		waitLockWait(t)
		h.release <- nil
		require.NoError(t, <-adminDone)
		res := <-bootDone
		require.NoError(t, res.err)
		require.Equal(t, 0, res.n, "bootstrap en conflicto no cuenta")

		got := row(t, tenantA, key)
		require.Equal(t, "IGNORE_STOCK", got.Value, "el valor admin sobrevive")
		require.Equal(t, adminCfg.ID, got.ID)
		require.Equal(t, ts(adminCfg.CreatedAt), ts(got.CreatedAt))
		require.Equal(t, ts(adminCfg.UpdatedAt), ts(got.UpdatedAt), "bootstrap no toca updated_at")
	})

	t.Run("repetir_bootstrap_no_cambia_ningun_campo", func(t *testing.T) {
		before := row(t, tenantA, key)
		n, err := usecase.NewBootstrapTenantConfigUseCase(plainBoot).Execute(ctx, tenantA)
		require.NoError(t, err)
		require.Equal(t, 0, n)
		require.Equal(t, before, row(t, tenantA, key))
	})

	t.Run("B_bootstrap_primero_admin_gana", func(t *testing.T) {
		tenantB := uuid.New()
		h := newHolder()
		boot := repository.NewPostgresTenantConfigRepositoryWithHook(bootDB, h.hook)
		bootCfg := entity.NewTenantConfig(tenantB, key, "REQUIRE_STOCK")
		bootDone := make(chan result, 1)
		go func() {
			n, err := boot.InsertMissing(ctx, tenantB, []*entity.TenantConfig{bootCfg})
			bootDone <- result{n, err}
		}()
		await(t, h.held, "TX bootstrap retenida")

		adminCfg := entity.NewTenantConfig(tenantB, key, "IGNORE_STOCK")
		adminDone := make(chan error, 1)
		go func() { adminDone <- plainAdmin.Save(ctx, adminCfg) }()
		waitLockWait(t)
		h.release <- nil
		res := <-bootDone
		require.NoError(t, res.err)
		require.Equal(t, 1, res.n)
		require.NoError(t, <-adminDone)

		got := row(t, tenantB, key)
		require.Equal(t, "IGNORE_STOCK", got.Value, "el valor admin queda final")
		require.Equal(t, bootCfg.ID, got.ID, "el upsert admin conserva el ID existente")
		require.Equal(t, ts(bootCfg.CreatedAt), ts(got.CreatedAt))
		require.Equal(t, ts(adminCfg.UpdatedAt), ts(got.UpdatedAt), "el admin sí puede mover updated_at")
	})

	t.Run("rollback_del_lote_devuelve_0_y_no_persiste", func(t *testing.T) {
		tenantR := uuid.New()
		boom := errors.New("rollback forzado antes del commit")
		failing := repository.NewPostgresTenantConfigRepositoryWithHook(bootDB, func(context.Context) error { return boom })
		lote := []*entity.TenantConfig{
			entity.NewTenantConfig(tenantR, key, "REQUIRE_STOCK"),
			entity.NewTenantConfig(tenantR, "catalog.otra", "X"),
		}
		n, err := failing.InsertMissing(ctx, tenantR, lote)
		require.ErrorIs(t, err, boom)
		require.Equal(t, 0, n)
		require.Equal(t, 0, countRows(t, tenantR), "ninguna fila del lote persistió")

		n, err = plainBoot.InsertMissing(ctx, tenantR, lote)
		require.NoError(t, err)
		require.Equal(t, 2, n, "liberada la TX, repetir cuenta sólo inserciones reales")
		n, err = plainBoot.InsertMissing(ctx, tenantR, lote)
		require.NoError(t, err)
		require.Equal(t, 0, n)
	})

	t.Run("lote_mixto_cuenta_solo_ausentes", func(t *testing.T) {
		tenantM := uuid.New()
		require.NoError(t, plainAdmin.Save(ctx, entity.NewTenantConfig(tenantM, key, "IGNORE_STOCK")))
		before := row(t, tenantM, key)
		n, err := plainBoot.InsertMissing(ctx, tenantM, []*entity.TenantConfig{
			entity.NewTenantConfig(tenantM, key, "REQUIRE_STOCK"),
			entity.NewTenantConfig(tenantM, "catalog.otra", "Y"),
		})
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.Equal(t, before, row(t, tenantM, key), "la preexistente queda intacta")
		require.Equal(t, "Y", row(t, tenantM, "catalog.otra").Value)
	})

	t.Run("dos_bootstraps_concurrentes_no_duplican_contadores", func(t *testing.T) {
		tenantC := uuid.New()
		h := newHolder()
		first := repository.NewPostgresTenantConfigRepositoryWithHook(adminDB, h.hook)
		firstDone, secondDone := make(chan result, 1), make(chan result, 1)
		go func() {
			n, err := usecase.NewBootstrapTenantConfigUseCase(first).Execute(ctx, tenantC)
			firstDone <- result{n, err}
		}()
		await(t, h.held, "primer bootstrap retenido")
		go func() {
			n, err := usecase.NewBootstrapTenantConfigUseCase(plainBoot).Execute(ctx, tenantC)
			secondDone <- result{n, err}
		}()
		waitLockWait(t)
		h.release <- nil
		r1, r2 := <-firstDone, <-secondDone
		require.NoError(t, r1.err)
		require.NoError(t, r2.err)
		require.Equal(t, countRows(t, tenantC), r1.n+r2.n, "suma de contadores = filas nuevas")
		require.Equal(t, len(usecase.DefaultConfigs), countRows(t, tenantC))
	})

	t.Run("lote_con_tenant_ajeno_se_rechaza", func(t *testing.T) {
		tenantX, otro := uuid.New(), uuid.New()
		n, err := plainBoot.InsertMissing(ctx, tenantX, []*entity.TenantConfig{entity.NewTenantConfig(otro, key, "X")})
		require.ErrorIs(t, err, repository.ErrTenantConfigTenantMismatch)
		require.Equal(t, 0, n)
		require.Equal(t, 0, countRows(t, otro))
	})

	t.Run("RLS_tenant_A_B_y_sin_GUC_residual", func(t *testing.T) {
		_, found, err := plainBoot.GetByKey(ctx, uuid.New(), key)
		require.NoError(t, err)
		require.False(t, found, "otro tenant no ve la config de A")
		// Ambas sesiones hicieron commits y (bootDB) un rollback: el GUC no sobrevive a la TX.
		for name, db := range map[string]*sql.DB{"admin": adminDB, "boot": bootDB} {
			var guc string
			var visible int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT coalesce(current_setting('app.tenant_id', true), '')`).Scan(&guc), name)
			require.Empty(t, guc, "%s: app.tenant_id residual", name)
			require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM tenant_config`).Scan(&visible), name)
			require.Zero(t, visible, "%s: sin GUC la RLS no expone filas", name)
		}
		var total int
		require.NoError(t, super.QueryRowContext(ctx, `SELECT count(*) FROM tenant_config`).Scan(&total))
		require.Positive(t, total, "las filas existen: el 0 de arriba es RLS, no tabla vacía")
	})
}

func superCount(ctx context.Context, super *sql.DB, tenantID uuid.UUID, n *int) error {
	return super.QueryRowContext(ctx, `SELECT count(*) FROM tenant_config WHERE tenant_id=$1`, tenantID).Scan(n)
}
