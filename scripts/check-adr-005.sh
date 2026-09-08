#!/usr/bin/env bash
# check-adr-005.sh — TEST de ACC-E04.T1: verifica el contrato del ADR-005 tal como
# está escrito en la épica (ACC-E04-tenancy-absorber-tenant-config.md, T1), NO lo que
# el ADR haga. Es un archivo de TEST: no toca producción.
#
# Criterios verificados (derivados del "Contrato" y "Hecho cuando" de T1):
#   (a) el ADR existe y cubre las tres decisiones DD-1/DD-2/DD-3, cada una con su
#       bloque de alternativas (≥2 evaluadas con criterio de elección/rechazo);
#       un ADR al que le falte cualquiera de los DD falla la tarea.
#   (a') DD-1 especifica la firma exacta del gate S2S (scope `config:read`,
#        `Authorize`/`RequireScope`) y del seteo de `app.tenant_id`
#        (`X-Tenant-ID` → `WithRLSInTransaction` dentro de la TX del repo).
#   (a'') DD-3 especifica el DDL objetivo de la unicidad: UNIQUE(namespace, slug).
#   (c)  NEGATIVA de proceso: si el ADR elige la opción (b) de DD-1 (S2S
#        gated-escape), DEBE especificar que `app.tenant_id` se fija sólo tras el
#        gate de scope — secuencia numerada gate→X-Tenant-ID→WithTenantID→TX y
#        sección "Por qué no es USING(true) encubierto". Un escape sin gate
#        escrito es `USING(true)` encubierto y este check lo RECHAZA.
#   (b)  REPORT-ONLY: el veredicto @dev-security + owner es el gate humano de la
#        fase REVIEW; este check sólo verifica que el registro del proceso existe
#        (escalación + bitácora) y reporta el estado de las firmas.
#
# Uso:
#   bash scripts/check-adr-005.sh               # verifica el ADR real
#   bash scripts/check-adr-005.sh <archivo>      # verifica un ADR arbitrario
#   bash scripts/check-adr-005.sh --self-test    # corre fixtures negativas (TEST)
#
# Exit: 0 = contrato satisfecho; ≠0 = hallazgo (se lista el criterio que falló).

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEFAULT_ADR="$REPO_ROOT/docs/adr/ADR-005-tenancy-config-namespace.md"
ESCALATION="$REPO_ROOT/../../management/escalations/2026-09-07_ACC-E04-T1-signoff.md"
BITACORA="$REPO_ROOT/../../management/projects/account-service/epicas/ACC-E04-bitacora.md"

FAILURES=0
fail()   { echo "🔴 FALLA  — $*"; FAILURES=$((FAILURES+1)); }
pass()   { echo "✅ $*"; }

# extrae_seccion <archivo> <header> — desde '^## <header>' hasta el próximo '^## '
extrae_seccion() {
  awk -v hdr="^## $2" '$0 ~ hdr {p=1; next} /^## /{p=0} p' "$1"
}

# extrae_alternativas <seccion> — desde 'Alternativas consideradas' hasta '### ' o fin
extrae_alternativas() {
  awk '/Alternativas consideradas/{p=1; next} /^### /{p=0} p' <<<"$1"
}

