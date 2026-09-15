#!/usr/bin/env bash
# check-tenants-unique_test.sh — tests de ACC-E04.T10 (check de unicidad de `tenants`).
#
# Verifica el CONTRATO de la tarea, no la implementación:
#   - positiva: contra el árbol del lab → exit 0 y reporta EXACTAMENTE 1 definición,
#     la de account-service/migrations/002;
#   - negativa: una segunda definición `CREATE TABLE tenants` → exit ≠ 0 y el mensaje
#     NOMBRA el archivo intruso (uno y dos servicios simultáneos); quitarla →
#     exit 0 (ciclo add/remove del Hecho cuando b);
#   - negativa de 0 definiciones → exit ≠ 0 (T12: la tabla desaparece de todo el
#     alcance; T4: mudada fuera de account-service);
#   - fail-closed: alcance incompleto (sin migraciones MC) → exit 2;
#     MC_SERVICES_DIR inexistente (typo en el workflow) → exit 2;
#   - precisión: no-SQL no cuenta (--include='*.sql'), subdirectorios sí (-r),
#     word boundary (\btenants\b no matchea tenants_archive ni legacy_tenants),
#     operaciones no-CREATE sobre tenants (ALTER/DROP/INSERT/INDEX) no cuentan;
#   - cableado de CI que falla el build: `grep check-tenants-unique .github/workflows/`
#     → ≥ 1 resultado NO-comentario, sin `|| true` ni `continue-on-error`.
#
# Los escenarios de mutación corren en un árbol ESPEJO (temp): el script se copia a un
# repo de juguete y MC_SERVICES_DIR apunta a servicios de juguete. Mismo mecanismo que
# el Hecho cuando (b) pide contra el árbol real, sin tocar repos con cambios sin
# commitear. La positiva sí corre contra el árbol real (verifica la resolución del
# layout del lab: ../../active/mercado-cercano/services).
#
# Uso: bash scripts/check-tenants-unique_test.sh   → exit 0 si todos pasan.

set -u

SCRIPT_UNDER_TEST="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/check-tenants-unique.sh"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

pass=0
fail=0

ok()   { echo "  ✅ PASS: $1"; pass=$((pass + 1)); }
bad()  { echo "  🔴 FAIL: $1"; fail=$((fail + 1)); }

run_case() {
  echo
  echo "== $1 =="
}

# --- fixtures ---------------------------------------------------------------

# Espejo del repo: <root>/repo/scripts/check-tenants-unique.sh + <root>/repo/migrations/
# y <root>/mc/<servicio>/migrations/. Devuelve el path al script espejo en $1.
make_mirror() {
  local root="$1"
  mkdir -p "$root/repo/scripts" "$root/repo/migrations" "$root/mc"
  cp "$SCRIPT_UNDER_TEST" "$root/repo/scripts/check-tenants-unique.sh"
}

add_tenants_table() {
  # $1: archivo .sql destino; $2: servicio (para comentario). Mismo enunciado que la
  # migración real: CREATE TABLE tenants con PK y unique — no un comentario vacío.
  cat >"$1" <<'SQL'
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(100) NOT NULL UNIQUE
);
SQL
}

add_innocent_migration() {
  # Migración sin `tenants`: verifica que el patrón no hace matches de más.
  cat >"$1" <<'SQL'
CREATE TABLE IF NOT EXISTS other_table (
    id UUID PRIMARY KEY
);
SQL
}

# run_mirror <root> → ejecuta el script espejo con MC_SERVICES_DIR=<root>/mc
# y devuelve stdout+stderr; el exit code queda en $?.
run_mirror() {
  MC_SERVICES_DIR="$1/mc" bash "$1/repo/scripts/check-tenants-unique.sh" 2>&1
}

# --- T1: positiva contra el árbol real del lab ------------------------------

run_case "T1 · positiva contra el árbol real: exit 0, exactamente 1 definición (account-service/migrations/002)"
if [[ ! -d "$REPO_ROOT/../../active/mercado-cercano/services" ]]; then
  echo "  ⏭ SKIP: sin layout del lab (fuera de \$DEVY_PATH) — correr en el lab"
else
  out="$(bash "$SCRIPT_UNDER_TEST" 2>&1)"
  rc=$?
  if [[ $rc -eq 0 ]]; then ok "exit 0"; else bad "esperaba exit 0, dio $rc"; fi
  if grep -q "Definiciones encontradas: 1" <<<"$out"; then
    ok "reporta exactamente 1 definición"
  else
    bad "no reporta 1 definición: $(grep 'Definiciones encontradas' <<<"$out")"
  fi
  if grep -q "account-service: migrations/002_create_tenants_table.up.sql" <<<"$out"; then
    ok "la definición reportada es la de account-service (migrations/002)"
  else
    bad "no apunta a migrations/002: $(grep '^  - ' <<<"$out" | head -1)"
  fi
fi

# --- T2: sandbox con 1 tabla propia y MC sin tenants → exit 0 ---------------

run_case "T2 · sandbox: 1 tabla en account-service, servicios MC sin la tabla → exit 0"
make_mirror "$TMP_ROOT/t2"
add_tenants_table "$TMP_ROOT/t2/repo/migrations/002_create_tenants_table.up.sql"
for svc in catalog-service stock-service customer-service; do
  mkdir -p "$TMP_ROOT/t2/mc/$svc/migrations"
  add_innocent_migration "$TMP_ROOT/t2/mc/$svc/migrations/001_x.sql"
