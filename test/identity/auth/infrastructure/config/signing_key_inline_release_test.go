package config_test

// ACC-E03 T3 — cierre de la objeción 1 de @dev-security sobre T2: ADR-003 §b
// fija JWT_PRIVATE_KEY (inline) "sólo para desarrollo local" y el código de T2
// no lo enforceaba — un deploy release con la privada inline en env pasaba la
// validación. Desde T3, GIN_MODE=release + inline → error (la env var es
// superficie de fuga real: docker inspect, /proc/*/environ, environ heredado).

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
)

func inlinePrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestLoadSigningKeyRejectsInlineKeyInRelease(t *testing.T) {
	inline := inlinePrivateKeyPEM(t)

	t.Setenv("JWT_PRIVATE_KEY", inline)
	t.Setenv("JWT_PRIVATE_KEY_FILE", "")
	t.Setenv("JWT_KEY_ID", "")
	t.Setenv("GIN_MODE", "release")

	key, err := config.LoadSigningKeyFromEnv()
	assert.Error(t, err, "la clave inline es dev-only (ADR-003 §b): en release debe rechazarse")
	assert.Nil(t, key)
	assert.Contains(t, err.Error(), "development-only")
}

func TestLoadSigningKeyAcceptsInlineKeyInDev(t *testing.T) {
	inline := inlinePrivateKeyPEM(t)

	t.Setenv("JWT_PRIVATE_KEY", inline)
	t.Setenv("JWT_PRIVATE_KEY_FILE", "")
	t.Setenv("JWT_KEY_ID", "")
	t.Setenv("GIN_MODE", "debug")

	key, err := config.LoadSigningKeyFromEnv()
	require.NoError(t, err, "la inline sigue siendo el mecanismo de `go run` en desarrollo (ADR-003 §b)")
	require.NotNil(t, key)
	assert.Equal(t, "acc-", key.KID[:4])
}