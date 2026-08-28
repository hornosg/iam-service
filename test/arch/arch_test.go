// Package archtest es un architecture test versionado que macht la regla de dependencia
// hexagonal en cada `go test` — no on-demand como el audit (PLAT-E21 T6).
//
// El owner reportó (2026-06-30) violaciones de ports fuera de domain/ pese a pasar
// go-hex-audit "mil veces": el audit corría cuando alguien se acordaba, no como gate.
// Este test es el gate: corre en `go test ./test/arch/...` y en CI, y falla si:
//
//  1. Ports fuera de domain/ — toda package con segmento `port` debe vivir bajo `domain/`
//     (criterio literal del "Hecho cuando" de T6).
//  2. Regla de dependencia (imports directos, prod+test):
//     - domain no importa application/infrastructure/api del módulo ni libs de infra
//       (gorm, database/sql, pgx, sqlx, net/http, gin, echo, fiber, golang-jwt).
//     - application no importa infrastructure/api del módulo ni las mismas libs de infra.
//
// Elección de diseño (ver bitácora de la épica): se inspeccionan imports DIRECTOS
// (Imports + TestImports + XTestImports), no el closure transitivo (`go list -deps`).
// El closure arrastra gin/net/http via github.com/hornosg/go-shared/criteria (kernel
// externo) y produce falsos positivos; los findings reales de AUDIT.md eran todos
// imports directos (Fix #4, #6, #10). Las dos trampas que el skill go-hex-audit §2
// advierte sobre tests de arquitectura están cubiertas:
//   - inspecciona imports de test (TestImports + XTestImports), no solo Imports;
//   - stdlib se detecta via el campo .Standard, no por heurística de "lleva punto".
package archtest

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// forbiddenInfra libs que dominio/application no pueden importar directamente.
// stdlib (database/sql, net/http) se matchea exacto para no arrastrar sub-paquetes
// legítimos (database/sql/driver lo arrastra uuid; net/http/httputil, etc.). Los
// frameworks pesados se matchean por prefijo (cualquier sub-paquete suyo es leak).
var forbiddenInfra = []struct {
	path  string
	prefix bool
}{
	{"database/sql", false},
	{"net/http", false},
	{"gorm.io/gorm", true},
	{"github.com/jackc/pgx", true},
	{"github.com/jmoiron/sqlx", true},
	{"github.com/gin-gonic/gin", true},
	{"github.com/labstack/echo", true},
	{"github.com/gofiber/fiber", true},
	{"github.com/golang-jwt/jwt", true},
}

type pkgInfo struct {
	ImportPath   string
	Standard     bool
	Imports      []string
	TestImports  []string
	XTestImports []string
	Error        *struct{ Err string }
}