done
out="$(run_mirror "$TMP_ROOT/t2")"
rc=$?
if [[ $rc -eq 0 ]]; then ok "exit 0"; else bad "esperaba exit 0, dio $rc"; fi
if grep -q "Definiciones encontradas: 1" <<<"$out"; then
  ok "cuenta exactamente 1"
else
  bad "conteo incorrecto: $(grep 'Definiciones encontradas' <<<"$out")"
fi

# --- T3: negativa — intruso en un segundo servicio MC -----------------------

run_case "T3 · negativa (Hecho cuando b): intruso en segundo servicio MC → exit ≠0 y nombra el archivo; quitado → exit 0"
make_mirror "$TMP_ROOT/t3"
add_tenants_table "$TMP_ROOT/t3/repo/migrations/002_create_tenants_table.up.sql"
mkdir -p "$TMP_ROOT/t3/mc/catalog-service/migrations" "$TMP_ROOT/t3/mc/stock-service/migrations"
add_innocent_migration "$TMP_ROOT/t3/mc/stock-service/migrations/001_x.sql"

intruder="$TMP_ROOT/t3/mc/catalog-service/migrations/010_create_tenants_copy.sql"
add_tenants_table "$intruder"
out="$(run_mirror "$TMP_ROOT/t3")"
rc=$?
if [[ $rc -ne 0 ]]; then ok "exit ≠ 0 con el intruso presente ($rc)"; else bad "exit 0 CON el intruso — el check no protege nada"; fi
# El check etiqueta al intruso con el path relativo a MC_SERVICES_DIR
# ("mc/<servicio>/migrations/<archivo>") — lo que el contrato pide es que NOMBRE
# el archivo intruso, no que imprima la ruta absoluta del sandbox.
intruder_rel="mc/catalog-service/migrations/010_create_tenants_copy.sql"
if grep -qF "$intruder_rel" <<<"$out"; then
  ok "el mensaje nombra el archivo intruso ($intruder_rel)"
else
  bad "no nombra el archivo intruso ($intruder_rel): $(grep INTRUSO <<<"$out")"
fi
if grep -q "INTRUSO" <<<"$out"; then
  ok "marca al intruso como INTRUSO (no como account-service)"
else
  bad "no etiqueta al intruso"
fi

rm -f "$intruder"
out="$(run_mirror "$TMP_ROOT/t3")"
rc=$?
if [[ $rc -eq 0 ]]; then ok "tras quitar el intruso → exit 0"; else bad "tras quitar el intruso dio $rc"; fi

# --- T4: negativa — 0 definiciones en account-service (la tabla se mudó) ----

run_case "T4 · negativa: la única definición queda fuera de account-service → exit ≠0"
make_mirror "$TMP_ROOT/t4"
mkdir -p "$TMP_ROOT/t4/repo/migrations" "$TMP_ROOT/t4/mc/catalog-service/migrations"
add_innocent_migration "$TMP_ROOT/t4/repo/migrations/002_create_tenants_table.up.sql" # ya no crea tenants
add_tenants_table "$TMP_ROOT/t4/mc/catalog-service/migrations/001_tenants.sql"
out="$(run_mirror "$TMP_ROOT/t4")"
rc=$?
if [[ $rc -ne 0 ]]; then ok "exit ≠ 0 con la identidad mudada fuera ($rc)"; else bad "exit 0 con `tenants` fuera de account-service"; fi
if grep -qi "fuera de account-service" <<<"$out"; then
  ok "el mensaje explica que la definición quedó fuera de account-service"
else
  bad "mensaje no explica el caso (sólo conteo): $(tail -2 <<<"$out")"
fi

# --- T5: negativa — dos definiciones dentro del propio repo -----------------

run_case "T5 · negativa: 2 definiciones dentro del propio repo → exit ≠0 y nombra ambas"
make_mirror "$TMP_ROOT/t5"
add_tenants_table "$TMP_ROOT/t5/repo/migrations/002_create_tenants_table.up.sql"
add_tenants_table "$TMP_ROOT/t5/repo/migrations/030_create_tenants_legacy.sql"
mkdir -p "$TMP_ROOT/t5/mc/catalog-service/migrations"
add_innocent_migration "$TMP_ROOT/t5/mc/catalog-service/migrations/001_x.sql"
out="$(run_mirror "$TMP_ROOT/t5")"
rc=$?
if [[ $rc -ne 0 ]]; then ok "exit ≠ 0 con 2 definiciones propias ($rc)"; else bad "exit 0 con 2 definiciones"; fi
if grep -q "Definiciones encontradas: 2" <<<"$out"; then
  ok "cuenta 2 (no 1 — no se traga el duplicado)"
else
  bad "conteo incorrecto con duplicado propio"
fi
if grep -qF "030_create_tenants_legacy.sql" <<<"$out"; then
  ok "nombra al segundo archivo propio"
else
  bad "no nombra 030_create_tenants_legacy.sql"
fi

# --- T6: fail-closed — alcance incompleto ------------------------------------

run_case "T6 · fail-closed: MC_SERVICES_DIR sin migraciones de servicios → exit ≠0 (no mira menos de lo prometido)"
make_mirror "$TMP_ROOT/t6"
add_tenants_table "$TMP_ROOT/t6/repo/migrations/002_create_tenants_table.up.sql"
mkdir -p "$TMP_ROOT/t6/mc" # vacío: ningún servicio con migrations/
out="$(MC_SERVICES_DIR="$TMP_ROOT/t6/mc" bash "$TMP_ROOT/t6/repo/scripts/check-tenants-unique.sh" 2>&1)"
rc=$?
if [[ $rc -ne 0 ]]; then ok "exit ≠ 0 ($rc) — no escanea menos de lo prometido"; else bad "exit 0 con alcance incompleto: no distingue '1 tabla' de 'no miré'"; fi
if grep -qi "alcance incompleto" <<<"$out"; then
  ok "el mensaje declara alcance incompleto"
