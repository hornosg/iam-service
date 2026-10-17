package controller_test

// ACC-E03 T8, criterio (c): el login nunca devuelve 5xx durante el simulacro
// de rotación. Corre el login REAL (AuthHandler → LoginUseCase) en cada fase
// de ADR-003 §c — inicial, publicar, firmar+gracia, retirar —, con el
// verificador armado como en router.go (clave de firma + keyring cargado por
// config desde JWT_PUBLIC_KEY_DIR). Además verifica que cada fase firme con el
// kid que le toca: en "publicar" se sigue firmando con kid1 (publicar ANTES de
// firmar), y que el access token emitido por el login de la fase "retirar"
// no se pueda reemplazar por uno de kid1 (NEGATIVO).

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sharedlog "github.com/hornosg/go-shared/infrastructure/logging"
	"github.com/hornosg/iam-service/src/identity/application/usecase"
	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
	"github.com/hornosg/iam-service/src/identity/infrastructure/config"
	"github.com/hornosg/iam-service/src/identity/infrastructure/controller"
	authrepo "github.com/hornosg/iam-service/test/identity/auth/infrastructure/persistence/repository"
)

func rotationKeyringDir(t *testing.T, keys ...*rsa.PrivateKey) string {
	t.Helper()
	dir := t.TempDir()
	for _, k := range keys {
		der, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, config.SigningKeyKID(&k.PublicKey)+".pem"),
			pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0o644))
	}
	return dir
}

// rotationPhase arma una "instancia recreada": verificador con keyring desde
// disco y el login real con ese mismo verificador como firmador.
func rotationPhase(t *testing.T, signer *rsa.PrivateKey, dir string, userSvc *MockUserService) (*adapter.JWTServiceAdapter, *controller.AuthHandler) {
	t.Helper()
	t.Setenv("JWT_PUBLIC_KEY_DIR", dir)
	ring, err := config.LoadVerificationKeyringFromEnv()
	require.NoError(t, err)
	jwtSvc := adapter.NewJWTServiceAdapterWithKeyring(
		&adapter.SigningKey{PrivateKey: signer, KID: config.SigningKeyKID(&signer.PublicKey)}, ring)

	cfg := usecase.AuthConfig{AccessTokenExpiry: 15 * time.Minute, RefreshTokenExpiry: 7 * 24 * time.Hour, Namespace: "mc"}
	repo := authrepo.NewMockAuthRepository()
	secLog := sharedlog.NewSecurityLoggerWithWriter(authTestSecurityLogger, io.Discard)
	h := controller.NewAuthHandler(
		usecase.NewLoginUseCase(cfg, repo, repo, userSvc, &MockTenantService{},
			jwtSvc, &MockRoleResolver{}, &MockPlanResolver{}, &MockGoogleTokenVerifier{}, secLog),
		usecase.NewRefreshTokenUseCase(cfg, repo, userSvc, &MockTenantService{},
			jwtSvc, &MockRoleResolver{}, &MockPlanResolver{}),
		usecase.NewLogoutUseCase(repo, secLog),
		usecase.NewRevokeAllUseCase(repo, 15*time.Minute, secLog),
	)
	return jwtSvc, h
}

func headerKID(t *testing.T, token string) string {
	t.Helper()
	parsed, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	require.NoError(t, err)
	kid, _ := parsed.Header["kid"].(string)
	return kid
}

func TestAuthRoutes_Login_RotacionT8_SinCincoXXEnNingunaFase(t *testing.T) {
	key1, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	key2, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	kid1 := config.SigningKeyKID(&key1.PublicKey)
	kid2 := config.SigningKeyKID(&key2.PublicKey)

	user := localUser(t, "rotacion-t8@test.com", "password123")
	userSvc := NewMockUserService()
	userSvc.SetupUser(user)
	body := map[string]interface{}{
		"email": user.Email, "password": "password123", "provider": "LOCAL",
		"tenant_id": user.TenantID.String(),
	}

	dirGracia := rotationKeyringDir(t, key1, key2)
	phases := []struct {
		name    string
		signer  *rsa.PrivateKey
		dir     string
		wantKID string
	}{
		{"inicial", key1, rotationKeyringDir(t, key1), kid1},
		{"publicar", key1, dirGracia, kid1}, // publicar ANTES de firmar: todavía kid1
		{"firmar+gracia", key2, dirGracia, kid2},
		{"retirar", key2, rotationKeyringDir(t, key2), kid2},
	}

	var accessByPhase = map[string]string{}
	var lastVerifier *adapter.JWTServiceAdapter
	for _, ph := range phases {
		verifier, h := rotationPhase(t, ph.signer, ph.dir, userSvc)
		router := newAuthRouter(h)
		for i := 0; i < 3; i++ {
			w := doAuthRequest(t, router, http.MethodPost, "/api/v1/auth/login", body, nil)
			require.Less(t, w.Code, 500, "(c) fase %s: el login devolvió %d", ph.name, w.Code)
			require.Equal(t, http.StatusOK, w.Code, "(c) fase %s: login con credenciales válidas", ph.name)

			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			access, _ := resp["access_token"].(string)
			require.NotEmpty(t, access)
			assert.Equal(t, ph.wantKID, headerKID(t, access), "fase %s: kid del token emitido", ph.name)
			_, err := verifier.Parse(access)
			assert.NoError(t, err, "fase %s: el token recién emitido valida en su propia instancia", ph.name)
			accessByPhase[ph.name] = access
		}
		lastVerifier = verifier
	}

	// NEGATIVO: el token kid1 emitido por el login en "inicial" no valida en la
	// instancia de "retirar"; el de "firmar+gracia" (kid2) sí.
	_, err = lastVerifier.Parse(accessByPhase["inicial"])
	assert.Error(t, err, "(b) NEGATIVO: un access token kid1 emitido por el login sigue validando tras retirar kid1")
	_, err = lastVerifier.Parse(accessByPhase["firmar+gracia"])
	assert.NoError(t, err, "(b) el access token kid2 emitido en gracia sigue validando tras retirar kid1")
}
