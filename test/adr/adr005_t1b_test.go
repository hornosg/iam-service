// Package adrtest — contrato de ACC-E04 T1b (ceremonia L4), tal como está escrito
// en la épica (ACC-E04-tenancy-absorber-tenant-config.md), no como el ADR quiera
// haberlo resuelto. Mismo movimiento de adr003_t1b_test.go: las condiciones del
// gate vivían en greps de la escalación; acá quedan ejecutables en cada `go test`.
//
// El contrato de T1b cierra los dos bloqueantes que @dev-security puso al gate de
// T1 (re-verificados 2026-09-08):
//
//	(1) PASO 0: `router.go:53` monta TenantValidation GLOBAL y `/api/v1/tenant/config`
//	    no matchea ninguna ExcludedRoutes (tenants* exige la s) → un request S2S muere
//	    con 401 ANTES del gated-escape de DD-1. El ADR debe declarar la exclusión
//	    como paso 0 de T4, con su motivo.
//	      - NEGATIVO de conducta: con el middleware SIN la exclusión, un request S2S
//	        a `/api/v1/tenant/config` → 401. Si no diera 401, la premisa del hallazgo
//	        era falsa y hay que revisarla.
//	      - NEGATIVO de diseño (enmienda 2026-09-15): una exclusión de MATCH EXACTO
//	        (`"/api/v1/tenant/config"`, sin wildcard) NO cubre el read
//	        `/api/v1/tenant/config/:key` — `path == route` falla y el 401 sobrevive.
//	        La exclusión correcta es con wildcard.
//	(2) RATE-LIMITING: la afirmación "el service-level de Kong aplica" era vaga y
//	    (según el gate) falsa. El "Hecho cuando" exige un rate-limiting del service
//	    `iam-service` en el template O que el ADR declare que el control no existe
//	    y por qué se acepta; y exige que la frase vieja quede con grep -ci == 0.
//	      - Verificación 2026-09-15 (enmienda): el control EXISTE, declarado como
//	        plugin top-level con `service: iam-service` (Kong declarativo: un plugin
//	        top-level con `service:` ES service-level). Este test verifica el control
//	        por su forma real, no por estar inline en el bloque del service.
//
// NEGATIVOS de verificador: cada checker se auto-verifica contra copias mutadas en
// memoria (el documento real NUNCA se toca) — un checker que nunca falló no prueba
// nada, igual que el arch test de ACC-E01.
//
// Lo que este test NO verifica: el runtime vivo (:8080) y la ruta registrada — esa
// re-verificación es el paso 0 de T4 con la ruta ya montada (lo fija el ADR §Paso 0).
// Acá la conducta del middleware se ejercita in-process contra go-shared v0.17.0
// real (el mismo motor que monta router.go), que es donde vive la semántica del
// hallazgo.
package adrtest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	tenantmw "github.com/hornosg/go-shared/infrastructure/middleware"
)

// ----------------------------------------------------------------------- rutas

func adr005Text(t *testing.T) string {
	t.Helper()
	return leer(t, filepath.Join(moduleRoot(t), "docs", "adr", "ADR-005-tenancy-config-namespace.md"))
}

func kongTemplateText(t *testing.T) string {
	t.Helper()
	// El template vive fuera del repo del servicio (cross-project, repo propio
	// hornosg/api-gateway): $DEVY_PATH/infra/api-gateway/kong.yml.template (ADR-001
	// del harness instala el harness UNA vez en $DEVY_PATH). Fallback relativo al
	// module root para corridas fuera del lab ($DEVY_PATH/platform/iam-service).
	if devy := os.Getenv("DEVY_PATH"); devy != "" {
		p := filepath.Join(devy, "infra", "api-gateway", "kong.yml.template")
		if _, err := os.Stat(p); err == nil {
			return leer(t, p)
		}
	}
	return leer(t, filepath.Join(moduleRoot(t), "..", "..", "infra", "api-gateway", "kong.yml.template"))
}

