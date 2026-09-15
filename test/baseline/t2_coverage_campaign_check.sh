#!/usr/bin/env bash
# t2_coverage_campaign_check.sh — Verificación ejecutable del contrato de ACC-E08.T2
# (cubrir src/identity/infrastructure/persistence/repository: la brecha más grande del mapa,
# 23,9% → ≥ 80%) contra el árbol vivo de iam-service y el registro
# platform/iam-service/total-coverage-baseline.json (el punto de partida que T1 congeló).
#
# POR QUÉ UN SCRIPT Y NO UN TEST GO: mismo criterio que t1_total_baseline_check.sh (ACC-E08.T1).
# El contrato de T2 se verifica con el comando canónico COMPLETO (go test -coverpkg ./src/...
# sobre ./test/... + go tool cover + la deduplicación por bloque del mapa de brechas de la
# épica) y con greps sobre los archivos nuevos de la tarea — eso es instrumentación de
# medición, no lógica de dominio. Además, un func Test nuevo movería el conteo que otros
# checkers del árbol cruzan contra sus baselines. El patrón bash auto-verificado es el ya
# establecido (t2_baseline_check.sh, t1_total_baseline_check.sh, coverage_gate_check.sh) y el
# runner de calidad lo ejecuta igual.
#
# Qué verifica (el contrato de T2 tal como está escrito en la épica):
#   T2-CAMP-01  (a) re-medición con el comando canónico registrado + awk de deduplicación
#               del mapa de brechas → el paquete identity/infrastructure/persistence/repository
#               está en la medición y queda ≥ 80,0%. El denominador se REPORTA contra el
#               registrado (T1: 284): si creció, entró código nuevo bajo la campaña y el
#               resto de los porcentajes del mapa ya no valen (epic: "manda la medición viva")
#               — es NOTE, no FAIL: el contrato de (a) es el número vivo.
#   T2-CAMP-02  (b) total global deduplicado ≥ coverage_total_percent del baseline (61,2):
#               la campaña sólo sube. Cae por debajo → alguien perdió tests o cubrimiento.
#   T2-CAMP-03  (c) GOWORK=off go vet ./... limpio (exit 0) y GOWORK=off go test ./... verde
#               (exit 0) con los tests nuevos incluidos.
#   T2-CAMP-04  (d) NEGATIVA ESTRUCTURAL: los archivos de test NUEVOS de esta tarea (git
#               status sobre test/identity/) NO llevan go:build — un test taggeado no corre
#               en el canónico y no mueve el número (lección estructural de test/rls, cierre
#               de ACC-E02 T8).
#   T2-CAMP-05  OBJETIVO (patrón): los archivos nuevos viven en el árbol externo test/,
#               extienden el harness fakedb ya existente (import test/testsupport/fakedb) y
#               NO introducen testcontainers/Docker — el riesgo de la épica lo prohíbe
#               expresamente: cambiaría la naturaleza del comando que mide el piso de T8.
#
# Controles negativos (parte del entregable: un checker que nunca falló no prueba nada).
# El script se auto-verifica con valores y archivos doctoreados en un tmpdir:
#   N1  paquete objetivo inyectado a 79,9% → T2-CAMP-01 debe fallar (está por debajo del piso)
#   N2  total inyectado por debajo del baseline → T2-CAMP-02 debe fallar (la campaña sólo sube)
#   N3  archivo con '//go:build integration' fabricado → T2-CAMP-04 debe fallar
#   N4  archivo de test sin fakedb (y con testcontainers) fabricado → T2-CAMP-05 debe fallar
# El registro real y los archivos de la tarea nunca se tocan.
#
# Uso:
#   ./test/baseline/t2_coverage_campaign_check.sh             # corrida completa (incl. go test + cobertura)
#   ./test/baseline/t2_coverage_campaign_check.sh --no-live   # sólo checks estáticos de archivos (04, 05)
#
# Exit 0 si todo pasa (incl. negativos); 1 si cualquier check falla; 2 setup.

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BASELINE_FILE="${T2_TOTAL_BASELINE_FILE:-$REPO_ROOT/total-coverage-baseline.json}"
TARGET_PKG_SUFFIX="src/identity/infrastructure/persistence/repository"
FLOOR="80.0"

LIVE="--live"
[[ "${1:-}" == "--no-live" ]] && LIVE="--no-live"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

