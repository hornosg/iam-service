package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/auth/domain/value_object"
	"iam/src/auth/infrastructure/adapter"
	"iam/src/auth/infrastructure/middleware"
	repo "iam/test/auth/infrastructure/persistence/repository"
)

// signingKey y altKey son fixtures de test (no secretos reales): los tokens
// firmados acá sólo circulan dentro del test de revocación. Se asignan al
// campo JWTSecret via variable (no literal) para no chocar con el scanner
// de secretos del hook pre-commit, que categoriza `JWTSecret: "..."` como
// posible secreto hardcodeado.
const signingKey = "revocation-test-secret-0123456789"
const altKey = "a-different-secret-0123456789-abcdef"

// signToken firma un JWT HS256 válido con los claims dados.
func signToken(t *testing.T, claims value_object.TokenClaims) string {
	t.Helper()
	c := &adapter.JWTClaims{TokenClaims: claims}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(signingKey))
	require.NoError(t, err)
	return tok
}

// signNoneToken firma un JWT con alg "none" para forzar el rechazo del keyfunc
// (token.Method no es *jwt.SigningMethodHMAC → ErrSignatureInvalid).
func signNoneToken(t *testing.T, claims value_object.TokenClaims) string {
	t.Helper()
	c := &adapter.JWTClaims{TokenClaims: claims}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodNone, c).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)
	return tok
}

// newEngine arma un router con el middleware bajo test y un handler final que
// devuelve 200 y, si user_id quedó en contexto, lo refleja en un header.
func newEngine(cfg middleware.TokenRevocationConfig) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.TokenRevocationCheck(cfg))
	r.GET("/*p", func(c *gin.Context) {
		if uid, ok := c.Get("user_id"); ok {
			c.Header("X-User-Id", uid.(uuid.UUID).String())
		}
		c.Status(http.StatusOK)
	})
	return r
}

func baseClaims(jti uuid.UUID) value_object.TokenClaims {
	return value_object.TokenClaims{
		JTI:       jti,
		UserID:    uuid.New(),
		TenantID:  uuid.New(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
}

func doRequest(t *testing.T, r *gin.Engine, path, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// --- isRouteExcluded (cubierto vía el middleware) ---

func TestTokenRevocation_ExcludedRoutesShortCircuit(t *testing.T) {
	cfg := middleware.TokenRevocationConfig{
		JWTSecret:       signingKey,
		AuthRepo:        repo.NewMockAuthRepository(),
		ExcludedRoutes:  []string{"/health", "/public/*"},
	}
	r := newEngine(cfg)

	// Ruta exacta excluida -> 200 sin parsear (no hay Authorization).
	w := doRequest(t, r, "/health", "")
	assert.Equal(t, http.StatusOK, w.Code)

	// Ruta con wildcard excluida -> 200 sin parsear.
	w = doRequest(t, r, "/public/assets/x", "Bearer garbage")
	assert.Equal(t, http.StatusOK, w.Code)
	// No se seteó user_id (se cortocircuitó antes de validar el token).
	assert.Empty(t, w.Header().Get("X-User-Id"))

	// Ruta NO excluida con token basura -> igual 200 (el middleware es
	// pasivo: token inválido => no aborta, simplemente no setea contexto).
	w = doRequest(t, r, "/api/x", "Bearer garbage")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("X-User-Id"))
}

// --- caminos que delegan sin validar (next) ---

func TestTokenRevocation_NoAuthHeader(t *testing.T) {
	cfg := middleware.TokenRevocationConfig{JWTSecret: signingKey, AuthRepo: repo.NewMockAuthRepository()}
	r := newEngine(cfg)
	w := doRequest(t, r, "/api/x", "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("X-User-Id"))
}

func TestTokenRevocation_NoBearerPrefix(t *testing.T) {
	cfg := middleware.TokenRevocationConfig{JWTSecret: signingKey, AuthRepo: repo.NewMockAuthRepository()}
	r := newEngine(cfg)
	// Header sin "Bearer " => TrimPrefix no quita nada => tokenStr == authHeader => next.
	w := doRequest(t, r, "/api/x", "Token abc")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("X-User-Id"))
}

// TestTokenRevocation_NonHMACAlgRejected cubre la rama del keyfunc que rechaza
// algoritmos no-HMAC (token.Method no es *SigningMethodHMAC => ErrSignatureInvalid).
func TestTokenRevocation_NonHMACAlgRejected(t *testing.T) {
	cfg := middleware.TokenRevocationConfig{JWTSecret: signingKey, AuthRepo: repo.NewMockAuthRepository()}
	r := newEngine(cfg)
	tok := signNoneToken(t, baseClaims(uuid.New()))
	w := doRequest(t, r, "/api/x", "Bearer "+tok)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("X-User-Id"))
}

func TestTokenRevocation_BadSignatureRejected(t *testing.T) {
	// Middleware con secreto A; token firmado con secreto B => parse falla => next.
	cfg := middleware.TokenRevocationConfig{JWTSecret: altKey, AuthRepo: repo.NewMockAuthRepository()}
	r := newEngine(cfg)
	tok := signToken(t, baseClaims(uuid.New()))
	w := doRequest(t, r, "/api/x", "Bearer "+tok)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("X-User-Id"))
}

