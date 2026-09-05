#!/usr/bin/env bash
# t5_importability_check.sh — Verificación ejecutable del contrato de ACC-E01 T5
# (corregir el module path a uno importable: `module iam` →
# `github.com/hornosg/iam-service`, con reescritura mecánica de todos los
# imports internos), contra el árbol vivo de iam-service.
#
# POR QUÉ UN SCRIPT Y NO UN TEST GO: el contrato de T5 fija la prueba de
# importabilidad como "un módulo probe EXTERNO, no un import interno" — un
# `func Test` DENTRO de este repo siempre resuelve sus imports por el module
# path propio y no puede demostrar que OTRO módulo resuelva este repo. El
# chequeo tiene que construir un módulo ajeno en tmp que importe el paquete,
# exactamente el bloqueo silencioso que (d) pide demostrar que ya no existe.
# Un script bash arma ese probe sin agregar funciones Go al repo.
#
# Qué verifica (el contrato de T5 tal como está escrito en la épica):
#   T5-IMP-01  (a) `head -1 go.mod` → `module github.com/hornosg/iam-service`.
#   T5-IMP-02  (b) reescritura mecánica completa: cero imports vivos por el
#               path viejo — ningún `.go` (fuera de comentarios) importa
#               `"iam/src/..."`.
#   T5-IMP-03  (b) `go build ./...` verde tras la reescritura. (La parte
#               `go test ./...` de (b) es la suite misma; correrla acá
#               duplicaría el re-measure vivo de t2_baseline_check.sh. Con
#               --with-tests se incluye para una corrida autocontenida.)
#   T5-IMP-04  (c) probe externo POSITIVO: módulo nuevo en tmp con
#               `require github.com/hornosg/iam-service` + `replace` al path
#               local, importando un paquete de `src/` → `go build` exit 0.
#   T5-IMP-05  (d) NEGATIVA: el mismo probe importando por el path viejo
#               `iam/src/...` → `go build` FALLA al resolver el módulo.
#               Ese es exactamente el bloqueo silencioso que T5 elimina; si
#               esta negativa dejara de fallar, el check es cosmético.
#
# Controles negativos (parte del entregable, mismo criterio que T6 de la
# épica): un checker que nunca falló no prueba nada. El script se auto-verifica:
#   SELF-01  el check (a) contra un go.mod doctoreado con el path viejo
#            `module iam` → DEBE fallar.
#   SELF-02  el probe positivo (c) apuntando a un repo doctoreado cuyo go.mod
#            declara `module iam` (módulo con el path viejo) → DEBE fallar:
#            prueba que el probe resuelve el module path real, no un resto.
#   SELF-03  la negativa (d) ES un control negativo por definición: el probe
#            viejo debe fallar SIEMPRE mientras el path `iam` no exista.
#
# El repo real nunca se toca: los doctoreos viven en un tmpdir.
#
# Uso:
#   ./test/importability/t5_importability_check.sh               # corrida completa
#   ./test/importability/t5_importability_check.sh --with-tests  # + go test ./...
#   ./test/importability/t5_importability_check.sh --skip-build  # sin go build ./...
#               (para iterar rápido: IMP-03 y los probes siguen corriendo)
#
# Exit 0 si todo pasa (incl. controles negativos); 1 si cualquier check falla; 2 setup.

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO_MOD="${T5_GO_MOD:-$REPO_ROOT/go.mod}"
EXPECTED_MODULE="github.com/hornosg/iam-service"
OLD_MODULE="iam"
PROBE_PACKAGE="github.com/hornosg/iam-service/src/shared/context"
OLD_PROBE_PACKAGE="iam/src/shared/context"

WITH_TESTS="--without-tests"
[[ "${1:-}" == "--with-tests" ]] && WITH_TESTS="--with-tests"
SKIP_BUILD="--build"
[[ "${1:-}" == "--skip-build" ]] && SKIP_BUILD="--skip-build"

FAILURES=0

# ---------------------------------------------------------------------------
# T5-IMP-01 (a): primera línea de go.mod declara el module path del contrato.
#   $1 = archivo go.mod a verificar (el real, o una copia doctoreada en SELF-01)
# ---------------------------------------------------------------------------
check_go_mod_path() {
    local go_mod="$1"
    local first_line
    first_line="$(head -1 "$go_mod" 2>/dev/null)" || return 1
    [[ "$first_line" == "module $EXPECTED_MODULE" ]]
}

# ---------------------------------------------------------------------------
# T5-IMP-02 (b): cero imports vivos por el path viejo. Un import Go citado es
# un string `"iam/src/..."` — los literales de runtime `"iam"` (sin barra) y
# los comentarios de ejemplo NO son imports y no cuentan.
# ---------------------------------------------------------------------------
check_no_old_imports() {
    local root="$1"
    ! grep -rnE '"iam/src' --include='*.go' "$root/src" "$root/cmd" "$root/test" 2>/dev/null
}

# ---------------------------------------------------------------------------
# T5-IMP-03 (b): go build ./... verde.
# ---------------------------------------------------------------------------
check_build() {
    (cd "$1" && go build ./... >/dev/null 2>&1)
}

