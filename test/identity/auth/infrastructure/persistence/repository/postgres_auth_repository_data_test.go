package repository_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/identity/infrastructure/persistence/repository"
	sharedctx "github.com/hornosg/iam-service/src/shared/context"
	"github.com/hornosg/iam-service/test/testsupport/fakedb"
)

// driverValue acorta driver.Value en los literales de filas programadas.
type driverValue = driver.Value

// ACC-E08 T2: complementa el test RLS de este directorio con los caminos que
// quedaron sin cubrir — los de lectura con fila presente (Scan real), los de
// error de motor diferenciados de "no encontrado", y los caminos felices que
// el test RLS sólo ejercitó por el lado del error.

func filaDeRefreshToken() []driverValue {
	return []driverValue{
		"44444444-4444-4444-4444-444444444444",
		"55555555-5555-5555-5555-555555555555",
		tenantDePrueba,
		"rt-xyz",
		time.Now().Add(time.Hour),
		time.Now(),
	}
}

func TestGetRefreshToken_FilaPresenteDevuelveElTokenYCommitea(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{filaDeRefreshToken()}})

	rt, err := repo.GetRefreshToken(context.Background(), "rt-xyz")

	require.NoError(t, err)
	assert.Equal(t, uuid.MustParse("44444444-4444-4444-4444-444444444444"), rt.ID)
	assert.Equal(t, uuid.MustParse(tenantDePrueba), rt.TenantID, "el tenant denormalizado es el que el caso de uso usa para fijar app.tenant_id")
	assert.Equal(t, "rt-xyz", rt.Token)

	stmts := conn.Statements()
	require.Len(t, stmts, 2)
	assert.Equal(t, "SET LOCAL app.refresh_digest = '"+refreshDigestParaTest("rt-xyz")+"'", stmts[0])
	assert.Contains(t, stmts[1], "FROM refresh_tokens")
	assert.True(t, conn.Committed)
}

func TestGetRefreshToken_ErrorDeMotorNoSeConfundeConNoEncontrado(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)
	conn.ExecErr = errors.New("conexión rota")
	conn.FailQueryContains = "FROM refresh_tokens"

	_, err := repo.GetRefreshToken(context.Background(), "rt-xyz")

	// Un fallo del motor NO es "token no encontrado": el caso de uso distingue
	// credencial inválida (401) de error interno (500).
	require.Error(t, err)
	assert.ErrorContains(t, err, "error obteniendo refresh token")
	assert.NotContains(t, err.Error(), "no encontrado")
	assert.True(t, conn.RolledBack, "la transacción desechable del escape se revierte")
}

func TestDeleteAllUserRefreshTokens_ConTenantEmiteSetLocalYCommitea(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantDePrueba))

	require.NoError(t, repo.DeleteAllUserRefreshTokens(ctx, uuid.New()))

	stmts := conn.Statements()
	require.Len(t, stmts, 2)
	assert.Equal(t, "SET LOCAL app.tenant_id = '"+tenantDePrueba+"'", stmts[0])
	assert.Contains(t, stmts[1], "DELETE FROM refresh_tokens WHERE user_id")
	assert.True(t, conn.Committed)
}

func TestIsTokenRevoked_MarcaPresenteDevuelveTrue(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantDePrueba))
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{{true}}})

	revoked, err := repo.IsTokenRevoked(ctx, uuid.New(), uuid.New(), time.Now().Unix())

	require.NoError(t, err)
	assert.True(t, revoked)

	stmts := conn.Statements()
	require.Len(t, stmts, 2)
	assert.Equal(t, "SET LOCAL app.tenant_id = '"+tenantDePrueba+"'", stmts[0])
	assert.Contains(t, stmts[1], "FROM revoked_tokens")
	assert.Contains(t, stmts[1], "scope = 'user'", "coteja también la marca de revoke-all (ACC-E02 T8i)")
	assert.True(t, conn.Committed)
}

