package repository_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/tenancy/domain/entity"
	"github.com/hornosg/iam-service/src/tenancy/infrastructure/persistence/repository"
)

// TEST-ACC-E04-T3 (unidad, sin DB). Verifican la forma de cada operación del repo de config:
// TX abierta, `SET LOCAL app.tenant_id` ANTES de cualquier SQL, SQL sólo por la TX, bootstrap
// insert-only y contador publicado sólo tras commit. NO prueban concurrencia PostgreSQL —eso lo
// prueba test/tenancy/integration con `-tags integration` (ADR-005 §Bootstrap atómico: un mock
// no sustituye esas pruebas); acá se fija lo que un mock sí puede afirmar.

const cfgKey = "catalog.stock_policy"

var (
	selectByKey   = `SELECT id, tenant_id, config_key, config_value, created_at, updated_at\s+FROM tenant_config\s+WHERE tenant_id = \$1 AND config_key = \$2`
	adminUpsert   = `INSERT INTO tenant_config .* ON CONFLICT \(tenant_id, config_key\)\s+DO UPDATE SET`
	insertOnly    = `INSERT INTO tenant_config .* ON CONFLICT \(tenant_id, config_key\) DO NOTHING\s*$`
	configColumns = []string{"id", "tenant_id", "config_key", "config_value", "created_at", "updated_at"}
)

type configRepo interface {
	GetByKey(ctx context.Context, tenantID uuid.UUID, key string) (*entity.TenantConfig, bool, error)
	Save(ctx context.Context, config *entity.TenantConfig) error
	InsertMissing(ctx context.Context, tenantID uuid.UUID, configs []*entity.TenantConfig) (int, error)
}

// build arma el repo real sobre sqlmock; expectativas en orden estricto (default de sqlmock),
// así un SQL que corra antes del SET LOCAL falla el test.
func build(t *testing.T, hook repository.BeforeCommitHook) (configRepo, sqlmock.Sqlmock) {
	t.Helper()
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, m.ExpectationsWereMet())
		_ = db.Close()
	})
	if hook == nil {
		return repository.NewPostgresTenantConfigRepository(db), m
	}
	return repository.NewPostgresTenantConfigRepositoryWithHook(db, hook), m
}

func expectRLS(m sqlmock.Sqlmock, tenantID uuid.UUID) {
	m.ExpectBegin()
	m.ExpectExec(regexp.QuoteMeta("SET LOCAL app.tenant_id = '" + tenantID.String() + "'")).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

func insertArgs(c *entity.TenantConfig) []driver.Value {
	return []driver.Value{c.ID, c.TenantID, c.Key, c.Value, c.CreatedAt, c.UpdatedAt}
}

// --- GetByKey ---

func TestTenantConfigRepo_GetByKey_FijaTenantAntesDelSelect(t *testing.T) {
	repo, m := build(t, nil)
	tenantID := uuid.New()
	now := time.Now()
	id := uuid.New()
	expectRLS(m, tenantID)
	m.ExpectQuery(selectByKey).WithArgs(tenantID, cfgKey).
		WillReturnRows(sqlmock.NewRows(configColumns).AddRow(id, tenantID, cfgKey, "IGNORE_STOCK", now, now))
	m.ExpectCommit()

	cfg, found, err := repo.GetByKey(context.Background(), tenantID, cfgKey)

	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, id, cfg.ID)
	assert.Equal(t, tenantID, cfg.TenantID)
	assert.Equal(t, "IGNORE_STOCK", cfg.Value)
}

func TestTenantConfigRepo_GetByKey_SinFila_NoEncontradoSinError(t *testing.T) {
	repo, m := build(t, nil)
	tenantID := uuid.New()
	expectRLS(m, tenantID)
	m.ExpectQuery(selectByKey).WithArgs(tenantID, cfgKey).WillReturnRows(sqlmock.NewRows(configColumns))
	m.ExpectCommit()

	cfg, found, err := repo.GetByKey(context.Background(), tenantID, cfgKey)

	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, cfg)
}

