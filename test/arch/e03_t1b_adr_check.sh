#!/usr/bin/env bash
# e03_t1b_adr_check.sh — Verificación ejecutable del contrato de ACC-E03 T1b
# (cerrar las dos condiciones del ADR que puso el gate L4 de T1) contra el ADR
# escrito y contra el "Hecho cuando" de T8 en la épica.
#
# POR QUÉ UN SCRIPT Y NO UN TEST GO: el sujeto del contrato de T1b es un
# DOCUMENTO (el ADR-003 §c/§f y el "Hecho cuando" de T8 en la épica), no código.
# No hay símbolo Go que ejercitar. Mismo precedente que e03_t1_adr_check.sh
# (T1) y t1_map_check.sh (ACC-E01.T1). Un script no mueve el conteo de
# `func Test` del baseline (test-baseline.json).
#
# NOTA DE RUTA (discrepancia de la épica, no de este check): el Objetivo y el
# "Hecho cuando" de T1b citan `management/projects/account-service/adr/
# ADR-003-*.md`, pero esa ruta NO existe — es un path vencido. El ADR vive en
# `platform/iam-service/docs/adr/ADR-003-firma-asimetrica-jwks.md`: lo fija el
# Objetivo de T1, lo confirmó el gate L4 (escalación 2026-09-08_ACC-E03-T1-
# signoff.md, "diff sin commitear de platform/iam-service — docs/adr/...") y lo
# usa e03_t1_adr_check.sh. Este check verifica el ADR en su ruta real; la épica
# (EPIC_FILE) se verifica aparte para el re-alcance de T8.
#
# Qué verifica (el contrato de T1b tal como está escrito en la épica — el
# "Hecho cuando", no lo que el ADR haya querido decir):
#   E03-T1b-01  condición (1) del gate: `grep -ci 'kid1\|401' <ADR>` ≥ 1 Y el
#               texto dice EXPLÍCITAMENTE que la rotación sin dual de borde
#               invalida tokens vivos (los tokens kid1 no expirados dan 401).
#   E03-T1b-02  condición (2) del gate: `grep -ci 'selección dual\|regla de
#               selección' <ADR>` ≥ 1 EN §f, con la regla explícita (HS256 →
#               sólo secreto viejo; RS256 → pública por kid; none/otro → rechazo).
#   E03-T1b-03  el "Hecho cuando" de T8 en la épica queda re-alcanceado a
#               verificación in-process y lo dice en su propio texto.
#   E03-T1b-04  NEGATIVA: `grep -c 'kid' <ADR>` → si da 0, la condición del gate
#               sigue abierta y la tarea FALLA (textual en el "Hecho cuando").
#
# Controles negativos (parte del entregable, como en e03_t1_adr_check.sh):
#   un checker que nunca falló no prueba nada. Se auto-verifica contra copias
#   doctoreadas del ADR y de la épica en un tmpdir: borrar el 401 de la rotación
#   (§c), borrar la regla de selección dual (§f), borrar el re-alcance in-process
#   de T8, y borrar todo 'kid' del ADR — cada mutación DEBE hacer fallar el check.
#   El ADR y la épica reales nunca se tocan.
#
# Uso:
#   ./test/arch/e03_t1b_adr_check.sh
#   ADR_FILE=/otra/ruta/ADR-003.md EPIC_FILE=/otra/ruta/ACC-E03.md ./test/arch/e03_t1b_adr_check.sh
#
# Exit 0 si todo pasa (incl. controles negativos); 1 si cualquier check falla.

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ADR_FILE="${ADR_FILE:-$REPO_ROOT/docs/adr/ADR-003-firma-asimetrica-jwks.md}"
EPIC_FILE="${EPIC_FILE:-$REPO_ROOT/../../management/projects/account-service/epicas/ACC-E03-firma-asimetrica-jwks.md}"

# ---------------------------------------------------------------------------
# Helpers de extracción
# ---------------------------------------------------------------------------

# Extrae el cuerpo de la subsección de decisión "### (<letra>)" (a-f).
# El header se consume; la sección termina en la primera línea que vuelve a
# empezar con '#'.
extract_section() {
    local letter="$1" file="$2"
    awk -v L="($letter)" '
        /^### \([a-f]\)/ { insec = (substr($0, 5, 3) == L); next }
        insec && /^#/   { insec = 0 }
        insec           { print }
    ' "$file"
}