func routerGoText(t *testing.T) string {
	t.Helper()
	return leer(t, filepath.Join(moduleRoot(t), "src", "router.go"))
}

// ------------------------------------------------------- checker 1: paso 0

// verificarPaso0 implementa la condición (1): grep -c 'TenantValidation' ≥ 1 Y el
// texto declara la exclusión de /api/v1/tenant/config como PASO 0 DE T4 CON SU
// MOTIVO. Un match global de "TenantValidation" sin la sección de decisión dejaría
// pasar un ADR que menciona el middleware pero no declara el paso; por eso la
// co-ocurrencia se exige dentro de la sección "Paso 0 (T4)".
func verificarPaso0(adr string) error {
	if grepCi(adr, "TenantValidation") < 1 {
		return fmt.Errorf("grep -c 'TenantValidation' ADR-005 == 0 (condición 1 de T1b: ≥ 1)")
	}
	var paso0 string
	for _, sec := range secciones(adr) {
		low := strings.ToLower(sec)
		if strings.Contains(low, "paso 0") && strings.Contains(low, "t4") {
			paso0 = sec
			break
		}
	}
	if paso0 == "" {
		return fmt.Errorf("ADR-005: ninguna sección declara el PASO 0 de T4 (exclusión de las rutas de config del middleware global)")
	}
	if !strings.Contains(paso0, "/api/v1/tenant/config") {
		return fmt.Errorf("la sección Paso 0 de ADR-005 no declara la exclusión de /api/v1/tenant/config")
	}
	if !strings.Contains(paso0, "/api/v1/tenant/config*") {
		// La forma del paso 0 es WILDCARD: el read lleva :key y una exclusión de
		// match exacto deja el 401 vivo tras "aplicar la corrección" (negativa de
		// diseño verificada 2026-09-15). Un paso 0 con match exacto no es el paso 0.
		return fmt.Errorf("la sección Paso 0 de ADR-005 declara /api/v1/tenant/config pero NO en forma wildcard (/api/v1/tenant/config*) — el read lleva :key y un match exacto no lo cubre")
	}
	if grepCi(paso0, "motivo", "por qué", "razón") == 0 {
		return fmt.Errorf("la sección Paso 0 de ADR-005 declara la exclusión sin su motivo (el contrato exige el paso 0 CON su motivo)")
	}
	return nil
}

// verificarFraseKongEliminada implementa la segunda negativa textual del "Hecho
// cuando": la afirmación vieja e inverificable quedó con grep -ci == 0.
func verificarFraseKongEliminada(adr string) error {
	if n := grepCi(adr, "el service-level de Kong aplica"); n != 0 {
		return fmt.Errorf("grep -ci 'el service-level de Kong aplica' ADR-005 == %d (exige 0: la afirmación sin cita volvió a colarse)", n)
	}
	return nil
}

// --------------------------------------------- checker 2: rate-limiting Kong

// esTopLevelPlugin reporta si la línea abre una entrada del array top-level de
// plugins (indent de 2 espacios, "- name:").
func esTopLevelPlugin(linea string) bool {
	return strings.HasPrefix(linea, "  - name: ")
}

// extraerEntradaPlugin devuelve el bloque de la entrada top-level `nombre` cuyo
// cuerpo referencia `serviceRef` (ej. rate-limiting + service: iam-service). En
// Kong declarativo un plugin top-level con `service:` es service-level: es la
// forma real del control que el gate del 2026-09-15 buscó inline y no encontró.
// Devuelve ok=false si la entrada no existe — que es EXACTAMENTE el estado que el
// gate original diagnosticó y por lo que el "Hecho cuando" permite la rama
// alternativa ("el ADR dice explícitamente que ese control no existe").
func extraerEntradaPlugin(kong, nombre, serviceRef string) (string, bool) {
	lineas := strings.Split(kong, "\n")
	apertura := "  - name: " + nombre
	for i, l := range lineas {
		if strings.TrimRight(l, " \t") != apertura {
			continue
		}
		bloque := []string{l}
		for j := i + 1; j < len(lineas); j++ {
			if esTopLevelPlugin(lineas[j]) {
				break // próxima entrada del array top-level
			}
			bloque = append(bloque, lineas[j])
		}
		texto := strings.Join(bloque, "\n")
		if strings.Contains(texto, serviceRef) {
			return texto, true
		}
	}
	return "", false
}