func TestTenantConfigRepo_GetByKey_ErrorDeQuery_RollbackYError(t *testing.T) {
	repo, m := build(t, nil)
	tenantID := uuid.New()
	expectRLS(m, tenantID)
	m.ExpectQuery(selectByKey).WithArgs(tenantID, cfgKey).WillReturnError(errors.New("db caída"))
	m.ExpectRollback()

	cfg, found, err := repo.GetByKey(context.Background(), tenantID, cfgKey)

	require.Error(t, err)
	assert.False(t, found)
	assert.Nil(t, cfg)
}

// NEGATIVO fail-closed: sin tenant no hay GUC posible → no se abre ni una TX. Si el repo
// consultara igual (pool sin GUC), sqlmock fallaría por Begin/Query no esperados.
func TestTenantConfigRepo_TenantNil_NoTocaLaBase(t *testing.T) {
	repo, _ := build(t, nil)
	ctx := context.Background()

	_, found, err := repo.GetByKey(ctx, uuid.Nil, cfgKey)
	require.Error(t, err)
	assert.False(t, found)

	require.Error(t, repo.Save(ctx, entity.NewTenantConfig(uuid.Nil, cfgKey, "X")))

	n, err := repo.InsertMissing(ctx, uuid.Nil, []*entity.TenantConfig{entity.NewTenantConfig(uuid.Nil, cfgKey, "X")})
	require.Error(t, err)
	assert.Equal(t, 0, n)
}

// --- Save (upsert administrativo) ---

func TestTenantConfigRepo_Save_UpsertAdminBajoGUCDelTenantDeLaConfig(t *testing.T) {
	repo, m := build(t, nil)
	cfg := entity.NewTenantConfig(uuid.New(), cfgKey, "IGNORE_STOCK")
	expectRLS(m, cfg.TenantID)
	m.ExpectExec(adminUpsert).WithArgs(insertArgs(cfg)...).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()

	require.NoError(t, repo.Save(context.Background(), cfg))
}

// --- InsertMissing (bootstrap insert-only, ADR-005 §Bootstrap atómico) ---

func TestTenantConfigRepo_InsertMissing_UnaTXInsertOnlyCuentaSoloInsertadas(t *testing.T) {
	repo, m := build(t, nil)
	tenantID := uuid.New()
	nueva := entity.NewTenantConfig(tenantID, cfgKey, "REQUIRE_STOCK")
	existente := entity.NewTenantConfig(tenantID, "catalog.otra", "Y")
	// Una sola TX para todo el lote: un solo Begin/SET LOCAL/Commit. Sin pre-read (ningún SELECT).
	expectRLS(m, tenantID)
	m.ExpectExec(insertOnly).WithArgs(insertArgs(nueva)...).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec(insertOnly).WithArgs(insertArgs(existente)...).WillReturnResult(sqlmock.NewResult(0, 0)) // conflicto
	m.ExpectCommit()

	n, err := repo.InsertMissing(context.Background(), tenantID, []*entity.TenantConfig{nueva, existente})

	require.NoError(t, err)
	assert.Equal(t, 1, n, "el conflicto no cuenta")
}

// NEGATIVO: un fallo a mitad de lote revierte TODO y el contador es 0, aunque una fila previa
// del mismo lote haya reportado RowsAffected=1. Publicar 1 acá sería afirmar una fila que no persistió.
func TestTenantConfigRepo_InsertMissing_ErrorAMitadDeLote_Rollback0(t *testing.T) {
	repo, m := build(t, nil)
	tenantID := uuid.New()
	a := entity.NewTenantConfig(tenantID, cfgKey, "REQUIRE_STOCK")
	b := entity.NewTenantConfig(tenantID, "catalog.otra", "Y")
	expectRLS(m, tenantID)
	m.ExpectExec(insertOnly).WithArgs(insertArgs(a)...).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec(insertOnly).WithArgs(insertArgs(b)...).WillReturnError(errors.New("conexión perdida"))
	m.ExpectRollback()

	n, err := repo.InsertMissing(context.Background(), tenantID, []*entity.TenantConfig{a, b})

	require.Error(t, err)
	assert.Equal(t, 0, n)
}

