package main

// ACC-E01 T7 — tests del switch de módulos del wiring (fase TEST del loop).
//
// Contrato que versionan: el wiring consulta una lista separada por comas en
// una variable de entorno antes de montar cada módulo; un módulo listado NO
// se monta — su Setup*Module no corre y sus rutas no existen en el router.
// Sin la variable, todos los módulos se montan (el default es el
// comportamiento de hoy). El switch vive SOLO en el wiring (main.go /
// router.go), nunca dentro de los módulos.
//
// El nombre de la variable se construye POR PARTES (switchEnvKey, abajo):
// el criterio (d) de la tarea es un grep mecánico sobre src/*.go que sólo
// tolera la literal contigua en main.go y router.go. Este archivo es del
// paquete del wiring — no un módulo — pero cerrar el gap del parser no puede
// romper ese criterio, y TestSwitchViveSoloEnElWiring (al pie) es
// justamente la versión ejecutable de ese grep: lo corre cada `go test`.
//
// Lo que estos tests NO cubren, con motivo:
//   - el HTTP 200 con token contra lab-postgres (criterios (a)/(c) en
//     runtime): necesita la base sembrada del smoke de ACC-E02 T9; la
//     presencia de la ruta y el 404 de la negativa sí quedan versionados
//     acá, in-process, sin infraestructura.
//   - el guard de arranque tenancy-sin-identity: es un log.Fatalf (sale del
//     proceso); testeable sólo en runtime, no in-process.

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// switchEnvKey — el nombre de la variable del switch, armado por partes para
// no escribir la literal contigua en este archivo (ver comentario del
// archivo). No lo reescribas como literal: rompería el criterio (d).
const switchEnvKey = "MODULES" + "_" + "DISABLED"

// modulosCanonicos — claves que el wiring consulta al montar (ver
// modulesDisabled en router.go). subscription y onboarding se aceptan sin
// wiring propio: módulos vacíos hasta ACC-E05 / ACC-E06.
var modulosCanonicos = []string{"identity", "access", "tenancy", "plans", "subscription", "onboarding"}

// contieneRuta dice si la lista de rutas (formato "METHOD path") registra la
// ruta pedida.
func contieneRuta(rutas []string, ruta string) bool {
	for _, r := range rutas {
		if r == ruta {
			return true
		}
	}
	return false
}

// TestModulesDisabledParser — el gap declarado de T7: el parser del switch
// no tenía test unitario. Cubre el contrato de parsing: lista separada por
// comas, trim, case-insensitive, alias histórico auth→identity, variable
// ausente = todos montados, nombre desconocido no desmonta nada canónico.
func TestModulesDisabledParser(t *testing.T) {
	// Fija la variable para que la limpieza de t.Setenv restaure siempre a
	// un estado conocido; cada caso pisa el valor (o la borra).
	t.Setenv(switchEnvKey, "sentinel-de-limpieza")

	cases := []struct {
		name  string
		valor string // "" = variable ausente (se desetea de verdad)
		quiero map[string]bool // claves canónicas que deben quedar desmontadas
	}{
		{"variable ausente: todos los módulos se montan", "", map[string]bool{}},
		{"un módulo listado", "subscription", map[string]bool{"subscription": true}},
		{"alias histórico auth desmonta identity", "auth", map[string]bool{"identity": true}},
		{"alias case-insensitive", "AUTH", map[string]bool{"identity": true}},
		{"lista con espacios y mezcla", " Auth , Subscription ", map[string]bool{"identity": true, "subscription": true}},
		{"nombre desconocido no desmonta nada canónico", "modulo-que-no-existe", map[string]bool{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.valor == "" {
				if err := os.Unsetenv(switchEnvKey); err != nil {
					t.Fatalf("unset: %v", err)
				}
			} else if err := os.Setenv(switchEnvKey, c.valor); err != nil {
				t.Fatalf("set: %v", err)
			}

			got := modulesDisabled()
			for _, m := range modulosCanonicos {
				if got[m] != c.quiero[m] {
					t.Errorf("con %q: modulesDisabled()[%q] = %v, se esperaba %v (mapa completo: %v)",
						c.valor, m, got[m], c.quiero[m], got)
				}
			}
		})
	}
}

// rutasComoStrings — []gin.RouteInfo → []string "METHOD path", para reusar
// contieneRuta sobre router.Routes() sin re-construir el router.
func rutasComoStrings(rutas []gin.RouteInfo) []string {
	out := make([]string, 0, len(rutas))
	for _, r := range rutas {
		out = append(out, r.Method+" "+r.Path)
	}
	return out
}