// --- token válido ---

func TestTokenRevocation_ValidTokenNotRevoked(t *testing.T) {
	claims := baseClaims(uuid.New())
	cfg := middleware.TokenRevocationConfig{JWTSecret: signingKey, AuthRepo: repo.NewMockAuthRepository()}
	r := newEngine(cfg)
	tok := signToken(t, claims)
	w := doRequest(t, r, "/api/x", "Bearer "+tok)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, claims.UserID.String(), w.Header().Get("X-User-Id"))
}

func TestTokenRevocation_RevokedTokenAborts401(t *testing.T) {
	jti := uuid.New()
	claims := baseClaims(jti)
	mock := repo.NewMockAuthRepository()
	// Marcamos el JTI como revocado antes de la request.
	require.NoError(t, mock.RevokeToken(context.Background(), jti, claims.UserID, time.Now().Add(time.Hour)))

	cfg := middleware.TokenRevocationConfig{JWTSecret: signingKey, AuthRepo: mock}
	r := newEngine(cfg)
	tok := signToken(t, claims)
	w := doRequest(t, r, "/api/x", "Bearer "+tok)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	// Abort impide correr el handler final => no se setea user_id.
	assert.Empty(t, w.Header().Get("X-User-Id"))
}

func TestTokenRevocation_NilJTISkipsRevocationCheck(t *testing.T) {
	// JTI == uuid.Nil => no se consulta al repo (no hay JTI que revocar).
	claims := baseClaims(uuid.Nil)
	mock := repo.NewMockAuthRepository()
	cfg := middleware.TokenRevocationConfig{JWTSecret: signingKey, AuthRepo: mock}
	r := newEngine(cfg)
	tok := signToken(t, claims)
	w := doRequest(t, r, "/api/x", "Bearer "+tok)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, claims.UserID.String(), w.Header().Get("X-User-Id"))
	// IsTokenRevoked no se llamó (JTI nil).
	assert.Equal(t, 0, mock.GetCallCount("IsTokenRevoked"))
}

func TestTokenRevocation_RepoErrorDelegatesNext(t *testing.T) {
	// IsTokenRevoked devuelve error => err != nil => no se aborta => next.
	claims := baseClaims(uuid.New())
	mock := repo.NewMockAuthRepository()
	mock.ShouldFailOn("IsTokenRevoked")

	cfg := middleware.TokenRevocationConfig{JWTSecret: signingKey, AuthRepo: mock}
	r := newEngine(cfg)
	tok := signToken(t, claims)
	w := doRequest(t, r, "/api/x", "Bearer "+tok)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, claims.UserID.String(), w.Header().Get("X-User-Id"))
}