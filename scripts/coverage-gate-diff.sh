#!/usr/bin/env bash
# coverage-gate-diff.sh — PLAT-E21 T5 — gate de cobertura del diff vs coverage-baseline.json.
#
# PLAT-E08.T5: la herramienta compartida (medición total por servicio) vive en
# management/scripts/coverage-gate.sh y el entrypoint scripts/coverage-gate.sh
# es un wrapper que delega acá la firma de diff-coverage de CI. Esta
# implementación queda local porque es específica de este repo: usa
# ./scripts/affected-packages.sh y los awk de este directorio.
#
# Calcula la "diff-coverage": el % de statements en líneas NUEVAS o MODIFICADAS
# del diff que están cubiertos por los tests del subconjunto afectado (T2/T3), y lo
# compara contra el umbral coverage_percent del baseline (T1). Falla (exit 1) si la
# diff-coverage queda por debajo del umbral; pasa (exit 0) si lo iguala o supera.
#
# ¿Por qué diff-coverage y no cobertura global? Correr solo los tests afectados
# (no la suite completa — ver objetivo de la épica) no reproduce la cobertura
# global del módulo: la mayor parte de ./src/... no se ejercita. Medir la cobertura
# de las líneas que el diff tocó es la métrica comparable y sensible al cambio, y
# usa el baseline como piso (no arrastrar el promedio del repo hacia abajo).
#
# El coverprofile se genera con -coverpkg=./src/... (OBLIGATORIO — caveat de T1:
# los tests viven en el árbol externo iam/test/...; sin -coverpkg el coverage por
# paquete reporta 0% aunque haya tests reales, porque go test sólo atribuye
# cobertura al paquete donde corren los tests, no al paquete bajo test).
#
# Uso:  coverage-gate.sh [--coverprofile <file>] <base-ref> [head-ref]
#   --coverprofile <file>   reutiliza un coverprofile existente (p.ej. el que genera
#                           el paso de tests de T3 en CI) en vez de volver a correr
#                           los tests afectados.
# Requiere: go, git, awk, jq. Reusa scripts/affected-packages.sh (T2).

set -euo pipefail

COVERPROFILE=""
if [ "${1:-}" = "--coverprofile" ]; then
  COVERPROFILE="${2:?--coverprofile requiere un path}"
  shift 2
fi

BASE_REF="${1:?Uso: $0 [--coverprofile <file>] <base-ref> [head-ref]}"
HEAD_REF="${2:-HEAD}"

ROOT="$(git rev-parse --show-toplevel)"
cd "${ROOT}"

MODULE="$(go list -m)"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 1. Paquetes afectados (T2) — para detectar el caso "sin cambios .go".
AFFECTED="$(./scripts/affected-packages.sh "${BASE_REF}" "${HEAD_REF}")"
if [ -z "${AFFECTED}" ]; then
  echo "coverage-gate: el diff no tocó .go/go.mod/go.sum — nada que medir."
  exit 0
fi

# 2. Coverprofile: provisto (CI, reusando T3) o generado ahora (standalone).
if [ -n "${COVERPROFILE}" ]; then
  if [ ! -f "${COVERPROFILE}" ]; then
    echo "coverage-gate: coverprofile provisto no existe: ${COVERPROFILE}" >&2
    exit 1
  fi
  COV_FILE="${COVERPROFILE}"
  echo "coverage-gate: reusando coverprofile ${COV_FILE}"
else
  COV_FILE="coverage-gate.out"
  echo "coverage-gate: corriendo tests afectados con cobertura (-coverpkg=./src/...)..." >&2
  # Sin -race: la instrumentación de cobertura no altera el resultado de los tests
  # (el paso de tests de T3 ya corrió -race), y evita la doble instrumentación.
  # shellcheck disable=SC2086
  go test -timeout 120s -coverpkg=./src/... -coverprofile="${COV_FILE}" ${AFFECTED} > /dev/null
fi

# 3. Hunks del lado NUEVO del diff (solo .go). Los archivos borrados los ignora el awk.
#
# --no-renames (ACC-E01 T3, 2026-09-04): con la rename detection por default de git,
# un `git mv` de similitud 100% no produce hunks — el diff muestra sólo el header
# "rename from/to" y el gate queda CIEGO ante la mudanza: imprime NOSTMTS ("Pasa")
# sin medir una sola línea. Verificado empíricamente moviendo un paquete cubierto
# al 100% (src/plan/application/response -> src/plans/...): sin el flag, 0 líneas
# medidas y exit 0; con el flag, 5/5 statements medidos y la negativa (borrar los
# tests que lo cubren) pasa de PASS a FAIL exit 1. Con --no-renames la mudanza
# aparece como baja (path viejo, la ignora parse-diff.awk) + alta (path nuevo,
# TODAS sus líneas cuentan como nuevas y se miden contra el coverprofile de HEAD,
# que ya las tiene en el path nuevo). Toda la reorganización de ACC-E01 T4 son
# git mv: sin este flag, el gate no defendería la mudanza que existe para proteger.
HUNKS_FILE="$(mktemp)"
trap 'rm -f "${HUNKS_FILE}"' EXIT
git diff --no-renames -U0 "${BASE_REF}...${HEAD_REF}" -- '*.go' \
  | awk -v mod="${MODULE}" -f "${SCRIPT_DIR}/parse-diff.awk" > "${HUNKS_FILE}"

# 4. Diff-coverage: intersección coverprofile ∩ hunks.
RESULT="$(awk -f "${SCRIPT_DIR}/diff-coverage.awk" "${HUNKS_FILE}" "${COV_FILE}")"

if [ "${RESULT}" = "NOSTMTS" ]; then
  echo "coverage-gate: el diff no tocó statements medibles (¿solo comentarios/imports/blank?). Pasa."
  exit 0
fi

DIFF_PCT="$(printf '%s' "${RESULT}" | awk '{print $1}')"
COVERED="$(printf '%s' "${RESULT}" | awk '{print $2}')"
TOTAL="$(printf '%s' "${RESULT}" | awk '{print $3}')"

# 5. Umbral del baseline (T1).
BASELINE_FILE="coverage-baseline.json"
if [ ! -f "${BASELINE_FILE}" ]; then
  echo "coverage-gate: falta ${BASELINE_FILE} (T1). No se puede comparar." >&2
  exit 1
fi
THRESH="$(jq -r '.coverage_percent' "${BASELINE_FILE}")"

# 6. Veredicto.
echo "coverage-gate: diff-coverage = ${DIFF_PCT}% (${COVERED}/${TOTAL} statements) — baseline ${THRESH}%"
if awk -v a="${DIFF_PCT}" -v b="${THRESH}" 'BEGIN{exit !(a < b)}'; then
  echo "coverage-gate: FAIL — la diff-coverage (${DIFF_PCT}%) está por debajo del baseline (${THRESH}%)."
  echo "coverage-gate: agregá/ajustá tests para las líneas nuevas o modificadas del diff."
  exit 1
fi
echo "coverage-gate: PASS — la diff-coverage (${DIFF_PCT}%) ≥ baseline (${THRESH}%)."