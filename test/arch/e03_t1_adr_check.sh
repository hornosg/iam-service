#!/usr/bin/env bash
# e03_t1_adr_check.sh — Verificación ejecutable del contrato de ACC-E03 T1
# (ADR-003: algoritmo, clave, kid, JWKS, Kong, cutover) contra el ADR escrito.
#
# POR QUÉ UN SCRIPT Y NO UN TEST GO: el sujeto del contrato de T1 es un
# DOCUMENTO (docs/adr/ADR-003-firma-asimetrica-jwks.md), no código. No hay
# símbolo Go que ejercitar; lo que hay que verificar es que el ADR resuelve
# las seis decisiones con criterio explícito, que registra la prueba contra
# el binario de Kong (no asumida) y que el gate L4 firmó. El repo ya tiene el
# precedente de checks de contrato como scripts shell (test/arch/t1_map_check.sh
# de ACC-E01.T1, test/baseline/t2_baseline_check.sh de ACC-E01.T2). Además, un
# script no mueve el conteo de `func Test` del baseline (401, test-baseline.json).
#
# Qué verifica (el contrato de T1 tal como está escrito en la épica — el
# "Hecho cuando", no lo que el ADR haya querido decir):
#   E03-T1-01  el archivo existe en la ruta que fija el Objetivo de T1.
#   E03-T1-02  las SEIS secciones de decisión (a)-(f), cada una con el contenido
#              que el contrato exige: (a) algoritmo con EdDSA descartado y motivo
#              (Kong OSS jwt); (b) PEM PKCS#8 + validación de arranque análoga a
#              JWT_SECRET + nunca en git; (c) kid + rotación con solapamiento
#              (publicar antes de firmar, gracia); (d) JWKS con ruta y array
#              keys kty/use/alg/kid; (e) Kong con PEM estático por credencial +
#              consecuencia del dual en el borde (key_claim_name iss→kid, 20
#              bloques); (f) cutover dual anclado al TTL de 15 min.
#   E03-T1-03  las CINCO secciones del formato ADR canónico del repo
#              (Contexto, Decisión, Alternativas, Consecuencias, Revisión —
#              ADR-002 usa "Revisión prevista", ADR-003 "Revisión previa":
#              se acepta el prefijo).
#   E03-T1-04  la salida de `docker exec lab-kong kong version` ESTÁ REGISTRADA
#              en el ADR (prueba contra el binario, no asumida). Si lab-kong
#              está arriba, la versión viva debe coincidir con la registrada:
#              un ADR verificado contra OTRO binario que el instalado es una
#              prueba vencida. Kong caído → SKIP de la comparación viva (la
#              presencia registrada ya se exigió).
#   E03-T1-05  evidencia de que ESA versión acepta una credencial
#              `algorithm: RS256` + `rsa_public_key`: la prueba viva con el
#              200 del token válido y los 401 de alg:none / otra clave.
#   E03-T1-06  NEGATIVA: el ADR no afirma que Kong auto-descarga el JWKS.
#              Toda mención de descarga debe ser una negación ("No hay
#              auto-descarga..."); una afirmación es el riesgo explícito de la
#              épica y falla la tarea.
#   E03-T1-07  los checkboxes `@dev-security GO` y `Owner GO` están MARCADOS.
#              Sin ambos es un L4 sin firma y la tarea FALLA — textual en el
#              "Hecho cuando" (b). Los marca el gate, no el autor del ADR ni
#              el que escribe este check.
#
# Controles negativos (parte del entregable, como en t1_map_check.sh):
#   un checker que nunca falló no prueba nada. El script se auto-verifica contra
#   copias doctoreadas del ADR en un tmpdir: borrar la sección (e), borrar el
#   registro de `kong version`, inyectar una afirmación de auto-descarga y
#   renombrar una sección del formato — cada mutación DEBE hacer fallar el
#   check. Y el control inverso: una copia con ambos checkboxes marcados DEBE
#   pasar todo — prueba que E03-T1-07 no falla por un grep roto sino por las
#   firmas ausentes. El ADR real nunca se toca.
#
# Uso:
#   ./test/arch/e03_t1_adr_check.sh
#   ADR_FILE=/otra/ruta/ADR-003.md ./test/arch/e03_t1_adr_check.sh
#
# Exit 0 si todo pasa (incl. controles negativos); 1 si cualquier check falla.

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ADR_FILE="${ADR_FILE:-$REPO_ROOT/docs/adr/ADR-003-firma-asimetrica-jwks.md}"

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

# Extrae desde "## Firmas" hasta el final (sección de checkboxes del gate).
firmas_section() {
    sed -n '/^## Firmas/,$p' "$1"
}

