package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ACC-E03 T2 — pruebas negativas de la carga de la clave de firma (ADR-003 §b):
// el patrón es el mismo que ValidateJWTSecret: la validación falla CERRADO.
// Las claves se generan efímeras por test; nada toca disco fuera de t.TempDir.

func marshalPKCS8PEM(t *testing.T, key any) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func writeKeyFile(t *testing.T, pemBytes []byte, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "jwt_signing_private.pem")
	require.NoError(t, os.WriteFile(path, pemBytes, mode))
	require.NoError(t, os.Chmod(path, mode))
	return path
}

func TestLoadSigningKeyFromEnv(t *testing.T) {
	validKey, err := rsa.GenerateKey(rand.Reader, minSigningKeyBits)
	require.NoError(t, err)
	validPEM := marshalPKCS8PEM(t, validKey)

	weakKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tests := []struct {
		name    string
		setup   func(t *testing.T)
		wantErr string // substr esperado; vacío = carga exitosa
	}{
		{
			name: "desde archivo montado 0600",
			setup: func(t *testing.T) {
				path := writeKeyFile(t, validPEM, 0o600)
				t.Setenv("JWT_PRIVATE_KEY_FILE", path)
			},
		},
		{
			name: "desde archivo read-only 0400 (montaje k8s)",
			setup: func(t *testing.T) {
				path := writeKeyFile(t, validPEM, 0o400)
				t.Setenv("JWT_PRIVATE_KEY_FILE", path)
			},
		},
		{
			name: "desde env inline (dev)",
			setup: func(t *testing.T) {
				t.Setenv("JWT_PRIVATE_KEY", string(validPEM))
			},
		},
		{
			name: "ambas fuentes → error de ambigüedad",
			setup: func(t *testing.T) {
				t.Setenv("JWT_PRIVATE_KEY_FILE", writeKeyFile(t, validPEM, 0o600))
				t.Setenv("JWT_PRIVATE_KEY", string(validPEM))
			},
			wantErr: "both set",
		},
		{
			name:  "ninguna fuente → error",
			setup: func(t *testing.T) {},
			wantErr: "JWT_PRIVATE_KEY_FILE",
		},
		{
			name: "archivo legible por otros → error de permisos",
			setup: func(t *testing.T) {
				t.Setenv("JWT_PRIVATE_KEY_FILE", writeKeyFile(t, validPEM, 0o644))
			},
			wantErr: "owner-only",
		},
		{
			name: "RSA-1024 → error de tamaño",
			setup: func(t *testing.T) {
				t.Setenv("JWT_PRIVATE_KEY_FILE", writeKeyFile(t, marshalPKCS8PEM(t, weakKey), 0o600))
			},
			wantErr: "got 1024 bits",
		},
		{
			name: "clave EC (no RSA) → error de tipo",
			setup: func(t *testing.T) {
				t.Setenv("JWT_PRIVATE_KEY_FILE", writeKeyFile(t, marshalPKCS8PEM(t, ecKey), 0o600))
			},
			wantErr: "must be RSA",
		},
		{
			name: "material que no es PEM → error",
			setup: func(t *testing.T) {
				t.Setenv("JWT_PRIVATE_KEY", "esto no es un PEM")
			},
			wantErr: "no PEM block",
		},
		{
			name: "JWT_KEY_ID que no matchea el material → error",
			setup: func(t *testing.T) {
				t.Setenv("JWT_PRIVATE_KEY", string(validPEM))
				t.Setenv("JWT_KEY_ID", "acc-000000000000")
			},
			wantErr: "does not match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JWT_PRIVATE_KEY_FILE", "")
			t.Setenv("JWT_PRIVATE_KEY", "")
			t.Setenv("JWT_KEY_ID", "")
			tt.setup(t)

			key, err := LoadSigningKeyFromEnv()

			if tt.wantErr == "" {
				require.NoError(t, err)
				require.NotNil(t, key)
				assert.Equal(t, minSigningKeyBits, key.PrivateKey.N.BitLen())
				assert.Regexp(t, `^acc-[0-9a-f]{12}$`, key.KID, "kid debe ser acc-<12 hex> (ADR-003 §c)")
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, key)
			}
		})
	}
}

func TestLoadSigningKeyFromEnv_KeyIDPinValido(t *testing.T) {
	validKey, err := rsa.GenerateKey(rand.Reader, minSigningKeyBits)
	require.NoError(t, err)
	t.Setenv("JWT_PRIVATE_KEY", string(marshalPKCS8PEM(t, validKey)))
	t.Setenv("JWT_KEY_ID", SigningKeyKID(&validKey.PublicKey))

	key, err := LoadSigningKeyFromEnv()

	require.NoError(t, err)
	assert.Equal(t, SigningKeyKID(&validKey.PublicKey), key.KID)
}

func TestSigningKeyKID(t *testing.T) {
	keyA, err := rsa.GenerateKey(rand.Reader, minSigningKeyBits)
	require.NoError(t, err)
	keyB, err := rsa.GenerateKey(rand.Reader, minSigningKeyBits)
	require.NoError(t, err)

	kidA1 := SigningKeyKID(&keyA.PublicKey)
	kidA2 := SigningKeyKID(&keyA.PublicKey)

	assert.Regexp(t, `^acc-[0-9a-f]{12}$`, kidA1)
	assert.Equal(t, kidA1, kidA2, "el kid es determinístico: mismo material, mismo kid")
	assert.NotEqual(t, kidA1, SigningKeyKID(&keyB.PublicKey), "claves distintas nunca comparten kid")
}

// TestSigningKeyKID_RFC7638Vector valida la derivación contra una
// implementación INDEPENDIENTE: el kid de esta clave pública se computó con
// python3 (json canónico + hashlib, sin reusar este paquete). Si este test
// falla tras tocar SigningKeyKID, el JWKS de T7 y los verificadores de T3
// divergirían del estándar. La clave es material público de un par descartable.
func TestSigningKeyKID_RFC7638Vector(t *testing.T) {
	// Modulo (base64url) de la clave pública del vector; exponente 65537.
	const vectorN = "oe4WIbLJGli_fLCkdaLKeu76hBplUbl0zJSemp-XFTPB_T1k7GJFE5olqfzABjSvvh2Oho71ves7pUtf6h9Cm6xR1fwTrAMC3y8--5KURMvagC87IqxznjUxcNI16f_iiDYxEMQhSgSZW4gFMka6sAkt5r8vMAdMJzLZrlZ454TyeapA8i-enM2F7cOLjiYuJaHyeN6QLomGyZioO7I8ToY0sS9UNWandNLrFfsvLf2HPUJzsjIsWW8zNc75bPf92jyONIq7v8qThq3qLb_g8nTiHkZeAuH1qLOfgAEGt2XXATvqpmvbZm9CVR41BZHmIzR0Gisb7VXAt1ezydit2w"
	const vectorE = 65537
	const wantKID = "acc-f0dd40856452" // computado con python3, 2026-09-08

	nBytes, err := base64.RawURLEncoding.DecodeString(vectorN)
	require.NoError(t, err)
	pub := &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: vectorE}

	assert.Equal(t, wantKID, SigningKeyKID(pub))
}