// verificarRateLimitingIam implementa la condición (2): existe un rate-limiting
// service-level para `iam-service` en el template, O el ADR dice explícitamente
// que ese control no existe y por qué se acepta. Las dos ramas son excluyentes:
// un ADR que apoye el control de borde en un plugin que no está (o viceversa)
// reproduce el defecto exacto que el gate rechazó.
func verificarRateLimitingIam(kong, adr string) error {
	if _, ok := extraerEntradaPlugin(kong, "rate-limiting", "service: iam-service"); ok {
		return nil // el control existe — el ADR puede citarlo, no se exige cómo
	}
	// Rama alternativa: el control no está → el ADR debe declararlo y justificarlo.
	if grepCi(adr, "rate-limiting") > 0 &&
		grepCi(adr, "no existe", "sin rate-limiting", "no hay rate-limiting") > 0 {
		return nil
	}
	return fmt.Errorf("kong.yml.template sin plugin rate-limiting para el service iam-service (inline ni top-level con service: iam-service) y el ADR no declara explícitamente que el control no existe — el paso 0 quedaría apoyado en un control inexistente")
}

// ------------------------------------------- checker 3: router.go (durable)

// parseExcludedRoutes extrae las entradas del bloque ExcludedRoutes de router.go.
// Vive como función pura para que los negativos la ejerzan contra fuentes
// sintéticas sin tocar el archivo real.
func parseExcludedRoutes(src string) ([]string, error) {
	i := strings.Index(src, "ExcludedRoutes: []string{")
	if i < 0 {
		return nil, fmt.Errorf("router.go: no se encontró el bloque ExcludedRoutes (¿cambió la forma de montar TenantValidation? re-revisar la premisa del hallazgo 1)")
	}
	resto := src[i+len("ExcludedRoutes: []string{"):]
	j := strings.Index(resto, "}")
	if j < 0 {
		return nil, fmt.Errorf("router.go: bloque ExcludedRoutes sin cerrar")
	}
	cuerpo := resto[:j]
	var rutas []string
	for _, l := range strings.Split(cuerpo, "\n") {
		l = strings.TrimSpace(l)
		l = strings.TrimSuffix(l, ",")
		l = strings.TrimSpace(l)
		if len(l) >= 2 && strings.HasPrefix(l, "\"") && strings.HasSuffix(l, "\"") {
			rutas = append(rutas, l[1:len(l)-1])
		}
	}
	if len(rutas) == 0 {
		return nil, fmt.Errorf("router.go: bloque ExcludedRoutes sin entradas parseables")
	}
	return rutas, nil
}

// verificarExclusionConfig verifica el contrato durable del paso 0 sobre el
// router REAL: mientras no exista exclusión para las rutas de config, la premisa
// del hallazgo 1 queda en pie (el request S2S muere con 401 — lo prueba el test de
// conducta); si T4 ya aplicó el paso 0, la exclusión DEBE ser wildcard. Lo único
// que este contrato rechaza siempre es la forma de match exacto: es la mutación
// silenciosa que dejaría el 401 vivo creyendo que el paso 0 está aplicado.
func verificarExclusionConfig(rutas []string) error {
	for _, r := range rutas {
		if strings.HasPrefix(r, "/api/v1/tenant/config") || strings.HasPrefix(r, "/api/v1/tenant/bootstrap") {
			if !strings.HasSuffix(r, "*") {
				return fmt.Errorf("router.go excluye %q con MATCH EXACTO — no cubre /api/v1/tenant/config/:key (path == route falla) y el 401 sobrevive: el paso 0 exige wildcard", r)
			}
		}
	}
	return nil
}

