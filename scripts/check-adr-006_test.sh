#!/usr/bin/env bash
# check-adr-006_test.sh — TEST de ACC-E09.T1 (inventario S2S versionado + ADR del token
# de servicio). Verifica el CONTRATO de la tarea tal como está escrito en la épica
# (ACC-E09-s2s-token-servicio-rs256.md, T1), NO lo que el ADR diga de sí mismo. Es un
# archivo de TEST: no toca producción; los fixtures van a un tmp fuera del repo.
#
# Criterios (derivados del "Contrato" y "Hecho cuando" de T1):
#   (a)  literal: `grep -c "^## " ADR-006` ≥ 5, y existen como headers `## ` las secciones
#        claims, registro, revocación, rotación y amenazas — falta una → falla.
#   (b)  Claims fija `iss`, `aud` (por servicio destino), `sub` (= servicio emisor),
#        `scope`, alcance de tenant, `exp` corto y `jti`. "Corto" se fija acá como
#        ≤ 900 s: un TTL mayor (o ausente) falla.
#   (c)  TTL y renovación escritos; registro reemplaza el `S2S_KEY_*` de ADR-002;
#        revocación = retiro del registro + expiración corta; rotación = la de ADR-003;
#        regla de `X-Tenant-ID` contra el claim con 4xx al discordar.
#   (d)  Amenazas: filas para servicio comprometido, token robado, confusión de `aud` y
#        replay; y declaran con código 4xx qué rechazan un `aud` ajeno, un token robado
#        tras revocar y un tenant discordante.
#   (R)  Restricción permanente (ACC-E03 T6): `alg` RS256 único y downgrade rechazado 4xx.
#   (e)  literal: `grep -n "S2S_API_KEY" ACC-E09-bitacora.md` lista CADA fila del
#        inventario y cada una tiene destino de migración (**T2**..**T8**).
#   (f)  revalidación contra el árbol vivo: todo archivo .go (no _test) que usa
#        `S2S_API_KEY` en código (no en comentario) bajo MC services/libs y platform/
#        aparece en una fila del inventario (servicio + archivo). Un emisor nuevo que el
#        inventario no lista → falla.
#   (g)  REPORT-ONLY: la aprobación de @dev-architect y @dev-security es el gate humano
#        (fase REVIEW). Se reporta el estado de las firmas del ADR y la escalación; no
#        cambia el exit code. Mientras diga "pendiente", T1 NO está hecha.
#
# Uso:
#   bash scripts/check-adr-006_test.sh               # verifica ADR + bitácora + árbol real
#   bash scripts/check-adr-006_test.sh --self-test   # fixtures negativas (el control falla
#                                                    # cuando el contrato no se cumple)
# Exit: 0 = contrato satisfecho; 1 = hallazgo; 2 = alcance incompleto (no finge pasar).

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LAB_ROOT="$(cd "$REPO_ROOT/../.." && pwd)"
DEFAULT_ADR="$REPO_ROOT/docs/adr/ADR-006-token-servicio-s2s.md"
DEFAULT_BITACORA="$LAB_ROOT/management/projects/account-service/epicas/ACC-E09-bitacora.md"
ESCALATION="$LAB_ROOT/management/escalations/2026-10-17_ACC-E09-T1.md"
TREE_ROOTS="${S2S_TREE_ROOTS:-$LAB_ROOT/active/mercado-cercano/services $LAB_ROOT/active/mercado-cercano/libs $LAB_ROOT/platform}"
TTL_MAX=900

FAILURES=0
fail() { echo "🔴 FALLA  — $*"; FAILURES=$((FAILURES+1)); }
pass() { echo "✅ $*"; }

# seccion <archivo> <regex del header> — desde '^## <hdr>' hasta el próximo '^## '
seccion() {
  awk -v hdr="^## $2" '$0 ~ hdr {p=1; next} /^## /{p=0} p' "$1"
}

# fila_amenaza <seccion> <regex> — la fila de tabla del modelo de amenazas que matchea
fila_amenaza() {
  grep -E '^\|' <<<"$1" | grep -iE "$2" | head -1
}

tiene_4xx() { grep -qE '\b4(0[0-9]|1[0-9]|2[0-9])\b' <<<"$1"; }

