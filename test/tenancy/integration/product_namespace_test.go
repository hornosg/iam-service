//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	sharedmetrics "github.com/hornosg/go-shared/infrastructure/metrics"
	sharedmigrate "github.com/hornosg/go-shared/migrate"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	iam "github.com/hornosg/iam-service"
	sharedctx "github.com/hornosg/iam-service/src/shared/context"
	"github.com/hornosg/iam-service/src/tenancy/application/request"
	"github.com/hornosg/iam-service/src/tenancy/application/usecase"
	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/src/tenancy/domain/exception"
	"github.com/hornosg/iam-service/src/tenancy/domain/value_object"
	"github.com/hornosg/iam-service/src/tenancy/infrastructure/persistence/repository"
)

// TEST-ACC-E04-T9-DB: no mocks for UNIQUE or RLS. Provision has no tenant context;
// its precheck cannot see other rows. The real INSERT must still map 23505 correctly.
func TestProductNamespace_Postgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pg, err := postgres.Run(ctx, "pgvector/pgvector:pg15", postgres.WithDatabase("iam_db"), postgres.WithUsername("postgres"), postgres.WithPassword("postgres"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pg.Terminate(context.Background())) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	super, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer super.Close()
	ddl, err := iam.MigrationsFS.ReadFile("migrations/024_create_migrator_role.up.sql")
	require.NoError(t, err)
	_, err = super.ExecContext(ctx, string(ddl))
	require.NoError(t, err)
	_, err = super.ExecContext(ctx, `ALTER ROLE account_migrator PASSWORD 'test-only-namespace'`)
	require.NoError(t, err)
	open := func(role string) *sql.DB {
		t.Helper()
		u, err := url.Parse(dsn)
		require.NoError(t, err)
		u.User = url.UserPassword(role, "test-only-namespace")
		db, err := sql.Open("postgres", u.String())
		require.NoError(t, err)
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		var actual string
		var privileged bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT current_user, rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&actual, &privileged))
		require.Equal(t, role, actual)
		require.False(t, privileged)
		return db
	}
	require.NoError(t, sharedmigrate.RunMigrations(open("account_migrator"), iam.MigrationsFS, "iam_db"))
	_, err = super.ExecContext(ctx, `ALTER ROLE account_app PASSWORD 'test-only-namespace'`)
	require.NoError(t, err)
	app := open("account_app")
	repo := repository.NewPostgresTenantRepository(app)
	create := usecase.NewCreateTenantUseCase(repo, sharedmetrics.NoopRecorder{})
	lookup := usecase.NewGetTenantBySlugUseCase(repo)
	owner := uuid.New()
	makeRequest := func(namespace, slug string) *request.CreateTenantRequest {
		return &request.CreateTenantRequest{Namespace: namespace, Name: "Store", Slug: slug, Description: "Namespace test", Type: "BUSINESS", OwnerID: owner.String()}
	}
	first, err := create.Execute(ctx, makeRequest("", "same"))
	require.NoError(t, err)
	require.Equal(t, "mercado-cercano", first.Namespace)
	second, err := create.Execute(ctx, makeRequest("whatsapp-agent", "same"))
	require.NoError(t, err)
	require.Equal(t, "whatsapp-agent", second.Namespace)
	exists, err := repo.ExistsBySlug(ctx, "mercado-cercano", "same")
	require.NoError(t, err)
	require.False(t, exists, "RLS precheck must stay fail-closed without authority")
	_, err = create.Execute(ctx, makeRequest("mercado-cercano", "same"))
	require.ErrorIs(t, err, exception.ErrSlugAlreadyExists, "23505 is authoritative even when precheck is false")
	admin := sharedctx.WithSystemAdmin(ctx) // Simulates already verified authority; never granted by namespace.
	for namespace, id := range map[string]uuid.UUID{"mercado-cercano": first.ID, "whatsapp-agent": second.ID} {
		got, err := lookup.Execute(admin, namespace, "same")
		require.NoError(t, err)
		require.Equal(t, id, got.ID)
		require.Equal(t, namespace, got.Namespace)
		found, err := repo.ExistsBySlug(admin, namespace, "same")
		require.NoError(t, err)
		require.True(t, found)
	}
	found, err := repo.ExistsBySlug(admin, "another-product", "same")
	require.NoError(t, err)
	require.False(t, found)
	_, err = lookup.Execute(ctx, "mercado-cercano", "same")
	require.ErrorIs(t, err, exception.ErrTenantNotFound)
	_, err = lookup.Execute(sharedctx.WithTenantID(ctx, first.ID), "whatsapp-agent", "same")
	require.ErrorIs(t, err, exception.ErrTenantNotFound)

	// TEST-T9-DB: con RLS el namespace incorrecto tampoco puede devolver la fila
	// propia sólo porque coincide el slug. Sin WHERE namespace, este caso falla.
	t.Run("own-tenant-requires-matching-product-namespace", func(t *testing.T) {
		own := sharedctx.WithTenantID(ctx, second.ID)
		got, err := lookup.Execute(own, "whatsapp-agent", "same")
		require.NoError(t, err)
		require.Equal(t, second.ID, got.ID)
		require.Equal(t, "whatsapp-agent", got.Namespace)

		for _, namespace := range []string{"mercado-cercano", ""} {
			got, err := lookup.Execute(own, namespace, "same")
			require.ErrorIs(t, err, exception.ErrTenantNotFound)
			require.Nil(t, got, "namespace erróneo no debe devolver ni la fila propia")
		}
	})
	t.Run("default-lookup-never-falls-back-to-another-product", func(t *testing.T) {
		// Arrange: slug que existe únicamente en whatsapp-agent.
		created, err := create.Execute(ctx, makeRequest("whatsapp-agent", "only-whatsapp"))
		require.NoError(t, err)
		got, err := lookup.Execute(admin, "whatsapp-agent", "only-whatsapp")
		require.NoError(t, err)
		require.Equal(t, created.ID, got.ID)

		// Act / Assert: ni default ni producto inexistente admiten fallback global.
		for _, namespace := range []string{"", "mercado-cercano", "missing-product"} {
			got, err := lookup.Execute(admin, namespace, "only-whatsapp")
			require.ErrorIs(t, err, exception.ErrTenantNotFound)
			require.Nil(t, got)
		}
	})
	t.Run("mc-is-rejected-with-otherwise-valid-input", func(t *testing.T) {
		got, err := create.Execute(ctx, makeRequest("mc", "forbidden-alias"))
		require.ErrorIs(t, err, exception.ErrInvalidNamespace)
		require.Nil(t, got)
		lookedUp, err := lookup.Execute(admin, "mc", "same")
		require.ErrorIs(t, err, exception.ErrInvalidNamespace)
		require.Nil(t, lookedUp)
		var count int
		require.NoError(t, super.QueryRowContext(ctx, `SELECT count(*) FROM tenants WHERE slug = 'forbidden-alias'`).Scan(&count))
		require.Zero(t, count, "rechazar mc no debe dejar escritura parcial")
	})
	// Both scan paths include namespace; updating unrelated properties must not reset it.
	own := sharedctx.WithTenantID(ctx, second.ID)
	got, err := repo.GetByID(own, second.ID)
	require.NoError(t, err)
	require.Equal(t, "whatsapp-agent", got.Namespace)
	got.UpdateDetails("Renamed", "Still same namespace")
	require.NoError(t, repo.Update(own, got))
	rows, err := repo.List(admin, 10, 0)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(rows), 2)
	for _, row := range rows {
		if row.ID == second.ID {
			require.Equal(t, "whatsapp-agent", row.Namespace)
		}
	}
	// Domain remains globally unique (same actual constraint on create and update).
	domain := entity.NewTenant("Domain", "domain", "Test", value_object.TenantTypeBusiness, owner)
	domain.Domain = "namespace.example.com"
	require.NoError(t, repo.Create(ctx, domain))
	duplicate := entity.NewTenant("Duplicate", "domain-other", "Test", value_object.TenantTypeBusiness, owner)
	duplicate.Namespace = "whatsapp-agent"
	duplicate.Domain = domain.Domain
	require.ErrorIs(t, repo.Create(ctx, duplicate), exception.ErrDomainAlreadyExists)
	got.Domain = domain.Domain
	require.ErrorIs(t, repo.Update(own, got), exception.ErrDomainAlreadyExists)
	var tenantGUC, adminGUC string
	require.NoError(t, app.QueryRowContext(ctx, `SELECT COALESCE(current_setting('app.tenant_id',true),''), COALESCE(current_setting('app.is_system_admin',true),'')`).Scan(&tenantGUC, &adminGUC))
	require.Empty(t, tenantGUC)
	require.Empty(t, adminGUC)
}
