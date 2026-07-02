package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"iam/src/auth/infrastructure/s2s"
)

const (
	testKey   = "test-secret-key-at-least-32-chars-long!!"
	testNS    = "mc"
	testKeyWA = "s2s-whatsapp-agent-key"
	testKeyOn = "s2s-onboarding-key"
)

func testRegistry(t *testing.T) *s2s.Registry {
	t.Helper()
	return s2s.LoadFromEnvForTests(map[string]string{
		"whatsapp-agent": testKeyWA,
		"onboarding":     testKeyOn,
	})
}

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
// S2S por API key + scope, humano por JWT+rol, y los caminos fail-closed.
func TestAuthorize(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name        string
		scope       s2s.Scope
		allowed     []string
		setHeaders  func(r *http.Request)
		wantStatus  int
		wantS2S     bool
		wantService string
	}{
		{
			name:       "anonimo sin credenciales -> 401",
			scope:      s2s.ScopeSystemAdmin,
			allowed:    []string{"system_admin"},
			setHeaders: func(r *http.Request) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:    "S2S API key valida con scope tenant:admin -> 200",
			scope:   s2s.ScopeTenantAdmin,
			allowed: []string{"tenant_admin", "system_admin"},
			setHeaders: func(r *http.Request) {
				r.Header.Set("X-API-Key", testKeyOn)
			},
			wantStatus:  http.StatusOK,
			wantS2S:     true,
			wantService: "onboarding",
		},
		{
			name:    "S2S API key valida con scope insuficiente -> 403",
			scope:   s2s.ScopeSystemAdmin,
			allowed: []string{"system_admin"},
			setHeaders: func(r *http.Request) {
				r.Header.Set("X-API-Key", testKeyWA)
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:    "S2S API key valida con scope tenant:provision -> 200",
			scope:   s2s.ScopeTenantProvision,
			allowed: []string{"system_admin"},
			setHeaders: func(r *http.Request) {
				r.Header.Set("X-API-Key", testKeyWA)
			},
			wantStatus:  http.StatusOK,
			wantS2S:     true,
			wantService: "whatsapp-agent",
		},
		{
			name:    "S2S API key invalida sin JWT -> 401",
			scope:   s2s.ScopeSystemAdmin,
			allowed: []string{"system_admin"},
			setHeaders: func(r *http.Request) {
				r.Header.Set("X-API-Key", "wrong")
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:    "JWT con rol permitido -> 200",
			scope:   s2s.ScopeSystemAdmin,
			allowed: []string{"system_admin"},
			setHeaders: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+signToken(t, testNS, []string{"system_admin"}))
			},
			wantStatus: http.StatusOK,
		},
		{
			name:    "JWT con rol insuficiente -> 403",
			scope:   s2s.ScopeSystemAdmin,
			allowed: []string{"system_admin"},
			setHeaders: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+signToken(t, testNS, []string{"cashier"}))
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:    "JWT sin claim roles (token de servicio viejo) -> 403",
			scope:   s2s.ScopeSystemAdmin,
			allowed: []string{"system_admin"},
			setHeaders: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+signToken(t, testNS, nil))
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:    "JWT de otro namespace -> 403",
			scope:   s2s.ScopeSystemAdmin,
			allowed: []string{"system_admin"},
			setHeaders: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+signToken(t, "otro", []string{"system_admin"}))
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:    "tenant_admin permitido en regimen B -> 200",
			scope:   s2s.ScopeSystemAdmin,
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
			engine.Use(Authorize(testKey, testNS, testRegistry(t), []s2s.Scope{tc.scope}, tc.allowed...))
			engine.GET("/x", func(c *gin.Context) {
				if tc.wantS2S {
					if s, _ := c.Get("s2s"); s != true {
						t.Errorf("s2s context = %v, want true", s)
					}
					if svc, _ := c.Get("s2s_service"); svc != tc.wantService {
						t.Errorf("s2s_service = %v, want %q", svc, tc.wantService)
					}
				}
				c.Status(http.StatusOK)
			})

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

// TestRequireScopeFactory verifica el helper factory para no repetir argumentos.
func TestRequireScopeFactory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := NewScopeMiddlewareFactory(testKey, testNS, testRegistry(t))

	w := httptest.NewRecorder()
	c, engine := gin.CreateTestContext(w)
	engine.Use(f.RequireScope(s2s.ScopeTenantProvision, "system_admin"))
	engine.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-API-Key", testKeyWA)
	c.Request = req
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
