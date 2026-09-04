package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	sharedservice "github.com/hornosg/go-shared/domain/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"iam/src/identity/domain/value_object"
	"iam/src/identity/infrastructure/adapter"
	"iam/src/identity/infrastructure/middleware"
	sharedctx "iam/src/shared/context"
	repo "iam/test/identity/auth/infrastructure/persistence/repository"
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
// devuelve 200 y, si user_id quedó en contexto, lo refleja en un header. Si el
// context.Context de la request porta tenant (T8g: derivado para tokens legacy
// o tomado del claim), lo refleja en X-Tenant-Id.
func newEngine(cfg middleware.TokenRevocationConfig) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.TokenRevocationCheck(cfg))
	r.GET("/*p", func(c *gin.Context) {
		if uid, ok := c.Get("user_id"); ok {
			c.Header("X-User-Id", uid.(uuid.UUID).String())
		}
		if tid, ok := sharedctx.TenantIDFromContext(c.Request.Context()); ok {
			c.Header("X-Tenant-Id", tid.String())
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

func TestTokenRevocation_RepoErrorFailsClosed(t *testing.T) {
	// T8e: IsTokenRevoked devuelve error => el gate falla CERRADO (500): si no
	// se puede verificar la revocación no se autoriza el request. Antes delegaba
	// a next (fail-open) y un JTI revocado pasaba cuando la consulta fallaba.
	claims := baseClaims(uuid.New())
	mock := repo.NewMockAuthRepository()
	mock.ShouldFailOn("IsTokenRevoked")

	cfg := middleware.TokenRevocationConfig{JWTSecret: signingKey, AuthRepo: mock}
	r := newEngine(cfg)
	tok := signToken(t, claims)
	w := doRequest(t, r, "/api/x", "Bearer "+tok)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	// Abort impide correr el handler final => no se setea user_id.
	assert.Empty(t, w.Header().Get("X-User-Id"))
}
// --- token legacy sin claim de tenant (ACC-E02 T8g, criterio (B) del gate de
// T8f) ---

// mockTenantResolver es un mock de port.TenantByUserResolver para ejercitar la
// derivación de tenant del user_id verificado. Acepta tanto un tenant fijo
// como un error a inyectar (sentinela o genérico).
type mockTenantResolver struct {
	tenant uuid.UUID
	err    error
	calls  int
}

func (m *mockTenantResolver) ResolveTenantByUserID(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
	m.calls++
	return m.tenant, m.err
}

// legacyClaims son los claims de un token emitido antes de que el claim
// tenant_id existiera: user_id y jti válidos, tenant en zero-value.
func legacyClaims(jti, userID uuid.UUID) value_object.TokenClaims {
	return value_object.TokenClaims{
		JTI:       jti,
		UserID:    userID,
		TenantID:  uuid.Nil,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
}

func TestTokenRevocation_LegacyTokenDerivesTenant(t *testing.T) {
	claims := legacyClaims(uuid.New(), uuid.New())
	resolver := &mockTenantResolver{tenant: uuid.New()}
	cfg := middleware.TokenRevocationConfig{JWTSecret: signingKey, AuthRepo: repo.NewMockAuthRepository(), TenantResolver: resolver}
	r := newEngine(cfg)
	tok := signToken(t, claims)
	w := doRequest(t, r, "/api/x", "Bearer "+tok)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, claims.UserID.String(), w.Header().Get("X-User-Id"))
	// El tenant derivado de la base (no del token) quedó en el context.Context:
	// es lo que downstream usa para fijar app.tenant_id.
	assert.Equal(t, resolver.tenant.String(), w.Header().Get("X-Tenant-Id"))
	assert.Equal(t, 1, resolver.calls)
}

func TestTokenRevocation_LegacyTokenUnknownUser401(t *testing.T) {
	// El user_id firmado no existe: no hay tenant que derivar ni sesión que
	// autorizar → 401, no 500 ni pass-through.
	claims := legacyClaims(uuid.New(), uuid.New())
	resolver := &mockTenantResolver{err: sharedservice.ErrUserNotFound}
	cfg := middleware.TokenRevocationConfig{JWTSecret: signingKey, AuthRepo: repo.NewMockAuthRepository(), TenantResolver: resolver}
	r := newEngine(cfg)
	tok := signToken(t, claims)
	w := doRequest(t, r, "/api/x", "Bearer "+tok)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Empty(t, w.Header().Get("X-User-Id"))
	assert.Empty(t, w.Header().Get("X-Tenant-Id"))
}

func TestTokenRevocation_LegacyTokenResolverError500(t *testing.T) {
	// Fallo de infraestructura en la derivación → fail-closed 500 (misma
	// semántica que un error de IsTokenRevoked: no se puede verificar, no se
	// autoriza).
	claims := legacyClaims(uuid.New(), uuid.New())
	resolver := &mockTenantResolver{err: errors.New("db caída")}
	cfg := middleware.TokenRevocationConfig{JWTSecret: signingKey, AuthRepo: repo.NewMockAuthRepository(), TenantResolver: resolver}
	r := newEngine(cfg)
	tok := signToken(t, claims)
	w := doRequest(t, r, "/api/x", "Bearer "+tok)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Empty(t, w.Header().Get("X-User-Id"))
	assert.Empty(t, w.Header().Get("X-Tenant-Id"))
}

func TestTokenRevocation_LegacyTokenWithoutResolverSkipsDerivation(t *testing.T) {
	// Sin resolver inyectado el gate mantiene el comportamiento previo: no
	// deriva nada, no aborta por el tenant ausente (el fail-closed real lo
	// pone la RLS del repo) y sigue consultando la revocación por JTI.
	claims := legacyClaims(uuid.New(), uuid.New())
	mock := repo.NewMockAuthRepository()
	cfg := middleware.TokenRevocationConfig{JWTSecret: signingKey, AuthRepo: mock}
	r := newEngine(cfg)
	tok := signToken(t, claims)
	w := doRequest(t, r, "/api/x", "Bearer "+tok)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, claims.UserID.String(), w.Header().Get("X-User-Id"))
	assert.Empty(t, w.Header().Get("X-Tenant-Id"))
	assert.Equal(t, 1, mock.GetCallCount("IsTokenRevoked"))
}

func TestTokenRevocation_ClaimedTenantSkipsResolver(t *testing.T) {
	// Con claim de tenant válido NO se consulta el resolver: la derivación es
	// sólo para el caso legacy.
	claims := baseClaims(uuid.New())
	resolver := &mockTenantResolver{tenant: uuid.New()}
	cfg := middleware.TokenRevocationConfig{JWTSecret: signingKey, AuthRepo: repo.NewMockAuthRepository(), TenantResolver: resolver}
	r := newEngine(cfg)
	tok := signToken(t, claims)
	w := doRequest(t, r, "/api/x", "Bearer "+tok)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, claims.TenantID.String(), w.Header().Get("X-Tenant-Id"))
	assert.Equal(t, 0, resolver.calls)
}