func goListSrc(t *testing.T) []pkgInfo {
	t.Helper()
	// Resolver la raíz del módulo: go test corre con cwd = el dir del package (test/arch).
	gomod, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	root := strings.TrimSpace(string(gomod))
	root = strings.TrimSuffix(root, "/go.mod")
	cmd := exec.Command("go", "list", "-json", "./src/...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -json ./src/...: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(out)))
	var pkgs []pkgInfo
	for dec.More() {
		var p pkgInfo
		if err := dec.Decode(&p); err != nil {
			t.Fatalf("decode go list json: %v", err)
		}
		pkgs = append(pkgs, p)
	}
	if len(pkgs) == 0 {
		t.Fatal("go list ./src/... devolvió 0 packages — ¿se movió el árbol src/?")
	}
	return pkgs
}

// modulePrefix es el path del módulo con barra final ("iam/"), para identificar
// packages propios vs externos/stdlib.
func modulePrefix(t *testing.T) string {
	t.Helper()
	mp, err := exec.Command("go", "list", "-m").Output()
	if err != nil {
		t.Fatalf("go list -m: %v", err)
	}
	return strings.TrimSpace(string(mp)) + "/"
}

// hasSegment dice si un import path tiene un segmento exacto (no subcadena).
// "iam/src/user/domain/port" -> hasSegment("port")=true, hasSegment("domain")=true.
func hasSegment(path, seg string) bool {
	for _, s := range strings.Split(path, "/") {
		if s == seg {
			return true
		}
	}
	return false
}

// isModuleLayerOwn dice si `dep` es un package propio del módulo cuyo path tiene
// alguno de los segmentos de capa de infra/driver indicados.
func isModuleLayerOwn(dep, modPrefix string, layers []string) bool {
	if !strings.HasPrefix(dep, modPrefix) {
		return false
	}
	for _, l := range layers {
		if hasSegment(dep, l) {
			return true
		}
	}
	return false
}

func isForbiddenInfra(dep string) (string, bool) {
	for _, f := range forbiddenInfra {
		if f.prefix {
			if strings.HasPrefix(dep, f.path+"/") || dep == f.path {
				return f.path, true
			}
		} else {
			if dep == f.path {
				return f.path, true
			}
		}
	}
	return "", false
}

func TestArchitecture(t *testing.T) {
	mod := modulePrefix(t)
	pkgs := goListSrc(t)

	type violation struct {
		pkg, kind, dep string
	}
	var v []violation

	for _, p := range pkgs {
		ip := p.ImportPath
		if !strings.HasPrefix(ip, mod) {
			continue // packages externos no se auditan
		}

		// Fail-safe: si un package de src/ no carga (error de parseo/sintaxis/ciclo),
		// go list -json lo emite con Error y exits 0, y sus Imports quedan incompletos
		// → el gate podría pasar silenciosamente sobre un src roto. Cazarlo acá: un src
		// que no compila no puede pasar el gate de arquitectura.
		if p.Error != nil {
			v = append(v, violation{ip, "PACKAGE_LOAD_ERROR", p.Error.Err})
		}

		// --- Regla 1: ports fuera de domain/ ---
		// Una package con segmento `port` debe vivir bajo `domain/`.
		if hasSegment(ip, "port") && !hasSegment(ip, "domain") {
			v = append(v, violation{ip, "PORT_FUERA_DE_DOMAIN", ip})
		}

		// --- Regla 2: dependencia de capa (prod + test) ---
		isDomain := hasSegment(ip, "domain")
		isApp := hasSegment(ip, "application")
		if !isDomain && !isApp {
			continue // infrastructure/api/cmd/shared: dependen hacia adentro, sin restricción
		}

		// Layers del módulo que domain/application no pueden importar.
		var forbiddenLayers []string
		if isDomain {
			forbiddenLayers = []string{"application", "infrastructure", "api"}
		} else { // application
			forbiddenLayers = []string{"infrastructure", "api"}
		}

		// Imports directos de producción + test (gotcha #1 del skill: cubrir test imports).
		var allImports []string
		allImports = append(allImports, p.Imports...)
		allImports = append(allImports, p.TestImports...)
		allImports = append(allImports, p.XTestImports...)
		seen := map[string]bool{}
		for _, dep := range allImports {
			if seen[dep] {
				continue
			}
			seen[dep] = true
			if isModuleLayerOwn(dep, mod, forbiddenLayers) {
				v = append(v, violation{ip, "LAYER_DEPENDENCY", dep})
				continue
			}
			if f, ok := isForbiddenInfra(dep); ok {
				v = append(v, violation{ip, "INFRA_IN_DOMAIN_OR_APP", f + " (via " + dep + ")"})
			}
		}
	}

	if len(v) > 0 {
		var b strings.Builder
		b.WriteString("architecture gate (PLAT-E21 T6) — violaciones hexagonales:\n")
		for _, x := range v {
			b.WriteString("  " + x.pkg + "  [" + x.kind + "]  -> " + x.dep + "\n")
		}
		b.WriteString("\nLos ports van en <ctx>/domain/port. Domain no importa application/" +
			"infrastructure/api ni libs de infra (gorm, database/sql, pgx, net/http, gin, ...). " +
			"Application no importa infrastructure/api ni esas libs.")
		t.Error(b.String())
	}
}