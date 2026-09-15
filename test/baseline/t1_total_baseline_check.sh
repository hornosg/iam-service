#!/usr/bin/env bash
# t1_total_baseline_check.sh — Verificación ejecutable del contrato de ACC-E08.T1
# (congelar el punto de partida REAL de la campaña de cobertura total, post-mudanza D-09)
# contra el árbol vivo de iam-service y el registro platform/iam-service/total-coverage-baseline.json.
#
# POR QUÉ UN SCRIPT Y NO UN TEST GO: mismo criterio que t2_baseline_check.sh (ACC-E01.T2).
# El contrato de T1 congela un NÚMERO del árbol vivo; un func Test nuevo aquí no lo mueve,
# pero el check necesita correr el comando canónico COMO ESTÁ REGISTRADO (go test -coverpkg
# ./test/...) y comparar por-paquete con el awk de deduplicación del mapa de brechas de la
# épica — eso no es lógica de dominio, es instrumentación de medición. Un checker bash
# auto-verificado es el patrón ya establecido en este árbol (t2_baseline_check.sh,
# coverage_gate_check.sh) y el runner de calidad lo ejecuta igual.
#
# Qué verifica (el contrato de T1 tal como está escrito en la épica):
#   T1-TOT-01  total-coverage-baseline.json existe, parsea como JSON y declara los campos
#              del contrato: measured_at (ISO válida, no futura), coverage_total_percent
#              (0-100), statements_covered/statements_total (enteros > 0), el comando
#              textual canónico (-coverpkg=./src/... + ./test/...) y el por-paquete
#              completo e internamente consistente (covered ≤ total, percent ≈
#              100*covered/total, las sumas del por-paquete ≡ los totales declarados).
#   T1-TOT-02  (a) re-medición EN VIVO del comando registrado → total dentro de ±0,5 del
#              registrado, y por-paquete: denominadores idénticos al vivo y covered dentro
#              de ±3 (varianza de startRevocationCleanup, ±0,2 en el total — ACC-E01.T2).
#   T1-TOT-03  (b) notes explica por qué 59,9 canónico ≠ 73,2 comparable: debe mencionar
#              D-09, "denominador" y ambos números. Un baseline sin el porqué es un número
#              huérfano: la próxima persona lo discute de memoria otra vez.
#   T1-TOT-04  (d) GOWORK=off go test ./... → 0 FAIL al momento de congelar.
#   T1-TOT-05  el archivo es NUEVO y NO pisa test-baseline.json (registro histórico de
#              ACC-E01: coverage_global_percent=73.2, measured_at=2026-09-04,
#              func_test_count=401 deben seguir intactos).
#
# Controles negativos (parte del entregable, criterio (c) del contrato: "un baseline que
# no se puede re-medir es decoración"; y el criterio general: un checker que nunca falló
# no prueba nada). El script se auto-verifica contra copias doctoreadas en un tmpdir:
#   N1  cobertura registrada incompatible con la viva → T1-TOT-02 falla
#   N2  measured_at borrado → T1-TOT-01 falla
#   N3  comando sin -coverpkg → T1-TOT-01 falla (sin ese flag el mismo árbol reporta ~10%,
#       el número falso que PLAT-E21 documentó en el gate local)
#   N4  notes sin la explicación D-09 → T1-TOT-03 falla
#   N5  statements_total inflado sin tocar el por-paquete → T1-TOT-01 falla (inconsistencia)
#   N6  denominador de un paquete mutado → T1-TOT-02 (por-paquete) falla  [sólo --live]
# El registro real nunca se toca.
#
# Uso:
#   ./test/baseline/t1_total_baseline_check.sh             # corrida completa
#   ./test/baseline/t1_total_baseline_check.sh --no-live   # sólo checks estáticos
#
# Exit 0 si todo pasa (incl. negativos); 1 si cualquier check falla; 2 setup.

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BASELINE_FILE="${T1_TOTAL_BASELINE_FILE:-$REPO_ROOT/total-coverage-baseline.json}"
HISTORIC_FILE="${T1_HISTORIC_BASELINE_FILE:-$REPO_ROOT/test-baseline.json}"

