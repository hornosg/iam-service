#!/usr/bin/env bash
# bootstrap_migrator.sh — ACC-E02 T8n: bootstrap one-shot del rol de migraciones
# account_migrator.
#
# POR QUÉ EXISTE ESTE SCRIPT (la circularidad, documentada y no implícita):
# desde T8n las migraciones de iam_db corren con el rol account_migrator, pero la
# migración 024 es la que CREA ese rol. En toda base donde 024 esté pendiente, el
# rol todavía no puede aplicar 024: hay que correrla UNA VEZ con un rol que ya
# pueda crear roles. Ese bootstrap es ESTE script: aplica el archivo 024 con
# postgres (superuser del lab) y fija la password out-of-band (patrón 017/018: un
# literal en una migración quedaría en git history para siempre).
#
# El registro de versión queda en manos de RunMigrations: este script NO toca
# schema_migrations. El próximo arranque del servicio re-aplica 024 vía
# golang-migrate — todos sus statements son no-ops por sus guards — y estampa v24
# en la única fila. NUNCA estampar schema_migrations a mano: el driver mantiene
# una sola fila (SetVersion = DELETE+INSERT) y el camino manual dejó tres y al
# servicio a un restart de no arrancar.
#
# Base NUEVA (nada aplicado): correr este script primero — crea el rol y le cede
# la base — y el primer arranque aplica 001→024 como account_migrator.
#
# Idempotente: re-ejecutar es seguro (todos los statements de 024 tienen guard;
# ALTER ROLE ... PASSWORD re-fija el mismo valor).
#
# Uso:
#   ./scripts/bootstrap_migrator.sh
#   LAB_POSTGRES_CONTAINER=lab-postgres IAM_MIGRATE_DB_PASSWORD=... ./scripts/bootstrap_migrator.sh
#
# Salidas: 0 si aplicó y verificó; !=0 si falló.
set -euo pipefail

CONTAINER="${LAB_POSTGRES_CONTAINER:-lab-postgres}"
ADMIN_USER="${POSTGRES_USER:-postgres}"
DB_NAME="${MIGRATE_DB_NAME:-iam_db}"
# Default de LAB, igual que lab_account_app / lab_iam_login (T5). En prod se
# overridea por env y NO se versiona; además no comparte secreto con los otros
# roles: una credencial comprometida no debe entregar las demás.
# SIN default a propósito. Este script fija la password de un rol que es DUEÑO de las tablas y
# tiene CREATEROLE: un valor por defecto en el repo es una credencial publicada para el rol más
# privilegiado del servicio. El hook de pre-commit lo rechazó, y tenía razón.
# El compose sí conserva un default de LAB para levantar sin fricción, pero el bootstrap —que es
# quien la ESTABLECE— exige que la digas.
if [[ -z "${IAM_MIGRATE_DB_PASSWORD:-}" ]]; then
  echo "IAM_MIGRATE_DB_PASSWORD no está definida." >&2
  echo "  Es la password de account_migrator (dueño de las tablas, CREATEROLE): no tiene default." >&2
  echo "  En lab, para reproducir el default del compose:" >&2
  echo "    IAM_MIGRATE_DB_PASSWORD=lab_account_migrator ./scripts/bootstrap_migrator.sh" >&2
  exit 2
fi
MIGRATE_PASSWORD="$IAM_MIGRATE_DB_PASSWORD"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATION_FILE="$SCRIPT_DIR/../migrations/024_create_migrator_role.up.sql"

if ! docker ps --format '{{.Names}}' | grep -qx "$CONTAINER"; then
  echo "ERROR: el contenedor '$CONTAINER' no está corriendo." >&2
  exit 1
fi
if [ ! -f "$MIGRATION_FILE" ]; then
  echo "ERROR: no existe $MIGRATION_FILE" >&2
  exit 1
fi

echo "→ Aplicando $MIGRATION_FILE con $ADMIN_USER (bootstrap one-shot de T8n)..."
docker exec -i "$CONTAINER" psql -U "$ADMIN_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 -1 \
  < "$MIGRATION_FILE"

echo "→ Fijando password de account_migrator (out-of-band, no versionado)..."
docker exec -i "$CONTAINER" psql -U "$ADMIN_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 -1 \
  < <(printf "ALTER ROLE account_migrator WITH PASSWORD '%s';\n" "$MIGRATE_PASSWORD")

echo "→ Verificación:"
docker exec "$CONTAINER" psql -U "$ADMIN_USER" -d "$DB_NAME" -c \
  "SELECT rolname, rolsuper, rolcreatedb, rolcreaterole, rolbypassrls, rolcanlogin
     FROM pg_roles WHERE rolname = 'account_migrator';"
docker exec "$CONTAINER" psql -U "$ADMIN_USER" -d "$DB_NAME" -tAc \
  "SELECT 'db_owner=' || pg_get_userbyid(datdba) FROM pg_database WHERE datname = '${DB_NAME}';"
echo "Bootstrap listo. El próximo arranque del servicio estampa v24 vía RunMigrations."