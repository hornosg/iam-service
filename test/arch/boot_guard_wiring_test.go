// Package archtest — Test de wiring del guard de arranque (ACC-E02 T8o).
//
// El "Hecho cuando" de T8o dice que con ALLOW_SUPERUSER_DB=true y un
// ENVIRONMENT de producción el servicio NO arranca y el log lo dice. La
// negativa vive en sharedpostgres.AssertNoRLSBypass y está cubierta por sus
// unit tests (guard_test.go: EscotillaAbortaEnProduccion/SinMarcador), pero
// esa negativa sólo sirve si el boot EFECTIVAMENTE consulta el guard en cada
// conexión que abre y aborta con log.Fatalf. Ese wiring no estaba pineado: si
// alguien borra el AssertNoRLSBypass(loginDB) de main(), o lo cambia por un
// log.Printf y sigue arrancando, o lo corre después de router.Run(), ningún
// test fallaba — la única evidencia era la prueba en vivo del implementador.
//
// Este test es el caso NEGATIVO pedido por la fase TEST: pinea el control
// contra su ausencia. Inspecciona src/main.go por AST (mismo espíritu que
// TestArchitecture, que inspecciona imports vía go list -json):
//
//  1. Cada postgres.Connect del archivo tiene su AssertNoRLSBypass: conteo de
//     guards == conteo de conexiones (hoy 3: account_app, iam_login,
//     account_migrator — T8n).
//  2. Cada guard aborta: el if que lo llama termina en log.Fatalf, no en un
//     log.Printf que deja arrancar el servicio con la RLS inerte.
//  3. Fail-fast antes de servir tráfico: en main(), todos los guards preceden
//     al router.Run() — un guard posterior al Run serviría requests cross-tenant
//     antes de negarse a arrancar.
package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// guardWiringModuleRoot resuelve la raíz del módulo igual que goListSrc: go test
// corre con cwd = el dir del package (test/arch).
func guardWiringModuleRoot(t *testing.T) string {
	t.Helper()
	gomod, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	root := strings.TrimSpace(string(gomod))
	root = strings.TrimSuffix(root, "/go.mod")
	if root == "" || strings.HasSuffix(root, "/") {
		t.Fatalf("GOMOD sin go.mod: %q", root)
	}
	return root
}

// isCallOnIdent dice si expr es una llamada del paquete identificador `pkg`
// (p. ej. postgres.Connect o sharedpostgres.AssertNoRLSBypass).
func isCallOnIdent(expr ast.Expr, pkg, fn string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == pkg && sel.Sel.Name == fn
}

// guardSite describe un call site del guard en main.go.
type guardSite struct {
	pos   token.Pos
	fatal bool // el if del guard termina en log.Fatal* (aborta el boot)
}

// collectGuardSites recorre los IfStmt del archivo y registra los que llaman
// a AssertNoRLSBypass en su init, con el veredicto de si su body aborta.
func collectGuardSites(file *ast.File) []guardSite {
	var sites []guardSite
	ast.Inspect(file, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		assign, ok := ifStmt.Init.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) == 0 {
			return true
		}
		if !isCallOnIdent(assign.Rhs[0], "sharedpostgres", "AssertNoRLSBypass") {
			return true
		}
		site := guardSite{pos: ifStmt.Pos()}
		for _, stmt := range ifStmt.Body.List {
			exprStmt, ok := stmt.(*ast.ExprStmt)
			if !ok {
				continue
			}
			call, ok := exprStmt.X.(*ast.CallExpr)
			if !ok {
				continue
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "log" &&
					(sel.Sel.Name == "Fatal" || sel.Sel.Name == "Fatalf") {
					site.fatal = true
				}
			}
		}
		sites = append(sites, site)
		return true
	})
	return sites
}

// TestBootGuardWiring pinea el wiring de T8o/T8n: el guard se consulta una
// vez por conexión abierta, aborta el arranque y corre antes de servir.
func TestBootGuardWiring(t *testing.T) {
	root := guardWiringModuleRoot(t)
	mainPath := filepath.Join(root, "src", "main.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, mainPath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", mainPath, err)
	}

	// (1) Conexiones abiertas por el servicio.
	var connects int
	ast.Inspect(file, func(n ast.Node) bool {
		if expr, ok := n.(ast.Expr); ok && isCallOnIdent(expr, "postgres", "Connect") {
			connects++
		}
		return true
	})

	sites := collectGuardSites(file)

	if connects == 0 {
		t.Fatal("no se encontró ningún postgres.Connect en src/main.go — ¿se movió el arranque a otro archivo?")
	}
	if len(sites) != connects {
		t.Errorf("wiring del guard roto: %d conexiones abiertas (postgres.Connect) pero %d "+
			"AssertNoRLSBypass — cada conexión que abre el servicio debe pasar el guard "+
			"(hoy: account_app, iam_login y account_migrator; ACC-E02 T8o/T8n)",
			connects, len(sites))
	}

	// (2) Cada guard aborta el boot: log.Fatal*, no log.Printf y seguir.
	for _, s := range sites {
		if !s.fatal {
			t.Errorf("guard en %s no aborta el arranque: el if de AssertNoRLSBypass debe "+
				"terminar en log.Fatal* — un log.Printf dejaría servir tráfico con la RLS "+
				"inerte (fail-open silencioso que T6 existe para eliminar)",
				fset.Position(s.pos))
		}
	}

	// (3) Fail-fast: en main(), todos los guards preceden al arranque del server.
	var guardPosInMain, runPos token.Pos
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "main" || fn.Body == nil {
			continue
		}
		for _, s := range sites {
			if s.pos >= fn.Body.Pos() && s.pos <= fn.Body.End() {
				if guardPosInMain == token.NoPos || s.pos > guardPosInMain {
					guardPosInMain = s.pos
				}
			}
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Run" {
				ident, ok := sel.X.(*ast.Ident)
				// router.Run(":port") — ignora RunMigrations (Sel distinto).
				if ok && ident.Name == "router" {
					runPos = call.Pos()
					return false
				}
			}
			return true
		})
	}

	if runPos == token.NoPos {
		t.Fatal("no se encontró router.Run en func main() de src/main.go — ¿cómo arranca el server?")
	}
	if guardPosInMain == token.NoPos {
		t.Fatalf("ningún AssertNoRLSBypass corre en func main(): el guard vive en un helper "+
			"y nada pinea que main() se niegue a arrancar antes de %s",
			fset.Position(runPos))
	}
	if guardPosInMain > runPos {
		t.Errorf("los guards corren DESPUÉS de router.Run (%s < %s): el servicio serviría "+
			"requests con la RLS inerte antes de negarse a arrancar. Fail-fast exige guard "+
			"antes de servir (ACC-E02 T8o: el servicio NO arranca)",
			fset.Position(runPos), fset.Position(guardPosInMain))
	}
}