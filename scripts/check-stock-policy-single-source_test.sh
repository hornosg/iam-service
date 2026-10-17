#!/usr/bin/env bash
# check-stock-policy-single-source_test.sh — tests de ACC-E04.T6 (fuente única de
# vendibilidad, DD-2).
#
# Verifica el CONTRATO de la tarea tal como está escrito en el archivo de la épica,
# no la implementación (que para T6 es "sin cambio de código": verificación +
# documentación). T6 no pide un script de producción cableado a CI (a diferencia de
# T10) — este test corre los greps LITERALES del "Hecho cuando" contra el árbol real
# y deja la verificación como artefacto repetible, en vez de confiar para siempre en
# una corrida manual.
#
#   - (a) positiva: `grep -rn "tenant_settings.*stock_policy" catalog-bff-service/src
#     --include='*.go' | grep -v _test` contra el árbol real → sin resultados (la
#     forma estricta; la del letter con `\|StockPolicy` matchea el identificador Go
#     StockPolicy y da 25 falsos positivos — corrección ya registrada en el bloque T6
#     del archivo de la épica y en la bitácora 2026-10-16).
#   - (a) NEGATIVA: el mismo grep contra un fixture AISLADO en /tmp con una violación
#     a propósito → SÍ debe detectarla. Sin esto, (a) podría estar pasando porque el
#     patrón nunca matchearía nada, no porque el control funcione.
#   - (b) positiva: el grep exacto del "Hecho cuando" (stock_policy + filtro
#     sellable/vendib/catalog, excluyendo tenant_settings/tenant-service) contra
#     active/mercado-cercano/services → TODOS los resultados vienen de
#     catalog-bff-service (único resolver de vendibilidad).
#   - (b) NEGATIVA: el mismo grep contra un fixture aislado que simula un segundo
#     resolver de vendibilidad fuera de catalog-bff-service → debe aparecer y no
#     pasar desapercibido.
#   - (c): el hallazgo de `tenant_settings.stock_policy` queda escrito en el bloque T6
#     del archivo de la épica, marcado como deuda de MC.
#
# No modifica código de producción: sólo lee el árbol real y escribe fixtures en un
# directorio temporal fuera del repo (borrado al salir).
#
# Uso: bash scripts/check-stock-policy-single-source_test.sh   → exit 0 si todos pasan.

set -u

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LAB_ROOT="$(cd "$REPO_ROOT/../.." && pwd)"
BFF_SRC="${BFF_SRC:-$LAB_ROOT/active/mercado-cercano/services/catalog-bff-service/src}"
MC_SERVICES="${MC_SERVICES_DIR:-$LAB_ROOT/active/mercado-cercano/services}"
EPIC_FILE="${EPIC_FILE:-$LAB_ROOT/management/projects/account-service/epicas/ACC-E04-tenancy-absorber-tenant-config.md}"

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

pass=0
fail=0

ok()  { echo "  ✅ PASS: $1"; pass=$((pass + 1)); }
bad() { echo "  🔴 FAIL: $1"; fail=$((fail + 1)); }

# --- fail-closed de alcance: si no encuentra el árbol real, no finge que pasó ---
if [ ! -d "$BFF_SRC" ]; then
  echo "scope incompleto: no existe $BFF_SRC" >&2
  exit 2
fi
if [ ! -d "$MC_SERVICES" ]; then
  echo "scope incompleto: no existe $MC_SERVICES" >&2
  exit 2
fi
if [ ! -f "$EPIC_FILE" ]; then
  echo "scope incompleto: no existe $EPIC_FILE" >&2
  exit 2
fi

echo "== (a) positiva: catalog-bff no lee tenant_settings.stock_policy =="
matches_a="$(grep -rn 'tenant_settings.*stock_policy' "$BFF_SRC" --include='*.go' 2>/dev/null | grep -v _test)"
if [ -z "$matches_a" ]; then
  ok "0 resultados en el árbol real (fuente doble no existe)"
else
  bad "catalog-bff-service referencia tenant_settings.stock_policy: $matches_a"
fi

echo "== (a) NEGATIVA: el grep SÍ detecta una violación introducida a propósito =="
FIXTURE_A="$TMP_ROOT/fixture_a"
mkdir -p "$FIXTURE_A"
cat > "$FIXTURE_A/rogue_resolver.go" <<'EOF'
package domain

// violación de prueba: resolver que decide vendibilidad leyendo la columna fiscal
func isSellable(row Row) bool {
	return row.tenant_settings.stock_policy != "IGNORE"
}
EOF
violation_a="$(grep -rn 'tenant_settings.*stock_policy' "$FIXTURE_A" --include='*.go' 2>/dev/null | grep -v _test)"
if [ -n "$violation_a" ]; then
  ok "el check detecta la violación del fixture aislado (el control no es vacío)"
else
  bad "el check NO detecta una violación a propósito — un (a) en verde no prueba nada"
fi

echo "== (b) positiva: catalog-bff es el único resolver de vendibilidad en el lab =="
matches_b="$(grep -rn "stock_policy" "$MC_SERVICES" --include='*.go' 2>/dev/null \
  | grep -v _test \
  | grep -iv "tenant_settings\|tenant-service/src/tenant" \
  | grep -i "sellable\|vendib\|catalog")"
if [ -z "$matches_b" ]; then
  bad "el grep de (b) no encontró ni al propio resolver de catalog-bff — revisar el filtro"
else
  non_bff="$(echo "$matches_b" | grep -v "catalog-bff-service" || true)"
  if [ -z "$non_bff" ]; then
    ok "todos los hits vienen de catalog-bff-service (sin segundo resolver)"
  else
    bad "existe un segundo resolver de vendibilidad fuera de catalog-bff: $non_bff"
  fi
fi

echo "== (b) NEGATIVA: un segundo resolver fuera de catalog-bff SÍ se detecta =="
FIXTURE_B="$TMP_ROOT/fixture_b/rogue-service/src"
mkdir -p "$FIXTURE_B"
cat > "$FIXTURE_B/rogue_vendibilidad_resolver.go" <<'EOF'
package domain

// violación de prueba: segundo resolver de vendibilidad fuera de catalog-bff
func IsVendible(cfg Config) bool {
	// stock_policy decide si la variante es vendible
	return cfg.stock_policy != "IGNORE"
}
EOF
violation_b="$(grep -rn "stock_policy" "$TMP_ROOT/fixture_b" --include='*.go' 2>/dev/null \
  | grep -v _test \
  | grep -iv "tenant_settings\|tenant-service/src/tenant" \
  | grep -i "sellable\|vendib\|catalog")"
non_bff_violation="$(echo "$violation_b" | grep -v "catalog-bff-service" || true)"
if [ -n "$non_bff_violation" ]; then
  ok "el check detecta el segundo resolver simulado fuera de catalog-bff"
else
  bad "el check NO detecta un segundo resolver simulado — (b) estaría pasando a ciegas"
fi

echo "== (c): el hallazgo de tenant_settings.stock_policy queda documentado en el bloque T6 =="
t6_block="$(grep -A 15 "T6 · Cerrar la duplicación" "$EPIC_FILE")"
if echo "$t6_block" | grep -q "tenant_settings.stock_policy" && echo "$t6_block" | grep -qi "deuda"; then
  ok "el bloque T6 del archivo de la épica documenta el hallazgo y lo marca como deuda de MC"
else
  bad "el bloque T6 no documenta el hallazgo de tenant_settings.stock_policy como deuda"
fi

echo
echo "== Resumen: $pass passed, $fail failed =="
[ "$fail" -eq 0 ]
