package repository_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/identity/domain/entity"
	"iam/src/identity/domain/port"
	"iam/src/identity/domain/value_object"
	"iam/src/identity/infrastructure/persistence/repository"
	sharedctx "iam/src/shared/context"
	"iam/test/testsupport/fakedb"
)

// ACC-E02 T5: el repositorio corre bajo account_app, que es NOBYPASSRLS. Si una
// escritura sale sin SET LOCAL app.tenant_id, la policy no la deja pasar y la
// fila se pierde en silencio. Estos tests fijan que el envoltorio RLS se aplica
// cuando —y sólo cuando— el contexto trae tenant.

const (
	tokenOpacoFalso = "no-real"
)

const tenantDePrueba = "33333333-3333-3333-3333-333333333333"

func tokenDePrueba() *entity.RefreshToken {
	return &entity.RefreshToken{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		Token:     tokenOpacoFalso,
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
	}
}

func TestCreateRefreshToken_ConTenantEnContextoEmiteSetLocalYCommitea(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantDePrueba))

	require.NoError(t, repo.CreateRefreshToken(ctx, tokenDePrueba()))

	stmts := conn.Statements()
	require.Len(t, stmts, 2, "debe emitir el SET LOCAL y el INSERT")
	assert.Equal(t, "SET LOCAL app.tenant_id = '"+tenantDePrueba+"'", stmts[0])
	assert.Contains(t, stmts[1], "INSERT INTO refresh_tokens")
	assert.True(t, conn.Committed)
}

// T8e: sin tenant ya no hay camino directo por el pool — el repositorio rechaza
// la operación explícito. Antes salía directa y el motor la rechazaba por el
// WITH CHECK (fail-closed, pero con un error de constraint y una ida a la base
// de más); desde la 021 la columna tenant_id es NOT NULL y el rechazo anticipado
// es también el contrato del repo. Mismo resultado (no se escribe nada), error
// antes y más claro.
func TestCreateRefreshToken_SinTenantNoAbreTransaccionRLS(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)

	err := repo.CreateRefreshToken(context.Background(), tokenDePrueba())

	assert.ErrorContains(t, err, "requiere tenant")
	assert.Empty(t, conn.Statements(), "sin tenant no se ejecuta ninguna sentencia")
	assert.False(t, conn.Committed, "sin tenant no hay transacción que commitear")
}

// Carry-forward (b) de T2: este UPDATE corre post-auth bajo account_app, nunca
// bajo iam_login (que no tiene GRANT UPDATE sobre users). Con tenant en el
// contexto tiene que ir envuelto en la transacción con SET LOCAL.
func TestLinkFederatedID_ConTenantEnContextoEmiteSetLocalYCommitea(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantDePrueba))

	err := repo.LinkFederatedID(ctx, uuid.New(), value_object.AuthProvider("GOOGLE"), "google-abc")

	require.NoError(t, err)
	stmts := conn.Statements()
	require.Len(t, stmts, 2)
	assert.Equal(t, "SET LOCAL app.tenant_id = '"+tenantDePrueba+"'", stmts[0])
	assert.Contains(t, stmts[1], "UPDATE users")
	assert.True(t, conn.Committed)
}

func TestLinkFederatedID_SinTenantNoAbreTransaccionRLS(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)

	err := repo.LinkFederatedID(context.Background(), uuid.New(), value_object.AuthProvider("GOOGLE"), "google-abc")

	require.NoError(t, err)
	stmts := conn.Statements()
	require.Len(t, stmts, 1)
	assert.Contains(t, stmts[0], "UPDATE users")
	assert.False(t, conn.Committed)
}

// — T8e: los métodos de sesión corren SIEMPRE bajo el GUC de tenant. Sin
// app.tenant_id las policies de refresh_tokens/revoked_tokens deniegan todo,
// así que el repositorio ya no permite el camino sin tenant: lo rechaza
// explícito en vez de degradar a un DELETE/INSERT de 0 filas en silencio
// (el modo de fallo que el checkpoint de T8 documentó en logout/revoke-all).

func TestMetodosDeSesion_SinTenantFallanExplícito(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)
	ctx := context.Background()

	assert.ErrorContains(t, repo.DeleteRefreshToken(ctx, "x"), "requiere tenant")
	assert.ErrorContains(t, repo.DeleteAllUserRefreshTokens(ctx, uuid.New()), "requiere tenant")
	assert.ErrorContains(t, repo.RevokeToken(ctx, uuid.New(), uuid.New(), time.Now()), "requiere tenant")
	assert.ErrorContains(t, repo.RevokeAllUserTokens(ctx, uuid.New(), time.Now()), "requiere tenant")
	_, err := repo.IsTokenRevoked(ctx, uuid.New(), uuid.New(), time.Now().Unix())
	assert.ErrorContains(t, err, "requiere tenant")
	assert.ErrorContains(t, repo.CreateRefreshToken(ctx, tokenDePrueba()), "requiere tenant")

	for _, s := range conn.Statements() {
		assert.NotContains(t, s, "DELETE FROM", "ninguna escritura debe llegar a la base sin tenant")
		assert.NotContains(t, s, "INSERT INTO", "ninguna escritura debe llegar a la base sin tenant")
	}
	assert.False(t, conn.Committed)
}