else
  bad "falla silenciosa sin explicar el alcance"
fi

# --- T7: cableado en CI ------------------------------------------------------

run_case "T7 · cableado de CI: .github/workflows invoca el check → ≥1 resultado"
hits="$(grep -rn "check-tenants-unique" "$REPO_ROOT/.github/workflows/" 2>/dev/null)"
if [[ -n "$hits" ]]; then
  ok "invocado en el pipeline: $(echo "$hits" | wc -l) referencia(s)"
else
  bad "sin referencias en .github/workflows — script suelto sin invocar (Hecho cuando c)"
fi

# --- T8: precision del alcance — archivos no-SQL no cuentan ------------------

run_case "T8 · precisión (--include='*.sql'): un README .md con 'CREATE TABLE tenants' NO cuenta como definición"
make_mirror "$TMP_ROOT/t8"
add_tenants_table "$TMP_ROOT/t8/repo/migrations/002_create_tenants_table.up.sql"
mkdir -p "$TMP_ROOT/t8/mc/catalog-service/migrations"
add_innocent_migration "$TMP_ROOT/t8/mc/catalog-service/migrations/001_x.sql"
# Documentación que cita el DDL en un .md: es una MENCION, no una definición.
# El contrato fija el escaneo con --include='*.sql' (Hecho cuando a).
printf '# Guía\n```sql\nCREATE TABLE tenants (id uuid);\n```\n' > "$TMP_ROOT/t8/mc/catalog-service/README.md"
out="$(run_mirror "$TMP_ROOT/t8")"
rc=$?
if [[ $rc -eq 0 ]]; then ok "exit 0 — la mención en .md no dispara el check"; else bad "exit $rc: una mención en .md disparó el check (falso positivo de alcance)"; fi
if grep -q "Definiciones encontradas: 1" <<<"$out"; then
  ok "cuenta 1 (ignora el no-SQL)"
else
  bad "conteo incorrecto con mención en .md: $(grep 'Definiciones encontradas' <<<"$out")"
fi

# --- T9: escaneo recursivo — intruso en subdirectorio de migrations/ ---------

run_case "T9 · recursivo (-r): intruso en migrations/anuales/ (subdirectorio) → exit ≠0 y nombra el archivo"
make_mirror "$TMP_ROOT/t9"
add_tenants_table "$TMP_ROOT/t9/repo/migrations/002_create_tenants_table.up.sql"
# Servicio MC sin la tabla en la raíz de migrations/ pero con un subdirectorio
# versionado que la define: el -r del grep prescripto debe alcanzarlo.
mkdir -p "$TMP_ROOT/t9/mc/catalog-service/migrations/anuales"
add_innocent_migration "$TMP_ROOT/t9/mc/catalog-service/migrations/001_x.sql"
cat >"$TMP_ROOT/t9/mc/catalog-service/migrations/anuales/099_tenants.sql" <<'SQL'
CREATE TABLE IF NOT EXISTS tenants (id UUID PRIMARY KEY);
SQL
out="$(run_mirror "$TMP_ROOT/t9")"
rc=$?
if [[ $rc -ne 0 ]]; then ok "exit ≠ 0 con intruso anidado ($rc)"; else bad "exit 0 con intruso en subdirectorio — el escaneo no es recursivo"; fi
if grep -qF "migrations/anuales/099_tenants.sql" <<<"$out"; then
  ok "nombra el archivo anidado"
else
  bad "no nombra el archivo anidado: $(grep INTRUSO <<<"$out")"
fi

# --- T10: precision del word boundary — tenants_archive no es tenants --------

run_case "T10 · word boundary (\\b): 'CREATE TABLE tenants_archive' NO es una definición de tenants"
make_mirror "$TMP_ROOT/t10"
add_tenants_table "$TMP_ROOT/t10/repo/migrations/002_create_tenants_table.up.sql"
mkdir -p "$TMP_ROOT/t10/mc/catalog-service/migrations"
add_innocent_migration "$TMP_ROOT/t10/mc/catalog-service/migrations/001_x.sql"
# Tabla HERMANA con prefijo: \\btenants\\b no debe matchear 'tenants_archive'
# (el _ es word char). Si el patrón perdiera el word boundary, un rename de
# tabla en MC rompería el check de account-service.
cat >"$TMP_ROOT/t10/mc/catalog-service/migrations/001_archive.sql" <<'SQL'
CREATE TABLE IF NOT EXISTS tenants_archive (id UUID PRIMARY KEY);
SQL
out="$(run_mirror "$TMP_ROOT/t10")"
rc=$?
if [[ $rc -eq 0 ]]; then ok "exit 0 — tabla hermana no cuenta como definición"; else bad "exit $rc: tenants_archive contó como tenants (patrón sin word boundary)"; fi
if grep -q "Definiciones encontradas: 1" <<<"$out"; then
  ok "cuenta 1 (el \\b excluye tenants_archive)"
else
  bad "conteo incorrecto con tenants_archive: $(grep 'Definiciones encontradas' <<<"$out")"
fi

# --- T11: cableado de CI que FALLA el build (no neutralizado) ---------------

run_case "T11 · cableado de CI real: la invocación existe (sin comentarios) y no está neutralizada ('|| true' / 'continue-on-error')"
# Hecho cuando (c) exige ≥1 resultado en .github/workflows — pero una referencia
# neuterada no falla el build, y un check que no puede fallar no protege nada.
refs="$(grep -rn "check-tenants-unique" "$REPO_ROOT/.github/workflows/" 2>/dev/null)"
invocation="$(grep -v 'check-tenants-unique_test' <<<"$refs" | grep -Ev ':[0-9]+:\s*#')"
if [[ -n "$invocation" ]]; then
  ok "hay invocación no-comentario en el pipeline"