LIVE="--live"
[[ "${1:-}" == "--no-live" ]] && LIVE="--no-live"

FAILURES=0

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
# Núcleo: corre T1-TOT-01..05 para un archivo de baseline dado.
#   $1 = archivo total-coverage-baseline.json a verificar
#   $2 = "covered\ttotal" del total vivo, o "-" para no comparar (sólo TOT-02
#        se saltea; los estáticos corren igual, son baratos)
# Imprime PASS/FAIL por check. Retorna 0 sólo si todos pasan.
# ---------------------------------------------------------------------------
check_baseline() {
    local baseline="$1" live="$2"
    local -i rc=0

    # --- T1-TOT-01: estructura del registro
    if [[ ! -f "$baseline" ]]; then
        echo "  FAIL T1-TOT-01: no existe $baseline"
        return 1
    fi
    if ! jq -e . "$baseline" >/dev/null 2>&1; then
        echo "  FAIL T1-TOT-01: $baseline no parsea como JSON"
        return 1
    fi
    local mad pct cov_s tot_s cmd npkg
    mad="$(jq -r '.measured_at // empty' "$baseline")"
    pct="$(jq -r '.coverage_total_percent // empty' "$baseline")"
    cov_s="$(jq -r '.statements_covered // empty' "$baseline")"
    tot_s="$(jq -r '.statements_total // empty' "$baseline")"
    cmd="$(jq -r '.command // empty' "$baseline")"
    if [[ -z "$mad" || "$mad" == "null" ]] || ! [[ "$mad" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] \
        || ! date -d "$mad" '+%Y-%m-%d' >/dev/null 2>&1; then
        echo "  FAIL T1-TOT-01: measured_at ausente o no es una fecha ISO válida: '$mad'"
        rc=1
    elif [[ "$(date -d "$mad" '+%Y-%m-%d')" > "$(date '+%Y-%m-%d')" ]]; then
        echo "  FAIL T1-TOT-01: measured_at '$mad' es una fecha futura — medido el día de la corrida, no estimado"
        rc=1
    fi
    if [[ -z "$pct" || "$pct" == "null" ]] || ! [[ "$pct" =~ ^[0-9]+([.][0-9]+)?$ ]] \
        || ! awk -v p="$pct" 'BEGIN { exit (p < 0 || p > 100) }'; then
        echo "  FAIL T1-TOT-01: coverage_total_percent ausente o no es un número 0-100: '$pct'"
        rc=1
    fi
    if ! [[ "$cov_s" =~ ^[0-9]+$ ]] || ! [[ "$tot_s" =~ ^[0-9]+$ ]] || (( cov_s <= 0 || tot_s <= 0 )) \
        || (( cov_s > tot_s )); then
        echo "  FAIL T1-TOT-01: statements_covered/statements_total ausentes, no enteros o inconsistentes: '$cov_s'/'$tot_s'"
        rc=1
    fi
    if [[ -z "$cmd" || "$cmd" == "null" ]]; then
        echo "  FAIL T1-TOT-01: command ausente — sin el comando textual el baseline no se puede re-medir"
        rc=1
    else
        if [[ "$cmd" != *"-coverpkg=./src/..."* ]]; then
            echo "  FAIL T1-TOT-01: el comando registrado no lleva -coverpkg=./src/... : '$cmd'"
            rc=1
        fi
        if [[ "$cmd" != *"./test/..."* ]]; then
            echo "  FAIL T1-TOT-01: el comando registrado no corre sobre ./test/... (árbol externo): '$cmd'"
            rc=1
        fi
        if [[ "$cmd" != *"go tool cover"* ]]; then
            echo "  FAIL T1-TOT-01: el comando registrado no incluye el paso 'go tool cover': '$cmd'"
            rc=1
        fi
    fi
    # por-paquete: completo e internamente consistente
    local -i n_pkgs
    n_pkgs="$(jq '.per_package | length' "$baseline" 2>/dev/null || echo -1)"
    if (( n_pkgs <= 0 )); then
        echo "  FAIL T1-TOT-01: per_package ausente o vacío — el contrato pide el por-paquete completo"
        rc=1
    else
        local bad_pkgs
        bad_pkgs="$(jq -r '
          .per_package[]
          | select(
              (.package // "" | length) == 0
              or (.covered // -1) < 0
              or (.total // -1) < 1
              or (.covered > .total)
              or (((.percent // -1) - (100 * .covered / .total) | if . < 0 then -. else . end) > 0.05)
            )
          | .package' "$baseline" 2>/dev/null | head -5)"
        if [[ -n "$bad_pkgs" ]]; then
            echo "  FAIL T1-TOT-01: entradas por-paquete inconsistentes (covered>total o percent ≠ 100*covered/total): $(echo "$bad_pkgs" | tr '\n' ' ')"
            rc=1
        fi
        local s_cov s_tot
        s_cov="$(jq '[.per_package[].covered] | add' "$baseline")"
        s_tot="$(jq '[.per_package[].total] | add' "$baseline")"
        if [[ "$s_cov" != "$cov_s" || "$s_tot" != "$tot_s" ]]; then
            echo "  FAIL T1-TOT-01: los totales declarados ($cov_s/$tot_s) no cuadran con la suma del por-paquete ($s_cov/$s_tot)"
            rc=1
        fi
        local pct_calc
        pct_calc="$(awk -v c="$cov_s" -v t="$tot_s" 'BEGIN { printf "%.2f", 100 * c / t }')"
        local d_pct
        d_pct="$(awk -v m="$pct" -v c="$pct_calc" 'BEGIN { d = m - c; if (d < 0) d = -d; printf "%.2f", d }')"
        if awk -v d="$d_pct" 'BEGIN { exit !(d > 0.05) }'; then
            echo "  FAIL T1-TOT-01: coverage_total_percent ($pct) no cuadra con covered/total ($pct_calc recomputado, delta $d_pct)"
            rc=1
        fi
    fi
    if [[ "$rc" -eq 0 ]]; then
        echo "  PASS T1-TOT-01: estructura completa y consistente ($cov_s/$tot_s = $pct%, $n_pkgs paquetes, measured_at=$mad)"
    fi

    # --- T1-TOT-02 (criterio a): re-medición viva dentro de ±0,5 y por-paquete
    if [[ "$live" == "-" ]]; then
        echo "  SKIP T1-TOT-02: sin medición viva (--no-live)"
    else
        # (live llega como "covered\ttotal" del total vivo, ya deduplicado)
        local live_cov l_cov l_tot
        l_cov="${live%%$'\t'*}"
        l_tot="${live##*$'\t'}"
        live_cov="$(awk -v c="$l_cov" -v t="$l_tot" 'BEGIN { printf "%.2f", 100 * c / t }')"
        local delta
        delta="$(awk -v m="$live_cov" -v r="$pct" 'BEGIN { d = m - r; if (d < 0) d = -d; printf "%.3f", d }')"
        if awk -v d="$delta" 'BEGIN { exit !(d > 0.5) }'; then
            echo "  FAIL T1-TOT-02: total vivo $live_cov% ($l_cov/$l_tot) difiere en $delta del registrado $pct% (tolerancia ±0,5) — el baseline es decoración"
            rc=1
        else
            echo "  PASS T1-TOT-02: total vivo $live_cov% dentro de ±0,5 del registrado $pct%"
        fi
        local pkg_bad pkg_missed
        pkg_bad=""
        pkg_missed=0
        local l_line l_pkg l_cov_p l_tot_p
        while IFS=$'\t' read -r l_cov_p l_tot_p l_pkg; do
            [[ -z "$l_pkg" ]] && continue
            local r_line r_cov r_tot
            r_line="$(jq -r --arg p "$l_pkg" '.per_package[] | select(.package == $p) | "\(.covered)\t\(.total)"' "$baseline")"
            if [[ -z "$r_line" ]]; then
                pkg_missed=$((pkg_missed + 1))
                continue
            fi
            r_cov="${r_line%%$'\t'*}"
            r_tot="${r_line##*$'\t'}"
            if (( l_tot_p != r_tot )); then
                pkg_bad+="  FAIL T1-TOT-02: denominador de $l_pkg cambió en el árbol vivo ($r_tot registrado → $l_tot_p vivo) — el baseline quedó viejo"$'\n'
            elif (( l_cov_p < r_cov - 3 || l_cov_p > r_cov + 3 )); then
                pkg_bad+="  FAIL T1-TOT-02: covered de $l_pkg diverge del registrado ($r_cov → $l_cov_p, tolerancia ±3)"$'\n'
            fi
        done < "$LIVE_PKG_FILE"
        # paquetes registrados que el vivo ya no ve (código movido/borrado)
        local r_pkg
        while IFS= read -r r_pkg; do
            [[ -z "$r_pkg" ]] && continue
            if ! cut -f3 "$LIVE_PKG_FILE" | grep -qxF "$r_pkg"; then
                pkg_bad+="  FAIL T1-TOT-02: $r_pkg registrado ya no aparece en la medición viva — el árbol cambió"$'\n'
            fi
        done < <(jq -r '.per_package[].package' "$baseline")
        if [[ -n "$pkg_bad" ]]; then
            echo -n "$pkg_bad"
            rc=1
        else
            echo "  PASS T1-TOT-02: por-paquete vivo congruente con el registrado ($pkg_missed paquetes nuevos no registrados)"
        fi
        if (( pkg_missed > 0 )); then
            echo "  FAIL T1-TOT-02: $pkg_missed paquete(s) del árbol vivo no están en el registro por-paquete — el por-paquete no está completo"
            rc=1
        fi
    fi

    # --- T1-TOT-03 (criterio b): notes explica 59,9 ≠ 73,2 (D-09)
    local notes
    notes="$(jq -r '.notes // ""' "$baseline")"
    local -i rc_notes=0
    [[ ${#notes} -lt 80 ]] && { echo "  FAIL T1-TOT-03: notes demasiado corto (${#notes} chars) — no puede explicar el porqué"; rc_notes=1; }
    for needle in "D-09" "denominador" "59.9" "73.2"; do
        if ! grep -qi -- "$needle" <<< "$notes"; then
            echo "  FAIL T1-TOT-03: notes no menciona '$needle' — falta la explicación de por qué 59,9 canónico ≠ 73,2 comparable"
            rc_notes=1
        fi
    done
    if [[ "$rc_notes" -eq 0 ]]; then
        echo "  PASS T1-TOT-03: notes explica D-09 (denominador creció, cobertura no cayó) con ambos números"
    else
        rc=1
    fi

    # --- T1-TOT-04 (criterio d): go test ./... verde — se corre UNA vez en main(),
    # no por copia doctoreada; acá sólo se reporta el resultado ya medido.
    if [[ "$live" == "-" ]]; then
        echo "  SKIP T1-TOT-04: la suite completa corre sólo en la pasada --live"
    else
        if [[ "$GO_TEST_RC" -ne 0 ]]; then
            echo "  FAIL T1-TOT-04: GOWORK=off go test ./... no está verde al congelar (exit $GO_TEST_RC)"
            rc=1
        else
            echo "  PASS T1-TOT-04: GOWORK=off go test ./... verde (0 FAIL)"
        fi
    fi

    # --- T1-TOT-05: archivo NUEVO, no pisa el registro histórico de ACC-E01
    if [[ ! -f "$HISTORIC_FILE" ]]; then
        echo "  FAIL T1-TOT-05: no existe $HISTORIC_FILE — T1 no debía pisar el registro histórico"
        rc=1
    else
        local h_pct h_mad h_ftc
        h_pct="$(jq -r '.coverage_global_percent // empty' "$HISTORIC_FILE" 2>/dev/null)"
        h_mad="$(jq -r '.measured_at // empty' "$HISTORIC_FILE" 2>/dev/null)"
        h_ftc="$(jq -r '.func_test_count // empty' "$HISTORIC_FILE" 2>/dev/null)"
        if [[ "$h_pct" != "73.2" || "$h_mad" != "2026-09-04" || "$h_ftc" != "401" ]]; then
            echo "  FAIL T1-TOT-05: test-baseline.json fue tocado (73.2/2026-09-04/401 esperados; hoy: $h_pct/$h_mad/$h_ftc)"
            rc=1
        else
            echo "  PASS T1-TOT-05: test-baseline.json intacto (73.2 / 2026-09-04 / 401)"
        fi
    fi

    return "$rc"
}

# ---------------------------------------------------------------------------
# Medición viva: corre el comando REGISTRADO (no uno inventado) desde la raíz,
# profile a tmpdir (sustituyendo c.out), y deja los artefactos en variables
# globales LIVE_PKG_FILE + TOT vivo.
# ---------------------------------------------------------------------------
measure_live() {
    local cmd tmp cov_profile
    cmd="$(jq -r '.command' "$BASELINE_FILE")"
    tmp="$(mktemp -d)"
    cov_profile="$tmp/t1-live-c.out"
    (cd "$REPO_ROOT" && eval "${cmd//c.out/$cov_profile}") >/dev/null 2>&1
    local rc=$?
    if [[ "$rc" -ne 0 ]]; then
        echo "ERROR: el comando de cobertura registrado falló (exit $rc):" >&2
        echo "  $cmd" >&2
        rm -rf "$tmp"
        return 1
    fi
    per_package_from_profile "$cov_profile" > "$tmp/pkg.txt"
    LIVE_PKG_FILE="$tmp/pkg.txt"
    LIVE_COV="$(awk -F'\t' '{c+=$1; t+=$2} END {print c "\t" t}' "$tmp/pkg.txt")"
    rm -f "$cov_profile"   # pkg.txt queda para los checks y se limpia al final del main
    return 0
}

# ---------------------------------------------------------------------------
# Controles negativos: mutar una COPIA del baseline y exigir que el check falle.
# Reutilizan la medición viva ya hecha (una sola corrida de go test) y los
# checks estáticos (baratos).
# ---------------------------------------------------------------------------
negative_controls() {
    local live="$1"
    local tmp rc_all=0
    tmp="$(mktemp -d)"

    # N1 — cobertura inflada: 61.2→99.9 → T1-TOT-02 debe fallar (fuera de ±0,5)
    if [[ "$live" == "-" ]]; then
        echo "  SKIP N1: sin medición viva (--no-live)"
    else
        jq '.coverage_total_percent = 99.9' "$BASELINE_FILE" > "$tmp/n1.json"
        if check_baseline "$tmp/n1.json" "$live" >/dev/null 2>&1; then
            echo "  FAIL N1: el checker NO detectó una cobertura registrada incompatible con la viva"
            rc_all=1
        else
            echo "  PASS N1: cobertura registrada incompatible con la viva → checker falla"
        fi
    fi

    # N2 — campo borrado: sin measured_at → T1-TOT-01 debe fallar
    jq 'del(.measured_at)' "$BASELINE_FILE" > "$tmp/n2.json"
    if check_baseline "$tmp/n2.json" "-" >/dev/null 2>&1; then
        echo "  FAIL N2: el checker NO detectó la ausencia de measured_at"
        rc_all=1
    else
        echo "  PASS N2: measured_at borrado → checker falla"
    fi

    # N3 — comando ciego: sacar -coverpkg → T1-TOT-01 debe fallar. Sin ese flag el
    # coverage reporta ~10% (tests en árbol externo) — el número falso de PLAT-E21.
    jq -r '.command |= gsub("-coverpkg=./src/... "; "")' "$BASELINE_FILE" > "$tmp/n3.json"
    if check_baseline "$tmp/n3.json" "-" >/dev/null 2>&1; then
        echo "  FAIL N3: el checker NO detectó un comando de cobertura sin -coverpkg"
        rc_all=1
    else
        echo "  PASS N3: comando sin -coverpkg=./src/... → checker falla"
    fi

    # N4 — el porqué borrado: notes vacío → T1-TOT-03 debe fallar (criterio b:
    # sin la explicación D-09 el número vuelve a discutirse de memoria)
    jq '.notes = ""' "$BASELINE_FILE" > "$tmp/n4.json"
    if check_baseline "$tmp/n4.json" "-" >/dev/null 2>&1; then
        echo "  FAIL N4: el checker NO detectó notes sin la explicación D-09"
        rc_all=1
    else
        echo "  PASS N4: notes vacío → checker falla"
    fi

    # N5 — inconsistencia interna: statements_total inflado sin tocar el por-paquete
    jq '.statements_total += 100' "$BASELINE_FILE" > "$tmp/n5.json"
    if check_baseline "$tmp/n5.json" "-" >/dev/null 2>&1; then
        echo "  FAIL N5: el checker NO detectó un statements_total que no cuadra con el por-paquete"
        rc_all=1
    else
        echo "  PASS N5: statements_total inflado sin tocar por-paquete → checker falla"
    fi

    # N6 — denominador de un paquete mutado → el cruce contra el vivo debe fallar
    if [[ "$live" == "-" ]]; then
        echo "  SKIP N6: sin medición viva (--no-live)"
    else
        jq '.per_package |= map(if .package | test("shared/postgres$") then .total += 1 else . end)' \
            "$BASELINE_FILE" > "$tmp/n6.json"
        if check_baseline "$tmp/n6.json" "$live" >/dev/null 2>&1; then
            echo "  FAIL N6: el checker NO detectó un denominador por-paquete mutado"
            rc_all=1
        else
            echo "  PASS N6: denominador por-paquete mutado → checker falla"
        fi
    fi

    rm -rf "$tmp"
    return "$rc_all"
}

# ---------------------------------------------------------------------------
main() {
    if ! command -v jq >/dev/null 2>&1; then
        echo "t1_total_baseline_check: falta jq" >&2
        exit 2
    fi
    if ! command -v go >/dev/null 2>&1; then
        echo "t1_total_baseline_check: falta go" >&2
        exit 2
    fi
    if [[ ! -f "$BASELINE_FILE" ]]; then
        echo "t1_total_baseline_check: no encuentro el baseline: $BASELINE_FILE" >&2
        exit 2
    fi

    echo "== ACC-E08.T1 — baseline de cobertura total (post-mudanza D-09) vs árbol vivo =="
    echo "baseline: $BASELINE_FILE"

    local live="-"
    if [[ "$LIVE" == "--live" ]]; then
        # (d) go test ./... — el criterio pide verde AL MOMENTO DE CONGELAR; acá
        # se re-verifica contra el árbol vivo (una sola corrida, compartida).
        echo ""
        echo "-- re-verificando (d): GOWORK=off go test ./... --"
        (cd "$REPO_ROOT" && GOWORK=off go test ./...) > "$REPO_ROOT/.t1_go_test.log" 2>&1
        GO_TEST_RC=$?
        if [[ "$GO_TEST_RC" -ne 0 ]]; then
            echo "go test ./... FALLÓ (ver .t1_go_test.log; últimos FAILs):"
            grep -E "^(FAIL|--- FAIL)" .t1_go_test.log | head -10 || tail -20 .t1_go_test.log
        else
            rm -f .t1_go_test.log
        fi

        echo ""
        echo "-- re-medición viva del comando registrado --"
        if ! measure_live; then
            echo "RESULTADO: FAIL — no pude medir la cobertura viva con el comando registrado"
            exit 1
        fi
        live="$LIVE_COV"
        echo "total vivo re-medido: $(awk -F'\t' -v c="${live%%$'\t'*}" -v t="${live##*$'\t'}" 'BEGIN { printf "%.2f", 100 * c / t }')% (${live%%$'\t'*}/${live##*$'\t'})"
    fi

    local -i rc_base=0 rc_neg=0
    check_baseline "$BASELINE_FILE" "$live" || rc_base=1

    echo ""
    echo "== Controles negativos (el checker debe fallar ante cada mutación) =="
    negative_controls "$live" || rc_neg=1

    rm -f "${LIVE_PKG_FILE:-}"

    echo ""
    if [[ "$rc_base" -ne 0 ]]; then
        echo "RESULTADO: FAIL — el baseline NO satisface el contrato de ACC-E08.T1 tal como está escrito"
        exit 1
    fi
    if [[ "$rc_neg" -ne 0 ]]; then
        echo "RESULTADO: FAIL — el checker no demuestra sus negativas"
        exit 1
    fi
    echo "RESULTADO: PASS — el baseline satisface el contrato de ACC-E08.T1 y el checker demuestra sus negativas"
    exit 0
}

main