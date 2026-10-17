// ACC-E03 T4b — hallazgo del review del gate de T4 (bitácora, 2026-09-14): el
// wiring de router.go pasa al gate un `*adapter.JWTServiceAdapter` NIL CON
// TIPO cuando identity está desmontado. Un puntero nil dentro de una interfaz
// no es nil: el guard `publicKeys == nil` de Authorize no dispara y, sin el
// arreglo, PublicKeyFor desreferencia el nil → panic, que gin.Recovery
// convierte en 500. No abre acceso, pero contradice el fail-closed declarado
// (authorize.go: "Un nil es fail-closed") y deja un stack trace por request.
//
// El criterio "Hecho cuando" de T4b cubierto acá:
//   - el test arma el gate con el MISMO tipo del wiring real (una variable
//     `*adapter.JWTServiceAdapter` nil pasada como resolver) y un Bearer RS256
//     con kid recibe 401, SIN panic. Un test que pase `nil` literal NO
//     reproduce el defecto: ese caso ya daba 401 antes del arreglo.
//   - PRUEBA NEGATIVA de reproducción: este mismo test contra el código de
//     `5bbc6f2` (sin el guard de receptor nil en PublicKeyFor) entra en panic.
//     La evidencia de esa corrida está en ACC-E03-bitacora.md.
package middleware_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/hornosg/iam-service/src/access/infrastructure/middleware"
	"github.com/hornosg/iam-service/src/access/infrastructure/s2s"
	"github.com/hornosg/iam-service/src/identity/infrastructure/adapter"
)

// CRITERIO DE T4b — el wiring real del gate (router.go:178/185) con identity
// desmontado: una variable DECLARADA con el tipo concreto del adapter y nil,
// pasada como PublicKeyResolver. El token RS256 con kid debe ser 401
// fail-closed, NO panic (que el harness sin gin.Recovery dejaría estallar
// hasta el propio test, como lo hace contra 5bbc6f2).
func TestAuthorize_T4b_VerifierNilConTipoRS256ConKidDa401SinPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	registry := s2s.LoadFromEnvForTests(map[string]string{
		"sales": salesKey,
	})

	// MISMO tipo del wiring real: el puntero concreto del adapter, nil porque
	// identity está desmontado (no hay NewJWTServiceAdapter que lo construya).
	// Pasar un `nil` sin tipo acá NO reproduciría el defecto y el criterio de
	// la tarea lo declara falla.
	var verifier *adapter.JWTServiceAdapter
	factory := authmw.NewScopeMiddlewareFactory(testNamespace, registry, verifier)

	router.GET("/tenant-scoped", factory.RequireScope(s2s.ScopeTenantAdmin, "tenant_admin"))

	token := t4SignRS(t, authorizeT4Key, []string{"tenant_admin"}, true)
	w := do(t, router, http.MethodGet, "/tenant-scoped", token, "")

	require.NotNil(t, w)
	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"con identity desmontado un RS256 con kid debe ser 401 fail-closed, no 500 por panic (T4b)")
}