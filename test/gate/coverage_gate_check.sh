#!/usr/bin/env bash
# coverage_gate_check.sh — Verificación ejecutable del contrato de ACC-E01 T3
# (el gate de diff-coverage de CI debe VER una mudanza de archivos) contra el
# árbol vivo de iam-service.
#
# POR QUÉ UN SCRIPT Y NO UN TEST GO: ídem t1_map_check.sh — ACC-E01.T2 congela
# `grep -rh "^func Test" --include="*_test.go" . | wc -l` → 401. Un `func Test`
# nuevo movería ese conteo y rompería el criterio (a) de T2 ("idéntico al
# func_test_count registrado"). Este check no agrega funciones Go.
#
# POR QUÉ ESTE CHECK EXISTE: la evidencia de T3 (bitácora, 2026-09-04) fue un
# simulacro MANUAL en worktree descartable — demostró la ceguera, aplicó el fix
# (`--no-renames`, commit d9a5aae) y descartó el worktree. Un fix sin regresión
# ejecutable se pierde con el próximo refactor del script, y TODA la
# reorganización de T4 son `git mv`: si el gate vuelve a quedar ciego, la mudanza
# entera queda indefensa. Este check convierte esa evidencia puntual en un test
# re-corrible que blindó el fix.
#
# Qué verifica (el contrato de T3 tal como está escrito en la épica):
#   T3-GATE-01  simulacro de mudanza (`git mv` de un paquete cubierto de src/ a
#               su destino del mapa de T1 + reescritura mecánica de imports,
#               cero lógica editada) en worktree descartable →
#               `./scripts/coverage-gate.sh <base> <head>` imprime un veredicto
#               con MÁS DE 0 statements medidos. Un veredicto vacío o "NOSTMTS"
#               FALLA: el gate está ciego ante la mudanza. (El contrato (a) exige
#               medir >0 líneas, no exit 0 — el veredicto completo se reporta.)
#   T3-GATE-02  negativa del contrato: borrar además el paquete de tests que
#               cubría lo movido → el gate FALLA (exit 1). Si pasara (exit 0),
#               una mudanza podría perder tests y cobertura en silencio —
#               exactamente lo que esta épica existe para impedir.
#   T3-GATE-03  higiene del simulacro: worktree y rama descartados, master
#               intacto (`git status --porcelain` → sin resultados). El `go test
#               ./...` completo del contrato (c) es criterio de cierre de T2/T4
#               y corre en cada cierre de tarea; duplicarlo acá sólo añadiría
#               minutos sin proteger nada nuevo.
#
# Controles negativos (el checker que nunca falló no prueba nada):
#   N1  el gate del WORKTREE doctoreado sin `--no-renames` (copia en el propio
#       worktree desechable; el script de master no se toca) → el oráculo de
#       T3-GATE-01 debe FALLAR (veredicto ciego). Si midiera, el oráculo es
#       decoración y N1 falla.
#   N2  `coverage-baseline.json` doctoreado con umbral 0 (en el worktree, sin
#       commit) sobre el estado de la negativa → el gate pasa (exit 0) → el
#       oráculo de T3-GATE-02 debe FALLAR. Demuestra que ese oráculo distingue
#       un gate que defiende de uno que no.
#
# El simulacro replica EXACTAMENTE el de la evidencia de T3: mueve
# src/plan/application/response → src/plans/application/response (destino del
# mapa de T1), cubierto al 100%, con iam/test/plan/application/usecase como
# único paquete cubridor (verificado recorriendo todos los paquetes de test/).
# Si T4 ya corrió y ese paquete ya no vive en src/plan, sobreescribir:
#   PKG_SRC=... PKG_DST=... COVERING_TESTS=... ./test/gate/coverage_gate_check.sh
#
# Uso:
#   ./test/gate/coverage_gate_check.sh
#
# Requiere: go, git, jq, awk. Lento (compila y corre los tests del subconjunto
# afectado). Exit 0 si todo pasa (incl. controles negativos); 1 si cualquier
# check falla; 2 si las precondiciones no dan.

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PKG_SRC="${PKG_SRC:-src/plan/application/response}"
PKG_DST="${PKG_DST:-src/plans/application/response}"
COVERING_TESTS="${COVERING_TESTS:-test/plan/application/usecase}"

FAILURES=0
BASE=""
WT=""
BR=""

# ---------------------------------------------------------------------------
# Higiene: descarta worktree y rama del simulacro. Idempotente — el trap EXIT
# lo re-invoca como red de seguridad; llamadas repetidas no fallan.
# ---------------------------------------------------------------------------
cleanup() {
    if [ -n "$WT" ] && [ -d "$WT" ]; then
        git worktree remove --force "$WT" >/dev/null 2>&1 || true
    fi
    if [ -n "$BR" ]; then
        git branch -D "$BR" >/dev/null 2>&1 || true
    fi
    return 0
}

