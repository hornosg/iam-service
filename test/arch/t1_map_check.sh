#!/usr/bin/env bash
# t1_map_check.sh — Verificación ejecutable del contrato de ACC-E01 T1
# (mapa de reorganización confirmado) contra el árbol vivo de iam-service.
#
# POR QUÉ UN SCRIPT Y NO UN TEST GO: ACC-E01.T2 congela
# `grep -rh "^func Test" --include="*_test.go" . | wc -l` → 401 (medido 2026-09-04).
# Un `func Test` nuevo en este repo movería ese conteo y rompería el criterio (a)
# de T2 ("idéntico al func_test_count registrado"). Este check no agrega funciones Go.
#
# Qué verifica (el contrato de T1 tal como está escrito en la épica):
#   T1-MAP-01  una fila por cada .go bajo src/: biyección exacta filas↔archivos
#              (mismo conteo, sin huecos, sin filas fantasma, sin duplicadas).
#              "Un mapa con agujeros no es un mapa, es una suposición."
#   T1-MAP-02  cada archivo de src/auth tiene destino explícito identity|access.
#   T1-MAP-03  cada función `func Setup*` viva bajo src/ tiene fila en la tabla
#              de wiring del mapa ("una fila por función").
#   T1-MAP-04  ningún archivo tiene destino onboarding|subscription (quedan
#              declarados VACÍOS por diseño).
#
# Controles negativos (parte del entregable, como en T6 de la propia épica):
#   un checker que nunca falló no prueba nada. El script se auto-verifica contra
#   copias doctoreadas de la épica en un tmpdir: quitar una fila, inventar una
#   fantasma, mandar un archivo de src/auth a `plans` y borrar una fila de
#   Setup — cada mutación DEBE hacer fallar el check. La épica real nunca se toca.
#
# Uso:
#   ./test/arch/t1_map_check.sh
#   T1_EPIC_FILE=/otra/ruta/a/la/epica.md ./test/arch/t1_map_check.sh
#
# Exit 0 si todo pasa (incl. controles negativos); 1 si cualquier check falla.

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EPIC_FILE="${T1_EPIC_FILE:-${DEVY_PATH:-$HOME/Projects}/management/projects/account-service/epicas/ACC-E01-fundacion-monolito-modular.md}"

FAILURES=0