else
  bad "sólo hay referencias en comentarios — el check no corre en CI"
fi
if grep -q '|| true' <<<"$invocation"; then
  bad "la invocación está neutralizada con '|| true' — el check corre pero no puede fallar el build"
else
  ok "sin '|| true' en la invocación — el exit code propaga"
fi
# `continue-on-error` en el job que invoca convertiría un exit 1 en warning silencioso.
# Alcance: el archivo del workflow; si aparece en el mismo archivo que la invocación,
# el cableado es débil (no intentamos parsear el YAML para acotar al job exacto).
if grep -q "continue-on-error" "$REPO_ROOT/.github/workflows/ci.yml"; then
  bad "continue-on-error presente en ci.yml — un fallo del check puede degradarse a warning"
else
  ok "sin continue-on-error en ci.yml"
fi

# --- T12: negativa — 0 definiciones en todo el alcance ----------------------

run_case "T12 · negativa: ninguna definición de tenants en el alcance → exit ≠0 (la identidad desapareció)"
# Distinto de T4 (la tabla mudó de servicio): acá no hay tabla en NINGÚN lado.
# Un check que diera exit 0 con 0 definiciones aceptaría un lab sin tabla de tenants.
make_mirror "$TMP_ROOT/t12"
mkdir -p "$TMP_ROOT/t12/repo/migrations" "$TMP_ROOT/t12/mc/catalog-service/migrations"
add_innocent_migration "$TMP_ROOT/t12/repo/migrations/001_other.sql"
add_innocent_migration "$TMP_ROOT/t12/mc/catalog-service/migrations/001_x.sql"
out="$(run_mirror "$TMP_ROOT/t12")"
rc=$?
if [[ $rc -ne 0 ]]; then ok "exit ≠ 0 con 0 definiciones ($rc)"; else bad "exit 0 con 0 definiciones — el check no distingue '1' de 'ninguna'"; fi
if grep -q "0 definiciones" <<<"$out"; then
  ok "el mensaje declara 0 definiciones"
else
  bad "el fallo no explica el conteo: $(tail -1 <<<"$out")"
fi

# --- T13: variante realista del intruso — DDL estilo pg_dump ---------------

run_case "T13 · intruso calificado por schema: 'CREATE TABLE public.tenants' → exit ≠0 (forma de pg_dump)"
# Un dump/export real rara vez escribe `CREATE TABLE tenants` pelado: pg_dump emite
# `CREATE TABLE public.tenants`. Sigue siendo una definición de la tabla y el patrón
# prescripto (`CREATE TABLE.*\btenants\b`) debe alcanzarla — el `.` antes de tenants
# es non-word char, así que \btenants\b matchea igual.
make_mirror "$TMP_ROOT/t13"
add_tenants_table "$TMP_ROOT/t13/repo/migrations/002_create_tenants_table.up.sql"
mkdir -p "$TMP_ROOT/t13/mc/customer-service/migrations"
add_innocent_migration "$TMP_ROOT/t13/mc/customer-service/migrations/001_x.sql"
cat >"$TMP_ROOT/t13/mc/customer-service/migrations/005_tenants_dump.sql" <<'SQL'
CREATE TABLE public.tenants (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug character varying(100)
);
SQL
out="$(run_mirror "$TMP_ROOT/t13")"
rc=$?
if [[ $rc -ne 0 ]]; then ok "exit ≠ 0 con el intruso calificado ($rc)"; else bad "exit 0 con 'CREATE TABLE public.tenants' — pg_dump-style se escapa del check"; fi
if grep -qF "mc/customer-service/migrations/005_tenants_dump.sql" <<<"$out"; then
  ok "nombra el archivo intruso calificado"
else
  bad "no nombra el intruso: $(grep INTRUSO <<<"$out")"
fi

# --- T14: modo CI (MC_SERVICES_DIR explícito) contra el scope real del lab ---

run_case "T14 · modo CI: MC_SERVICES_DIR apuntando al scope real del lab → exit 0, 1 definición"
# La positiva (T1) ejercita la resolución del layout del lab; el job de Actions nunca
# usa ese camino: setea MC_SERVICES_DIR (clones de la org). Si el modo env explícito
# estuviera roto, el check estaría verde en el lab y rojo siempre en CI — y como el
# workflow no corrió nunca en Actions, ese defecto sería invisible hasta el primer push.
lab_services="$REPO_ROOT/../../active/mercado-cercano/services"
if [[ ! -d "$lab_services" ]]; then
  echo "  ⏭ SKIP: sin layout del lab (fuera de \$DEVY_PATH) — correr en el lab"
else
  out="$(MC_SERVICES_DIR="$lab_services" bash "$SCRIPT_UNDER_TEST" 2>&1)"
  rc=$?
  if [[ $rc -eq 0 ]]; then ok "exit 0 con MC_SERVICES_DIR explícito"; else bad "el modo CI (MC_SERVICES_DIR explícito) falla contra el scope real: $rc"; fi
  if grep -q "Definiciones encontradas: 1" <<<"$out"; then
    ok "cuenta exactamente 1 en modo CI"
  else
    bad "conteo incorrecto en modo CI: $(grep 'Definiciones encontradas' <<<"$out")"
  fi
fi

# --- T15: la invocación de CI setea MC_SERVICES_DIR (el check puede PASAR en CI) ---