verificar_adr() {
  local adr="$1"
  echo "--- (ADR) contrato de ACC-E09.T1 sobre: $adr"
  if [[ ! -f "$adr" ]]; then
    fail "(a) el ADR no existe — T1 exige ADR-006"
    return
  fi

  # (a) conteo literal del "Hecho cuando" + las cinco secciones nombradas
  local n
  n="$(grep -c "^## " "$adr")"
  if (( n >= 5 )); then pass "(a) grep -c '^## ' = $n (≥ 5)"; else fail "(a) grep -c '^## ' = $n (< 5)"; fi
  local s
  for s in "Claims" "Registro" "Revocación" "Rotación" "(Modelo de a|A)menazas"; do
    if grep -qE "^## $s" "$adr"; then pass "(a) sección ## $s presente"
    else fail "(a) falta la sección ## $s"; fi
  done

  # (b) claims
  local claims c
  claims="$(seccion "$adr" "Claims")"
  for c in iss aud sub scope exp jti; do
    if grep -qE "^\| *\`$c\`|\`$c\`[^|]*\|" <<<"$(grep -E '^\|' <<<"$claims")"; then
      pass "(b) claim \`$c\` definido en la tabla de Claims"
    else
      fail "(b) claim \`$c\` no definido en la tabla de Claims"
    fi
  done
  if grep -E '^\| *`sub`' <<<"$claims" | grep -qi "emisor"; then pass "(b) \`sub\` = servicio emisor"
  else fail "(b) \`sub\` no se define como el servicio emisor"; fi
  if grep -E '^\| *`aud`' <<<"$claims" | grep -qiE "receptor|destino"; then pass "(b) \`aud\` por servicio destino"
  else fail "(b) \`aud\` no se define por servicio destino"; fi
  if grep -qE '`tenant_(scope|id)`' <<<"$claims" && grep -qiE '^### Alcance de tenant' <<<"$claims"; then
    pass "(b) alcance de tenant definido (claim + subsección)"
  else
    fail "(b) alcance de tenant no fijado en Claims"
  fi

  # (b) exp corto: todo TTL declarado en segundos debe existir y ser ≤ TTL_MAX
  local ttls t malo=0
  ttls="$(grep -oE 'exp = iat \+ [0-9]+ ?s|TTL del token: \**[0-9]+ ?s' "$adr" | grep -oE '[0-9]+ ?s$' | tr -d ' s')"
  if [[ -z "$ttls" ]]; then
    fail "(b) el ADR no fija un TTL numérico para \`exp\` (exp = iat + N s / TTL del token)"
  else
    for t in $ttls; do (( t > TTL_MAX )) && malo=1; done
    if (( malo )); then fail "(b) \`exp\` no es corto: TTL declarado(s) $(echo $ttls) s > $TTL_MAX s"
    else pass "(b) \`exp\` corto: TTL $(echo $ttls | tr ' ' '/') s ≤ $TTL_MAX s"; fi
  fi

  # (c) TTL y renovación
  if grep -qiE '^## .*renovaci' "$adr" || grep -qiE 'renueva' "$adr"; then pass "(c) renovación especificada"
  else fail "(c) no especifica la renovación del token"; fi

  # (c) registro reemplaza S2S_KEY_* de ADR-002
  local reg
  reg="$(seccion "$adr" "Registro")"
  if grep -q "ADR-002" <<<"$reg" && grep -q "S2S_KEY_" <<<"$reg" && grep -qiE "reemplaza" <<<"$reg"; then
    pass "(c) Registro reemplaza el registro S2S_KEY_* de ADR-002"
  else
    fail "(c) Registro no declara que reemplaza el S2S_KEY_* de ADR-002"
  fi
  if grep -qiE 'client_secret|secreto compartido' <<<"$reg" && ! grep -qiE 'sin rama de secreto compartido|nunca sale|sólo .*clave.*p[uú]blica|PUBLIC KEY' <<<"$reg"; then
    fail "(R) el Registro usa un secreto compartido para autenticar emisores"
  else
    pass "(R) el Registro no autentica emisores con secreto compartido"
  fi

  # (c) revocación: retiro del registro + expiración corta
  local rev
  rev="$(seccion "$adr" "Revocación")"
  if grep -qiE "retir(ar|o) (el|del) registro" <<<"$rev"; then pass "(c) revocación por retiro del registro"
  else fail "(c) revocación no contempla el retiro del registro"; fi
  if grep -qiE "expiraci[oó]n corta" <<<"$rev"; then pass "(c) revocación por expiración corta"
  else fail "(c) revocación no contempla la expiración corta"; fi

  # (c) rotación: misma que ADR-003
  if grep -q "ADR-003" <<<"$(seccion "$adr" "Rotación")"; then pass "(c) rotación referida a ADR-003"
  else fail "(c) la Rotación no se ata a ADR-003"; fi

  # (c) regla de X-Tenant-ID contra el claim, con 4xx al discordar
  local disc
  disc="$(grep -E 'X-Tenant-ID|^\|' "$adr" | grep -E '≠|discordante|mismatch' | grep -vE '^\| \*\*(Servicio|Tenant discordante)' )"
  if [[ -n "$disc" ]] && tiene_4xx "$disc"; then pass "(c) regla de X-Tenant-ID: discordante → 4xx"
  else fail "(c) la regla de X-Tenant-ID no rechaza con 4xx un tenant discordante"; fi

  # (d) modelo de amenazas
  local am fila
  am="$(seccion "$adr" "(Modelo de a|A)menazas")"
  for s in "servicio comprometido" "token robado" "confusi[oó]n de .?aud" "replay"; do
    fila="$(fila_amenaza "$am" "$s")"
    if [[ -n "$fila" ]]; then pass "(d) amenaza '$s' modelada"
    else fail "(d) amenaza '$s' ausente del modelo de amenazas"; fi
  done
  fila="$(fila_amenaza "$am" "confusi[oó]n de .?aud")"
  if grep -qiE "ajeno" <<<"$fila" && tiene_4xx "$fila"; then pass "(d) \`aud\` ajeno → 4xx declarado"
  else fail "(d) no declara con 4xx el rechazo de un \`aud\` ajeno"; fi
  fila="$(fila_amenaza "$am" "token robado")"
  if grep -qiE "revoca" <<<"$fila" && tiene_4xx "$fila"; then pass "(d) token robado tras revocar → 4xx declarado"
  else fail "(d) no declara con 4xx el rechazo de un token robado tras revocar"; fi
  fila="$(fila_amenaza "$am" "tenant discordante")"
  if tiene_4xx "$fila"; then pass "(d) tenant discordante → 4xx declarado"
  else fail "(d) no declara con 4xx el rechazo de un tenant discordante"; fi

  # (R) RS256 único y downgrade rechazado
  if grep -qE '`alg: RS256`.*único' "$adr"; then pass "(R) alg RS256 único"
  else fail "(R) no fija RS256 como único alg aceptado"; fi
  fila="$(fila_amenaza "$am" "downgrade")"
  if grep -q "HS256" <<<"$fila" && tiene_4xx "$fila"; then pass "(R) downgrade (HS256/none) → 4xx"
  else fail "(R) no declara el rechazo 4xx de un downgrade a HS256/none"; fi
}