# verificar_adr <archivo> — todas las aserciones del contrato. Solo escribe a stdout.
verificar_adr() {
  local adr="$1"
  echo "--- Verificando contrato de ACC-E04.T1 sobre: $adr"

  # (a) existencia
  if [[ ! -f "$adr" ]]; then
    fail "(a) el archivo ADR no existe — el contrato exige un ADR con las tres decisiones"
    return
  fi
  pass "(a) el archivo ADR existe"

  # (a) las tres decisiones, una por una: falta cualquiera → falla la tarea
  local total
  total="$(grep -cE '^## (DD-1|DD-2|DD-3|Alternativas|Decisión)' "$adr")"
  local dd
  for dd in DD-1 DD-2 DD-3; do
    if grep -qE "^## $dd" "$adr"; then
      pass "(a) sección ## $dd presente"
    else
      fail "(a) falta la sección ## $dd — un ADR al que le falte cualquiera de DD-1/DD-2/DD-3 FALLA la tarea (un diseño con agujeros no es un diseño)"
    fi
  done
  echo "   (grep de la épica '^## (DD-1|DD-2|DD-3|Alternativas|Decisión)' → $total; esperado ≥ 4: Decisión + 3 DDs)"
  if (( total < 4 )); then
    fail "(a) el conteo de headers de decisión ($total) no cubre Decisión + los tres DDs"
  fi

  # (a) cada DD con su bloque de alternativas: ≥2 evaluadas con criterio
  for dd in DD-1 DD-2 DD-3; do
    local sec alt nalt ncrit
    sec="$(extrae_seccion "$adr" "$dd")"
    if [[ -z "$sec" ]]; then continue; fi   # ya se reportó arriba
    alt="$(extrae_alternativas "$sec")"
    if [[ -z "$alt" ]]; then
      fail "(a) $dd no tiene bloque de alternativas ('Alternativas consideradas')"
      continue
    fi
    nalt="$(grep -cE '^ *- \*\*' <<<"$alt")"
    ncrit="$(grep -ciE 'rechazada|elegida|descartada' <<<"$alt")"
    if (( nalt >= 2 )); then
      pass "(a) $dd evalúa $nalt alternativas"
    else
      fail "(a) $dd evalúa sólo $nalt alternativa(s) — el contrato exige ≥2 evaluadas con criterio de elección"
    fi
    if (( ncrit >= 1 )); then
      pass "(a) $dd registra criterio de elección/rechazo en sus alternativas"
    else
      fail "(a) las alternativas de $dd no registran cuál fue elegida ni por qué se rechazaron las otras"
    fi
  done

  local dd1
  dd1="$(extrae_seccion "$adr" "DD-1")"

  # (a') firma exacta del gate S2S y del seteo de app.tenant_id
  local firma
  for firma in 'config:read' 'Authorize|RequireScope' 'app\.tenant_id' 'X-Tenant-ID' 'WithRLSInTransaction'; do
    if grep -qE "$firma" <<<"$dd1"; then
      pass "(a') DD-1 especifica la firma: $firma"
    else
      fail "(a') DD-1 no especifica la firma exigida por el contrato: $firma (gate S2S / seteo de app.tenant_id)"
    fi
  done

  # (a'') DDL objetivo de la unicidad en DD-3
  local dd3
  dd3="$(extrae_seccion "$adr" "DD-3")"
  if grep -qiE 'UNIQUE\s*\(\s*namespace\s*,\s*slug' <<<"$dd3"; then
    pass "(a'') DD-3 fija el DDL objetivo de unicidad: UNIQUE(namespace, slug)"
  else
    fail "(a'') DD-3 no fija el DDL objetivo de la unicidad UNIQUE(namespace, slug)"
  fi

  # (c) NEGATIVA de proceso: si elige (b), el gate debe estar ESCRITO
  if grep -qiE '\(b\).*(S2S|scoped read).*elegida|elegida.*S2S scoped read|opción \(b\)' <<<"$dd1" \
     || grep -qE 'S2S scoped read' <<<"$dd1"; then
    # elige (b): exigir el gate escrito — orden, secuencia, y sección USING(true)
    if grep -iE '(sólo tras|sólo después|solo tras).*(gate|scope|Authorize)' <<<"$dd1" | grep -q .; then
      pass "(c) DD-1 especifica que app.tenant_id se fija SÓLO TRAS el gate de scope"
    else
      fail "(c) NEGATIVA: DD-1 elige el S2S gated-escape sin especificar que app.tenant_id se fija sólo tras el gate — un escape sin gate escrito es USING(true) encubierto; la revisión de seguridad debe rechazarlo"
    fi
    local nsteps
    nsteps="$(grep -cE '^[0-9]+ *\.' <<<"$dd1")"
    if (( nsteps >= 3 )); then
      pass "(c) DD-1 escribe la secuencia numerada del handler ($nsteps pasos: gate → X-Tenant-ID → WithTenantID → TX)"
    else
      fail "(c) NEGATIVA: DD-1 elige (b) sin la secuencia numerada gate→X-Tenant-ID→WithTenantID→TX ($nsteps pasos) — el orden ES el control"
    fi
    if grep -qF 'USING(true)' <<<"$dd1"; then
      pass "(c) DD-1 tiene la sección dedicada 'Por qué no es USING(true) encubierto'"
    else
      fail "(c) NEGATIVA: DD-1 elige (b) sin la sección 'Por qué no es USING(true) encubierto' — el escape queda sin justificación de seguridad"
    fi
  else
    echo "   (DD-1 no declara opción (b); la negativa (c) no aplica)"
  fi
}

# report_proceso — criterio (b): registro del gate humano (REPORT-ONLY, no aserta el GO)
report_proceso() {
  echo "--- Criterio (b) — veredicto @dev-security + owner (fase REVIEW, report-only)"
  if [[ -f "$ESCALATION" ]]; then
    pass "(b) existe la escalación del gate: management/escalations/2026-09-07_ACC-E04-T1-signoff.md"
  else
    fail "(b) no existe la escalación del gate en management/escalations/ — el veredicto L4 no tiene dónde quedar"
  fi
  if [[ -f "$BITACORA" ]] && grep -qE '^## T1' "$BITACORA"; then
    pass "(b) la bitácora ACC-E04 registra la entrada de T1"
  else
    fail "(b) la bitácora ACC-E04 no registra T1 — el veredicto L4 no vive en el archivo de la épica (D-10)"
  fi
  local adr="${1:-$DEFAULT_ADR}"
  if [[ -f "$adr" ]]; then
    if grep -qE '^\- \[[xX]\] \*\*@dev-security GO' "$adr"; then
      pass "(b) @dev-security GO firmado"
    else
      echo "🟡 PENDIENTE — @dev-security GO sin marcar (fase REVIEW)"
    fi
    if grep -qE '^\- \[[xX]\] \*\*Owner GO' "$adr"; then
      pass "(b) Owner GO firmado"
    else
      echo "🟡 PENDIENTE — Owner GO sin marcar (sign-off, D-12)"
    fi
  fi
}

