package repository_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/go-shared/criteria"

	"github.com/hornosg/iam-service/src/identity/domain/entity"
	"github.com/hornosg/iam-service/src/identity/domain/exception"
	"github.com/hornosg/iam-service/src/identity/domain/value_object"
	"github.com/hornosg/iam-service/src/identity/infrastructure/persistence/repository"
	sharedctx "github.com/hornosg/iam-service/src/shared/context"
	"github.com/hornosg/iam-service/test/testsupport/fakedb"
	usermother "github.com/hornosg/iam-service/test/identity/user/domain/entity"
)

// userMother construye un usuario válido con el Object Mother del árbol de test.
func userMother() *entity.User {
	return usermother.Create().WithDefaults()
}

// ACC-E08 T2: cubre los caminos de PostgresUserRepository con la base falsa de
// testsupport/fakedb — el mismo harness del test RLS de auth, extendido con
// result sets programables para afirmar sobre los Scan y los filtros.

type driverValue = driver.Value

const (
	tenantIDDePrueba = "33333333-3333-3333-3333-333333333333"
	userIDDePrueba   = "55555555-5555-5555-5555-555555555555"
	roleIDDePrueba   = "66666666-6666-6666-6666-666666666666"
)

// filaCompleta son las 10 columnas del SELECT post-auth
// (id, email, password_hash, tenant_id, role_id, status, provider, federated_id, created_at, updated_at).
func filaCompleta() []driverValue {
	now := time.Now()
	return []driverValue{
		userIDDePrueba,
		"user@example.com",
		"$2a$10$hash",
		tenantIDDePrueba,
		roleIDDePrueba,
		"ACTIVE",
		"LOCAL",
		nil, // federated_id nullable
		now,
		now,
	}
}

// filaCredencial son las 8 columnas del SELECT pre-auth de GetByEmail (sin
// timestamps: el grant de iam_login de la migración 017 no las otorga).
func filaCredencial() []driverValue {
	return []driverValue{
		userIDDePrueba,
		"user@example.com",
		"$2a$10$hash",
		tenantIDDePrueba,
		roleIDDePrueba,
		"ACTIVE",
		"LOCAL",
		nil,
	}
}

func TestCreate_ConTenantEmiteSetLocalYCommitea(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))
	mother := userMother()

	require.NoError(t, repo.Create(ctx, mother))

	stmts := conn.Statements()
	require.Len(t, stmts, 2)
	assert.Equal(t, "SET LOCAL app.tenant_id = '"+tenantIDDePrueba+"'", stmts[0])
	assert.Contains(t, stmts[1], "INSERT INTO users")
	assert.True(t, conn.Committed)
}

func TestCreate_SinTenantCorrePorPoolDirecto(t *testing.T) {
	// withRLS degrada al pool cuando el contexto no trae tenant. Contra Postgres
	// real el INSERT es denegado por el WITH CHECK (fail-closed); acá se fija
	// que NO se disfraza de transacción RLS.
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	mother := userMother()

	require.NoError(t, repo.Create(context.Background(), mother))

	stmts := conn.Statements()
	require.Len(t, stmts, 1)
	assert.Contains(t, stmts[0], "INSERT INTO users")
	assert.False(t, conn.Committed, "sin tenant no hay transacción RLS que abrir")
}

func TestCreate_EmailDuplicadoDevuelveYaExiste(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	conn.ExecErr = &pq.Error{Code: "23505", Constraint: "users_email_tenant_unique"}
	conn.FailQueryContains = "INSERT INTO users"
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))

	err := repo.Create(ctx, userMother())

	assert.ErrorIs(t, err, exception.ErrUserAlreadyExists, "el constraint único se traduce al sentinela del dominio, no a un 500")
}

func TestCreate_ErrorDeMotorSeEnvuelve(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	conn.ExecErr = errors.New("conexión rota")
	conn.FailQueryContains = "INSERT INTO users"
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))

	err := repo.Create(ctx, userMother())

	require.Error(t, err)
	assert.ErrorContains(t, err, "error creating user")
	assert.NotErrorIs(t, err, exception.ErrUserAlreadyExists)
}