FAILURES=0
fail() { echo "  FAIL $1"; FAILURES=$((FAILURES + 1)); }

# ---------------------------------------------------------------------------
# Núcleo: corre E03-T1-01..07 contra un archivo ADR dado.
# ---------------------------------------------------------------------------

check_adr() {
    local adr="$1"
    FAILURES=0

    # --- E03-T1-01: existencia en la ruta del Objetivo
    if [[ -f "$adr" ]]; then
        echo "  PASS E03-T1-01: ADR-003 existe ($adr)"
    else
        echo "  FAIL E03-T1-01: no existe $adr — el Objetivo de T1 fija esa ruta exacta"
        return 1
    fi

    # --- E03-T1-02: las seis decisiones (a)-(f) con su contenido exigido
    local sec
    # (a) algoritmo — EdDSA descartado CON motivo anclado al plugin jwt de Kong
    sec="$(extract_section a "$adr")"
    if [[ -n "$sec" ]] \
       && grep -qi "eddsa" <<<"$sec" \
       && grep -qi "descart" <<<"$sec" \
       && grep -qiE "kong|plugin" <<<"$sec" \
       && grep -qE "RS256|ES256" <<<"$sec"; then
        echo "  PASS E03-T1-02(a): algoritmo con EdDSA descartado y motivo (plugin jwt de Kong)"
    else
        fail "E03-T1-02(a): falta la decisión de algoritmo completa (elegido + EdDSA descartado + motivo Kong)"
    fi
    # (b) clave privada — PEM PKCS#8, validación de arranque análoga a JWT_SECRET, nunca en git
    sec="$(extract_section b "$adr")"
    if [[ -n "$sec" ]] \
       && grep -qE "PKCS?#?8" <<<"$sec" \
       && grep -qiE "validaci[oó]n de arranque|ValidateJWTSecret|log\.Fatal" <<<"$sec" \
       && grep -qiE "versiona|no versionado|nunca en git" <<<"$sec"; then
        echo "  PASS E03-T1-02(b): clave privada PEM PKCS#8 + validación de arranque + no versionada"
    else
        fail "E03-T1-02(b): falta formato PKCS#8, validación de arranque análoga a JWT_SECRET o la no-versionación"
    fi
    # (c) kid + rotación con solapamiento: publicar ANTES de firmar, gracia
    sec="$(extract_section c "$adr")"
    if [[ -n "$sec" ]] \
       && grep -qi "kid" <<<"$sec" \
       && grep -qiE "rotaci[oó]n" <<<"$sec" \
       && grep -qiE "solapamiento|gracia" <<<"$sec" \
       && grep -qi "antes" <<<"$sec"; then
        echo "  PASS E03-T1-02(c): esquema kid + rotación con solapamiento (publicar→firmar→gracia→retirar)"
    else
        fail "E03-T1-02(c): falta el esquema kid o la política de rotación con solapamiento"
    fi
    # (d) JWKS — ruta y array keys con kty/use/alg/kid + material público
    sec="$(extract_section d "$adr")"
    if [[ -n "$sec" ]] \
       && grep -qE "\.well-known|jwks\.json" <<<"$sec" \
       && grep -q '"keys"' <<<"$sec" \
       && grep -q '"kty"' <<<"$sec" && grep -q '"use"' <<<"$sec" \
       && grep -q '"alg"' <<<"$sec" && grep -q '"kid"' <<<"$sec"; then
        echo "  PASS E03-T1-02(d): forma del JWKS (ruta + keys con kty/use/alg/kid)"
    else
        fail "E03-T1-02(d): falta la ruta o la forma JSON del JWKS (keys + kty/use/alg/kid)"
    fi
    # (e) Kong — PEM estático por credencial + consecuencia del dual en el borde
    sec="$(extract_section e "$adr")"
    if [[ -n "$sec" ]] \
       && grep -q "rsa_public_key" <<<"$sec" \
       && grep -qiE "est[aá]tico" <<<"$sec" \
       && grep -q "key_claim_name" <<<"$sec" \
       && grep -qE "\b20\b" <<<"$sec"; then
        echo "  PASS E03-T1-02(e): Kong con PEM estático por credencial + blast radius del dual (key_claim_name, 20 bloques)"
    else
        fail "E03-T1-02(e): falta el PEM estático por credencial o la consecuencia del dual en el borde"
    fi
    # (f) cutover dual anclado al TTL de 15 min
    sec="$(extract_section f "$adr")"
    if [[ -n "$sec" ]] \
       && grep -qi "dual" <<<"$sec" \
       && grep -qE "15 min" <<<"$sec"; then
        echo "  PASS E03-T1-02(f): cutover dual anclado al TTL de 15 min del access token"
    else
        fail "E03-T1-02(f): falta la aceptación dual o el anclaje al TTL de 15 min"
    fi

    # --- E03-T1-03: las cinco secciones del formato ADR canónico
    local -a wanted=("Contexto" "Decisión" "Alternativas" "Consecuencias" "Revisión")
    local missing_fmt=""
    local w
    for w in "${wanted[@]}"; do
        grep -q "^## $w" "$adr" || missing_fmt+="$w "
    done
    if [[ -z "$missing_fmt" ]]; then
        echo "  PASS E03-T1-03: formato ADR canónico completo (Contexto, Decisión, Alternativas, Consecuencias, Revisión)"
    else
        fail "E03-T1-03: secciones del formato ausentes: $missing_fmt"
    fi

    # --- E03-T1-04: `docker exec lab-kong kong version` REGISTRADO en el ADR
    local registered=""
    registered="$(grep -A1 -m1 'docker exec lab-kong kong version' "$adr" \
        | sed -n '2p' | tr -d '[:space:]')"
    if [[ "$registered" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo "  PASS E03-T1-04: salida de 'docker exec lab-kong kong version' registrada: $registered"
        # Comparación viva: sólo si lab-kong está arriba. Un mismatch significa
        # que el ADR se verificó contra OTRO binario que el instalado.
        local live=""
        live="$(docker exec lab-kong kong version 2>/dev/null | head -1 | tr -d '[:space:]')"
        if [[ -z "$live" ]]; then
            echo "  SKIP E03-T1-04: lab-kong no accesible — no se compara la versión viva (presencia ya verificada)"
        elif [[ "$live" == "$registered" ]]; then
            echo "  PASS E03-T1-04: binario vivo coincide con el registrado ($live)"
        else
            fail "E03-T1-04: el ADR registra $registered pero lab-kong vivo es $live — prueba vencida, re-verificar §e"
        fi
    else
        fail "E03-T1-04: el ADR no registra la salida de 'docker exec lab-kong kong version' — la restricción quedó asumida, no probada"
    fi

    # --- E03-T1-05: evidencia de que ESA versión acepta RS256 + rsa_public_key
    sec="$(extract_section e "$adr")"
    if [[ -n "$sec" ]] \
       && grep -qiE "prueba viva|en vivo|descartable" <<<"$sec" \
       && grep -q "RS256" <<<"$sec" \
       && grep -q "rsa_public_key" <<<"$sec" \
       && grep -q "200" <<<"$sec" \
       && grep -q "401" <<<"$sec" \
       && grep -qi "alg:none" <<<"$sec" \
       && grep -qi "otra clave" <<<"$sec"; then
        echo "  PASS E03-T1-05: evidencia viva de credencial RS256+rsa_public_key aceptada (200 válido; 401 alg:none/otra clave)"
    else
        fail "E03-T1-05: falta la evidencia de la credencial RS256+rsa_public_key aceptada por esa versión (200/401 de la prueba viva)"
    fi

    # --- E03-T1-06: NEGATIVA — el ADR no afirma auto-descarga de JWKS
    local bad_download=""
    local line
    while IFS= read -r line; do
        if ! grep -qiE 'no hay|no auto|no la |no lo |no descarga|no puede|no est[aá]|nunca|sin |nada m[aá]s' <<<"$line"; then
            bad_download+="$line"
        fi
    done < <(grep -iE 'auto-?descarga|descarga.*(jwks|documento)|jwks.*descarga' "$adr")
    if [[ -z "$bad_download" ]]; then
        echo "  PASS E03-T1-06: ninguna afirmación de auto-descarga de JWKS (sólo negaciones)"
    else
        fail "E03-T1-06: el ADR AFIRMA que Kong descarga el JWKS: $bad_download"
    fi

    # --- E03-T1-07: checkboxes del gate L4 marcados
    local firmas
    firmas="$(firmas_section "$adr")"
    if [[ -z "$firmas" ]]; then
        fail "E03-T1-07: no existe la sección '## Firmas' con los checkboxes del gate"
    else
        local sec_go=0 own_go=0
        grep -qE '^- \[[xX]\].*@dev-security GO' <<<"$firmas" && sec_go=1
        grep -qE '^- \[[xX]\].*Owner GO' <<<"$firmas" && own_go=1
        if [[ "$sec_go" -eq 1 && "$own_go" -eq 1 ]]; then
            echo "  PASS E03-T1-07: @dev-security GO y Owner GO ambos marcados"
        else
            [[ "$sec_go" -eq 0 ]] && fail "E03-T1-07: checkbox @dev-security GO sin marcar — L4 sin firma, T1 falla (los marca el gate)"
            [[ "$own_go" -eq 0 ]] && fail "E03-T1-07: checkbox Owner GO sin marcar — L4 sin firma, T1 falla (los marca el gate)"
        fi
    fi

    return "$FAILURES"
}

# ---------------------------------------------------------------------------
# Controles negativos: mutar una COPIA del ADR y exigir que el check reaccione.
# ---------------------------------------------------------------------------
negative_controls() {
    local adr="$1"
    local tmp rc_all=0
    tmp="$(mktemp -d)"

    # NC1 — sin sección (e): borrar el header "### (e)..." → E03-T1-02(e) debe fallar
    sed '/^### (e)/d' "$adr" > "$tmp/nc1.md"
    if check_adr "$tmp/nc1.md" >/dev/null 2>&1; then
        echo "  FAIL NC1: el checker NO detectó la sección de decisión (e) borrada"
        rc_all=1
    else
        echo "  PASS NC1: sección (e) borrada → checker falla"
    fi

    # NC2 — prueba asumida: borrar el registro de `kong version` → E03-T1-04 debe fallar
    sed -e '/docker exec lab-kong kong version/d' \
        -e '/^[0-9]\+\.[0-9]\+\.[0-9]\+$/d' "$adr" > "$tmp/nc2.md"
    if check_adr "$tmp/nc2.md" >/dev/null 2>&1; then
        echo "  FAIL NC2: el checker NO detectó la ausencia del registro de 'kong version'"
        rc_all=1
    else
        echo "  PASS NC2: registro de kong version borrado → checker falla"
    fi

    # NC3 — la suposición falsa: afirmar auto-descarga → E03-T1-06 debe fallar
    { cat "$adr"; echo; echo "Kong auto-descarga el documento JWKS en runtime."; } > "$tmp/nc3.md"
    if check_adr "$tmp/nc3.md" >/dev/null 2>&1; then
        echo "  FAIL NC3: el checker NO detectó una afirmación de auto-descarga de JWKS"
        rc_all=1
    else
        echo "  PASS NC3: afirmación de auto-descarga inyectada → checker falla"
    fi

    # NC4 — formato roto: renombrar la sección Consecuencias → E03-T1-03 debe fallar
    sed 's/^## Consecuencias$/## ConsecuenciasX/' "$adr" > "$tmp/nc4.md"
    if check_adr "$tmp/nc4.md" >/dev/null 2>&1; then
        echo "  FAIL NC4: el checker NO detectó una sección del formato ADR ausente"
        rc_all=1
    else
        echo "  PASS NC4: sección de formato renombrada → checker falla"
    fi

    # NC5 — control INVERSO: copia con ambos checkboxes marcados debe pasar TODO.
    # Prueba que E03-T1-07 falla por las firmas ausentes, no por un grep roto.
    sed -e 's/^- \[ \] \*\*@dev-security GO\*\*/- [x] **@dev-security GO**/' \
        -e 's/^- \[ \] \*\*Owner GO\*\*/- [x] **Owner GO**/' "$adr" > "$tmp/nc5.md"
    if check_adr "$tmp/nc5.md" >/dev/null 2>&1; then
        echo "  PASS NC5: copia con ambas firmas marcadas → checker pasa todo (E03-T1-07 no es un falso negativo)"
    else
        echo "  FAIL NC5: el checker falla incluso con las firmas marcadas — algún check además de E03-T1-07 está roto"
        rc_all=1
    fi

    rm -rf "$tmp"
    return "$rc_all"
}

# ---------------------------------------------------------------------------
main() {
    if [[ ! -f "$ADR_FILE" ]]; then
        echo "e03_t1_adr_check: no encuentro el ADR: $ADR_FILE" >&2
        exit 2
    fi

    echo "== ACC-E03.T1 — contrato del ADR-003 (firma asimétrica + JWKS) =="
    echo "adr: $ADR_FILE"
    local -i rc_adr=0 rc_neg=0
    check_adr "$ADR_FILE" || rc_adr=1

    echo ""
    echo "== Controles negativos (el checker debe reaccionar ante cada mutación) =="
    negative_controls "$ADR_FILE" || rc_neg=1

    echo ""
    if [[ "$rc_adr" -ne 0 ]]; then
        echo "RESULTADO: FAIL — el ADR-003 NO satisface el contrato de T1 tal como está escrito"
        [[ "$rc_neg" -ne 0 ]] && echo "(además: el checker no demuestra sus negativas)"
        exit 1
    fi
    if [[ "$rc_neg" -ne 0 ]]; then
        echo "RESULTADO: FAIL — el checker no demuestra sus negativas"
        exit 1
    fi
    echo "RESULTADO: PASS — el ADR-003 satisface el contrato de T1 y el checker demuestra sus negativas"
    exit 0
}

main