# ---------------------------------------------------------------------------
# Por-paquete desde un coverprofile — la fórmula de deduplicación del mapa de
# brechas de la épica (el profile tiene bloques duplicados, uno por binario de
# test que los instrumentó; un bloque cuenta cubierto si ALGÚN binario lo
# ejecutó). Emite:  covered<TAB>total<TAB>package
# ---------------------------------------------------------------------------
per_package_from_profile() {
    local profile="$1"
    awk 'NR>1 {if (!($1 in s)) {s[$1]; split($1,b,":"); sub(/\/[^\/]*\.go$/,"",b[1]);
      tot[b[1]]+=$2; ns[$1]=$2} if ($3>0) covb[$1]=1}
END {for (x in covb) {split(x,b,":"); sub(/\/[^\/]*\.go$/,"",b[1]); cov[b[1]]+=ns[x]}
for (q in tot) printf "%d\t%d\t%s\n", cov[q]+0, tot[q], q}' "$profile" | sort -t$'\t' -k3
}

# ---------------------------------------------------------------------------
# T2-CAMP-01 (criterio a): el paquete objetivo en la medición deduplicada.
#   $1 = "covered<TAB>total" del paquete, o "" si no apareció
# Imprime PASS/FAIL. Retorna 0 sólo si el paquete existe y ≥ FLOOR.
# ---------------------------------------------------------------------------
check_target_pkg() {
    local pkg_live="$1"
    if [[ -z "$pkg_live" ]]; then
        if [[ "$LIVE" == "--no-live" ]]; then
            echo "  SKIP T2-CAMP-01: sin medición viva (--no-live)"
            return 0
        fi
        echo "  FAIL T2-CAMP-01: $TARGET_PKG_SUFFIX no apareció en la medición — el árbol o el comando cambiaron"
        return 1
    fi
    local l_cov="${pkg_live%%$'\t'*}" l_tot="${pkg_live##*$'\t'}"
    local pct
    pct="$(awk -v c="$l_cov" -v t="$l_tot" 'BEGIN { printf "%.2f", 100 * c / t }')"
    if awk -v p="$pct" -v f="$FLOOR" 'BEGIN { exit !(p < f) }'; then
        echo "  FAIL T2-CAMP-01: $TARGET_PKG_SUFFIX en $pct% ($l_cov/$l_tot) < $FLOOR% del piso del contrato (a)"
        return 1
    fi
    local reg_tot
    reg_tot="$(jq -r --arg suf "$TARGET_PKG_SUFFIX" \
        '.per_package[] | select(.package | endswith($suf)) | .total // empty' "$BASELINE_FILE" 2>/dev/null)"
    local note=""
    if [[ -n "$reg_tot" && "$reg_tot" != "$l_tot" ]]; then
        note="  NOTE T2-CAMP-01: el denominador del paquete cambió contra el registro de T1 ($reg_tot → $l_tot) — entró código nuevo bajo la campaña; re-medir el resto del mapa antes de las tareas siguientes"
    fi
    echo "  PASS T2-CAMP-01: $TARGET_PKG_SUFFIX en $pct% ($l_cov/$l_tot) ≥ $FLOOR%"
    [[ -n "$note" ]] && echo "$note"
    return 0
}

# ---------------------------------------------------------------------------
# T2-CAMP-02 (criterio b): total vivo ≥ coverage_total_percent del baseline.
#   $1 = "covered<TAB>total" del total vivo, o "" para no comparar
# ---------------------------------------------------------------------------
check_total_monotonic() {
    local live="$1"
    if [[ -z "$live" ]]; then
        echo "  SKIP T2-CAMP-02: sin medición viva (--no-live)"
        return 0
    fi
    local baseline_pct
    baseline_pct="$(jq -r '.coverage_total_percent // empty' "$BASELINE_FILE" 2>/dev/null)"
    if [[ -z "$baseline_pct" || "$baseline_pct" == "null" ]]; then
        echo "  FAIL T2-CAMP-02: coverage_total_percent ausente en $BASELINE_FILE — sin referencia, la monotonicidad no se puede verificar"
        return 1
    fi
    local l_cov="${live%%$'\t'*}" l_tot="${live##*$'\t'}"
    local pct
    pct="$(awk -v c="$l_cov" -v t="$l_tot" 'BEGIN { printf "%.2f", 100 * c / t }')"
    if awk -v m="$pct" -v r="$baseline_pct" 'BEGIN { exit !(m < r) }'; then
        echo "  FAIL T2-CAMP-02: total vivo $pct% ($l_cov/$l_tot) < baseline registrado $baseline_pct% — la campaña retrocedió (el contrato pide: sólo sube)"
        return 1
    fi
    echo "  PASS T2-CAMP-02: total vivo $pct% ($l_cov/$l_tot) ≥ baseline $baseline_pct% (la campaña sólo sube)"
    return 0
}