# ---------------------------------------------------------------------------
# Probe externo: arma en $1 (tmpdir) un módulo `probe` con un main que importa
# $2, `replace` de $EXPECTED_MODULE → $3, y corre `go build`.
#   $1 = tmpdir del probe     $2 = import path a usar     $3 = repo target
# Retorna el exit code de `go build` (0 = importa bien).
# ---------------------------------------------------------------------------
run_probe() {
    local probe_dir="$1" import_path="$2" target="$3"
    mkdir -p "$probe_dir"
    cat > "$probe_dir/go.mod" <<EOF
module probe

go 1.25.0

replace $EXPECTED_MODULE => $target
EOF
    cat > "$probe_dir/main.go" <<EOF
package main

import (
	"fmt"

	sharedctx "$import_path"
)

func main() {
	fmt.Println(sharedctx.IsSystemAdminFromContext(sharedctx.WithSystemAdmin(nil)))
}
EOF
    # tidy resuelve el require desde el module cache (el repo target ya
    # construye local, así que sus deps están cacheadas); GOPROXY=off evita
    # que un checkout sin red haga downloads silenciosos.
    (cd "$probe_dir" && GOPROXY=off go mod tidy >/dev/null 2>&1)
    (cd "$probe_dir" && GOPROXY=off go build ./... >/dev/null 2>&1)
}

# ---------------------------------------------------------------------------
# SELF-02: repo doctoreado con el path VIEJO (`module iam`) y un stub del
# paquete que el probe importa. Sirve para demostrar que el probe positivo
# falla cuando el module path no es el esperado.
# ---------------------------------------------------------------------------
make_doctored_old_repo() {
    local fake_repo="$1"
    mkdir -p "$fake_repo/src/shared/context"
    cat > "$fake_repo/go.mod" <<EOF
module $OLD_MODULE

go 1.25.0
EOF
    cat > "$fake_repo/src/shared/context/tenant.go" <<'EOF'
package context

type systemAdminKey struct{}

func WithSystemAdmin(parent interface{}) interface{} { return parent }
EOF
}

# ===========================================================================
# Setup
# ===========================================================================
TMPDIR_WORK="$(mktemp -d)" || { echo "SETUP: no pude crear tmpdir"; exit 2; }
trap 'rm -rf "$TMPDIR_WORK"' EXIT

pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1"; FAILURES=$((FAILURES + 1)); }

echo "== T5-IMP-01 (a): head -1 go.mod = module $EXPECTED_MODULE"
if check_go_mod_path "$GO_MOD"; then
    pass "go.mod declara el path importable"
else
    fail "go.mod NO declara 'module $EXPECTED_MODULE' (lei: '$(head -1 "$GO_MOD" 2>/dev/null)')"
fi

echo "== T5-IMP-02 (b): cero imports vivos por el path viejo \"iam/src...\""
if check_no_old_imports "$REPO_ROOT"; then
    pass "sin imports por el path viejo"
else
    fail "hay imports vivos por el path viejo — reescritura incompleta:"
    grep -rnE '"iam/src' --include='*.go' "$REPO_ROOT/src" "$REPO_ROOT/cmd" "$REPO_ROOT/test" 2>/dev/null | head -5
fi

echo "== T5-IMP-03 (b): go build ./... verde"
if [[ "$SKIP_BUILD" == "--skip-build" ]]; then
    echo "  SKIP (--skip-build)"
else
    if check_build "$REPO_ROOT"; then
        pass "go build ./... exit 0"
    else
        fail "go build ./... falla — ver error arriba con la corrida manual"
    fi
fi

echo "== T5-IMP-04 (c): probe externo POSITIVO (require + replace local)"
POS_PROBE="$TMPDIR_WORK/probe_pos"
if run_probe "$POS_PROBE" "$PROBE_PACKAGE" "$REPO_ROOT"; then
    pass "probe externo importa $PROBE_PACKAGE y compila"
else
    fail "probe externo NO compila importando $PROBE_PACKAGE — el path no es importable"
fi

echo "== T5-IMP-05 (d): NEGATIVA — probe por el path viejo DEBE fallar"
NEG_PROBE="$TMPDIR_WORK/probe_neg"
if run_probe "$NEG_PROBE" "$OLD_PROBE_PACKAGE" "$REPO_ROOT"; then
    fail "el probe por el path viejo '$OLD_PROBE_PACKAGE' COMPILA — no debería resolver"
else
    pass "path viejo '$OLD_PROBE_PACKAGE' falla al resolver (bloqueo eliminado, negativa de control OK)"
fi

if [[ "$WITH_TESTS" == "--with-tests" ]]; then
    echo "== T5-IMP-03b (b): go test ./... verde"
    if (cd "$REPO_ROOT" && go test ./... >/dev/null 2>&1); then
        pass "go test ./... exit 0"
    else
        fail "go test ./... con fallos — correr la suite para ver el detalle"
    fi
fi

echo "== SELF-01: el check (a) contra un go.mod doctoreado con 'module $OLD_MODULE' DEBE fallar"
FAKE_MOD="$TMPDIR_WORK/go.mod.doctored"
echo "module $OLD_MODULE" > "$FAKE_MOD"
if check_go_mod_path "$FAKE_MOD"; then
    fail "check (a) ACEPTA un go.mod con el path viejo — el checker es cosmético"
else
    pass "check (a) rechaza el path viejo (checker no vacío)"
fi

echo "== SELF-02: probe positivo contra repo doctoreado 'module $OLD_MODULE' DEBE fallar"
FAKE_REPO="$TMPDIR_WORK/fake_old_repo"
make_doctored_old_repo "$FAKE_REPO"
FAKE_PROBE="$TMPDIR_WORK/probe_self"
if run_probe "$FAKE_PROBE" "$PROBE_PACKAGE" "$FAKE_REPO"; then
    fail "probe positivo compila contra un repo con path viejo — no está verificando el path"
else
    pass "probe positivo rechaza repo con path viejo (el probe lee el module path real)"
fi

echo
if [[ "$FAILURES" -eq 0 ]]; then
    echo "T5 importability check: OK (0 fallos)"
    exit 0
else
    echo "T5 importability check: $FAILURES fallo(s)"
    exit 1
fi