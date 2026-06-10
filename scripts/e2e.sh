#!/usr/bin/env bash
set -euo pipefail

# e2e: levanta Postgres via Docker Compose, corre el servicio, ejecuta Newman.
# Requiere: docker, docker compose, go, newman (npm i -g newman)
# Uso: ./scripts/e2e.sh [--no-cleanup]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
PORT=${PORT:-8081}
NO_CLEANUP=${1:-}

cd "${PROJECT_DIR}"

# --- Dependencias ---
for dep in docker go newman; do
  command -v "${dep}" >/dev/null 2>&1 || { echo "ERROR: ${dep} no está en PATH"; exit 1; }
done

# --- Cleanup anterior ---
cleanup() {
  echo "--- Cleanup ---"
  [ -n "${APP_PID:-}" ] && kill "${APP_PID}" 2>/dev/null || true
  [ "${NO_CLEANUP}" = "--no-cleanup" ] || docker compose -f docker-compose.e2e.yml down -v 2>/dev/null || true
}
trap cleanup EXIT

# --- Postgres e2e ---
echo "--- Levantando Postgres e2e ---"
docker compose -f docker-compose.e2e.yml up -d postgres-e2e
echo "  Esperando que iam_e2e esté lista (incluyendo seed)..."
DB_READY=0
for i in $(seq 1 60); do
  count=$(docker compose -f docker-compose.e2e.yml exec -T postgres-e2e \
    psql -U postgres -d iam_e2e -t -c "SELECT COUNT(*) FROM users WHERE email='admin@saasadmin.com'" 2>/dev/null \
    | tr -d ' \n' || echo "0")
  if [ "${count:-0}" -gt 0 ] 2>/dev/null; then
    echo "  DB lista en intento ${i}"
    DB_READY=1
    break
  fi
  sleep 1
done
[ "${DB_READY}" -eq 1 ] || { echo "TIMEOUT: base de datos no lista"; exit 1; }

# --- Build ---
echo "--- Build ---"
go build -o /tmp/iam-e2e-service ./src/main.go

# --- Arrancar servicio ---
echo "--- Arrancando servicio en :${PORT} ---"
DB_HOST=localhost \
DB_PORT=5433 \
DB_USER=postgres \
DB_PASSWORD=postgres \
DB_NAME=iam_e2e \
DB_SSLMODE=disable \
PORT="${PORT}" \
JWT_SECRET="e2e-test-secret-key-32-chars-min!!" \
  /tmp/iam-e2e-service &
APP_PID=$!

# --- Esperar health ---
echo "  Esperando health en :${PORT}..."
for i in $(seq 1 30); do
  curl -sf "http://localhost:${PORT}/health" >/dev/null 2>&1 && break
  sleep 1
  [ "${i}" -eq 30 ] && { echo "TIMEOUT: servicio no responde"; exit 1; }
done
echo "  Servicio listo"

# --- Newman ---
echo "--- Newman ---"
newman run postman/collection.json \
  -e postman/environment.local.json \
  --env-var "baseUrl=http://localhost:${PORT}/api/v1" \
  --env-var "hostUrl=http://localhost:${PORT}" \
  --delay-request 50 \
  --reporters cli,json \
  --reporter-json-export newman-report.json \
  --bail

echo "--- e2e OK ---"