# filas del inventario: '| <n> | ...'
filas_inventario() { grep -E '^\| *[0-9]+ *\|' "$1"; }

verificar_bitacora() {
  local bit="$1"
  echo "--- (bitácora) inventario de ACC-E09.T1 sobre: $bit"
  if [[ ! -f "$bit" ]]; then fail "(e) la bitácora no existe"; return; fi
  local filas total con_key sin_dest
  filas="$(filas_inventario "$bit")"
  total="$(grep -c . <<<"$filas")"
  [[ -z "$filas" ]] && total=0
  if (( total == 0 )); then fail "(e) la bitácora no tiene tabla de llamadas"; return; fi
  # literal del Hecho cuando: el grep de S2S_API_KEY debe listar cada llamada
  con_key="$(grep -n "S2S_API_KEY" "$bit" | grep -cE '^[0-9]+:\| *[0-9]+ *\|')"
  if (( con_key == total )); then pass "(e) grep -n S2S_API_KEY lista las $total llamadas del inventario"
  else fail "(e) grep -n S2S_API_KEY lista $con_key de $total llamadas (filas sin la credencial declarada)"; fi
  sin_dest="$(grep -vE '\*\*T[2-8][ab]?\*\*' <<<"$filas" | grep -oE '^\| *[0-9]+' | tr -d '| ' | paste -sd, -)"
  if [[ -z "$sin_dest" ]]; then pass "(e) cada llamada tiene destino de migración (T2..T8)"
  else fail "(e) filas sin destino de migración: $sin_dest"; fi
}