# Extrae la línea "Hecho cuando:" de la tarea T8 en la épica (la primera tras el
# header "- [ ] **T8 · ..."). Es el texto que el contrato exige re-alcancear.
extract_t8_hecho() {
    local file="$1"
    awk '
        /^- \[[ x~]\] \*\*T8 ·/ { int8 = 1; next }
        int8 && /Hecho cuando:/ { print; exit }
    ' "$file"
}

FAILURES=0
fail() { echo "  FAIL $1"; FAILURES=$((FAILURES + 1)); }

# ---------------------------------------------------------------------------
# Núcleo: corre E03-T1b-01..04 contra el ADR y la épica dados.
# ---------------------------------------------------------------------------

check_contract() {
    local adr="$1" epic="$2"
    FAILURES=0

    # --- E03-T1b-01: condición (1) — rotación sin dual de borde deja kid1 vivos en 401
    local sec sec_norm kid401
    kid401="$(grep -ci 'kid1\|401' "$adr" 2>/dev/null || echo 0)"
    sec="$(extract_section c "$adr")"
    # "no expirados" puede estar partido por un salto de línea del wrap markdown
    # (y la línea de continuación lleva indentación); se colapsan saltos y se
    # comprimen espacios para que el grep lo vea como una frase.
    sec_norm="$(tr '\n' ' ' <<<"$sec" | tr -s ' ')"
    if [[ "$kid401" -ge 1 ]] \
       && grep -q 'kid1' <<<"$sec" \
       && grep -q '401' <<<"$sec" \
       && grep -qiE 'sin dual de borde|rotar sin dual' <<<"$sec" \
       && grep -qiE 'no expirados|vivos' <<<"$sec_norm"; then
        echo "  PASS E03-T1b-01: §c declara que rotar sin dual de borde deja los tokens kid1 vivos en 401"
    else
        fail "E03-T1b-01: falta la declaración explícita de que la rotación sin dual de borde invalida tokens kid1 vivos (401)"
    fi

    # --- E03-T1b-02: condición (2) — regla de selección dual explícita en §f
    sec="$(extract_section f "$adr")"
    if [[ -n "$sec" ]] \
       && grep -qiE 'selección dual|regla de selección' <<<"$sec" \
       && grep -q 'HS256' <<<"$sec" \
       && grep -q 'RS256' <<<"$sec" \
       && grep -qiE 'rechazo|jamás' <<<"$sec" \
       && grep -qiE 'secreto viejo|JWT_SECRET' <<<"$sec"; then
        echo "  PASS E03-T1b-02: §f explicita la regla de selección dual (HS256→secreto viejo, RS256→pública por kid, none/otro→rechazo)"
    else
        fail "E03-T1b-02: falta la regla de selección dual explícita en §f (qué kid elige el verificador con dos publicados)"
    fi

    # --- E03-T1b-03: el "Hecho cuando" de T8 re-alcanceado a in-process
    local t8
    t8="$(extract_t8_hecho "$epic")"
    if [[ -n "$t8" ]] \
       && grep -qiE 'in-process' <<<"$t8" \
       && grep -qiE 'no contra Kong' <<<"$t8"; then
        echo "  PASS E03-T1b-03: el 'Hecho cuando' de T8 quedó re-alcanceado a verificación in-process (no contra Kong)"
    else
        fail "E03-T1b-03: el 'Hecho cuando' de T8 no está re-alcanceado a in-process (o no lo dice en su propio texto)"
    fi

    # --- E03-T1b-04: NEGATIVA — grep -c 'kid' → 0 significa condición abierta
    local kid_count
    kid_count="$(grep -c 'kid' "$adr" 2>/dev/null || echo 0)"
    if [[ "$kid_count" -gt 0 ]]; then
        echo "  PASS E03-T1b-04: el ADR menciona 'kid' ($kid_count líneas) — la condición del gate no quedó abierta"
    else
        fail "E03-T1b-04: grep -c 'kid' da 0 — la condición del gate sigue abierta y la tarea FALLA"
    fi

    return "$FAILURES"
}

