package main

// Test de arquitectura de ACC-E01 T6: frontera ejecutable entre módulos.
//
// La condición de diseño de PLAT-PROP-013 es que el camino de login no puede
// depender de `subscription`: si billing se rompe, el login de todo el lab
// sigue funcionando. Hasta acá esa condición vivía en un documento; este test
// la vuelve ejecutable — parsea el AST (go/parser, patrón del arch_test de
// pocs/chatql) de cada archivo bajo `src/identity` y `src/access` y rechaza
// cualquier import de un módulo prohibido.
//
// Por qué AST y no `go list -deps`: `src/subscription` y `src/onboarding`
// todavía están VACÍOS (llegan en ACC-E06/ACC-E05), y un paquete sin archivos
// .go no se puede importar — una violación real moriría en el compilador antes
// de que cualquier análisis de dependencias corriera. El parser lee el import
// path tal como está escrito en el fuente y detecta la violación aunque el
// paquete destino todavía no exista.
//
// El test vigila archivos de producción Y de test (_test.go): un import que
// vive sólo en un test de identity es acoplamiento igual (lección O1 del
// arch_test de chatql) — el login no puede quedar colgado de billing por la
// puerta del test.
//
// Prohibiciones codificadas por el contrato de T6 (la lista autoritativa es
// la tabla `prohibiciones` de abajo, no este comentario):
//
//	src/identity ↛ src/subscription   — el login no puede depender de billing
//	src/identity ↛ src/onboarding     — ni de la saga de alta
//	src/access   ↛ src/subscription   — la autorización no puede caer con billing
//
// `access ↛ onboarding` NO está prohibido por el contrato de T6. Si el owner
// decide prohibirlo, es una fila más en `prohibiciones` — nada más cambia.
//
// Coexiste con `test/arch/arch_test.go` (el arch test de capas de ACC-E02,
// que vive en el árbol externo): ese vigila la regla hexagonal
// dominio/application; este vigila fronteras entre módulos del monolito.
// El module path se resuelve con `go list -m` en runtime, así el rename a
// `account-service` de ACC-E07 no lo pudre.

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// prohibiciones es la frontera estructural de la épica, como datos: cada fila
// dice qué módulo (bajo src/) no puede importar a cuál. El match es por
// prefijo — el módulo prohibido entero, con todos sus subpaquetes.
var prohibiciones = []struct {
	vigilado  string // módulo vigilado, bajo src/
	prohibido string // módulo cuyo import se rechaza, bajo src/
}{
	{"identity", "subscription"},
	{"identity", "onboarding"},
	{"access", "subscription"},
}

// moduleRoot devuelve el directorio raíz del módulo Go (patrón del arch_test
// de chatql: `go test` corre con cwd = directorio del paquete, y las rutas a
// caminar son relativas a la raíz del módulo).
func moduleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}").Output()
	if err != nil {
		t.Fatalf("go list -m -f {{.Dir}}: %v", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		t.Fatal("no se pudo resolver el directorio del módulo")
	}
	return root
}

// moduleImportPath devuelve el import path del módulo
// (github.com/hornosg/iam-service; account-service tras ACC-E07).
func moduleImportPath(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m").Output()
	if err != nil {
		t.Fatalf("go list -m: %v", err)
	}
	mod := strings.TrimSpace(string(out))
	if mod == "" {
		t.Fatal("no se pudo resolver el import path del módulo")
	}
	return mod
}

// esPrefijo reporta si path es igual a base o cae bajo base/ (el módulo
// prohibido o cualquiera de sus subpaquetes).
func esPrefijo(path, base string) bool {
	return path == base || strings.HasPrefix(path, base+"/")
}

// importsProhibidos devuelve los import paths de `imports` que el módulo
// `vigilado` no puede importar según la tabla.
//
// Función pura —sin filesystem, sin exec— para que las reglas mismas tengan
// test sintético (TestArchitectureReglas): la única prueba de que un
// guardrail detecta no puede ser una inyección manual a la que alguien miró
// una vez; eso no queda registrado ni vuelve a correr solo.
func importsProhibidos(vigilado string, imports []string, modulePath string) []string {
	var malos []string
	for _, regla := range prohibiciones {
		if regla.vigilado != vigilado {
			continue
		}
		base := modulePath + "/src/" + regla.prohibido
		for _, imp := range imports {
			if esPrefijo(imp, base) {
				malos = append(malos, imp)
			}
		}
	}
	return malos
}