verificar_arbol() {
  local bit="$1" roots="$2"
  echo "--- (árbol) revalidación del inventario contra: $roots"
  local filas root f rel svc corto base hallados=0
  filas="$(filas_inventario "$bit" 2>/dev/null)"
  for root in $roots; do
    if [[ ! -d "$root" ]]; then echo "scope incompleto: no existe $root" >&2; exit 2; fi
  done
  while IFS= read -r f; do
    [[ -z "$f" ]] && continue
    hallados=$((hallados+1))
    for root in $roots; do
      [[ "$f" == "$root/"* ]] && { rel="${f#$root/}"; break; }
    done
    svc="${rel%%/*}"; corto="${svc%-service}"; base="$(basename "$f")"
    if [[ "$base" == "main.go" ]]; then
      if grep -qi -- "$corto" <<<"$filas"; then pass "(f) $svc/main.go: emisor $corto en el inventario"
      else fail "(f) $svc/main.go usa S2S_API_KEY y el emisor $corto no está en el inventario"; fi
    elif grep -i -- "$corto" <<<"$filas" | grep -qF -- "$base"; then
      pass "(f) $rel en el inventario"
    else
      fail "(f) $rel usa S2S_API_KEY y no figura en el inventario de la bitácora"
    fi
  done < <(for root in $roots; do
             grep -rlw --include='*.go' "S2S_API_KEY" "$root" 2>/dev/null
           done | grep -v '_test\.go$' | sort -u | while IFS= read -r g; do
             # sólo si hay al menos un uso fuera de comentario
             grep -w "S2S_API_KEY" "$g" | grep -qvE '^\s*//' && echo "$g"
           done)
  if (( hallados == 0 )); then
    fail "(f) no se encontró ningún uso de S2S_API_KEY en el árbol — alcance sospechoso, no se puede revalidar"
  fi
}

report_firmas() {
  local adr="$1"
  echo "--- (g) REPORT-ONLY — gate L4 (no cambia el exit code)"
  if [[ -f "$ESCALATION" ]]; then echo "   escalación presente: ${ESCALATION#$LAB_ROOT/}"
  else echo "   ⚠️  escalación ausente: ${ESCALATION#$LAB_ROOT/}"; fi
  local firmas
  firmas="$(seccion "$adr" "Firmas" | grep -E '@dev-(architect|security)')"
  if [[ -z "$firmas" ]]; then echo "   ⚠️  el ADR no tiene sección de firmas de @dev-architect/@dev-security"; return; fi
  echo "$firmas" | sed 's/^/   /'
  if grep -qiE 'pendiente' <<<"$firmas"; then
    echo "   ⏳ gate L4 PENDIENTE — el 'Hecho cuando' exige la aprobación antes de T2; T1 no está hecha"
  fi
}

