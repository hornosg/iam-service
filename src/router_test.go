package main

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	_ "github.com/lib/pq" // registra el driver "postgres"; sql.Open con DSN dummy NO conecta
)

// ACC-E01 T8: golden de rutas HTTP. La mudanza de T4 no puede agregar, quitar
// ni mover una sola ruta: este test es el que lo detecta.
//
// El golden se capturó del árbol pre-mudanza con:
//
//	ROUTES_GOLDEN_UPDATE=1 go test ./src/ -run TestRoutesGolden
//
// El modo update existe para regenerar el golden sólo cuando un cambio de
// contrato HTTP es deliberado (y reviewado en el diff de git). La corrida
// normal del loop NUNCA lo setea: compara sin escribir.

const routesGoldenPath = "routes.golden"

// buildRouterForTest cablea el router real con DSNs dummy: sql.Open valida la
// cadena pero no abre conexión, y ningún Setup* del wiring toca la red en
// montaje (sólo constructores).
func buildRouterForTest(t *testing.T) *gin.Engine {
	t.Helper()

	appDB, err := sql.Open("postgres", "host=localhost port=5432 user=test password=test dbname=test sslmode=disable")
	if err != nil {
		t.Fatalf("sql.Open appDB: %v", err)
	}
	t.Cleanup(func() { _ = appDB.Close() })

	loginDB, err := sql.Open("postgres", "host=localhost port=5432 user=test password=test dbname=test sslmode=disable")
	if err != nil {
		t.Fatalf("sql.Open loginDB: %v", err)
	}
	t.Cleanup(func() { _ = loginDB.Close() })

	return buildRouter(appDB, loginDB)
}

// registeredRoutes devuelve la lista "METHOD path" en el orden de registro de
// gin (determinístico: es el orden de montaje del wiring).
func registeredRoutes(t *testing.T) []string {
	t.Helper()
	router := buildRouterForTest(t)
	routes := router.Routes()
	out := make([]string, 0, len(routes))
	for _, r := range routes {
		out = append(out, r.Method+" "+r.Path)
	}
	return out
}

func TestRoutesGolden(t *testing.T) {
	// PROMETHEUS_ENABLED=true es la configuración desplegada del lab
	// (.env.example) y registra el set completo de rutas, incluido /metrics.
	t.Setenv("PROMETHEUS_ENABLED", "true")
	// Secreto dummy: evita el warning de ValidateJWTSecret y hace la corrida
	// determinística sin acercarse a una credencial real.
	t.Setenv("JWT_SECRET", "golden-test-secret-not-a-real-credential-0123456789")
	gin.SetMode(gin.TestMode)

	got := registeredRoutes(t)

	if os.Getenv("ROUTES_GOLDEN_UPDATE") == "1" {
		content := strings.Join(got, "\n") + "\n"
		if err := os.WriteFile(routesGoldenPath, []byte(content), 0o644); err != nil {
			t.Fatalf("escribir %s: %v", routesGoldenPath, err)
		}
		t.Logf("golden regenerado: %d rutas", len(got))
		return
	}

	raw, err := os.ReadFile(routesGoldenPath)
	if err != nil {
		t.Fatalf("leer %s: %v (¿falta capturar con ROUTES_GOLDEN_UPDATE=1?)", routesGoldenPath, err)
	}
	want := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")

	if len(got) != len(want) {
		t.Errorf("cantidad de rutas: obtenidas %d, golden %d", len(got), len(want))
	}
	for i := 0; i < len(got) && i < len(want); i++ {
		if got[i] != want[i] {
			t.Errorf("ruta[%d]: obtenida %q, golden %q", i, got[i], want[i])
		}
	}
	if len(got) > len(want) {
		t.Errorf("rutas nuevas no presentes en el golden:\n  %s", strings.Join(got[len(want):], "\n  "))
	}
	if len(want) > len(got) {
		t.Errorf("rutas del golden que ya no se registran:\n  %s", strings.Join(want[len(got):], "\n  "))
	}
}