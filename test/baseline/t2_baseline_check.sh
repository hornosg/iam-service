#!/usr/bin/env bash
# t2_baseline_check.sh — Verificación ejecutable del contrato de ACC-E01 T2
# (congelar el baseline de tests y cobertura global antes de tocar nada)
# contra el árbol vivo de iam-service y el registro platform/iam-service/test-baseline.json.
#
# POR QUÉ UN SCRIPT Y NO UN TEST GO: el propio contrato de T2 congela
# `grep -rh "^func Test" --include="*_test.go" . | wc -l` → 401. Un `func Test`
# nuevo en este repo movería ese conteo y rompería el criterio (a) de T2
# ("idéntico al func_test_count registrado"). Este check no agrega funciones Go.
#
# Qué verifica (el contrato de T2 tal como está escrito en la épica):
#   T2-BASE-01  test-baseline.json existe, parsea como JSON y declara los TRES
#               campos del contrato: func_test_count (entero > 0),
#               coverage_global_percent (número 0-100) y measured_at (fecha
#               ISO válida, no futura). "Tres campos medidos hoy, no estimados."
#   T2-BASE-02  (a) el conteo canónico vivo == func_test_count registrado.
#   T2-BASE-03  el comando de cobertura registrado lleva `-coverpkg=./src/...`
#               en AMBOS registros (test-baseline.json y coverage-baseline.json,
#               que el contrato cita como fuente del comando) y ambos registran
#               el MISMO comando. Sin ese flag el coverage reporta ~0% porque
#               los tests viven en el árbol externo test/ (ver PLAT-E21: el
#               gate local sin -coverpkg daba un número falso).
#   T2-BASE-04  (b) re-corrida EN VIVO del comando de cobertura registrado →
#               total dentro de ±0,5 del coverage_global_percent registrado.
#
# Controles negativos (parte del entregable, mismo criterio que T6 de la épica):
#   un checker que nunca falló no prueba nada. El script se auto-verifica contra
#   copias doctoreadas de test-baseline.json en un tmpdir: inflar el conteo,
#   inflar la cobertura, borrar measured_at y sacar el -coverpkg del comando —
#   cada mutación DEBE hacer fallar el check. El registro real nunca se toca.
#   Es exactamente el criterio (c) del contrato: "una re-medición viva que NO
#   reproduzca los números del registro falla la tarea — un baseline que no se
#   puede re-medir es decoración".
#
# Uso:
#   ./test/baseline/t2_baseline_check.sh            # corrida completa (incl. go test)
#   ./test/baseline/t2_baseline_check.sh --no-live  # sólos checks estáticos (sin go test)
#
# Exit 0 si todo pasa (incl. controles negativos); 1 si cualquier check falla; 2 setup.

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BASELINE_FILE="${T2_BASELINE_FILE:-$REPO_ROOT/test-baseline.json}"
COVERAGE_BASELINE_FILE="${T2_COVERAGE_BASELINE_FILE:-$REPO_ROOT/coverage-baseline.json}"

LIVE_COVERAGE="--live"
[[ "${1:-}" == "--no-live" ]] && LIVE_COVERAGE="--no-live"

FAILURES=0

