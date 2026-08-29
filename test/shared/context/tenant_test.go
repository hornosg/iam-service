package context_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	sharedctx "iam/src/shared/context"
)

func TestTenantIDFromContext_DevuelveElTenantInyectado(t *testing.T) {
	want := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	got, ok := sharedctx.TenantIDFromContext(sharedctx.WithTenantID(context.Background(), want))

	assert.True(t, ok)
	assert.Equal(t, want, got)
}

// Sin tenant en el contexto el repositorio NO debe abrir la transacción con
// RLS (ACC-E02 T5): el miss tiene que ser explícito, no un uuid.Nil silencioso.
func TestTenantIDFromContext_SinTenantDevuelveFalse(t *testing.T) {
	got, ok := sharedctx.TenantIDFromContext(context.Background())

	assert.False(t, ok)
	assert.Equal(t, uuid.Nil, got)
}

func TestTenantIDFromContext_NoConfundeOtrasClaves(t *testing.T) {
	//nolint:staticcheck // string key a propósito: simula a otro paquete escribiendo en el ctx
	ctx := context.WithValue(context.Background(), "tenantID", uuid.New())

	_, ok := sharedctx.TenantIDFromContext(ctx)

	assert.False(t, ok, "la clave privada no debe colisionar con una string")
}