# ---------------------------------------------------------------------------
# Controles negativos: mutar una COPIA del ADR/épica y exigir que el check
# reaccione. El ADR y la épica reales nunca se tocan.
# ---------------------------------------------------------------------------
negative_controls() {
    local adr="$1" epic="$2"
    local tmp rc_all=0
    tmp="$(mktemp -d)"

    # NC1 — sin el 401 de la rotación: reemplazar 401→200 en el ADR → E03-T1b-01
    # debe fallar (el texto ya no dice que los tokens kid1 vivos dan 401).
    sed 's/401/200/g' "$adr" > "$tmp/nc1.md"
    if check_contract "$tmp/nc1.md" "$epic" >/dev/null 2>&1; then
        echo "  FAIL NC1: el checker NO detectó la ausencia del 401 en la rotación sin dual de borde"
        rc_all=1
    else
        echo "  PASS NC1: 401 de la rotación borrado → checker falla"
    fi

    # NC2 — sin la regla de selección dual: borrar el header de la regla en §f
    # → E03-T1b-02 debe fallar.
    sed '/Regla de selección dual/d' "$adr" > "$tmp/nc2.md"
    if check_contract "$tmp/nc2.md" "$epic" >/dev/null 2>&1; then
        echo "  FAIL NC2: el checker NO detectó la ausencia de la regla de selección dual en §f"
        rc_all=1
    else
        echo "  PASS NC2: regla de selección dual borrada → checker falla"
    fi

    # NC3 — sin el re-alcance in-process de T8: reemplazar in-process→out-of-process
    # en la épica → E03-T1b-03 debe fallar.
    sed 's/in-process/out-of-process/g' "$epic" > "$tmp/nc3.md"
    if check_contract "$adr" "$tmp/nc3.md" >/dev/null 2>&1; then
        echo "  FAIL NC3: el checker NO detectó la ausencia del re-alcance in-process de T8"
        rc_all=1
    else
        echo "  PASS NC3: re-alcance in-process de T8 borrado → checker falla"
    fi

    # NC4 — sin 'kid' en absoluto: borrar toda ocurrencia de 'kid' → E03-T1b-04
    # (y las que dependen de kid) debe fallar.
    sed 's/kid//g' "$adr" > "$tmp/nc4.md"
    if check_contract "$tmp/nc4.md" "$epic" >/dev/null 2>&1; then
        echo "  FAIL NC4: el checker NO detectó la ausencia total de 'kid' en el ADR"
        rc_all=1
    else
        echo "  PASS NC4: 'kid' borrado del ADR → checker falla"
    fi

    rm -rf "$tmp"
    return "$rc_all"
}

# ---------------------------------------------------------------------------
main() {
    if [[ ! -f "$ADR_FILE" ]]; then
        echo "e03_t1b_adr_check: no encuentro el ADR: $ADR_FILE" >&2
        exit 2
    fi
    if [[ ! -f "$EPIC_FILE" ]]; then
        echo "e03_t1b_adr_check: no encuentro la épica: $EPIC_FILE" >&2
        exit 2
    fi

    echo "== ACC-E03.T1b — contrato (cerrar las dos condiciones del gate L4 de T1) =="
    echo "adr:  $ADR_FILE"
    echo "epic: $EPIC_FILE"
    local -i rc_contract=0 rc_neg=0
    check_contract "$ADR_FILE" "$EPIC_FILE" || rc_contract=1

    echo ""
    echo "== Controles negativos (el checker debe reaccionar ante cada mutación) =="
    negative_controls "$ADR_FILE" "$EPIC_FILE" || rc_neg=1

    echo ""
    if [[ "$rc_contract" -ne 0 ]]; then
        echo "RESULTADO: FAIL — el ADR/la épica NO satisfacen el contrato de T1b tal como está escrito"
        [[ "$rc_neg" -ne 0 ]] && echo "(además: el checker no demuestra sus negativas)"
        exit 1
    fi
    if [[ "$rc_neg" -ne 0 ]]; then
        echo "RESULTADO: FAIL — el checker no demuestra sus negativas"
        exit 1
    fi
    echo "RESULTADO: PASS — el contrato de T1b se satisface y el checker demuestra sus negativas"
    exit 0
}

main