# ---------------------------------------------------------------------------
# Núcleo: corre T2-BASE-01..04 para un archivo de baseline dado.
#   $1 = archivo test-baseline.json a verificar
#   $2 = cobertura viva ya medida (número) o "-" para no comparar (sólo BASE-04
#        se saltea; BASE-01..03 corren igual, son baratos)
# Imprime PASS/FAIL por check. Retorna 0 sólo si todos los checks pasan.
# ---------------------------------------------------------------------------
check_baseline() {
    local baseline="$1" live_cov="$2"
    local -i rc=0

    # --- T2-BASE-01: estructura del registro
    if [[ ! -f "$baseline" ]]; then
        echo "  FAIL T2-BASE-01: no existe $baseline"
        return 1
    fi
    if ! jq -e . "$baseline" >/dev/null 2>&1; then
        echo "  FAIL T2-BASE-01: $baseline no parsea como JSON"
        return 1
    fi
    local ftc cgp mad
    ftc="$(jq -r '.func_test_count // empty' "$baseline")"
    cgp="$(jq -r '.coverage_global_percent // empty' "$baseline")"
    mad="$(jq -r '.measured_at // empty' "$baseline")"
    if [[ -z "$ftc" || "$ftc" == "null" ]] || ! [[ "$ftc" =~ ^[0-9]+$ ]] || (( ftc <= 0 )); then
        echo "  FAIL T2-BASE-01: func_test_count ausente o no es un entero > 0: '$ftc'"
        rc=1
    fi
    if [[ -z "$cgp" || "$cgp" == "null" ]] || ! [[ "$cgp" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
        echo "  FAIL T2-BASE-01: coverage_global_percent ausente o no es un número 0-100: '$cgp'"
        rc=1
    fi
    if [[ -z "$mad" || "$mad" == "null" ]] || ! [[ "$mad" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] \
        || ! date -d "$mad" '+%Y-%m-%d' >/dev/null 2>&1; then
        echo "  FAIL T2-BASE-01: measured_at ausente o no es una fecha ISO válida: '$mad'"
        rc=1
    elif [[ "$(date -d "$mad" '+%Y-%m-%d')" > "$(date '+%Y-%m-%d')" ]]; then
        echo "  FAIL T2-BASE-01: measured_at '$mad' es una fecha futura — medido hoy, no estimado"
        rc=1
    fi
    if [[ "$rc" -eq 0 ]]; then
        echo "  PASS T2-BASE-01: tres campos del contrato presentes y bien tipados (count=$ftc cov=$cgp date=$mad)"
    fi

    # --- T2-BASE-02 (criterio a): conteo canónico vivo == registrado
    local -i live_count
    live_count="$(cd "$REPO_ROOT" && grep -rh "^func Test" --include="*_test.go" . | wc -l)"
    if (( live_count != ftc )); then
        echo "  FAIL T2-BASE-02: conteo vivo $live_count ≠ func_test_count registrado $ftc"
        rc=1
    else
        echo "  PASS T2-BASE-02: conteo canónico vivo = $live_count = registrado"
    fi

    # --- T2-BASE-03: el comando registrado lleva -coverpkg=./src/... en ambos registros
    local cmd_tb cmd_cb
    cmd_tb="$(jq -r '.command_coverage_global // empty' "$baseline")"
    cmd_cb="$(jq -r '.command // empty' "$COVERAGE_BASELINE_FILE" 2>/dev/null)"
    if [[ "$cmd_tb" != *"-coverpkg=./src/..."* ]]; then
        echo "  FAIL T2-BASE-03: command_coverage_global de test-baseline.json no lleva -coverpkg=./src/... : '$cmd_tb'"
        rc=1
    elif [[ -z "$cmd_cb" || "$cmd_cb" == "null" ]]; then
        echo "  FAIL T2-BASE-03: no pude leer el comando de $COVERAGE_BASELINE_FILE"
        rc=1
    elif [[ "$cmd_cb" != *"-coverpkg=./src/..."* ]]; then
        echo "  FAIL T2-BASE-03: el comando de coverage-baseline.json (fuente citada por el contrato) no lleva -coverpkg=./src/... : '$cmd_cb'"
        rc=1
    elif [[ "$cmd_tb" != "$cmd_cb" ]]; then
        echo "  FAIL T2-BASE-03: los dos registros no declaran el MISMO comando:"
        echo "    test-baseline.json:     $cmd_tb"
        echo "    coverage-baseline.json: $cmd_cb"
        rc=1
    else
        echo "  PASS T2-BASE-03: comando con -coverpkg=./src/... e idéntico en ambos registros"
    fi

    # --- T2-BASE-04 (criterio b): cobertura viva dentro de ±0,5 del registro
    if [[ "$live_cov" == "-" ]]; then
        echo "  SKIP T2-BASE-04: sin medición viva (--no-live)"
    else
        local delta
        delta="$(awk -v m="$live_cov" -v r="$cgp" 'BEGIN { d = m - r; if (d < 0) d = -d; printf "%.3f", d }')"
        local ok
        ok="$(awk -v d="$delta" 'BEGIN { print (d <= 0.5) ? 1 : 0 }')"
        if [[ "$ok" != "1" ]]; then
            echo "  FAIL T2-BASE-04: cobertura viva $live_cov% difiere en $delta del registrado $cgp% (tolerancia ±0,5)"
            rc=1
        else
            echo "  PASS T2-BASE-04: cobertura viva $live_cov% dentro de ±0,5 del registrado $cgp%"
        fi
    fi

    return "$rc"
}

# ---------------------------------------------------------------------------
# Medición viva: corre el comando de cobertura REGISTRADO (no uno inventado)
# desde la raíz del repo y devuelve el total de `go tool cover -func`.
# El profile se escribe a un tmpdir para no dejar artefactos en el árbol.
# ---------------------------------------------------------------------------
measure_live_coverage() {
    local cmd cov_profile
    cmd="$(jq -r '.command_coverage_global' "$BASELINE_FILE")"
    local tmp
    tmp="$(mktemp -d)"
    # Sustitución mínima del destino del profile (coverage.out → tmpdir) para no
    # ensuciar el árbol — el comando lo referencia dos veces (-coverprofile y
    # -func). El comando en sí (paquetes, flags) va tal como está registrado.
    cov_profile="$tmp/baseline-cov.out"
    (cd "$REPO_ROOT" && eval "${cmd//coverage.out/$cov_profile}") >/dev/null 2>&1
    local rc=$?
    if [[ "$rc" -ne 0 ]]; then
        echo "ERROR: el comando de cobertura registrado falló (exit $rc):" >&2
        echo "  $cmd" >&2
        rm -rf "$tmp"
        return 1
    fi
    go tool cover -func="$cov_profile" | awk '$1 == "total:" { gsub(/%/, "", $3); print $3 }'
    rm -rf "$tmp"
}

# ---------------------------------------------------------------------------
# Controles negativos: mutar una COPIA de test-baseline.json y exigir que el
# check falle. Reutilizan la cobertura viva ya medida (una sola corrida de
# go test) y el conteo vivo (barato).
# ---------------------------------------------------------------------------
negative_controls() {
    local live_cov="$1"
    local tmp rc_all=0
    tmp="$(mktemp -d)"

    # N1 — conteo inflado: 401→402 → T2-BASE-02 debe fallar (re-medición viva
    # que no reproduce el registro). Es el criterio (c) del contrato.
    jq '.func_test_count += 1' "$BASELINE_FILE" > "$tmp/n1.json"
    if check_baseline "$tmp/n1.json" "$live_cov" >/dev/null 2>&1; then
        echo "  FAIL N1: el checker NO detectó un func_test_count inflado"
        rc_all=1
    else
        echo "  PASS N1: func_test_count inflado → checker falla"
    fi

    # N2 — cobertura inflada: 73.2→99.9 → T2-BASE-04 debe fallar (fuera de ±0,5)
    if [[ "$live_cov" == "-" ]]; then
        echo "  SKIP N2: sin medición viva (--no-live)"
    else
        jq '.coverage_global_percent = 99.9' "$BASELINE_FILE" > "$tmp/n2.json"
        if check_baseline "$tmp/n2.json" "$live_cov" >/dev/null 2>&1; then
            echo "  FAIL N2: el checker NO detectó una cobertura fuera de ±0,5"
            rc_all=1
        else
            echo "  PASS N2: cobertura registrada incompatible con la viva → checker falla"
        fi
    fi

    # N3 — campo borrado: sin measured_at → T2-BASE-01 debe fallar
    jq 'del(.measured_at)' "$BASELINE_FILE" > "$tmp/n3.json"
    if check_baseline "$tmp/n3.json" "$live_cov" >/dev/null 2>&1; then
        echo "  FAIL N3: el checker NO detectó la ausencia de measured_at"
        rc_all=1
    else
        echo "  PASS N3: measured_at borrado → checker falla"
    fi

    # N4 — comando ciego: sacar -coverpkg del comando registrado → T2-BASE-03
    # debe fallar. Sin ese flag el coverage reporta ~0% (tests en árbol externo):
    # es exactamente el número falso que PLAT-E21 descubrió en el gate local.
    jq -r '.command_coverage_global |= gsub("-coverpkg=./src/... "; "")' "$BASELINE_FILE" > "$tmp/n4.json"
    if check_baseline "$tmp/n4.json" "$live_cov" >/dev/null 2>&1; then
        echo "  FAIL N4: el checker NO detectó un comando de cobertura sin -coverpkg"
        rc_all=1
    else
        echo "  PASS N4: comando sin -coverpkg=./src/... → checker falla"
    fi

    rm -rf "$tmp"
    return "$rc_all"
}

# ---------------------------------------------------------------------------
main() {
    if ! command -v jq >/dev/null 2>&1; then
        echo "t2_baseline_check: falta jq" >&2
        exit 2
    fi
    if ! command -v go >/dev/null 2>&1; then
        echo "t2_baseline_check: falta go" >&2
        exit 2
    fi
    if [[ ! -f "$BASELINE_FILE" ]]; then
        echo "t2_baseline_check: no encuentro el baseline: $BASELINE_FILE" >&2
        exit 2
    fi

    echo "== ACC-E01.T2 — baseline de tests y cobertura vs árbol vivo =="
    echo "baseline: $BASELINE_FILE"

    local live_cov="-"
    if [[ "$LIVE_COVERAGE" == "--live" ]]; then
        live_cov="$(measure_live_coverage)"
        if [[ -z "$live_cov" ]]; then
            echo "RESULTADO: FAIL — no pude medir la cobertura viva con el comando registrado"
            exit 1
        fi
        echo "cobertura viva re-medida: ${live_cov}%"
    fi

    local -i rc_base=0 rc_neg=0
    check_baseline "$BASELINE_FILE" "$live_cov" || rc_base=1

    echo ""
    echo "== Controles negativos (el checker debe fallar ante cada mutación) =="
    negative_controls "$live_cov" || rc_neg=1

    echo ""
    if [[ "$rc_base" -ne 0 ]]; then
        echo "RESULTADO: FAIL — el baseline NO satisface el contrato de T2 tal como está escrito"
        exit 1
    fi
    if [[ "$rc_neg" -ne 0 ]]; then
        echo "RESULTADO: FAIL — el checker no demuestra sus negativas"
        exit 1
    fi
    echo "RESULTADO: PASS — el baseline satisface el contrato de T2 y el checker demuestra sus negativas"
    exit 0
}

main