# ---------------------------------------------------------------------------
# T2-CAMP-03 (criterio c): go vet + go test ./... — se corren UNA vez en main()
# (--live); acá sólo se reporta el resultado ya medido.
# ---------------------------------------------------------------------------
report_vet_and_test() {
    if [[ "$LIVE" == "--no-live" ]]; then
        echo "  SKIP T2-CAMP-03: vet+suite completos corren sólo en la pasada --live"
        return 0
    fi
    if [[ "$VET_RC" -ne 0 ]]; then
        echo "  FAIL T2-CAMP-03: GOWORK=off go vet ./... no está limpio (exit $VET_RC)"
        return 1
    fi
    if [[ "$GO_TEST_RC" -ne 0 ]]; then
        echo "  FAIL T2-CAMP-03: GOWORK=off go test ./... no está verde con los tests nuevos incluidos (exit $GO_TEST_RC)"
        return 1
    fi
    echo "  PASS T2-CAMP-03: go vet limpio y go test ./... verde (0 FAIL)"
    return 0
}

# ---------------------------------------------------------------------------
# T2-CAMP-04 (criterio d): un archivo de test NUEVO de la tarea no lleva go:build.
#   $1 = path del archivo
# ---------------------------------------------------------------------------
check_no_build_tag() {
    local f="$1"
    if [[ ! -f "$f" ]]; then
        echo "  FAIL T2-CAMP-04: el archivo nuevo $f desapareció del árbol"
        return 1
    fi
    if grep -q "go:build" "$f"; then
        echo "  FAIL T2-CAMP-04: $f lleva go:build — un test taggeado no corre en el canónico y no mueve el número (lección test/rls, ACC-E02 T8)"
        return 1
    fi
    echo "  PASS T2-CAMP-04: $f sin go:build (corre en el canónico)"
    return 0
}

# ---------------------------------------------------------------------------
# T2-CAMP-05 (objetivo): patrón fakedb, árbol externo, sin testcontainers/Docker.
#   $1 = path del archivo
# ---------------------------------------------------------------------------
check_fakedb_pattern() {
    local f="$1"
    if [[ ! -f "$f" ]]; then
        echo "  FAIL T2-CAMP-05: el archivo nuevo $f desapareció del árbol"
        return 1
    fi
    local -i rc=0
    if [[ "$f" != "$REPO_ROOT/test/"* ]]; then
        echo "  FAIL T2-CAMP-05: $f no vive en el árbol externo test/ (el objetivo de T2 lo exige)"
        rc=1
    fi
    if ! grep -q "test/testsupport/fakedb" "$f"; then
        echo "  FAIL T2-CAMP-05: $f no extiende el harness fakedb existente (el patrón que el objetivo de T2 manda reutilizar)"
        rc=1
    fi
    if grep -qi "testcontainers" "$f" || grep -qi "docker" "$f"; then
        echo "  FAIL T2-CAMP-05: $f mete testcontainers/Docker — el riesgo de la épica lo prohíbe (cambiaría el comando que mide el piso de T8)"
        rc=1
    fi
    if [[ "$rc" -eq 0 ]]; then
        echo "  PASS T2-CAMP-05: $f — fakedb, árbol externo, sin Docker"
    fi
    return "$rc"
}

# ---------------------------------------------------------------------------
# Enumera los archivos de test nuevos de esta tarea: git status sobre
# test/identity/ (nuevos ?? y modificados M). Si no hay git, la lista explícita
# de los tres archivos que el contrato de T2 menciona, para no bloquear el
# check en un entorno sin git.
# ---------------------------------------------------------------------------
new_task_test_files() {
    local files=""
    if command -v git >/dev/null 2>&1 && git -C "$REPO_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        files="$(git -C "$REPO_ROOT" status --porcelain -- test/identity/ 2>/dev/null \
            | awk '{ $1=""; sub(/^ /, ""); print }' \
            | grep '\*_test\.go$' || true)"
    fi
    if [[ -z "$files" ]]; then
        files="test/identity/auth/infrastructure/persistence/repository/postgres_auth_repository_data_test.go
test/identity/user/infrastructure/persistence/repository/postgres_user_repository_test.go
test/identity/user/infrastructure/persistence/repository/postgres_user_repository_errors_test.go"
    fi
    echo "$files"
}