// ------------------------------------------------------------ tests: ADR

// TestADR005T1b_Paso0TenantValidation — condición (1) del gate: el ADR declara el
// paso 0 con su motivo y en forma wildcard. Contra el ADR real.
func TestADR005T1b_Paso0TenantValidation(t *testing.T) {
	if err := verificarPaso0(adr005Text(t)); err != nil {
		t.Fatalf("contrato ACC-E04.T1b hallazgo (1) INCUMPLIDO: %v", err)
	}
}

// TestADR005T1b_Negativo_Paso0AusenteFalla — negativo de verificador: un ADR sin
// la sección Paso 0 (el estado exacto que el gate de 2026-09-08 rechazó) DEBE
// hacer fallar al checker.
func TestADR005T1b_Negativo_Paso0AusenteFalla(t *testing.T) {
	adr := adr005Text(t)
	i := strings.Index(adr, "## Paso 0 (T4)")
	if i < 0 {
		t.Fatal("el ADR real no tiene la sección Paso 0 — este negativo necesita el ADR enmendado para poder mutarlo")
	}
	// Amputa desde el header del paso 0 hasta el próximo header de mismo nivel.
	resto := adr[i:]
	if j := strings.Index(resto, "\n## "); j > 0 {
		resto = resto[:j]
	}
	mutado := strings.Replace(adr, resto, "", 1)
	if err := verificarPaso0(mutado); err == nil {
		t.Fatal("el checker ACEPTÓ un ADR sin la sección Paso 0 — un verificador que nunca falló no prueba nada (el estado que el gate rechazó pasaría como válido)")
	}
}

// TestADR005T1b_Negativo_Paso0MatchExactoFalla — negativo de diseño: un paso 0
// con la exclusión en forma de match exacto DEBE hacer fallar al checker, aunque
// el resto del ADR esté bien. La mutación expresa TODA la declaración (bloque de
// decisión Y prosa que la describe) como match exacto: modela el ADR que "aplicó
// la corrección" sin wildcard — el read /:key quedaría bloqueado con 401.
func TestADR005T1b_Negativo_Paso0MatchExactoFalla(t *testing.T) {
	adr := strings.ReplaceAll(adr005Text(t), `"/api/v1/tenant/config*"`, `"/api/v1/tenant/config"`)
	if err := verificarPaso0(adr); err == nil {
		t.Fatal("el checker ACEPTÓ un paso 0 con exclusión de match exacto — el read /:key quedaría bloqueado con 401 tras aplicar la corrección")
	}
}

// TestADR005T1b_FraseKongEliminada — negativa textual del "Hecho cuando":
// grep -ci 'el service-level de Kong aplica' → 0.
func TestADR005T1b_FraseKongEliminada(t *testing.T) {
	if err := verificarFraseKongEliminada(adr005Text(t)); err != nil {
		t.Fatalf("contrato ACC-E04.T1b hallazgo (2), negativa textual INCUMPLIDA: %v", err)
	}
}

// TestADR005T1b_Negativo_FraseKongReintroducidaFalla — negativo de verificador:
// si la frase vaga vuelve al ADR, el checker DEBE fallar.
func TestADR005T1b_Negativo_FraseKongReintroducidaFalla(t *testing.T) {
	adr := adr005Text(t) + "\nAdemás, el service-level de Kong aplica.\n"
	if err := verificarFraseKongEliminada(adr); err == nil {
		t.Fatal("el checker ACEPTÓ un ADR con la afirmación vaga 'el service-level de Kong aplica' — exactamente el defecto que el gate rechazó")
	}
}

// TestADR005T1b_RateLimitingIamService — condición (2): el control de borde
// service-level para iam-service existe en el template (plugin top-level con
// `service: iam-service`, kong.yml.template:1290-1296) O el ADR declara su
// ausencia con justificación.
func TestADR005T1b_RateLimitingIamService(t *testing.T) {
	if err := verificarRateLimitingIam(kongTemplateText(t), adr005Text(t)); err != nil {
		t.Fatalf("contrato ACC-E04.T1b hallazgo (2) INCUMPLIDO: %v", err)
	}
}