func TestGetByID_FilaCompletaHidrataLaEntidad(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{filaCompleta()}})

	user, err := repo.GetByID(ctx, uuid.MustParse(userIDDePrueba))

	require.NoError(t, err)
	assert.Equal(t, "user@example.com", user.Email.Value())
	assert.Equal(t, value_object.StatusActive, user.Status)
	assert.Equal(t, "", user.FederatedID, "federated_id NULL se hidrata como cadena vacía")
	assert.Equal(t, uuid.MustParse(tenantIDDePrueba), user.TenantID)

	stmts := conn.Statements()
	require.Len(t, stmts, 2)
	assert.Equal(t, "SET LOCAL app.tenant_id = '"+tenantIDDePrueba+"'", stmts[0])
	assert.Contains(t, stmts[1], "FROM users")
	assert.Contains(t, stmts[1], "WHERE id = $1")
	assert.True(t, conn.Committed)
}

func TestGetByID_SinFilasDevuelveNoEncontrado(t *testing.T) {
	db, _ := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))

	_, err := repo.GetByID(ctx, uuid.New())

	assert.ErrorIs(t, err, exception.ErrUserNotFound, "sql.ErrNoRows se traduce al sentinela del dominio, no a un error crudo")
}

func TestGetByID_EmailCorruptoEnLaFilaFalla(t *testing.T) {
	// Un dato inválido en la fila (email sin formato) no se hidrata a la fuerza:
	// el error nombra la causa real en vez de propagar un value object inválido.
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))
	fila := filaCompleta()
	fila[1] = "no-es-un-email"
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{fila}})

	_, err := repo.GetByID(ctx, uuid.MustParse(userIDDePrueba))

	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid email in database")
}

func TestGetByID_StatusDesconocidoEnLaFilaFalla(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))
	fila := filaCompleta()
	fila[5] = "ESTADO_RARO"
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{fila}})

	_, err := repo.GetByID(ctx, uuid.MustParse(userIDDePrueba))

	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid status in database")
}

func TestGetByEmail_ConTenantFiltraPorTenant(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	tenant := uuid.MustParse(tenantIDDePrueba)
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{filaCredencial()}})

	user, err := repo.GetByEmail(context.Background(), "user@example.com", &tenant)

	require.NoError(t, err)
	assert.Equal(t, "user@example.com", user.Email.Value())

	stmts := conn.Statements()
	require.Len(t, stmts, 1)
	assert.Contains(t, stmts[0], "AND tenant_id = $2")
	// Pre-auth: el SELECT de credencial NO trae timestamps (grant de la 017).
	assert.NotContains(t, stmts[0], "created_at", "el path de credencial no lee created_at (grant de iam_login)")
}

func TestGetByEmail_SinTenantBuscaGlobal(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{filaCredencial()}})

	user, err := repo.GetByEmail(context.Background(), "user@example.com", nil)

	require.NoError(t, err)
	assert.Equal(t, "user@example.com", user.Email.Value())

	stmts := conn.Statements()
	require.Len(t, stmts, 1)
	assert.NotContains(t, stmts[0], "AND tenant_id")
}

func TestUpdate_UnaFilaEsElCasoFeliz(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))

	require.NoError(t, repo.Update(ctx, userMother()))

	stmts := conn.Statements()
	require.Len(t, stmts, 2)
	assert.Equal(t, "SET LOCAL app.tenant_id = '"+tenantIDDePrueba+"'", stmts[0])
	assert.Contains(t, stmts[1], "UPDATE users")
	assert.True(t, conn.Committed)
}

func TestUpdate_CeroFilasDevuelveNoEncontrado(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	conn.ZeroRows = true
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))

	err := repo.Update(ctx, userMother())

	assert.ErrorIs(t, err, exception.ErrUserNotFound)
}