// archivosGo lista todos los .go (producción y test) bajo dir, como paths
// relativos a root — legibles en el mensaje de falla.
func archivosGo(t *testing.T, root, dir string) []string {
	t.Helper()
	var archivos []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			rel, rerr := filepath.Rel(root, path)
			if rerr != nil {
				return rerr
			}
			archivos = append(archivos, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return archivos
}

// TestArchitecture es la frontera de T6: falla si algún archivo de producción
// o de test bajo un módulo vigilado importa un módulo prohibido. Corre en
// cada `go test ./src/` y en CI — no cuando alguien se acuerda.
func TestArchitecture(t *testing.T) {
	root := moduleRoot(t)
	mod := moduleImportPath(t)
	fset := token.NewFileSet()

	// Módulos vigilados = los que aparecen como vigilado en la tabla, sin
	// duplicar. Si la tabla está vacía, el guardrail no vigilaría nada —
	// mejor reventar que pasar de forma vacua.
	vigilados := []string{}
	visto := map[string]bool{}
	for _, regla := range prohibiciones {
		if !visto[regla.vigilado] {
			visto[regla.vigilado] = true
			vigilados = append(vigilados, regla.vigilado)
		}
	}
	if len(vigilados) == 0 {
		t.Fatal("la tabla de prohibiciones está vacía — el guardrail no vigilaría nada")
	}

	for _, vigilado := range vigilados {
		dir := filepath.Join(root, "src", vigilado)
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("no existe el módulo vigilado src/%s (¿se ejecutó la mudanza de T4?): %v", vigilado, err)
		}
		archivos := archivosGo(t, root, dir)
		if len(archivos) == 0 {
			t.Errorf("src/%s no tiene archivos .go — el guardrail pasa de forma vacua sobre un módulo que debe existir (login/RBAC viven acá)", vigilado)
			continue
		}
		for _, archivo := range archivos {
			f, err := parser.ParseFile(fset, filepath.Join(root, archivo), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("no se pudo parsear %s: %v", archivo, err)
			}
			var imports []string
			for _, spec := range f.Imports {
				imports = append(imports, strings.Trim(spec.Path.Value, `"`))
			}
			for _, malo := range importsProhibidos(vigilado, imports, mod) {
				t.Errorf("%s importa %q — frontera de ACC-E01 T6 violada.\n"+
					"La condición de diseño de PLAT-PROP-013 es que un bug de subscription/onboarding\n"+
					"no pueda tumbar el login del lab. Si el acoplamiento es realmente necesario,\n"+
					"la frontera se negocia en una propuesta — no se rompe en un import.", archivo, malo)
			}
		}
	}
}

// TestArchitectureReglas es la verificación negativa VERSIONADA del guardrail:
// casos sintéticos que prueban que cada prohibición de la tabla detecta y que
// los imports legítimos no se marcan. Las inyecciones manuales de la evidencia
// de T6 demostraron la detección una vez; este test la re-prueba en cada
// `go test`, para siempre (patrón Test_violations del arch_test de chatql).
func TestArchitectureReglas(t *testing.T) {
	const mod = "github.com/ejemplo/servicio"
	var (
		sub = mod + "/src/subscription"
		onb = mod + "/src/onboarding"
	)

	casos := []struct {
		nombre   string
		vigilado string
		imports  []string
		esperado []string
	}{
		{"identity con imports legítimos no se marca", "identity",
			[]string{mod + "/src/shared/context", mod + "/src/identity/domain/entity"},
			nil},
		{"identity importa subscription (prohibido)", "identity",
			[]string{mod + "/src/shared/context", sub},
			[]string{sub}},
		{"identity importa onboarding (prohibido)", "identity",
			[]string{onb},
			[]string{onb}},
		{"identity importa un subpaquete de subscription (prohibido)", "identity",
			[]string{sub + "/agenda"},
			[]string{sub + "/agenda"}},
		{"access importa subscription (prohibido)", "access",
			[]string{mod + "/src/access/domain/entity", sub},
			[]string{sub}},
		{"access importa onboarding (NO prohibido por el contrato de T6)", "access",
			[]string{onb},
			nil},
		{"prefijo como substring no matchea", "identity",
			[]string{sub + "x"}, // sin la barra: NO es subpaquete de subscription
			nil},
		{"tenancy no está vigilado por esta tabla", "tenancy",
			[]string{sub, onb},
			nil},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := importsProhibidos(c.vigilado, c.imports, mod)
			if strings.Join(got, ",") != strings.Join(c.esperado, ",") {
				t.Errorf("importsProhibidos(%q, %v) = %v, esperado %v", c.vigilado, c.imports, got, c.esperado)
			}
		})
	}
}