// TestADR005T1b_Negativo_RateLimitingQuitadoFalla — negativo de verificador: con
// el plugin removido del template (el estado que el gate original creía ver) Y un
// ADR que no declara la ausencia, el checker DEBE fallar. Prueba que el checker
// distingue "control existe" de "control no existe sin declaración".
func TestADR005T1b_Negativo_RateLimitingQuitadoFalla(t *testing.T) {
	kong := kongTemplateText(t)
	entrada, ok := extraerEntradaPlugin(kong, "rate-limiting", "service: iam-service")
	if !ok {
		t.Fatal("el template real no tiene el plugin rate-limiting de iam-service — el estado real ya es el del hallazgo; este negativo necesita el control presente para poder mutarlo")
	}
	mutado := strings.Replace(kong, entrada, "", 1)
	// ADR sin declaración de ausencia: la rama alternativa tampoco salva al checker.
	adr := "ADR sin declaración de ausencia de rate-limiting."
	if err := verificarRateLimitingIam(mutado, adr); err == nil {
		t.Fatal("el checker ACEPTÓ un template sin rate-limiting para iam-service sin que el ADR declare la ausencia — el paso 0 quedaría apoyado en un control inexistente")
	}
}

// ------------------------------------------------------------ tests: conducta

// excludedRoutesMontadas es la lista montada HOY en router.go:56-65 — la copia
// literal del estado que el gate verificó (paso 0 AÚN no aplicado: es decisión de
// T4). El negativo de conducta del contrato corre contra esta lista: sin la
// exclusión, el request S2S muere con 401 antes del gated-escape.
var excludedRoutesMontadas = []string{
	"/health",
	"/api/v1/health",
	"/metrics",
	"/api/v1/auth/*",
	"/api/v1/tenants*",
	"/api/v1/users*",
	"/api/v1/roles*",
	"/api/v1/plans*",
}

// routerDePrueba arma un gin engine con TenantValidation (el mismo middleware que
// router.go monta global) y la ruta de config SIN implementar: el handler Next
// registra que el request llegó — llegar acá ES "pasar el middleware y quedar
// expuesto al gate Authorize", que es lo que el contrato llama "llega al gate".
func routerDePrueba(excluidas []string) (*gin.Engine, *bool) {
	gin.SetMode(gin.TestMode)
	llegoAlHandler := false
	r := gin.New()
	r.Use(tenantmw.TenantValidation(tenantmw.TenantValidationConfig{
		ExcludedRoutes: excluidas,
	}))
	r.GET("/api/v1/tenant/config/:key", func(c *gin.Context) {
		llegoAlHandler = true
		c.JSON(http.StatusOK, gin.H{"value": "REQUIRE_STOCK"})
	})
	return r, &llegoAlHandler
}

