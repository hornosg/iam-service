// Package archtest — Negativa versionada de la frontera de ACC-E01 T6.
//
// El "Hecho cuando" de T6 exige que las tres prohibiciones se DEMUESTREN
// introduciendo y revirtiendo a mano cada import: identity ↛ subscription,
// identity ↛ onboarding, access ↛ subscription. La evidencia de la tarea
// (commit 921477f) registró inyecciones manuales + TestArchitectureReglas,
// que versiona la detección de la función pura `importsProhibidos` — pero la
// mitad del guardrail (walk del árbol real + parse + falla con mensaje propio)
// seguía demostrada una sola vez, a mano, ante testigos. Este test pinea esa
// mitad: la misma inyección, corrida y reversión, versionada.
//
// Por qué el probe vive en un SUBPAQUETE nuevo (src/<módulo>/qa_probe_t6/)
// y no en un archivo existente: el probe importa un paquete VACÍO
// (subscription/onboarding), así que si tocara un archivo de un paquete real
// o si `go test ./...` compilara el probe, la falla sería del COMPILADOR, no
// del guardrail — y la evidencia volvería a ser la de un detector que nunca
// corrió. Un subpaquete que nadie importa no entra al build de
// `go test ./src/ -run TestArchitecture`, el guardrail lo camina igual (es
// recursivo sobre el módulo vigilado) y la falla es la suya.
//
// Por qué está gated tras QA_T6_E2E=1 y no corre en cada `go test ./...`:
// mientras el probe existe, un `go test ./...` paralelo matchea src/<módulo>/
// qa_probe_t6 como paquete y muere compilando el import del paquete vacío —
// un flake del suite por una ventana que nadie pidió. Correrlo explícito:
//
//	QA_T6_E2E=1 go test ./test/arch/ -run TestT6FronteraNegativa -v
//
// La contraparte positiva del pipeline (walk + parse del árbol real) ya
// corre en cada `go test ./src/` vía TestArchitecture de src/arch_test.go.
package archtest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// prohibicionesT6 son las TRES prohibiciones del contrato de T6, hardcodeadas
// ACÁ y no leídas de la tabla del guardrail a propósito: este test pinea el
// CONTRATO tal como está escrito en la épica. Si alguien edita la tabla de
// src/arch_test.go, este test sigue exigiendo las tres filas del contrato.
var prohibicionesT6 = []struct {
	vigilado  string
	prohibido string
}{
	{"identity", "subscription"},
	{"identity", "onboarding"},
	{"access", "subscription"},
}

// TestT6ContratoArchTest pinea, sin gate, lo que el contrato de T6 exige del
// entregable MISMO y no de su comportamiento: vive en src/arch_test.go (el
// contrato nombra el archivo), declara TestArchitecture y usa go/parser con
// el patrón del arch_test de chatql — "no se inventa uno nuevo" es parte del
// contrato, no un gusto del implementador.
func TestT6ContratoArchTest(t *testing.T) {
	root := guardWiringModuleRoot(t)
	arch, err := os.ReadFile(filepath.Join(root, "src", "arch_test.go"))
	if err != nil {
		t.Fatalf("el contrato de T6 exige el test en src/arch_test.go: %v", err)
	}
	src := string(arch)
	for _, requisito := range []struct{ fragmento, porque string }{
		{"func TestArchitecture", "el guardrail de T6 debe exponerse como TestArchitecture (el Hecho cuando lo invoca por nombre)"},
		{"\"go/parser\"", "el contrato manda parsear el AST con go/parser (patrón chatql), no un mecanismo inventado"},
		{"parser.ImportsOnly", "el patrón del arch_test de chatql usa ImportsOnly"},
	} {
		if !strings.Contains(src, requisito.fragmento) {
			t.Errorf("src/arch_test.go: falta %q — %s", requisito.fragmento, requisito.porque)
		}
	}
}

// runArchTest corre `go test ./src/ -run TestArchitecture` desde la raíz del
// módulo y devuelve exit code + salida combinada.
func runArchTest(t *testing.T, root string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "./src/", "-run", "TestArchitecture")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("go test ./src/: %v — %s", err, string(out))
	}
	return code, string(out)
}

// TestT6FronteraNegativa es la demostración negativa de T6, versionada: para
// cada prohibición del contrato inyecta un probe que importa el módulo
// prohibido dentro del módulo vigilado, y exige que el guardrail falle con SU
// mensaje (frontera de ACC-E01 T6 violada) — no con un error de compilación,
// que sería evidencia del compilador y no del detector. Cada probe se revierte
// antes del caso siguiente, y al final el árbol limpio debe dar PASS de
// vuelta: sin la reversión que pasa, la falla no prueba nada del guardrail.
func TestT6FronteraNegativa(t *testing.T) {
	if os.Getenv("QA_T6_E2E") == "" {
		t.Skip("gated tras QA_T6_E2E=1: inyecta archivos en src/ y un `go test ./...` paralelo moriría compilando el probe (ver cabecera del archivo). Correr: QA_T6_E2E=1 go test ./test/arch/ -run TestT6FronteraNegativa -v")
	}
	root := guardWiringModuleRoot(t)

	modOut, err := exec.Command("go", "list", "-m").Output()
	if err != nil {
		t.Fatalf("go list -m: %v", err)
	}
	mod := strings.TrimSpace(string(modOut))

	for _, regla := range prohibicionesT6 {
		t.Run(regla.vigilado+"_neq_"+regla.prohibido, func(t *testing.T) {
			probeDir := filepath.Join(root, "src", regla.vigilado, "qa_probe_t6")
			probe := "package qa_probe_t6\n\nimport (\n\t_ \"" + mod + "/src/" + regla.prohibido + "\"\n)\n"
			if err := os.MkdirAll(probeDir, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", probeDir, err)
			}
			defer func() {
				if err := os.RemoveAll(probeDir); err != nil {
					t.Fatalf("cleanup %s: %v", probeDir, err)
				}
			}()
			if err := os.WriteFile(filepath.Join(probeDir, "probe.go"), []byte(probe), 0o644); err != nil {
				t.Fatalf("escribir probe: %v", err)
			}

			code, out := runArchTest(t, root)
			if code == 0 {
				t.Fatalf("src/%s importa src/%s y el guardrail PASÓ (exit 0) — la frontera de T6 es decorativa:\n%s",
					regla.vigilado, regla.prohibido, out)
			}
			if !strings.Contains(out, "frontera de ACC-E01 T6 violada") {
				t.Fatalf("src/%s → src/%s: falló (exit %d) pero sin el mensaje del arch test — la falla es del compilador, no del guardrail:\n%s",
					regla.vigilado, regla.prohibido, code, out)
			}
		})
	}

	// Reversión: árbol limpio → el guardrail vuelve a PASS. El contrato exige
	// cada reversión → PASS; si el guardrail quedara fallando sin violación
	// real, su señal no distinguiría frontera rota de ruido.
	t.Run("reversion_pasa", func(t *testing.T) {
		if code, out := runArchTest(t, root); code != 0 {
			t.Fatalf("con el árbol limpio el guardrail sigue fallando (exit %d) — la reversión no pasa:\n%s", code, out)
		}
	})
}