self_test() {
  local tmp; tmp="$(mktemp -d)"
  cp "$DEFAULT_ADR" "$tmp/ok.md"
  cp "$DEFAULT_BITACORA" "$tmp/bit-ok.md"

  # N1: ADR sin la sección de amenazas
  sed 's/^## Modelo de amenazas/### (sin modelo)/' "$tmp/ok.md" > "$tmp/sin-amenazas.md"
  # N2: la fila de confusión de aud no declara el 4xx
  sed '/^| \*\*Confusión de `aud`/ s/\*\*401\*\*/rechazo/' "$tmp/ok.md" > "$tmp/aud-sin-4xx.md"
  # N3: tenant discordante sin 4xx (regla de X-Tenant-ID y fila de amenazas)
  sed -E '/(≠|discordante|mismatch)/ s/\*\*403\*\*|403/se loguea/g' "$tmp/ok.md" > "$tmp/tenant-sin-4xx.md"
  # N4: exp largo (24 h)
  sed -E 's/exp = iat \+ 300 s/exp = iat + 86400 s/; s/TTL del token: \*\*300 s\*\*/TTL del token: **86400 s**/' "$tmp/ok.md" > "$tmp/exp-largo.md"
  # N5: falta el claim jti
  sed '/^| `jti`/d' "$tmp/ok.md" > "$tmp/sin-jti.md"
  # N6: token robado sin rechazo tras revocar
  sed -E '/^\| \*\*Token robado/ s/\*\*401\*\*|401/aceptado/g' "$tmp/ok.md" > "$tmp/robado-sin-4xx.md"
  # N7: bitácora con una fila sin destino de migración
  sed -E '0,/^\| 3 \|/ s/(^\| 3 \|.*)\*\*T7a\*\*/\1(a definir)/' "$tmp/bit-ok.md" > "$tmp/bit-sin-dest.md"
  # N8: árbol con un emisor que el inventario no lista
  mkdir -p "$tmp/tree/ghost-service/src/client"
  printf 'package client\n\nvar k = os.Getenv("S2S_API_KEY")\n' > "$tmp/tree/ghost-service/src/client/ghost_client.go"
  # P8: el mismo árbol sólo con un comentario no cuenta como uso
  mkdir -p "$tmp/tree-ok/sales-service/src"
  printf 'package x\n// antes había S2S_API_KEY\n' > "$tmp/tree-ok/sales-service/src/doc.go"
  mkdir -p "$tmp/tree-ok/sales-service/src/client"
  printf 'package client\nvar k = os.Getenv("S2S_API_KEY")\n' > "$tmp/tree-ok/sales-service/src/client/pim_client.go"

  local st_fail=0 caso tipo arg esperado desc resto out hallados
  for caso in "adr:ok.md:0:fixture válida (ADR real) pasa" \
              "adr:sin-amenazas.md:1:negativa: ADR sin modelo de amenazas" \
              "adr:aud-sin-4xx.md:1:negativa: aud ajeno sin 4xx declarado" \
              "adr:tenant-sin-4xx.md:1:negativa: tenant discordante sin 4xx" \
              "adr:exp-largo.md:1:negativa: exp de 24 h no es corto" \
              "adr:sin-jti.md:1:negativa: falta el claim jti" \
              "adr:robado-sin-4xx.md:1:negativa: token robado tras revocar sin 4xx" \
              "adr:no-existe.md:1:negativa: ADR ausente" \
              "bit:bit-ok.md:0:bitácora real pasa" \
              "bit:bit-sin-dest.md:1:negativa: fila de inventario sin destino" \
              "tree:tree:1:negativa: emisor S2S_API_KEY fuera del inventario" \
              "tree:tree-ok:0:uso inventariado + comentario no cuentan como hallazgo"; do
    tipo="${caso%%:*}"; resto="${caso#*:}"
    arg="${resto%%:*}"; resto="${resto#*:}"
    esperado="${resto%%:*}"; desc="${resto#*:}"
    case "$tipo" in
      adr)  out="$(FAILURES=0; verificar_adr "$tmp/$arg")" ;;
      bit)  out="$(FAILURES=0; verificar_bitacora "$tmp/$arg")" ;;
      tree) out="$(FAILURES=0; verificar_arbol "$tmp/bit-ok.md" "$tmp/$arg")" ;;
    esac
    hallados="$(grep -c '🔴 FALLA' <<<"$out")"
    if { (( esperado == 0 )) && (( hallados == 0 )); } || { (( esperado == 1 )) && (( hallados >= 1 )); }; then
      echo "✅ self-test: $desc"
      [[ -n "${SELF_TEST_VERBOSE:-}" ]] && echo "$out" | grep '🔴' | sed 's/^/      /'
    else
      echo "🔴 self-test FALLA: $desc (hallazgos=$hallados, esperado=$esperado)"
      echo "$out" | grep -E '🔴|FALLA' | sed 's/^/      /'
      st_fail=$((st_fail+1))
    fi
  done
  if (( st_fail > 0 )); then
    echo "🔴 self-test: $st_fail caso(s) no se comportan como el contrato exige"
    rm -rf "$tmp"
    return 1
  fi
  echo "✅ self-test: 12 casos según el contrato (9 negativos)"
  rm -rf "$tmp"
}

main() {
  if [[ "${1:-}" == "--self-test" ]]; then
    self_test
    return $?
  fi
  verificar_adr "$DEFAULT_ADR"
  verificar_bitacora "$DEFAULT_BITACORA"
  verificar_arbol "$DEFAULT_BITACORA" "$TREE_ROOTS"
  report_firmas "$DEFAULT_ADR"
  echo "----------------------------------------"
  if (( FAILURES == 0 )); then
    echo "RESULTADO: contrato documental de ACC-E09.T1 satisfecho (gate L4 aparte, ver (g))"
    return 0
  fi
  echo "RESULTADO: $FAILURES hallazgo(s) contra el contrato de ACC-E09.T1"
  return 1
}

main "$@"