// NEGATIVO: si el commit falla, el resultado es ambiguo → 0 + error (nunca el contador parcial).
func TestTenantConfigRepo_InsertMissing_CommitFalla_0YError(t *testing.T) {
	repo, m := build(t, nil)
	tenantID := uuid.New()
	a := entity.NewTenantConfig(tenantID, cfgKey, "REQUIRE_STOCK")
	expectRLS(m, tenantID)
	m.ExpectExec(insertOnly).WithArgs(insertArgs(a)...).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit().WillReturnError(errors.New("commit: conexión perdida"))

	n, err := repo.InsertMissing(context.Background(), tenantID, []*entity.TenantConfig{a})

	require.Error(t, err)
	assert.Equal(t, 0, n)
}

func TestTenantConfigRepo_InsertMissing_RowsAffectedFalla_0YRollback(t *testing.T) {
	repo, m := build(t, nil)
	tenantID := uuid.New()
	a := entity.NewTenantConfig(tenantID, cfgKey, "REQUIRE_STOCK")
	expectRLS(m, tenantID)
	m.ExpectExec(insertOnly).WithArgs(insertArgs(a)...).
		WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected no disponible")))
	m.ExpectRollback()

	n, err := repo.InsertMissing(context.Background(), tenantID, []*entity.TenantConfig{a})

	require.Error(t, err)
	assert.Equal(t, 0, n)
}

func TestTenantConfigRepo_InsertMissing_HookPreCommitFalla_Rollback0(t *testing.T) {
	boom := errors.New("rollback forzado")
	repo, m := build(t, func(context.Context) error { return boom })
	tenantID := uuid.New()
	a := entity.NewTenantConfig(tenantID, cfgKey, "REQUIRE_STOCK")
	expectRLS(m, tenantID)
	m.ExpectExec(insertOnly).WithArgs(insertArgs(a)...).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectRollback()

	n, err := repo.InsertMissing(context.Background(), tenantID, []*entity.TenantConfig{a})

	require.ErrorIs(t, err, boom)
	assert.Equal(t, 0, n)
}

// NEGATIVO: una config de otro tenant en el lote se rechaza ANTES de abrir la TX. Si se
// insertara, la fila quedaría con un tenant_id distinto al GUC (lo frena el WITH CHECK, pero
// el repo no debe depender sólo de eso ni gastar una TX).
func TestTenantConfigRepo_InsertMissing_TenantAjeno_SinTX(t *testing.T) {
	repo, _ := build(t, nil)
	tenantID := uuid.New()
	ajena := entity.NewTenantConfig(uuid.New(), cfgKey, "X")

	n, err := repo.InsertMissing(context.Background(), tenantID, []*entity.TenantConfig{
		entity.NewTenantConfig(tenantID, "catalog.otra", "Y"), ajena,
	})

	require.ErrorIs(t, err, repository.ErrTenantConfigTenantMismatch)
	assert.Equal(t, 0, n)
}

// --- Estructural (criterio (f) de T3 + ADR-005: SQL sólo por la TX) ---

// NEGATIVO de bypass: el repo de config no abre conexiones propias ni usa credenciales
// privilegiadas, corre todo por WithRLSInTransaction y nunca ejecuta SQL contra r.db (el pool)
// en lugar de la TX que tiene el GUC fijado.
func TestTenantConfigRepo_NoBypasseaRLS(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "..")
	files, err := filepath.Glob(filepath.Join(root, "src", "tenancy", "infrastructure", "persistence", "repository", "*config*.go"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "no se encontró el repo de config: el path cambió y este test quedaría vacío")

	forbidden := regexp.MustCompile(`sql\.Open|superuser|postgres:postgres|r\.db\.(Exec|Query|Begin|Prepare)`)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		require.NoError(t, err)
		src := string(raw)
		assert.Empty(t, forbidden.FindAllString(src, -1), "%s: patrón de bypass de RLS", filepath.Base(f))
		if !strings.HasSuffix(f, "_test.go") {
			assert.Contains(t, src, "WithRLSInTransaction", "%s: no fija app.tenant_id por TX", filepath.Base(f))
			assert.NotRegexp(t, `ON CONFLICT[^;]*DO UPDATE[^;]*DO NOTHING|DO NOTHING[^;]*DO UPDATE`, src)
		}
	}
}