run_case "T15 · cableado que puede pasar: la invocación en ci.yml setea MC_SERVICES_DIR"
# El fail-closed (exit 2) es correcto ante alcance incompleto — pero en el runner de
# Actions NO existe el layout del lab: si la invocación no setea MC_SERVICES_DIR, el
# job termina exit 2 en cada corrida. El cableado existiría (T11 pasa) y el build
# quedaría siempre rojo: un check que nunca puede pasar no verifica nada.
invocation_t15="$(grep -v 'check-tenants-unique_test' <<<"$(grep -rn 'check-tenants-unique' "$REPO_ROOT/.github/workflows/" 2>/dev/null)" | grep -Ev ':[0-9]+:\s*#')"
if grep -q 'MC_SERVICES_DIR' <<<"$invocation_t15"; then
  ok "la invocación en el workflow setea MC_SERVICES_DIR (puede pasar en CI)"
else
  bad "la invocación no setea MC_SERVICES_DIR — en Actions el check cortaría exit 2 (fail-closed) siempre: build siempre rojo"
fi

# --- T16: negativa contra el árbol REAL del lab (Hecho cuando b literal) ----

run_case "T16 · negativa contra el árbol real: intruso en un servicio MC vivo → exit ≠0 y nombra el archivo; quitado → exit 0"
# T3 cubre el ciclo add/remove en espejo; el Hecho cuando (b) lo pide contra el
# árbol real, y ése es el único caso que ejercita la clasificación propia/intruso
# con los paths reales del lab (REPO_ROOT bajo platform/ y mc_dir bajo active/ —
# prefijos distintos a los del sandbox). El intruso es un artefacto transitorio
# del test: se crea y se borra SIEMPRE (trap), y el blanco (customer-service)
# se elige con el repo limpio para que quede rastro cero en git.
lab_services="$REPO_ROOT/../../active/mercado-cercano/services"
if [[ ! -d "$lab_services" ]]; then
  echo "  ⏭ SKIP: sin layout del lab — correr en el lab"