// get pide la key de config como un llamador S2S: X-API-Key pero SIN Authorization
// — el contrato que DD-1 crea y que el middleware global mata antes del gate.
func get(r *gin.Engine, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenant/config/"+key, nil)
	req.Header.Set("X-API-Key", "key-s2s-de-test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestADR005T1b_NegativoConducta_SinExclusionDa401 — el NEGATIVO que prueba el
// paso 0: con el middleware SIN la exclusión, un request S2S a
// /api/v1/tenant/config → 401 "Authorization header required". Si esto no diera
// 401, la premisa del hallazgo (1) era falsa y hay que revisarla (texto del
// "Hecho cuando").
func TestADR005T1b_NegativoConducta_SinExclusionDa401(t *testing.T) {
	r, llego := routerDePrueba(excludedRoutesMontadas)
	w := get(r, "catalog.stock_policy")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("premisa del hallazgo (1) FALSA: request S2S a /api/v1/tenant/config sin exclusión dio %d, no 401 — revisar la premisa del gate, no el test", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Authorization header required") {
		t.Fatalf("el 401 vino de otro control que el esperado (body: %s)", w.Body.String())
	}
	if *llego {
		t.Fatal("el request llegó al handler pese al 401 — el middleware no abortó")
	}
}

// TestADR005T1b_PositivaConExclusionWildcardLlegaAlGate — con la exclusión
// WILDCARD aplicada (lo que T4 hará), el mismo request pasa el middleware y llega
// al handler, donde el único control es el gate (Authorize + uuid.Nil + RLS).
func TestADR005T1b_PositivaConExclusionWildcardLlegaAlGate(t *testing.T) {
	excluidas := append(append([]string{}, excludedRoutesMontadas...), "/api/v1/tenant/config*")
	r, llego := routerDePrueba(excluidas)
	w := get(r, "catalog.stock_policy")
	if w.Code != http.StatusOK {
		t.Fatalf("con la exclusión wildcard el request no llegó al gate: %d (body: %s)", w.Code, w.Body.String())
	}
	if !*llego {
		t.Fatal("status 200 pero el handler no corrió")
	}
}

// TestADR005T1b_NegativoConducta_ExclusionExactaDejaEl401Vivo — negativo de
// diseño (enmienda 2026-09-15): una exclusión de MATCH EXACTO
// "/api/v1/tenant/config" NO cubre el read /api/v1/tenant/config/:key —
// `path == route` falla y el 401 sobrevive. La exclusión correcta es con wildcard.
func TestADR005T1b_NegativoConducta_ExclusionExactaDejaEl401Vivo(t *testing.T) {
	excluidas := append(append([]string{}, excludedRoutesMontadas...), "/api/v1/tenant/config")
	r, _ := routerDePrueba(excluidas)
	if w := get(r, "catalog.stock_policy"); w.Code != http.StatusUnauthorized {
		t.Fatalf("la exclusión de match exacto dejó pasar el read /:key con %d — contradice la semántica isExcluded (path == route) verificada el 2026-09-15", w.Code)
	}
}

// ------------------------------------------------------------ test: router real

// TestADR005T1b_RouterReal_ExclusionSoloWildcards — contrato durable sobre el
// router REAL: ninguna exclusión de las rutas de config puede montarse en forma de
// match exacto. Antes del paso 0 (hoy) no hay ninguna entrada de config y la
// premisa del hallazgo queda en pie; cuando T4 aplique el paso 0, la entrada debe
// ser wildcard. El único estado que este test rechaza siempre es el match exacto
// silencioso.
func TestADR005T1b_RouterReal_ExclusionSoloWildcards(t *testing.T) {
	rutas, err := parseExcludedRoutes(routerGoText(t))
	if err != nil {
		t.Fatalf("premisa del hallazgo (1) sin verificar contra el router real: %v", err)
	}
	if err := verificarExclusionConfig(rutas); err != nil {
		t.Fatalf("contrato ACC-E04.T1b sobre router.go INCUMPLIDO: %v", err)
	}
}

// TestADR005T1b_Negativo_RouterConMatchExactoFalla — negativo de verificador del
// parser y del contrato: una ExcludedRoutes con "/api/v1/tenant/config" pelada DEBE
// hacer fallar al checker (fuente sintética, el router real nunca se toca).
func TestADR005T1b_Negativo_RouterConMatchExactoFalla(t *testing.T) {
	fuente := `router.Use(tenantmw.TenantValidation(tenantmw.TenantValidationConfig{
		ExcludedRoutes: []string{
			"/health",
			"/api/v1/tenants*",
			"/api/v1/tenant/config",
		},
	}))`
	rutas, err := parseExcludedRoutes(fuente)
	if err != nil {
		t.Fatalf("parser no pudo leer una fuente válida: %v", err)
	}
	if err := verificarExclusionConfig(rutas); err == nil {
		t.Fatal("el checker ACEPTÓ una exclusión de match exacto para /api/v1/tenant/config — dejaría el 401 vivo con el paso 0 'aplicado'")
	}
}
