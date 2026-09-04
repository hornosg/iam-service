package main

import (
	"database/sql"
	"fmt"
	"os"
	"reflect"
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

// routeDiff compara las rutas registradas contra el golden y devuelve la lista
// de discrepancias; vacía = el contrato se cumple. Vive acá para que
// TestRoutesGolden y los casos negativos ejerciten LA MISMA lógica de
// comparación — un negativo contra una copia probaría la copia, no el control.
func routeDiff(got, want []string) []string {
	var diffs []string
	if len(got) != len(want) {
		diffs = append(diffs, fmt.Sprintf("cantidad de rutas: obtenidas %d, golden %d", len(got), len(want)))
	}
	for i := 0; i < len(got) && i < len(want); i++ {
		if got[i] != want[i] {
			diffs = append(diffs, fmt.Sprintf("ruta[%d]: obtenida %q, golden %q", i, got[i], want[i]))
		}
	}
	if len(got) > len(want) {
		diffs = append(diffs, "rutas nuevas no presentes en el golden:\n  "+strings.Join(got[len(want):], "\n  "))
	}
	if len(want) > len(got) {
		diffs = append(diffs, "rutas del golden que ya no se registran:\n  "+strings.Join(want[len(got):], "\n  "))
	}
	return diffs
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

	for _, d := range routeDiff(got, want) {
		t.Error(d)
	}
}

// TestRoutesGoldenDetectaCorrupcion — negativa del contrato de T8, hecha
// ejecutable: si la comparación deja de detectar cualquiera de estas
// corrupciones del golden, una mudanza que pierda o cambie una ruta pasa en
// silencio. La demostración a mano del implementador probó que falló UNA VEZ;
// este test la regresa en cada corrida (misma doctrina que T6 para arch tests).
func TestRoutesGoldenDetectaCorrupcion(t *testing.T) {
	got := []string{
		"GET /api/v1/users",
		"POST /api/v1/auth/login",
		"DELETE /api/v1/roles/:id",
	}
	cases := []struct {
		name       string
		want       []string
		quieroDiff bool // true: la corrupción DEBE detectarse; false: control sano
	}{
		{"ruta quitada del golden", got[:2], true},
		{"ruta nueva que el golden no registra", append(append([]string{}, got...), "PATCH /api/v1/tenants/:id/features"), true},
		{"metodo cambiado", []string{"GET /api/v1/users", "PUT /api/v1/auth/login", "DELETE /api/v1/roles/:id"}, true},
		{"path cambiado", []string{"GET /api/v1/users", "POST /api/v1/auth/logins", "DELETE /api/v1/roles/:id"}, true},
		{"golden vacio", []string{}, true},
		{"control: golden identico", got, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diffs := routeDiff(got, c.want)
			if c.quieroDiff && len(diffs) == 0 {
				t.Errorf("la comparación del golden NO detectó la corrupción %q — un golden que no detecta es decoración", c.name)
			}
			if !c.quieroDiff && len(diffs) != 0 {
				t.Errorf("control sano reportó diferencias que no existen:\n  %s", strings.Join(diffs, "\n  "))
			}
		})
	}
}

// TestRoutesGoldenOrdenDeterminista — el golden compara en orden, así que el
// orden de Routes() tiene que ser estable entre construcciones. Si el wiring
// alguna vez itera un map para montar módulos, este test revienta antes de que
// el golden empiece a fallar al azar.
func TestRoutesGoldenOrdenDeterminista(t *testing.T) {
	t.Setenv("PROMETHEUS_ENABLED", "true")
	t.Setenv("JWT_SECRET", "golden-test-secret-not-a-real-credential-0123456789")
	gin.SetMode(gin.TestMode)

	primera := registeredRoutes(t)
	segunda := registeredRoutes(t)

	if !reflect.DeepEqual(primera, segunda) {
		t.Errorf("router.Routes() no es determinista entre construcciones:\n  primera:  %v\n  segunda: %v", primera, segunda)
	}
}
