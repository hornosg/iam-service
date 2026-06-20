package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	testKey = "test-secret-key-at-least-32-chars-long!!"
	testNS     = "mc"
	testS2SKey = "s2s-internal"
)

// signToken firma un JWT HS256 con los claims dados (namespace + roles).
func signToken(t *testing.T, namespace string, roles []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"namespace": namespace,
		"user_id":   "123e4567-e89b-12d3-a456-426614174000",
		"tenant_id": "123e4567-e89b-12d3-a456-426614174003",
		"exp":       time.Now().Add(time.Hour).Unix(),
	}
	if roles != nil {
		claims["roles"] = roles
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testKey))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return tok
}

// TestAuthorize cubre el gate de acceso a los endpoints de gestión del IAM:
// S2S por API key, humano por JWT+rol, y los caminos fail-closed.
func TestAuthorize(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name       string
		allowed    []string
		setHeaders func(r *http.Request)
		wantStatus int
	}{
		{
			name:       "anonimo sin credenciales -> 401",
			allowed:    []string{"system_admin"},
			setHeaders: func(r *http.Request) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:    "S2S API key valida -> 200",
			allowed: []string{"system_admin"},
			setHeaders: func(r *http.Request) {
				r.Header.Set("X-API-Key", testS2SKey)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:    "S2S API key invalida sin JWT -> 401",
			allowed: []string{"system_admin"},
			setHeaders: func(r *http.Request) {
				r.Header.Set("X-API-Key", "wrong")
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:    "JWT con rol permitido -> 200",
			allowed: []string{"system_admin"},
			setHeaders: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+signToken(t, testNS, []string{"system_admin"}))
			},
			wantStatus: http.StatusOK,
		},
		{
			name:    "JWT con rol insuficiente -> 403",
			allowed: []string{"system_admin"},
			setHeaders: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+signToken(t, testNS, []string{"cashier"}))
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:    "JWT sin claim roles (token de servicio viejo) -> 403",
			allowed: []string{"system_admin"},
			setHeaders: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+signToken(t, testNS, nil))
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:    "JWT de otro namespace -> 403",
			allowed: []string{"system_admin"},
			setHeaders: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+signToken(t, "otro", []string{"system_admin"}))
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:    "tenant_admin permitido en regimen B -> 200",
			allowed: []string{"tenant_admin", "system_admin"},
			setHeaders: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+signToken(t, testNS, []string{"tenant_admin"}))
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, engine := gin.CreateTestContext(w)
			engine.Use(Authorize(testKey, testNS, testS2SKey, tc.allowed...))
			engine.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			tc.setHeaders(req)
			c.Request = req
			engine.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d (body=%s)", w.Code, tc.wantStatus, w.Body.String())
			}
		})
	}
}