else
  # Blanco dinámico: el primer servicio MC con migrations/ y repo limpio (el
  # implementador puede estar tocando otros servicios en paralelo).
  intruder_svc=""
  for d in "$lab_services"/*/migrations; do
    svc="$(basename "$(dirname "$d")")"
    if [[ -z "$(git -C "$lab_services/$svc" status --porcelain 2>/dev/null)" ]]; then
      intruder_svc="$lab_services/$svc"
      break
    fi
  done
  if [[ -z "$intruder_svc" ]]; then
    echo "  ⏭ SKIP: ningún servicio MC limpio para poner el intruso transitorio — no se toca un repo sucio"
  else
    intruder_name="$(basename "$intruder_svc")"
    intruder_file="$intruder_svc/migrations/zz_t10_test_intruder.sql"
    cleanup_intruder() { rm -f "$intruder_file"; }
    # El trap encadena el cleanup de TMP_ROOT: este trap REEMPLAZA al global de la
    # línea de arriba, y sin encadenarlo cada corrida de la suite filtra su
    # TMP_ROOT en /tmp — y una interrupción a mitad de T16 dejaría el intruso
    # en el repo real sin cleanup.
    trap 'cleanup_intruder; rm -rf "$TMP_ROOT"' EXIT
    add_tenants_table "$intruder_file"
    out="$(bash "$SCRIPT_UNDER_TEST" 2>&1)"
    rc=$?
    if [[ $rc -ne 0 ]]; then ok "exit ≠ 0 con el intruso presente en el árbol real ($rc)"; else bad "exit 0 CON el intruso en el árbol real — el check no protege nada"; fi
    if grep -qF "mc/$intruder_name/migrations/zz_t10_test_intruder.sql" <<<"$out"; then
      ok "nombra al intruso del árbol real"
    else
      bad "no nombra al intruso del árbol real: $(grep INTRUSO <<<"$out")"
    fi
    if grep -q "INTRUSO" <<<"$out"; then
      ok "clasifica al intruso del árbol real como INTRUSO (no como propio)"
    else
      bad "no etiqueta al intruso del árbol real"
    fi
    cleanup_intruder
    # Se restaura el trap global original (sólo TMP_ROOT): el intruso ya no existe.
    trap 'rm -rf "$TMP_ROOT"' EXIT
    out="$(bash "$SCRIPT_UNDER_TEST" 2>&1)"
    rc=$?
    if [[ $rc -eq 0 ]]; then ok "tras quitar el intruso del árbol real → exit 0"; else bad "tras quitar el intruso del árbol real dio $rc"; fi
    leftover="$(git -C "$intruder_svc" status --porcelain 2>/dev/null)"
    if [[ -z "$leftover" ]]; then
      ok "el servicio intruso quedó sin cambios (rastro cero)"
    else
      bad "quedaron cambios en $intruder_name: $leftover"
    fi
  fi
fi

# --- T17: CWD-independencia --------------------------------------------------

run_case "T17 · CWD-independiente: el resultado no depende del directorio desde el que se corre"
# La resolución de alcance usa BASH_SOURCE (absoluta) y el realpath del candidato
# del lab; nada debería depender del cwd. Pero si algo se colara relativo
# (p. ej. un patrón de path en roots o en label), el check sería verde desde la
# raíz del repo y rojo/exit 2 desde cualquier otro lado — exactamente el tipo de
# defecto que sólo se ve corriendolo desde afuera.
lab_services="$REPO_ROOT/../../active/mercado-cercano/services"
if [[ ! -d "$lab_services" ]]; then
  echo "  ⏭ SKIP: sin layout del lab — correr en el lab"
else
  out="$(cd "$TMP_ROOT" && bash "$SCRIPT_UNDER_TEST" 2>&1)"
  rc=$?
  if [[ $rc -eq 0 ]]; then ok "exit 0 corrido desde un CWD ajeno"; else bad "exit $rc corrido desde un CWD ajeno (esperaba 0)"; fi
  if grep -q "Definiciones encontradas: 1" <<<"$out"; then
    ok "cuenta exactamente 1 desde un CWD ajeno"
  else
    bad "conteo incorrecto desde un CWD ajeno: $(grep 'Definiciones encontradas' <<<"$out")"
  fi
  if grep -q "account-service: migrations/002_create_tenants_table.up.sql" <<<"$out"; then
    ok "la etiqueta de la definición no cambia con el CWD"
  else
    bad "el label cambia con el CWD: $(grep '^  - ' <<<"$out" | head -1)"
  fi
fi

# --- T18: el clone de servicios MC en ci.yml sigue fail-closed ---------------

run_case "T18 · cableado: el step de clone de servicios MC en ci.yml es fail-closed (un clone que fallo no reduce el alcance en silencio)"
# T15 verifica que la invocación setea MC_SERVICES_DIR — pero si el step que llena
# ese directorio se suavizara después ('|| true' en el clone, sin exit ante API
# vacía), MC_SERVICES_DIR quedaría con menos servicios y el script seguiría contando
# 1 tabla: pass con alcance reducido, el intruso sin clonar es invisible. La
# defensa de dos capas del job (API vacía → exit 1; clone fallido → exit 1) es lo
# que impide ese falso pass, y el comentario del workflow la declara.
workflow="$REPO_ROOT/.github/workflows/ci.yml"
clone_step="$(sed -n '/Clonar servicios MC/,/Check — una sola tabla tenants/p' "$workflow" 2>/dev/null)"
if [[ -n "$clone_step" ]]; then
  ok "el step de clone existe en ci.yml"
else
  clone_step=""
  bad "no se encontró el step 'Clonar servicios MC' en ci.yml — el alcance del check en CI depende de algo que ya no está"
fi
if grep -q "orgs/mercadocercano/repos" <<<"$clone_step"; then
  ok "el clone lista los repos de la org (el scope no está hardcodeado a un subconjunto)"
else
  bad "el step no lista los repos de la org mercadocercano"
fi
if grep -q "exit 1" <<<"$clone_step"; then
  ok "el step corta el build ante API vacía o clone fallido (fail-closed)"
else
  bad "sin exit 1 en el step de clone — un clone que falla reduce el alcance y el check pasa igual (falso pass)"
fi
if grep -q 'clone.*||\s*true\|||\s*true.*clone' <<<"$clone_step"; then
  bad "el clone está neutralizado con '|| true' — alcance reducido en silencio"
else
  ok "sin '|| true' en el clone — un fallo de clone corta el build"
fi

# --- T19: precision del word boundary — prefijo pegado ----------------------

run_case "T19 · word boundary (\\b) simétrico: 'CREATE TABLE legacy_tenants' NO es una definición de tenants"
# T10 cubre el sufijo (tenants_archive); el prefijo pegado con '_' es el mismo
# mecanismo por el otro lado: el '_' entre legacy y tenants es word char, así que
# no hay boundary y el patrón prescripto no debe matchear. Si lo matcheara, un
# rename de tabla en MC (tenants → legacy_tenants en una migración de limpieza)
# rompería el check de account-service sin que exista una segunda tabla.
make_mirror "$TMP_ROOT/t19"
add_tenants_table "$TMP_ROOT/t19/repo/migrations/002_create_tenants_table.up.sql"
mkdir -p "$TMP_ROOT/t19/mc/catalog-service/migrations"
add_innocent_migration "$TMP_ROOT/t19/mc/catalog-service/migrations/001_x.sql"
cat >"$TMP_ROOT/t19/mc/catalog-service/migrations/002_legacy.sql" <<'SQL'
CREATE TABLE IF NOT EXISTS legacy_tenants (id UUID PRIMARY KEY);
SQL
out="$(run_mirror "$TMP_ROOT/t19")"
rc=$?
if [[ $rc -eq 0 ]]; then ok "exit 0 — tabla con prefijo pegado no cuenta"; else bad "exit $rc: legacy_tenants contó como tenants (word boundary por el otro lado)"; fi
if grep -q "Definiciones encontradas: 1" <<<"$out"; then
  ok "cuenta 1 (el \\b excluye legacy_tenants)"
else
  bad "conteo incorrecto con legacy_tenants: $(grep 'Definiciones encontradas' <<<"$out")"
fi

# --- T20: identificador citado — '"tenants"' también es la tabla -------------

run_case "T20 · negativa: 'CREATE TABLE \"tenants\"' (identificador citado) → exit ≠0"
# pg_dump cita identificadores cuando el DDL original lo pide; un dump que viajó
# a un servicio MC con el nombre citado sigue definiendo la tabla. El '"' es
# non-word char, así que \btenants\b matchea igual que con el '.' de T13 —
# este test fija que la cita no es una escapatoria más del patrón prescripto.
make_mirror "$TMP_ROOT/t20"
add_tenants_table "$TMP_ROOT/t20/repo/migrations/002_create_tenants_table.up.sql"
mkdir -p "$TMP_ROOT/t20/mc/catalog-service/migrations"
add_innocent_migration "$TMP_ROOT/t20/mc/catalog-service/migrations/001_x.sql"
cat >"$TMP_ROOT/t20/mc/catalog-service/migrations/006_tenants_quoted.sql" <<'SQL'
CREATE TABLE "tenants" (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug character varying(100)
);
SQL
out="$(run_mirror "$TMP_ROOT/t20")"
rc=$?
if [[ $rc -ne 0 ]]; then ok "exit ≠ 0 con el identificador citado ($rc)"; else bad "exit 0 con 'CREATE TABLE \"tenants\"' — la cita del identificador escapa del check"; fi
if grep -qF "mc/catalog-service/migrations/006_tenants_quoted.sql" <<<"$out"; then
  ok "nombra el archivo del intruso citado"
else
  bad "no nombra el intruso citado: $(grep INTRUSO <<<"$out")"
fi

# --- T21: intruso en .down.sql + código de salida exacto de intrusión (1) ----

run_case "T21 · negativa: intruso en un .down.sql → exit 1 exacto y nombra el archivo"
# El filtro prescripto es --include='*.sql': los .down.sql son .sql y el grep los
# alcanza. Un rollback que recrea la tabla (`CREATE TABLE tenants` en un .down)
# ES una segunda definición en el árbol y el check no debe distinguir por sufijo.
# Acá además se fija el CÓDIGO exacto: el header del script documenta
# 0=ok / 1=intrusión / 2=alcance incompleto — la intrusión es exactamente 1, no
# cualquier exit ≠0 (un CI futuro puede ramificar por código).
make_mirror "$TMP_ROOT/t21"
add_tenants_table "$TMP_ROOT/t21/repo/migrations/002_create_tenants_table.up.sql"
mkdir -p "$TMP_ROOT/t21/mc/catalog-service/migrations"
add_innocent_migration "$TMP_ROOT/t21/mc/catalog-service/migrations/001_x.sql"
cat >"$TMP_ROOT/t21/mc/catalog-service/migrations/010_create_tenants_copy.down.sql" <<'SQL'
-- Rollback que recrea lo que su .up dropeó: una definición en el árbol igual.
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY
);
SQL
out="$(run_mirror "$TMP_ROOT/t21")"
rc=$?
if [[ $rc -eq 1 ]]; then ok "exit exactamente 1 (intrusión) como documenta el header del script"; else bad "esperaba exit 1 para intrusión, dio $rc"; fi
if grep -qF "mc/catalog-service/migrations/010_create_tenants_copy.down.sql" <<<"$out"; then
  ok "nombra el archivo .down.sql del intruso (el sufijo no lo esconde)"
else
  bad "no nombra el intruso en .down.sql: $(grep INTRUSO <<<"$out")"
fi

# --- T22: código de salida exacto del fail-closed (2) ------------------------

run_case "T22 · fail-closed: el código de salida es exactamente 2 (no se confunde con intrusión)"
# T6 verifica exit ≠0 ante alcance incompleto; acá se fija el código exacto que el
# header del script documenta. 2 ≠ 1 es lo que permitiría a un CI futuro
# distinguir 'el check no pudo mirar' de 'miró y encontró dos' — con ambos en 1,
# un alcance incompleto se reportaría como violación de unicidad (diagnóstico falso).
make_mirror "$TMP_ROOT/t22"
add_tenants_table "$TMP_ROOT/t22/repo/migrations/002_create_tenants_table.up.sql"
mkdir -p "$TMP_ROOT/t22/mc" # vacío: ningún servicio con migrations/
out="$(MC_SERVICES_DIR="$TMP_ROOT/t22/mc" bash "$TMP_ROOT/t22/repo/scripts/check-tenants-unique.sh" 2>&1)"
rc=$?
if [[ $rc -eq 2 ]]; then ok "exit exactamente 2 (alcance incompleto) como documenta el header del script"; else bad "esperaba exit 2 para alcance incompleto, dio $rc"; fi

# --- T23: path de MC_SERVICES_DIR INEXISTENTE (typo en el workflow) ---------

run_case "T23 · fail-closed: MC_SERVICES_DIR apunta a un path INEXISTENTE (typo en el workflow) → exit 2"
# T6/T22 usan un directorio existente pero vacío. El typo real es otro modo de
# falla: el path no existe, el glob no produce nada y el check quedaría
# "mirando" un directorio que no está. Mismo veredicto que alcance vacío:
# menos alcance no es un pass, es 'no miré' — y con un path mal escrito el
# resultado sería un pass en CI con CERO servicios MC escaneados.
make_mirror "$TMP_ROOT/t23"
add_tenants_table "$TMP_ROOT/t23/repo/migrations/002_create_tenants_table.up.sql"
out="$(MC_SERVICES_DIR="$TMP_ROOT/t23/mc-NO-EXISTE" bash "$TMP_ROOT/t23/repo/scripts/check-tenants-unique.sh" 2>&1)"
rc=$?
if [[ $rc -eq 2 ]]; then ok "exit exactamente 2 con path inexistente (fail-closed, no un pass con alcance vacío)"; else bad "con MC_SERVICES_DIR inexistente dio $rc — un typo en el workflow dejaría el check sin alcance"; fi
if grep -qi "alcance incompleto" <<<"$out"; then
  ok "el mensaje declara alcance incompleto (el typo no pasa en silencio)"
else
  bad "falla sin explicar el alcance: $(tail -1 <<<"$out")"
fi

# --- T24: DOS intrusos simultáneos en DOS servicios MC distintos ------------

run_case "T24 · negativa: intrusos simultáneos en dos servicios MC → exit 1 y nombra AMBOS archivos"
# T3 cubre un intruso y T5 dos definiciones propias; con violadores en DOS
# servicios distintos, el fallo debe enumerar a todos — si sólo nombrara el
# primero, el desarrollador arreglaría uno, re-correría, y el segundo seguiría
# en el árbol con un check que recién en la próxima corrida lo señala.
make_mirror "$TMP_ROOT/t24"
add_tenants_table "$TMP_ROOT/t24/repo/migrations/002_create_tenants_table.up.sql"
mkdir -p "$TMP_ROOT/t24/mc/catalog-service/migrations" "$TMP_ROOT/t24/mc/stock-service/migrations" "$TMP_ROOT/t24/mc/customer-service/migrations"
add_innocent_migration "$TMP_ROOT/t24/mc/customer-service/migrations/001_x.sql"
intruder_a="$TMP_ROOT/t24/mc/catalog-service/migrations/010_tenants_a.sql"
intruder_b="$TMP_ROOT/t24/mc/stock-service/migrations/011_tenants_b.sql"
add_tenants_table "$intruder_a"
add_tenants_table "$intruder_b"
out="$(run_mirror "$TMP_ROOT/t24")"
rc=$?
if [[ $rc -eq 1 ]]; then ok "exit 1 con dos servicios intrusos"; else bad "con dos intrusos simultáneos dio $rc"; fi
if grep -q "Definiciones encontradas: 3" <<<"$out"; then
  ok "cuenta 3 (propia + 2 intrusos)"
else
  bad "conteo incorrecto con dos intrusos: $(grep 'Definiciones encontradas' <<<"$out")"
fi
if grep -qF "mc/catalog-service/migrations/010_tenants_a.sql" <<<"$out" && grep -qF "mc/stock-service/migrations/011_tenants_b.sql" <<<"$out"; then
  ok "nombra AMBOS archivos intrusos en un solo fallo"
else
  bad "no enumera a los dos intrusos: $(grep INTRUSO <<<"$out")"
fi

# --- T25: precision — operaciones NO-CREATE sobre `tenants` no son definiciones ---

run_case "T25 · precisión: ALTER/DROP/INSERT/INDEX sobre \`tenants\` NO cuentan como definición"
# El árbol real tiene migraciones evolutivas sobre la tabla (008_add_features_to_tenants,
# 010_add_namespace_to_tenants hacen ALTER TABLE tenants). El patrón prescripto ancla en
# 'CREATE TABLE', así que ninguna operación no-CREATE debe contar — si contaran, el
# propio árbol del lab daría ≥3 y el check estaría rojo SIEMPRE. T1 lo prueba de facto
# contra el árbol real; este caso fija el mecanismo en sandbox (un fixture que mezcle
# todo junto, para que un solo archivo ruidoso no dispare el check).
make_mirror "$TMP_ROOT/t25"
add_tenants_table "$TMP_ROOT/t25/repo/migrations/002_create_tenants_table.up.sql"
mkdir -p "$TMP_ROOT/t25/mc/catalog-service/migrations"
add_innocent_migration "$TMP_ROOT/t25/mc/catalog-service/migrations/001_x.sql"
cat >"$TMP_ROOT/t25/mc/catalog-service/migrations/020_evolve.sql" <<'SQL'
ALTER TABLE tenants ADD COLUMN namespace VARCHAR(100);
DROP TABLE IF EXISTS tenants_staging;
INSERT INTO tenants (id, slug) VALUES (gen_random_uuid(), 'x');
CREATE INDEX idx_tenants_slug ON tenants (slug);
COMMENT ON TABLE tenants IS 'tabla raiz de identidad';
SQL
out="$(run_mirror "$TMP_ROOT/t25")"
rc=$?
if [[ $rc -eq 0 ]]; then ok "exit 0 — ninguna operación no-CREATE disparó el check"; else bad "exit $rc: una operación no-CREATE sobre tenants contó como definición"; fi
if grep -q "Definiciones encontradas: 1" <<<"$out"; then
  ok "cuenta 1 (el ancla CREATE TABLE excluye ALTER/DROP/INSERT/INDEX/COMMENT)"
else
  bad "conteo incorrecto con operaciones no-CREATE: $(grep 'Definiciones encontradas' <<<"$out")"
fi

# --- T26: el workflow que invoca el check se dispara en builds reales --------

run_case "T26 · cableado vivo: ci.yml dispara en push y pull_request a main/master (un workflow sin trigger es un check dormido)"
# T7/T11/T15/T18 verifican que el job existe y está bien armado — pero si el
# bloque `on:` se quitara o quedara apuntando a un branch que no existe, TODOS
# esos tests seguirían verdes y el check no correría nunca: cableado en un
# archivo muerto. El propio workflow declara el push directo como requisito
# (el owner no abre PRs — sin `push` el job queda inerte).
workflow_ci="$REPO_ROOT/.github/workflows/ci.yml"
on_block="$(sed -n '/^on:/,/^[a-z_]*:$/p' "$workflow_ci" 2>/dev/null)"
if grep -q "branches: \[main, master\]" <<<"$on_block"; then
  ok "el bloque on: apunta a main y master"
else
  bad "el bloque on: no apunta a main/master — el check podría no correr en el branch real"
fi
grep -q "push:" <<<"$on_block" \
  && ok "dispara en push (el owner no abre PRs — sin push el job queda inerte)" \
  || bad "sin trigger push: el check no corre en los pushes directos del owner"
grep -q "pull_request:" <<<"$on_block" \
  && ok "dispara en pull_request" \
  || bad "sin trigger pull_request: el check no corre en PRs"

# --- resumen ----------------------------------------------------------------

echo
echo "== Resultado: $pass pass / $fail fail =="
if (( fail > 0 )); then
  echo "🔴 HAY TESTS FALLANDO — es información sobre el check, no se tapa."
  exit 1
fi
exit 0