# self_test — fixtures negativas: el check DEBE fallar cuando el control no está
self_test() {
  local tmp; tmp="$(mktemp -d)"
  # el trap se limpia antes de salir: en bash un trap RETURN persiste y dispararía
  # con $tmp ya fuera de scope (set -u lo toma como unbound)
  trap 'rm -rf "$tmp"' RETURN

  # Fixture OK: satisface TODO el contrato → debe pasar
  cat > "$tmp/ok.md" <<'EOF'
# ADR fixture OK
## Decisión
Se decide todo bien.
## DD-1 · Contrato S2S
### Decisión
Opción (b) S2S scoped read elegida.
El tenant_id pedido se fija como app.tenant_id **sólo tras** pasar el gate de scope.
1. Gate primero: Authorize exige config:read.
2. Sólo tras el gate se lee X-Tenant-ID.
3. WithTenantID y WithRLSInTransaction fijan app.tenant_id en la TX.
**Por qué no es USING(true) encubierto**: escape acotado y gated.
### Alternativas consideradas
- **(a) Forward del JWT** — rechazada: rompe el browse anónimo.
- **(b) S2S scoped read** — elegida: cubre el caso anónimo.
## DD-2 · stock_policy
### Alternativas consideradas
- **Unificar** — rechazada: dominios distintos.
- **Declararlos distintos** — elegida: fuente única.
## DD-3 · namespace
El DDL objetivo es UNIQUE (namespace, slug) desde la migración 010.
### Alternativas consideradas
- **Adoptar 'mc'** — rechazada: colisiona con el namespace del JWT.
- **Sigla prefijada en slug** — rechazada: duplica la estructura.
EOF

  # Fixture N1: sin DD-3 → DEBE fallar (criterio (a): diseño con agujeros)
  sed '/^## DD-3/,$d' "$tmp/ok.md" > "$tmp/sin-dd3.md"

  # Fixture N2: elige (b) SIN gate escrito → DEBE fallar (negativa (c))
  awk '
    /^## DD-1/ {indd1=1}
    indd1 && /^[0-9]+ *\./ {next}                       # sin secuencia numerada
    indd1 && /sólo tras/ {next}                          # sin orden gate→seteo
    indd1 && /USING\(true\)/ {next}                      # sin sección del escape
    {print}
  ' "$tmp/ok.md" > "$tmp/sin-gate.md"

  # Fixture N3: DD-2 con UNA alternativa → DEBE fallar (criterio (a): ≥2)
  awk '
    /^## DD-2/ {indd2=1}
    indd2 && /- \*\*Declararlos distintos\*\*/ {next}    # deja una sola alternativa
    {print}
  ' "$tmp/ok.md" > "$tmp/pocas-alt.md"

  local st_fail=0
  local nombre esperado desc hallados out
  # Cada fixture se verifica de forma aislada; el veredicto está en los 🔴 FALLA emitidos
  for caso in "ok.md:0:fixture válida pasa el contrato" \
             "sin-dd3.md:1:negativa (a): ADR sin DD-3 es rechazado" \
             "sin-gate.md:1:negativa (c): opción (b) sin gate escrito es rechazada (USING(true) encubierto)" \
             "pocas-alt.md:1:negativa (a): DD con <2 alternativas es rechazado"; do
    nombre="${caso%%:*}"; resto="${caso#*:}"
    esperado="${resto%%:*}"; desc="${resto#*:}"
    out="$(verificar_adr "$tmp/$nombre")"
    hallados="$(grep -c '🔴 FALLA' <<<"$out")"
    if { (( esperado == 0 )) && (( hallados == 0 )); } || { (( esperado == 1 )) && (( hallados >= 1 )); }; then
      echo "✅ self-test: $desc"
    else
      echo "🔴 self-test FALLA: $desc (hallazgos=$hallados, esperado=$esperado)"
      echo "$out" | sed 's/^/      /'
      st_fail=$((st_fail+1))
    fi
  done
  if (( st_fail > 0 )); then
    echo "🔴 self-test: $st_fail caso(s) negativo(s) NO se comportan como el contrato exige"
    trap - RETURN
    rm -rf "$tmp"
    return 1
  fi
  echo "✅ self-test: las 4 fixtures se comportan según el contrato (3 negativas incluidas)"
  trap - RETURN
  rm -rf "$tmp"
  return 0
}

main() {
  if [[ "${1:-}" == "--self-test" ]]; then
    self_test
    return $?
  fi
  local adr="${1:-$DEFAULT_ADR}"
  FAILURES=0
  verificar_adr "$adr"
  report_proceso "$adr"
  echo "----------------------------------------"
  if (( FAILURES == 0 )); then
    echo "RESULTADO: contrato de ACC-E04.T1 satisfecho por el ADR ($adr)"
    return 0
  fi
  echo "RESULTADO: $FAILURES hallazgo(s) contra el contrato de ACC-E04.T1"
  return 1
}

main "$@"