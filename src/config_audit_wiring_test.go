package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	tenantConfig "github.com/hornosg/iam-service/src/tenancy/infrastructure/config"
)

// ACC-E04 T4 (ADR-005 §Aceptación T4): el composition root construye el adapter de
// auditoría de producción con NewCanonicalLogger("iam") — stdout, no nil/Discard — y lo
// cablea en el wrapper de la ruta de read ANTES del gate (un 401 del gate se audita).
// Se redirige os.Stdout a un pipe antes de buildRouter: si el emitter escribiera a otro
// lado (o no estuviera cableado) el pipe no recibe el evento.
func TestConfigAuditWiring_ProductionEmitterWritesStdout(t *testing.T) {
	t.Setenv("JWT_SECRET", "t4-wiring-test-secret-0123456789abcdef")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	router := buildRouterForTest(t)
	os.Stdout = orig

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenant/config/catalog.stock_policy", nil)
	req.Header.Set("X-Tenant-ID", "11111111-1111-1111-1111-111111111111")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	w.Close()

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin credencial el gate Authorize debe responder 401 (paso 0: TenantValidation excluido), got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Authorization header required") {
		t.Fatalf("el 401 debe venir de Authorize, got %s", rec.Body.String())
	}
	var found map[string]any
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		var ev map[string]any
		if json.Unmarshal(sc.Bytes(), &ev) == nil && ev["event"] == "tenant.config_access" {
			found = ev
		}
	}
	if found == nil {
		t.Fatal("el adapter de producción no emitió tenant.config_access a stdout")
	}
	if found["service"] != "iam" || found["outcome"] != "denied" || found["s2s_service"] != "unattributed" || found["operation"] != "config.read" {
		t.Fatalf("evento inesperado: %v", found)
	}
	if _, ok := found["tenant_id"]; ok {
		t.Fatalf("sin gate no se publica tenant_id: %v", found)
	}
}

// ACC-E04 T4 (review L4, C1): mismo cableado de producción para el bootstrap S2S. Si alguien
// invierte el orden wrapper→gate en tenant_module.go, el 401 del gate dejaría de auditarse.
func TestConfigAuditWiring_BootstrapDenialIsAudited(t *testing.T) {
	t.Setenv("JWT_SECRET", "t4-wiring-test-secret-0123456789abcdef")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	router := buildRouterForTest(t)
	os.Stdout = orig

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenant/bootstrap", nil)
	req.Header.Set("X-Tenant-ID", "11111111-1111-1111-1111-111111111111")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	w.Close()

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bootstrap sin credencial debe responder 401 del gate, got %d: %s", rec.Code, rec.Body.String())
	}
	var found map[string]any
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		var ev map[string]any
		if json.Unmarshal(sc.Bytes(), &ev) == nil && ev["event"] == "tenant.config_access" {
			found = ev
		}
	}
	if found == nil {
		t.Fatal("el 401 del gate de bootstrap no se auditó: wrapper cableado después del gate")
	}
	if found["outcome"] != "denied" || found["s2s_service"] != "unattributed" || found["operation"] != "config.bootstrap" {
		t.Fatalf("evento inesperado: %v", found)
	}
}

// Los tres gates son obligatorios: un gate nil dejaría una ruta de config sin control.
func TestSetupTenantConfigModule_PanicsWithoutGates(t *testing.T) {
	pass := func(*gin.Context) {}
	casos := map[string]tenantConfig.TenantConfigGates{
		"sin read":      {Set: pass, Bootstrap: pass},
		"sin set":       {Read: pass, Bootstrap: pass},
		"sin bootstrap": {Read: pass, Set: pass},
	}
	for nombre, gates := range casos {
		t.Run(nombre, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("SetupTenantConfigModule debe hacer panic si falta un gate")
				}
			}()
			gin.SetMode(gin.TestMode)
			tenantConfig.SetupTenantConfigModule(gin.New().Group("/api/v1"), nil, gates, nil)
		})
	}
}