# ---------------------------------------------------------------------------
# Núcleo: corre T1-MAP-01..04 para un archivo de épica dado.
# Imprime PASS/FAIL por check y detalles de lo que no cerró.
# Retorna 0 sólo si todos los checks pasan.
# ---------------------------------------------------------------------------
check_map() {
    local epic="$1" root="$2"
    local -i rc=0

    # --- Extraer la sección «Mapa de reorganización (confirmado)»
    if ! grep -q '^## Mapa de reorganización (confirmado)$' "$epic"; then
        echo "  FAIL: la épica no tiene sección '## Mapa de reorganización (confirmado)'"
        return 1
    fi
    local section
    section="$(sed -n '/^## Mapa de reorganización (confirmado)$/,$p' "$epic")"

    local tmp
    tmp="$(mktemp -d)"

    # Filas de archivo `| `src/...go` | destino | nota |` → TSV path<TAB>destino,
    # sin backticks ni espacios de borde (comparable con la salida de find).
    printf '%s\n' "$section" | grep -E '^\| `src/.*\.go` \|' \
        | awk -F'|' '{gsub(/`/, "", $2); gsub(/^[ \t]+|[ \t]+$/, "", $2);
                      gsub(/^[ \t]+|[ \t]+$/, "", $3); print $2"\t"$3}' \
        > "$tmp/map_rows.tsv"

    (cd "$root" && find src -name '*.go') | sort > "$tmp/live.txt"
    awk -F'\t' '{print $1}' "$tmp/map_rows.tsv" | sort > "$tmp/rows.txt"

    # --- T1-MAP-01: conteo + biyección
    local -i live_n row_n
    live_n=$(wc -l < "$tmp/live.txt")
    row_n=$(wc -l < "$tmp/rows.txt")
    if [[ "$live_n" -ne "$row_n" ]]; then
        echo "  FAIL T1-MAP-01: conteo no cierra — archivos vivos: $live_n, filas del mapa: $row_n"
        rc=1
    fi
    local holes phantoms dupes
    holes=$(comm -23 "$tmp/live.txt" "$tmp/rows.txt" | sed 's/^/    sin fila: /')
    phantoms=$(comm -13 "$tmp/live.txt" "$tmp/rows.txt" | sed 's/^/    fila sin archivo vivo: /')
    dupes=$(uniq -d "$tmp/rows.txt" | sed 's/^/    duplicada: /')
    if [[ -n "$holes$phantoms$dupes" ]]; then
        echo "  FAIL T1-MAP-01: el mapa no es biyectivo con el árbol"
        [[ -n "$holes" ]]    && echo "$holes"
        [[ -n "$phantoms" ]] && echo "$phantoms"
        [[ -n "$dupes" ]]    && echo "$dupes"
        rc=1
    elif [[ "$rc" -eq 0 ]]; then
        echo "  PASS T1-MAP-01: $row_n filas ↔ $live_n archivos, biyección exacta"
    fi

    # --- T1-MAP-02: destino explícito identity|access para todo src/auth
    local -i auth_n=0 bad_auth=0 live_auth
    while IFS=$'\t' read -r path dest; do
        case "$path" in
            src/auth/*)
                auth_n+=1
                if [[ "$dest" != "identity" && "$dest" != "access" ]]; then
                    echo "  FAIL T1-MAP-02: $path → destino '$dest' (debe ser identity|access)"
                    bad_auth+=1
                fi
                ;;
        esac
    done < "$tmp/map_rows.tsv"
    # cada archivo vivo de src/auth debe tener fila (cubierto por MAP-01; el
    # contrato lo pide explícito, así que se re-chequea barato)
    live_auth=$(grep -c '^src/auth/' "$tmp/live.txt" || true)
    if [[ "$auth_n" -ne "$live_auth" ]]; then
        echo "  FAIL T1-MAP-02: src/auth vivo: $live_auth archivos, con fila y destino: $auth_n"
        rc=1
    fi
    if [[ "$bad_auth" -eq 0 && "$auth_n" -eq "$live_auth" && "$auth_n" -gt 0 ]]; then
        echo "  PASS T1-MAP-02: $auth_n archivos de src/auth con destino explícito (identity|access)"
    else
        rc=1
    fi

    # --- T1-MAP-03: cada `func Setup*` vivo tiene fila en la tabla de wiring
    (cd "$root" && grep -rhoE '^func Setup[A-Za-z0-9]+\(' src --include='*.go' \
        | sed 's/^func //; s/($//; s/(//') | sort -u > "$tmp/live_setup.txt"
    printf '%s\n' "$section" | grep -E '^\| `Setup[A-Za-z0-9]+` \|' \
        | awk -F'|' '{gsub(/`/, "", $2); gsub(/^[ \t]+|[ \t]+$/, "", $2); print $2}' | sort -u > "$tmp/setup_rows.txt"
    local missing_setup
    missing_setup=$(comm -23 "$tmp/live_setup.txt" "$tmp/setup_rows.txt" | sed 's/^/    sin fila en la tabla de wiring: /')
    if [[ -n "$missing_setup" ]]; then
        echo "  FAIL T1-MAP-03: función(es) Setup* del árbol sin fila en el mapa"
        echo "$missing_setup"
        rc=1
    else
        echo "  PASS T1-MAP-03: toda función Setup* del árbol tiene fila de wiring"
    fi

    # --- T1-MAP-04: nada destinado a onboarding|subscription (declarados vacíos)
    local -i vacios
    vacios=$(awk -F'\t' '$2 == "onboarding" || $2 == "subscription"' "$tmp/map_rows.tsv" | wc -l)
    if [[ "$vacios" -ne 0 ]]; then
        echo "  FAIL T1-MAP-04: $vacios archivo(s) con destino onboarding|subscription — deben quedar VACÍOS"
        rc=1
    else
        echo "  PASS T1-MAP-04: ningún archivo destinado a onboarding|subscription"
    fi

    rm -rf "$tmp"
    return "$rc"
}

# ---------------------------------------------------------------------------
# Controles negativos: mutar una COPIA de la épica y exigir que el check falle.
# ---------------------------------------------------------------------------
negative_controls() {
    local epic="$1" root="$2"
    local tmp rc_all=0
    tmp="$(mktemp -d)"

    # N1 — hueco: borrar una fila de archivo → T1-MAP-01 debe fallar
    sed '/^| `src\/auth\/domain\/entity\/refresh_token.go` |/d' "$epic" > "$tmp/n1.md"
    if check_map "$tmp/n1.md" "$root" >/dev/null 2>&1; then
        echo "  FAIL N1: el checker NO detectó una fila de archivo borrada (hueco)"
        rc_all=1
    else
        echo "  PASS N1: fila borrada → checker falla"
    fi

    # N2 — fantasma: inventar una fila de archivo inexistente → T1-MAP-01 debe fallar.
    # El `echo` inicial separa: si la épica no termina en newline, cat pegaría la
    # fila fantasma al último párrafo y `^\|` no la vería — mutación nula.
    { cat "$epic"; echo; echo '| `src/shared/fantasma_test.go` | identity | fila inventada |'; } > "$tmp/n2.md"
    if check_map "$tmp/n2.md" "$root" >/dev/null 2>&1; then
        echo "  FAIL N2: el checker NO detectó una fila fantasma"
        rc_all=1
    else
        echo "  PASS N2: fila fantasma → checker falla"
    fi

    # N3 — corte violado: mandar un archivo de src/auth a `plans` → T1-MAP-02 debe fallar
    sed 's/^| `src\/auth\/application\/usecase\/login.go` | identity |/| `src\/auth\/application\/usecase\/login.go` | plans |/' "$epic" > "$tmp/n3.md"
    if check_map "$tmp/n3.md" "$root" >/dev/null 2>&1; then
        echo "  FAIL N3: el checker NO detectó destino inválido para un archivo de src/auth"
        rc_all=1
    else
        echo "  PASS N3: src/auth con destino que no es identity|access → checker falla"
    fi

    # N4 — wiring: borrar la fila de un Setup* llamado por el wiring → T1-MAP-03 debe fallar
    sed '/^| `SetupAuthModule` |/d' "$epic" > "$tmp/n4.md"
    if check_map "$tmp/n4.md" "$root" >/dev/null 2>&1; then
        echo "  FAIL N4: el checker NO detectó una función de wiring sin fila"
        rc_all=1
    else
        echo "  PASS N4: función Setup* sin fila en el mapa → checker falla"
    fi

    rm -rf "$tmp"
    return "$rc_all"
}

# ---------------------------------------------------------------------------
main() {
    if [[ ! -f "$EPIC_FILE" ]]; then
        echo "t1_map_check: no encuentro la épica: $EPIC_FILE" >&2
        exit 2
    fi
    if [[ ! -d "$REPO_ROOT/src" ]]; then
        echo "t1_map_check: no encuentro src/ bajo $REPO_ROOT" >&2
        exit 2
    fi

    # Guard post-T4 (ACC-E01.T4, 2026-09-04): este check valida la biyección
    # mapa↔árbol PRE-mudanza (las filas listan paths viejos: src/auth, src/plan,
    # ...). Tras ejecutado el mapa, src/auth ya no existe y el check no tiene
    # sujeto. No es un FAIL: el mapa se ejecutó exactamente como estaba escrito.
    # El sucesor de este check post-mudanza es el test de arquitectura de T6
    # (fronteras entre módulos). Sigue corriendo completo en árboles pre-T4
    # (worktrees/branches históricas).
    if [[ ! -d "$REPO_ROOT/src/auth" ]]; then
        echo "SKIP ACC-E01.T1: el árbol ya está post-mudanza (T4 ejecutó el mapa;"
        echo "src/auth no existe). Sucesor: test de arquitectura de T6."
        exit 0
    fi

    echo "== ACC-E01.T1 — mapa de reorganización (confirmado) vs árbol vivo =="
    echo "épica: $EPIC_FILE"
    local -i rc_map=0 rc_neg=0
    check_map "$EPIC_FILE" "$REPO_ROOT" || rc_map=1

    echo ""
    echo "== Controles negativos (el checker debe fallar ante cada mutación) =="
    negative_controls "$EPIC_FILE" "$REPO_ROOT" || rc_neg=1

    echo ""
    if [[ "$rc_map" -ne 0 ]]; then
        echo "RESULTADO: FAIL — el mapa NO satisface el contrato de T1 tal como está escrito"
        exit 1
    fi
    if [[ "$rc_neg" -ne 0 ]]; then
        echo "RESULTADO: FAIL — el checker no demuestra sus negativas"
        exit 1
    fi
    echo "RESULTADO: PASS — el mapa satisface el contrato de T1 y el checker demuestra sus negativas"
    exit 0
}

main