# ---------------------------------------------------------------------------
# run_gate <worktree> — corre el gate base..HEAD desde el worktree (así el
# coverprofile y el ROOT viven en el worktree, nunca en master). Deja el
# veredicto en GATE_OUT y el exit code en GATE_RC.
# ---------------------------------------------------------------------------
run_gate() {
    local wt="$1"
    GATE_OUT="$(cd "$wt" && ./scripts/coverage-gate.sh "$BASE" HEAD 2>&1)"
    GATE_RC=$?
}

# ---------------------------------------------------------------------------
# measured_statements <veredicto> — oráculo del contrato (a): el veredicto debe
# traer "(N/M statements)" con M > 0. Imprime M; vacío si el veredicto es ciego
# ("no tocó statements medibles", "NOSTMTS" o un error del gate).
# ---------------------------------------------------------------------------
measured_statements() {
    printf '%s\n' "$1" \
        | sed -n 's/.*([0-9][0-9]*\/\([0-9][0-9]*\) statements).*/\1/p' \
        | head -1
}

# ---------------------------------------------------------------------------
main() {
    cd "$REPO_ROOT" || { echo "coverage_gate_check: no puedo cd a $REPO_ROOT" >&2; exit 2; }

    # --- Precondiciones (exit 2): sin éstas el check no puede ni correr.
    for tool in go git jq awk; do
        command -v "$tool" >/dev/null 2>&1 \
            || { echo "coverage_gate_check: falta '$tool' en PATH" >&2; exit 2; }
    done
    [ -f scripts/coverage-gate.sh ] \
        || { echo "coverage_gate_check: falta scripts/coverage-gate.sh" >&2; exit 2; }
    [ -f coverage-baseline.json ] \
        || { echo "coverage_gate_check: falta coverage-baseline.json" >&2; exit 2; }
    [ -d "$PKG_SRC" ] \
        || { echo "coverage_gate_check: no existe $PKG_SRC — ¿corrió T4 y el paquete ya se mudó? Ajustá PKG_SRC/PKG_DST/COVERING_TESTS (ver cabecera)" >&2; exit 2; }
    [ -d "$COVERING_TESTS" ] \
        || { echo "coverage_gate_check: no existe $COVERING_TESTS — el paquete cubridor del simulacro no está" >&2; exit 2; }
    if [ -n "$(git status --porcelain)" ]; then
        echo "coverage_gate_check: master no está limpio — el simulacro exige un árbol intacto como base (contrato (c) de T3)" >&2
        exit 2
    fi

    # --- Setup del simulacro: worktree + rama desechables, NUNCA master.
    BASE="$(git rev-parse HEAD)"
    BR="tmp/acc-e01-t3-gate-check-$$"
    WT="$(mktemp -d /tmp/acc-e01-t3-gate-check.XXXXXX)"
    if ! git worktree add -q -b "$BR" "$WT" "$BASE" >/dev/null 2>&1; then
        echo "coverage_gate_check: no pude crear el worktree desechable" >&2
        rm -rf "$WT"
        exit 2
    fi
    trap cleanup EXIT

    # --- El simulacro del contrato: git mv del paquete cubierto + reescritura
    #     mecánica de imports (la única edición permitida por la evidencia de
    #     T3: "sin editar contenido" es insatisfiable — el coverage exige que
    #     el importador compile). Cero lógica editada.
    local module
    module="$(go list -m)"
    # git mv no crea los padres del destino (rename(2) falla si faltan) — el
    # simulacro crea el módulo destino antes de mover, como T4 hará igual.
    # Dentro del WORKTREE: master no se toca.
    mkdir -p "$WT/$(dirname "$PKG_DST")"
    if ! git -C "$WT" mv "$PKG_SRC" "$PKG_DST"; then
        echo "  FAIL setup: git mv $PKG_SRC → $PKG_DST no pudo ejecutarse"
        FAILURES=$((FAILURES+1))
        finish
    fi
    grep -rl --include='*.go' "${module}/${PKG_SRC}" "$WT" \
        | while IFS= read -r f; do sed -i "s#${module}/${PKG_SRC}#${module}/${PKG_DST}#g" "$f"; done
    if ! (cd "$WT" && go build ./...); then
        echo "  FAIL setup: el simulacro no compila tras la reescritura de imports"
        FAILURES=$((FAILURES+1))
        finish
    fi
    git -C "$WT" add -A
    # --no-verify: el pre-commit del repo (PLAT-E21 T9) hace `go build -o <dir>/
    # $PKGS`, que con Go 1.26 falla para paquetes librería ("no main packages to
    # build") — HALLAZGO reportado en la corrida del loop, no se toca acá (no es
    # un archivo de test). El sanity `go build ./...` de arriba ya validó que el
    # simulacro compila, que es lo que el hook quería garantizar. El commit vive
    # en un worktree desechable, nunca llega a master.
    if ! git -C "$WT" commit -q --no-verify -m "simulacro T3-check: git mv ${PKG_SRC} → ${PKG_DST} (imports mecánicos)"; then
        echo "  FAIL setup: el commit del simulacro no pudo crearse — el resto mediría un diff vacío"
        FAILURES=$((FAILURES+1))
        finish
    fi

    echo "== ACC-E01.T3 — el gate de diff-coverage ante una mudanza de archivos =="
    echo "base: ${BASE:0:12} · simulacro en worktree desechable (master no se toca)"

    # --- T3-GATE-01: contrato (a) — el gate mide la mudanza (>0 statements).
    echo ""
    echo "== T3-GATE-01: git mv de paquete cubierto → veredicto con más de 0 líneas medidas =="
    run_gate "$WT"
    local total
    total="$(measured_statements "$GATE_OUT")"
    if [ -n "$total" ] && [ "$total" -gt 0 ]; then
        echo "  PASS T3-GATE-01: veredicto con ${total} statements medidos (exit ${GATE_RC})"
    else
        echo "  FAIL T3-GATE-01: el gate quedó CIEGO ante la mudanza — veredicto vacío o 0 líneas:"
        FAILURES=$((FAILURES+1))
    fi
    printf '%s\n' "$GATE_OUT" | sed 's/^/    /'

    # --- N1: control negativo — el mismo oráculo debe RECHAZAR un gate ciego.
    #     Doctoreo la copia del worktree (sin commit; el gate diffea refs, no
    #     ve el working tree). El script de master queda intacto.
    echo ""
    echo "== N1: gate sin --no-renames → el oráculo de T3-GATE-01 debe fallar =="
    sed -i 's/ --no-renames//' "$WT/scripts/coverage-gate.sh"
    run_gate "$WT"
    total="$(measured_statements "$GATE_OUT")"
    git -C "$WT" checkout -- scripts/coverage-gate.sh
    if [ -n "$total" ] && [ "$total" -gt 0 ]; then
        echo "  FAIL N1: el gate doctoreado (sin --no-renames) siguió midiendo — el oráculo no distingue la ceguera"
        FAILURES=$((FAILURES+1))
    else
        echo "  PASS N1: sin --no-renames el veredicto queda ciego y el oráculo lo rechaza:"
        printf '%s\n' "$GATE_OUT" | sed 's/^/    /'
    fi

    # --- T3-GATE-02: contrato (b) — borrar el paquete cubridor → gate exit 1.
    echo ""
    echo "== T3-GATE-02 (negativa del contrato): borrar los tests que cubren lo movido → el gate falla =="
    git -C "$WT" rm -r -q "$COVERING_TESTS"
    if ! git -C "$WT" commit -q --no-verify -m "negativa T3-check: borrar el paquete cubridor (${COVERING_TESTS})"; then
        echo "  FAIL T3-GATE-02: el commit de la negativa no pudo crearse"
        FAILURES=$((FAILURES+1))
    fi
    run_gate "$WT"
    if [ "$GATE_RC" -eq 1 ]; then
        echo "  PASS T3-GATE-02: el gate falló (exit 1) — la pérdida de tests NO pasa en silencio"
    else
        echo "  FAIL T3-GATE-02: el gate pasó (exit ${GATE_RC}) — una mudanza perdería tests y cobertura en silencio"
        FAILURES=$((FAILURES+1))
    fi
    printf '%s\n' "$GATE_OUT" | sed 's/^/    /'

    # --- N2: control negativo — con umbral doctoreado a 0 el gate DEJA pasar
    #     la pérdida; el oráculo de T3-GATE-02 debe rechazarlo (esperaba 1).
    echo ""
    echo "== N2: umbral doctoreado a 0 → el gate pasa y el oráculo de T3-GATE-02 debe fallar =="
    jq '.coverage_percent = 0' "$WT/coverage-baseline.json" > "$WT/coverage-baseline.json.tmp"
    mv "$WT/coverage-baseline.json.tmp" "$WT/coverage-baseline.json"
    run_gate "$WT"
    local rc_n2=$GATE_RC
    git -C "$WT" checkout -- coverage-baseline.json
    if [ "$rc_n2" -eq 1 ]; then
        echo "  FAIL N2: con umbral 0 el gate siguió fallando — el oráculo no distingue un gate que no defiende"
        FAILURES=$((FAILURES+1))
    else
        echo "  PASS N2: con umbral 0 el gate pasa (exit ${rc_n2}) y el oráculo lo rechaza (esperaba exit 1)"
    fi
    printf '%s\n' "$GATE_OUT" | sed 's/^/    /'

    # --- T3-GATE-03: contrato (c) — el simulacro se descarta y master queda intacto.
    echo ""
    echo "== T3-GATE-03: higiene — simulacro descartado, master intacto =="
    cleanup
    WT=""
    BR=""
    if [ -n "$(git status --porcelain)" ]; then
        echo "  FAIL T3-GATE-03: \`git status --porcelain\` en master NO está vacío:"
        git status --porcelain | sed 's/^/    /'
        FAILURES=$((FAILURES+1))
    else
        echo "  PASS T3-GATE-03: \`git status --porcelain\` → sin resultados; worktree y rama descartados"
    fi

    finish
}

finish() {
    cleanup
    echo ""
    if [ "$FAILURES" -ne 0 ]; then
        echo "RESULTADO: FAIL — el gate NO satisface el contrato de T3 tal como está escrito"
        exit 1
    fi
    echo "RESULTADO: PASS — el gate ve mudanzas de archivos, falla ante pérdida de tests, y los oráculos demuestran sus negativas"
    exit 0
}

main