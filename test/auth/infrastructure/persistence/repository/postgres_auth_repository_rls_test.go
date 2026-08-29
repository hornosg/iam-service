package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/auth/domain/entity"
	"iam/src/auth/domain/value_object"
	"iam/src/auth/infrastructure/persistence/repository"
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

// Sin tenant no hay transacción RLS: la escritura sale directa por el pool. La
// policy de la base la rechaza igual (fail-closed del lado del motor), pero acá
// se fija el comportamiento del código para que un cambio futuro no lo invierta
// en silencio.
func TestCreateRefreshToken_SinTenantNoAbreTransaccionRLS(t *testing.T) {
	db, conn := fakedb.New(t)
	repo := repository.NewPostgresAuthRepository(db)

	require.NoError(t, repo.CreateRefreshToken(context.Background(), tokenDePrueba()))

	stmts := conn.Statements()
	require.Len(t, stmts, 1)
	assert.Contains(t, stmts[0], "INSERT INTO refresh_tokens")
	assert.NotContains(t, stmts[0], "SET LOCAL")
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
