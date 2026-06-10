#!/usr/bin/env bash
set -euo pipefail

# Runner Newman standalone — úsalo contra cualquier instancia del IAM service.
# Uso:    ./postman/run.sh [BASE_URL]
# Entorno: BASE_URL (default: http://localhost:8080)
# Deps:   newman  +  newman-reporter-htmlextra (npm i -g newman-reporter-htmlextra)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASE_URL="${1:-${BASE_URL:-http://localhost:8080}}"
REPORTS_DIR="${SCRIPT_DIR}/reports"
TIMESTAMP="$(date '+%Y%m%d-%H%M%S')"

mkdir -p "${REPORTS_DIR}"

echo ""
echo "┌──────────────────────────────────────────┐"
echo "│   IAM Service — Newman e2e (43 requests) │"
echo "└──────────────────────────────────────────┘"
echo "  Target : ${BASE_URL}"
echo "  Reporte: ${REPORTS_DIR}/report-${TIMESTAMP}.html"
echo ""

HTML_REPORT="${REPORTS_DIR}/report-${TIMESTAMP}.html"
JSON_REPORT="${REPORTS_DIR}/report-${TIMESTAMP}.json"

newman run "${SCRIPT_DIR}/collection.json" \
  -e "${SCRIPT_DIR}/environment.local.json" \
  --env-var "baseUrl=${BASE_URL}/api/v1" \
  --env-var "hostUrl=${BASE_URL}" \
  --delay-request 50 \
  --reporters cli,htmlextra,json \
  --reporter-cli-no-banner \
  --reporter-htmlextra-export "${HTML_REPORT}" \
  --reporter-htmlextra-title "IAM Service — e2e Report ${TIMESTAMP}" \
  --reporter-htmlextra-darkTheme \
  --reporter-htmlextra-showEnvironmentData \
  --reporter-json-export "${JSON_REPORT}" \
  --bail

echo ""
echo "  Reporte JSON: ${JSON_REPORT}"

# Abrir reporte HTML automáticamente en macOS
if [ -f "${HTML_REPORT}" ] && command -v open >/dev/null 2>&1; then
  echo "  Abriendo reporte HTML..."
  open "${HTML_REPORT}"
fi