func TestMetodosDeSesion_ConTenantEmiteSetLocalYCommitea(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantDePrueba))

	require.NoError(t, repo.DeleteRefreshToken(ctx, "rt-x"))
	require.NoError(t, repo.RevokeToken(ctx, uuid.New(), uuid.New(), time.Now()))
	require.NoError(t, repo.RevokeAllUserTokens(ctx, uuid.New(), time.Now()))

	// Cada operación abre su propia transacción: SET LOCAL seguido de la DML.
	stmts := conn.Statements()
	require.Len(t, stmts, 6)
	for i := 0; i < len(stmts); i += 2 {
		assert.Equal(t, "SET LOCAL app.tenant_id = '"+tenantDePrueba+"'", stmts[i], "sentencia %d", i)
	}
	assert.True(t, conn.Committed)
}

// T8f: la propiedad single-use exige exactamente una fila borrada. Un DELETE
// de 0 filas ya no pasa inadvertido: el repo devuelve el sentinela del dominio
// envuelto, y el caso de uso lo traduce a credencial inválida (no a 500).
func TestDeleteRefreshToken_FilasCeroDevuelveYaConsumido(t *testing.T) {
	db, conn := fakedb.New(t)
	conn.ZeroRows = true // la fila ya no existe: la consumió otro request
	repo := repository.NewPostgresAuthRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantDePrueba))

	err := repo.DeleteRefreshToken(ctx, "rt-ya-consumido")

	assert.ErrorIs(t, err, port.ErrRefreshTokenAlreadyConsumed,
		"un DELETE de 0 filas debe devolver el sentinela single-use, no nil")
	stmts := conn.Statements()
	require.Len(t, stmts, 2)
	assert.Equal(t, "SET LOCAL app.tenant_id = '"+tenantDePrueba+"'", stmts[0])
	assert.Contains(t, stmts[1], "DELETE FROM refresh_tokens")
}

// El mismo sentinela, por contraste, no aparece cuando el DELETE toca la fila
// esperada — el caso feliz no se degrada.
func TestDeleteRefreshToken_UnaFilaEliminadaEsElCasoFeliz(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)
	ctx := sharedctx.WithTenantID(context.Background(), uuid.MustParse(tenantDePrueba))

	assert.NoError(t, repo.DeleteRefreshToken(ctx, "rt-fresco"))
	assert.True(t, conn.Committed)
}

func TestCleanupExpiredRevocations_EmiteGUCDeMantenimiento(t *testing.T) {
	// Alcance: sólo el envoltorio (orden de sentencias y commit). El
	// comportamiento de las POLICIES que ese GUC habilita no es demostrable con
	// un mock (objeción (A) del gate de T8f) — esa evidencia vive en
	// test/rls/rls_mantenimiento_test.go (T8h), contra Postgres real.
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)

	_, err := repo.CleanupExpiredRevocations(context.Background())

	require.NoError(t, err)
	stmts := conn.Statements()
	require.Len(t, stmts, 2)
	assert.Equal(t, "SET LOCAL app.token_maintenance = 'on'", stmts[0], "la limpieza corre con el GUC de mantenimiento, nunca con tenant de request")
	assert.Contains(t, stmts[1], "DELETE FROM revoked_tokens")
	assert.True(t, conn.Committed)
}

func TestGetRefreshToken_PresentaciónEmiteDigestYSinTenantID(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)

	// El result set es vacío (ErrNoRows) pero alcanza para afirmar que el
	// SELECT de resolución pre-auth sale con el GUC de presentación y SIN el
	// GUC de tenant: no hay tenant conocido hasta leer la fila (contrato T8e).
	_, err := repo.GetRefreshToken(context.Background(), "rt-presentado")

	assert.Error(t, err, "el result set de fakedb es vacío: la fila no existe")
	stmts := conn.Statements()
	require.Len(t, stmts, 2)
	assert.Equal(t, "SET LOCAL app.refresh_digest = '"+refreshDigestParaTest("rt-presentado")+"'", stmts[0])
	assert.Contains(t, stmts[1], "FROM refresh_tokens")
	assert.NotContains(t, stmts[0], "app.tenant_id", "la resolución pre-auth no debe fijar app.tenant_id")
}

// El digest NO es el token crudo: la credencial presentada no viaja por SET
// LOCAL (queda expuesta en pg_stat_activity).
func TestRefreshDigest_NoExponeElTokenCrudo(t *testing.T) {
	assert.NotEqual(t, "rt-secreto", refreshDigestParaTest("rt-secreto"))
	assert.Len(t, refreshDigestParaTest("rt-secreto"), 64, "sha256 hex")
	assert.Equal(t, refreshDigestParaTest("rt-secreto"), refreshDigestParaTest("rt-secreto"), "determinístico: la policy compara por igualdad")
}

// refreshDigestParaTest replica el digest del repositorio sin importar el
// paquete interno (la función es privada a propósito).
func refreshDigestParaTest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