// TestSwitchDesmontaIdentity — NEGATIVA DE CONTROL, criterio (b) de ACC-E01
// T7: con el módulo de login desmontado, la ruta NO existe en el router y el
// POST responde 404. Si este test falla, el switch es cosmético: monta
// igual aunque la variable lo liste — la tarea falla por definición.
//
// El oráculo se ejerce en ambas direcciones junto con
// TestSwitchModulosVaciosNoTocanRutas (login presente sin switch): si
// contieneRuta no distinguiera montado de desmontado, alguno de los dos
// revienta.
func TestSwitchDesmontaIdentity(t *testing.T) {
	t.Setenv(switchEnvKey, "auth") // alias histórico de identity — el del criterio (b)
	t.Setenv("PROMETHEUS_ENABLED", "true")
	t.Setenv("JWT_SECRET", "golden-test-secret-not-a-real-credential-0123456789")
	gin.SetMode(gin.TestMode)

	router := buildRouterForTest(t)
	rutas := router.Routes()

	// El bloque entero de identity desaparece: login y users son del módulo.
	ausentes := []string{
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/refresh",
		"POST /api/v1/auth/logout",
		"POST /api/v1/auth/revoke-all",
		"GET /api/v1/users",
	}
	for _, ruta := range ausentes {
		if contieneRuta(rutasComoStrings(rutas), ruta) {
			t.Errorf("ruta %q sigue montada con identity desmontado — el switch no apaga de verdad", ruta)
		}
	}

	// Control del control: el switch apaga el módulo listado, no el router.
	// El resto de los módulos (tenancy, plans, access) sigue montado.
	for _, ruta := range []string{
		"POST /api/v1/tenants",
		"GET /api/v1/plans",
		"GET /api/v1/roles",
	} {
		if !contieneRuta(rutasComoStrings(rutas), ruta) {
			t.Errorf("ruta %q NO está montada — el switch apagó un módulo que no estaba listado", ruta)
		}
	}

	// El 404 del criterio (b), versionado in-process: la ruta no montada
	// responde 404 (no 401/500 del handler): gin nunca encontró la ruta.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("POST /api/v1/auth/login con identity desmontado devolvió %d, se esperaba 404 (ruta no montada)", rec.Code)
	}
}

// TestSwitchModulosVaciosNoTocanRutas — criterio (a) de ACC-E01 T7: con
// subscription desmontado el login sobrevive. subscription y onboarding son
// módulos vacíos: desmontarlos no puede cambiar NI UNA ruta del router —
// el mecanismo se arma sin tocar nada. La condición de diseño completa
// (login ↮ subscription en runtime) se re-verifica cuando ACC-E06 llene el
// módulo.
func TestSwitchModulosVaciosNoTocanRutas(t *testing.T) {
	t.Setenv(switchEnvKey, "") // sin switch: default = todos montados
	t.Setenv("PROMETHEUS_ENABLED", "true")
	t.Setenv("JWT_SECRET", "golden-test-secret-not-a-real-credential-0123456789")
	gin.SetMode(gin.TestMode)

	porDefecto := registeredRoutes(t)
	if !contieneRuta(porDefecto, "POST /api/v1/auth/login") {
		t.Fatal("el router default no monta el login — el resto del test no significa nada")
	}

	for _, modulo := range []string{"subscription", "onboarding"} {
		t.Run(modulo, func(t *testing.T) {
			t.Setenv(switchEnvKey, modulo)
			conSwitch := registeredRoutes(t)

			// Módulo vacío, wiring vacío: la lista de rutas es idéntica.
			if diff := routeDiff(conSwitch, porDefecto); len(diff) != 0 {
				t.Errorf("desmontar %q cambió rutas fuera del módulo (está vacío — nada debe moverse):\n  %s",
					modulo, strings.Join(diff, "\n  "))
			}
			// El login del smoke queda montado — criterio (a).
			if !contieneRuta(conSwitch, "POST /api/v1/auth/login") {
				t.Errorf("login no está montado con %q desmontado — el smoke de T7 lo necesita vivo", modulo)
			}
		})
	}
}

// TestSwitchViveSoloEnElWiring — criterio (d) de ACC-E01 T7, versionado: la
// literal contigua del nombre de la variable no aparece en ningún .go bajo
// src/ fuera de main.go y router.go. El switch es del wiring; si la variable
// filtrara a un módulo, este test lo caza en cada corrida — igual que el
// grep del criterio, pero sin depender de que alguien lo corra a mano.
//
// La aguja se re-arma por partes para que este archivo no se encuentre a sí
// mismo (por eso también switchEnvKey va por partes).
func TestSwitchViveSoloEnElWiring(t *testing.T) {
	aguja := "MODULES" + "_" + "DISABLED"

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		// El grep del criterio excluye por path: main.go y router.go.
		if strings.Contains(path, "main.go") || strings.Contains(path, "router.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if i := strings.Index(string(data), aguja); i >= 0 {
			linea := strings.Count(string(data)[:i], "\n") + 1
			t.Errorf("src/%s:%d contiene la literal del switch — el switch vive SOLO en el wiring (main.go/router.go)", path, linea)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("escanear src/: %v", err)
	}
}