# ---------------------------------------------------------------------------
# Controles negativos: el checker debe FALLAR ante cada mutación. Reutilizan
# la medición viva ya hecha (una sola corrida de go test) y fabrican archivos
# en tmpdir — nunca tocan los reales.
# ---------------------------------------------------------------------------
negative_controls() {
    local pkg_live="$1" total_live="$2"
    local -i rc_all=0

    # N1 — paquete por debajo del piso: 79,9% → T2-CAMP-01 debe fallar
    if [[ -z "$pkg_live" ]]; then
        echo "  SKIP N1: sin medición viva (--no-live)"
    else
        if check_target_pkg "$(awk -v t="${pkg_live##*$'\t'}" 'BEGIN { c = int(t * 0.799); print c "\t" t }' <<< "$pkg_live")" >/dev/null 2>&1; then
            echo "  FAIL N1: el checker NO detectó el paquete objetivo por debajo del piso de 80%"
            rc_all=1
        else
            echo "  PASS N1: paquete objetivo a 79,9% → checker falla"
        fi
    fi

    # N2 — total por debajo del baseline → T2-CAMP-02 debe fallar
    if [[ -z "$total_live" ]]; then
        echo "  SKIP N2: sin medición viva (--no-live)"
    else
        local baseline_cov
        baseline_cov="$(jq -r '.statements_covered // empty' "$BASELINE_FILE" 2>/dev/null)"
        local l_tot="${total_live##*$'\t'}"
        local n2_live
        n2_live="$(awk -v b="${baseline_cov:-0}" -v t="$l_tot" 'BEGIN { c = b - 1; if (c < 0) c = 0; print c "\t" t }')"
        if check_total_monotonic "$n2_live" >/dev/null 2>&1; then
            echo "  FAIL N2: el checker NO detectó un total por debajo del baseline"
            rc_all=1
        else
            echo "  PASS N2: total menor al baseline → checker falla"
        fi
    fi

    # N3 — archivo con go:build fabricado → T2-CAMP-04 debe fallar
    local n3="$TMP_DIR/tagged_test.go"
    printf '//go:build integration\n\npackage repository_test\n' > "$n3"
    if check_no_build_tag "$n3" >/dev/null 2>&1; then
        echo "  FAIL N3: el checker NO detectó un test taggeado"
        rc_all=1
    else
        echo "  PASS N3: test con go:build → checker falla"
    fi

    # N4 — archivo sin fakedb / con testcontainers → T2-CAMP-05 debe fallar
    local n4="$TMP_DIR/docker_test.go"
    printf 'package repository_test\nimport "github.com/testcontainers/testcontainers-go"\n' > "$n4"
    if check_fakedb_pattern "$n4" >/dev/null 2>&1; then
        echo "  FAIL N4: el checker NO detectó un test que abandona el patrón fakedb por testcontainers"
        rc_all=1
    else
        echo "  PASS N4: test con testcontainers y sin fakedb → checker falla"
    fi

    return "$rc_all"
}

# ---------------------------------------------------------------------------
main() {
    if ! command -v jq >/dev/null 2>&1; then
        echo "t2_coverage_campaign_check: falta jq" >&2
        exit 2
    fi
    if [[ "$LIVE" == "--live" ]] && ! command -v go >/dev/null 2>&1; then
        echo "t2_coverage_campaign_check: falta go" >&2
        exit 2
    fi
    if [[ ! -f "$BASELINE_FILE" ]]; then
        echo "t2_coverage_campaign_check: no encuentro el baseline de T1: $BASELINE_FILE" >&2
        exit 2
    fi

    echo "== ACC-E08.T2 — campaña de cobertura del paquete identity/infrastructure/persistence/repository =="
    echo "baseline de referencia: $BASELINE_FILE"

    local pkg_live="" total_live=""
    if [[ "$LIVE" == "--live" ]]; then
        # (c) go vet + go test ./... — una corrida, compartida por CAMP-03.
        echo ""
        echo "-- re-verificando (c): GOWORK=off go vet ./... y go test ./... --"
        (cd "$REPO_ROOT" && GOWORK=off go vet ./...) > "$TMP_DIR/vet.log" 2>&1
        VET_RC=$?
        (cd "$REPO_ROOT" && GOWORK=off go test ./...) > "$TMP_DIR/gotest.log" 2>&1
        GO_TEST_RC=$?
        if [[ "$GO_TEST_RC" -ne 0 ]]; then
            echo "go test ./... FALLÓ (últimos FAILs):"
            grep -E "^(FAIL|--- FAIL)" "$TMP_DIR/gotest.log" | head -10 || tail -20 "$TMP_DIR/gotest.log"
        fi

        # (a)+(b) re-medición viva con el comando REGISTRADO en el baseline (no
        # uno inventado), profile a tmpdir (sustituyendo c.out).
        echo ""
        echo "-- re-medición viva del comando canónico registrado --"
        local cmd cov_profile
        cmd="$(jq -r '.command // empty' "$BASELINE_FILE")"
        if [[ -z "$cmd" || "$cmd" == "null" ]]; then
            echo "RESULTADO: FAIL — el baseline no registra el comando canónico; la re-medición no es reproducible"
            exit 1
        fi
        cov_profile="$TMP_DIR/t2-live-c.out"
        (cd "$REPO_ROOT" && eval "${cmd//c.out/$cov_profile}") > "$TMP_DIR/canon.log" 2>&1
        local canon_rc=$?
        if [[ "$canon_rc" -ne 0 ]]; then
            echo "ERROR: el comando de cobertura registrado falló (exit $canon_rc):" >&2
            echo "  $cmd" >&2
            tail -20 "$TMP_DIR/canon.log" >&2
            exit 1
        fi
        per_package_from_profile "$cov_profile" > "$TMP_DIR/pkg.txt"
        pkg_live="$(awk -F'\t' -v suf="$TARGET_PKG_SUFFIX" '$3 ~ (suf "$") { print $1 "\t" $2 }' "$TMP_DIR/pkg.txt")"
        total_live="$(awk -F'\t' '{c+=$1; t+=$2} END {print c "\t" t}' "$TMP_DIR/pkg.txt")"

        if [[ -n "$pkg_live" ]]; then
            echo "paquete objetivo vivo: ${pkg_live%%$'\t'*}/${pkg_live##*$'\t'} statements"
        else
            echo "paquete objetivo: SIN BLOQUES EN LA MEDICIÓN (ver T2-CAMP-01)"
        fi
        echo "total vivo deduplicado: $(awk -F'\t' -v l="$total_live" 'BEGIN { split(l, a, "\t"); printf "%.2f%% (%s/%s)", 100*a[1]/a[2], a[1], a[2] }')"
    fi

    local -i rc_checks=0 rc_neg=0
    check_target_pkg "$pkg_live" || rc_checks=1
    check_total_monotonic "$total_live" || rc_checks=1
    report_vet_and_test || rc_checks=1

    echo ""
    echo "-- (d) y patrón sobre los archivos nuevos de la tarea --"
    local files any_missing=0
    files="$(new_task_test_files)"
    if [[ -z "$files" ]]; then
        echo "  FAIL T2-CAMP-04/05: no pude enumerar los archivos nuevos de la tarea (ni por git ni por la lista del contrato)"
        rc_checks=1
    else
        local f
        while IFS= read -r f; do
            [[ -z "$f" ]] && continue
            if [[ ! -f "$REPO_ROOT/$f" ]]; then
                any_missing=1
                continue
            fi
            check_no_build_tag "$REPO_ROOT/$f" || rc_checks=1
            check_fakedb_pattern "$REPO_ROOT/$f" || rc_checks=1
        done <<< "$files"
        if [[ "$any_missing" -eq 1 ]]; then
            echo "  FAIL T2-CAMP-04: al menos un archivo nuevo de la tarea ya no está en el árbol"
            rc_checks=1
        fi
    fi

    echo ""
    echo "== Controles negativos (el checker debe fallar ante cada mutación) =="
    negative_controls "$pkg_live" "$total_live" || rc_neg=1

    echo ""
    if [[ "$rc_checks" -ne 0 ]]; then
        echo "RESULTADO: FAIL — la tarea NO satisface el contrato de ACC-E08.T2 tal como está escrito"
        exit 1
    fi
    if [[ "$rc_neg" -ne 0 ]]; then
        echo "RESULTADO: FAIL — el checker no demuestra sus negativas"
        exit 1
    fi
    echo "RESULTADO: PASS — la tarea satisface el contrato de ACC-E08.T2 y el checker demuestra sus negativas"
    exit 0
}

main