func TestIsTokenRevoked_SinFilasDevuelveErrorDeMotor(t *testing.T) {
	// EXISTS nunca devuelve cero filas contra un Postgres real, pero el repo
	// trata el camino igual: el error sube envuelto, no un false silencioso.
	db, _ := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantDePrueba))

	_, err := repo.IsTokenRevoked(ctx, uuid.New(), uuid.New(), time.Now().Unix())

	require.Error(t, err)
	assert.ErrorContains(t, err, "error verificando token revocado")
}

func TestCleanupExpiredRevocations_ErrorDeMotorDevuelveCeroYError(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)
	conn.ExecErr = errors.New("conexión rota")
	conn.FailQueryContains = "DELETE FROM revoked_tokens"

	count, err := repo.CleanupExpiredRevocations(context.Background())

	assert.Equal(t, int64(0), count)
	require.Error(t, err)
	assert.True(t, conn.RolledBack)
}

func TestGetUserByFederatedID_ConTenantFiltraPorTenant(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)
	tenant := uuid.MustParse(tenantDePrueba)
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{{
		"55555555-5555-5555-5555-555555555555",
		"user@example.com",
		"$2a$10$hash",
		tenant.String(),
		"66666666-6666-6666-6666-666666666666",
		"ACTIVE",
		"GOOGLE",
		"google-abc",
	}}})

	user, err := repo.GetUserByFederatedID(context.Background(), "GOOGLE", "google-abc", &tenant)

	require.NoError(t, err)
	assert.Equal(t, "user@example.com", user.Email)
	assert.Equal(t, tenant, user.TenantID)

	stmts := conn.Statements()
	require.Len(t, stmts, 1)
	assert.Contains(t, stmts[0], "FROM users")
	assert.Contains(t, stmts[0], "AND tenant_id = $3", "con tenant conocido el escape de federación queda acotado al tenant")
}

func TestGetUserByFederatedID_SinTenantNoAgregaElFiltro(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)
	conn.EnqueueResult(&fakedb.ResultSet{Rows: [][]driverValue{{
		"55555555-5555-5555-5555-555555555555",
		"user@example.com",
		"$2a$10$hash",
		tenantDePrueba,
		"66666666-6666-6666-6666-666666666666",
		"ACTIVE",
		"GOOGLE",
		"google-abc",
	}}})

	user, err := repo.GetUserByFederatedID(context.Background(), "GOOGLE", "google-abc", nil)

	require.NoError(t, err)
	assert.Equal(t, "GOOGLE", user.Provider)

	stmts := conn.Statements()
	require.Len(t, stmts, 1)
	assert.NotContains(t, stmts[0], "AND tenant_id", "sin tenant no hay filtro de tenant — el SELECT base ya trae la columna tenant_id")
}

func TestGetUserByFederatedID_SinFilasDevuelveUsuarioNoEncontrado(t *testing.T) {
	db, _ := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)

	_, err := repo.GetUserByFederatedID(context.Background(), "GOOGLE", "google-desconocido", nil)

	require.Error(t, err)
	assert.ErrorContains(t, err, "usuario no encontrado")
}

func TestCreateRefreshToken_ErrorDeMotorFallaYHaceRollback(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)
	conn.ExecErr = errors.New("conexión rota")
	conn.FailQueryContains = "INSERT INTO refresh_tokens"
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantDePrueba))

	err := repo.CreateRefreshToken(ctx, tokenDePrueba())

	require.Error(t, err)
	assert.ErrorContains(t, err, "error creando refresh token")
	assert.True(t, conn.RolledBack, "la transacción RLS se revierte ante un fallo del motor")
	assert.False(t, conn.Committed)
}

func TestLinkFederatedID_ErrorDeMotorSeEnvuelve(t *testing.T) {
	// Camino sin tenant (pool directo): el error del UPDATE sube envuelto y
	// sin transacción que revertir — la deuda de UPDATE en silencio con RLS
	// está anotada en el propio repositorio (T5/T7 de ACC-E02).
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)
	conn.ExecErr = errors.New("conexión rota")
	conn.FailQueryContains = "UPDATE users"

	err := repo.LinkFederatedID(context.Background(), uuid.New(), "GOOGLE", "google-abc")

	require.Error(t, err)
	assert.ErrorContains(t, err, "error vinculando ID federado")
	assert.False(t, conn.Committed)
}