func TestDelete_CeroFilasDevuelveNoEncontrado(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	conn.ZeroRows = true
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))

	err := repo.Delete(ctx, uuid.New())

	assert.ErrorIs(t, err, exception.ErrUserNotFound)
}

func TestDelete_UnaFilaEsElCasoFeliz(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))

	require.NoError(t, repo.Delete(ctx, uuid.New()))

	stmts := conn.Statements()
	require.Len(t, stmts, 2)
	assert.Contains(t, stmts[1], "SET status = $2", "el delete es soft: cambia el status, no borra la fila")
	assert.True(t, conn.Committed)
}

func TestGetByTenant_DevuelveVariasFilasExcluyendoEliminados(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{filaCompleta(), filaCompleta()}})

	users, err := repo.GetByTenant(ctx, uuid.MustParse(tenantIDDePrueba), 10, 0)

	require.NoError(t, err)
	require.Len(t, users, 2)
	assert.Contains(t, conn.Statements()[1], "status != $2", "el listado excluye los soft-deleted")
}

func TestGetByStatus_DevuelveFilas(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{filaCompleta()}})

	users, err := repo.GetByStatus(ctx, value_object.StatusPending, 10, 0)

	require.NoError(t, err)
	require.Len(t, users, 1)
}

func TestGetByRole_DevuelveFilas(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{filaCompleta()}})

	users, err := repo.GetByRole(ctx, uuid.MustParse(roleIDDePrueba), 10, 0)

	require.NoError(t, err)
	require.Len(t, users, 1)
}

func TestScanUsers_EmailInvalidoEnUnaFilaFallaElListado(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))
	fila := filaCompleta()
	fila[1] = "no-es-un-email"
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{filaCompleta(), fila}})

	_, err := repo.GetByTenant(ctx, uuid.MustParse(tenantIDDePrueba), 10, 0)

	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid email in database")
}

func TestExistsByEmail_ConsultaExcluyendoEliminados(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	tenant := uuid.MustParse(tenantIDDePrueba)
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{{true}}})

	exists, err := repo.ExistsByEmail(context.Background(), "user@example.com", &tenant)

	require.NoError(t, err)
	assert.True(t, exists)
	assert.Contains(t, conn.Statements()[0], "status != $3", "un usuario soft-deleted no bloquea el re-registro del email")
}

func TestExistsByEmail_SinTenantNoFiltraPorTenant(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{{false}}})

	exists, err := repo.ExistsByEmail(context.Background(), "user@example.com", nil)

	require.NoError(t, err)
	assert.False(t, exists)
	assert.NotContains(t, conn.Statements()[0], "AND tenant_id")
}

func TestCountByTenant_Y_CountByStatus_DevuelvenElConteo(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{{int64(7)}}})

	count, err := repo.CountByTenant(ctx, uuid.MustParse(tenantIDDePrueba))

	require.NoError(t, err)
	assert.Equal(t, 7, count)

	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{{int64(3)}}})
	statusCount, err := repo.CountByStatus(ctx, value_object.StatusActive)

	require.NoError(t, err)
	assert.Equal(t, 3, statusCount)
}

func TestSearchByCriteria_AplicaElFiltroYDevuelveFilas(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{filaCompleta()}})

	crit := criteria.NewCriteria(
		criteria.NewFilters(criteria.NewFilter("status", criteria.OpEqual, value_object.StatusActive.String())),
		[]criteria.Order{criteria.NewOrder("created_at", criteria.OrderDesc)},
		criteria.NewPagination(1, 10),
	)

	users, err := repo.SearchByCriteria(ctx, crit)

	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, "user@example.com", users[0].Email.Value())
}

func TestCountByCriteria_DevuelveElConteo(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresUserRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantIDDePrueba))
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{{int64(12)}}})

	count, err := repo.CountByCriteria(ctx, criteria.NewCriteria(
		criteria.NewFilters(criteria.NewFilter("tenant_id", criteria.OpEqual, tenantIDDePrueba)),
		nil,
		criteria.NewPagination(1, 10),
	))

	require.NoError(t, err)
	assert.Equal(t, 12, count)
}