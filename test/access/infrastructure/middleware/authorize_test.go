// ACC-E02 T8d — derivación del flag de system_admin que alimenta el GUC
// `app.is_system_admin` (branch cross-tenant de la policy de `tenants`, 019).
//
// Condición vinculante del gate L4 de T8b (objeción (C)-1): el flag se deriva
// SÓLO de una autoridad verificada por Authorize — scope S2S `system:admin`
// resuelto contra el registry, o rol `system_admin` cotejado contra
// allowedRoles — y JAMÁS de un claim crudo del JWT. Un `is_system_admin`
// inyectado en un token tenant_admin no abre nada.
package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/hornosg/iam-service/src/access/infrastructure/middleware"
	"github.com/hornosg/iam-service/src/access/infrastructure/s2s"
	sharedctx "github.com/hornosg/iam-service/src/shared/context"
)

const (
	testJWTSecret = "t8d-unit-test-secret-at-least-32-bytes"
	testNamespace = "mc"
)

// s2sKeys usa las políticas del ServicePolicy real: sales → system:admin,
// whatsapp-agent → tenant:provision a secas (el caso que el gate de provision
// acepta y que NO debe ganar autoridad system_admin).
const (
	salesKey    = "t8d-sales-key-0123456789abcdef"
	whatsappKey = "t8d-whatsapp-key-0123456789abcdef"
)

// newRouter arma un router con un grupo tenant-scoped (system:admin o
// tenant:admin) y otro de provision (tenant:provision o system:admin), ambos
// exponiendo un endpoint que reporta el flag verificado que Authorize dejó en
// el context.Context de la request.
func newRouter(t *testing.T) (*gin.Engine, *map[string]bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	registry := s2s.LoadFromEnvForTests(map[string]string{
		"sales":          salesKey,
		"whatsapp-agent": whatsappKey,
	})
	factory := authmw.NewScopeMiddlewareFactory(testJWTSecret, testNamespace, registry)

	report := map[string]bool{}
	reportFlag := func(c *gin.Context) {
		report["is_system_admin"] = sharedctx.IsSystemAdminFromContext(c.Request.Context())
		c.Status(http.StatusOK)
	}

	tenantScoped := router.Group("", factory.RequireScopes(
		[]s2s.Scope{s2s.ScopeSystemAdmin, s2s.ScopeTenantAdmin}, "tenant_admin", "system_admin"))
	tenantScoped.GET("/tenant-scoped", reportFlag)

	provision := router.Group("", factory.RequireScopes(
		[]s2s.Scope{s2s.ScopeTenantProvision, s2s.ScopeSystemAdmin}, "system_admin"))
	provision.POST("/provision", reportFlag)

	return router, &report
}

func makeToken(t *testing.T, roles []string, extraClaims map[string]interface{}) string {
	t.Helper()
	claims := jwt.MapClaims{
		"user_id":   uuid.New().String(),
		"tenant_id": uuid.New().String(),
		"roles":     roles,
		"namespace": testNamespace,
		"exp":       time.Now().Add(15 * time.Minute).Unix(),
	}
	for k, v := range extraClaims {
		claims[k] = v
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	str, err := token.SignedString([]byte(testJWTSecret))
	require.NoError(t, err)
	return str
}

func do(t *testing.T, router *gin.Engine, method, path, bearer, apiKey string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func assertAuthorized(t *testing.T, w *httptest.ResponseRecorder, msg string) {
	t.Helper()
	if w.Code != http.StatusOK {
		body, _ := json.Marshal(map[string]string{"body": w.Body.String()})
		t.Fatalf("%s: esperaba 200, recibí %d %s", msg, w.Code, body)
	}
}

// El scope S2S system:admin verificado por el registry SÍ habilita el flag.
func TestAuthorize_S2SSystemAdminScopeDerivaFlag(t *testing.T) {
	router, report := newRouter(t)
	w := do(t, router, http.MethodGet, "/tenant-scoped", "", salesKey)
	assertAuthorized(t, w, "sales (system:admin) debe pasar el gate tenant-scoped")
	assert.True(t, (*report)["is_system_admin"],
		"un scope system:admin verificado por Authorize debe derivar el flag")
}

// El scope tenant:provision pasa el gate de provision pero NO deriva el flag:
// la autoridad verificada es otra, y el GUC no puede abrirse desde ella
// (condición vinculante del gate de T8b).
func TestAuthorize_S2SProvisionScopeNoDerivaFlag(t *testing.T) {
	router, report := newRouter(t)
	w := do(t, router, http.MethodPost, "/provision", "", whatsappKey)
	assertAuthorized(t, w, "whatsapp-agent (tenant:provision) debe pasar el gate de provision")
	assert.False(t, (*report)["is_system_admin"],
		"tenant:provision verificado NO debe derivar el flag de system_admin")
}

// El rol system_admin verificado contra allowedRoles SÍ habilita el flag.
func TestAuthorize_RolSystemAdminDerivaFlag(t *testing.T) {
	router, report := newRouter(t)
	token := makeToken(t, []string{"system_admin"}, nil)
	w := do(t, router, http.MethodGet, "/tenant-scoped", token, "")
	assertAuthorized(t, w, "un JWT con rol system_admin debe pasar el gate tenant-scoped")
	assert.True(t, (*report)["is_system_admin"],
		"el rol system_admin verificado por el gate debe derivar el flag")
}

// CRITERIO (b) DE T8d — nivel gate: un token con el claim `is_system_admin`
// inyectado pero sin la autoridad verificada (rol tenant_admin) NO abre el
// branch. Authorize jamás consulta ese claim.
func TestAuthorize_ClaimCrudoIsSystemAdminNoDerivaFlag(t *testing.T) {
	router, report := newRouter(t)
	token := makeToken(t, []string{"tenant_admin"}, map[string]interface{}{
		"is_system_admin": true,
	})
	w := do(t, router, http.MethodGet, "/tenant-scoped", token, "")
	assertAuthorized(t, w, "el token tenant_admin pasa el gate (ese es su scope legítimo)")
	assert.False(t, (*report)["is_system_admin"],
		"el claim crudo is_system_admin no debe derivar el flag: el gate no lo coteja")
}

// Un rol system_admin sobre una ruta que NO autoriza esa autoridad tampoco
// deriva el flag: la derivación es de la decisión de ESTA ruta, no del claim.
func TestAuthorize_RolSystemAdminEnRutaSinEsaAutoridadNoDerivaFlag(t *testing.T) {
	router, report := newRouter(t)
	// provisionGroup autoriza humanos sólo por system_admin, así que usamos un
	// rol ajeno + claim inyectado para probar la combinación ruta/flag.
	token := makeToken(t, []string{"tenant_admin"}, map[string]interface{}{
		"is_system_admin": true,
	})
	w := do(t, router, http.MethodPost, "/provision", token, "")
	assert.Equal(t, http.StatusForbidden, w.Code,
		"provisionGroup no autoriza el rol tenant_admin")
	assert.False(t, (*report)["is_system_admin"])
}
