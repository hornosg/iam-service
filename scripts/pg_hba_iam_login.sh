#!/usr/bin/env bash
# pg_hba_iam_login.sh — T1-D4 (ACC-E02 T2): blast-radius control del rol iam_login
#
# iam_login lee password_hash de TODOS los usuarios sin filtro de tenant (por diseño,
# ver migrations/017). Este script acota desde DÓNDE se puede usar ese rol: sólo la red
# de iam-service (lab-network). Cualquier otro origen se rechaza. Es defensa en profundidad
# sobre el password (que se fija out-of-band, nunca versionado): si las credenciales de DB
# leak, igual no se pueden usar desde fuera del lab.
#
# Idempotente: reescribe el bloque marcado en cada corrida (no duplica líneas).
# Sin restart: sólo pg_reload_conf() — las conexiones existentes no se ven afectadas.
# Aditivo: las reglas sólo afectan al rol iam_login sobre iam_db; todo lo demás cae al
# catch-all existente. No rompe otros servicios del lab.
#
# Orden de pg_hba = first-match, por eso las reglas van ANTES del catch-all
# (`host all all all scram-sha-256`): sin la línea reject, el catch-all dejaría pasar
# a iam_login desde cualquier host.
#
# Producción (k3s): el equivalente es un NetworkPolicy + host allowlist; este script es
# el control del lab (lab-postgres). Re-aplicar tras recrear el volumen de lab-postgres.
#
# Uso:
#   ./scripts/pg_hba_iam_login.sh                 # usa defaults
#   LAB_POSTGRES_CONTAINER=lab-postgres IAM_LOGIN_NETWORK=172.18.0.0/16 ./scripts/...
#
# Salidas: 0 si aplicó y recargó; !=0 si falló.
set -euo pipefail

CONTAINER="${LAB_POSTGRES_CONTAINER:-lab-postgres}"
ADMIN_USER="${POSTGRES_USER:-postgres}"
ALLOW_CIDR="${IAM_LOGIN_NETWORK:-172.18.0.0/16}"   # lab-network = red de iam-service
HBA_PATH="/var/lib/postgresql/data/pg_hba.conf"
MARK_BEGIN="# >>> iam_login blast-radius (ACC-E02 T2 / T1-D4) >>>"
MARK_END="# <<< iam_login blast-radius (ACC-E02 T2 / T1-D4) <<<"

if ! docker ps --format '{{.Names}}' | grep -qx "$CONTAINER"; then
  echo "ERROR: el contenedor '$CONTAINER' no está corriendo." >&2
  exit 1
fi

# awk corre dentro del contenedor (como postgres) para preservar ownership del archivo.
docker exec -i -u postgres "$CONTAINER" bash -s -- "$HBA_PATH" "$ALLOW_CIDR" "$MARK_BEGIN" "$MARK_END" <<'AWK'
set -euo pipefail
HBA="$1"; ALLOW="$2"; BEGIN="$3"; END="$4"
TMP="$(mktemp)"

awk -v allow="$ALLOW" -v begin="$BEGIN" -v end="$END" '
  # saltar bloque marcado preexistente (idempotencia)
  $0 == begin { inblock=1; next }
  inblock && $0 == end { inblock=0; next }
  inblock { next }

  # insertar el bloque antes del catch-all `host all all all ...`
  $0 ~ /^host[[:space:]]+all[[:space:]]+all[[:space:]]+all[[:space:]]+/ && !done {
    print begin
    print "# iam_login lee password_hash de TODOS los usuarios sin filtro de tenant (por diseno)."
    print "# Acota el blast radius: solo la red de iam-service (lab-network) puede usarlo;"
    print "# el resto se rechaza. Password out-of-band (no versionado). Prod (k3s): NetworkPolicy."
    print "host iam_db iam_login " allow " scram-sha-256"
    print "host iam_db iam_login 0.0.0.0/0 reject"
    print "host iam_db iam_login ::/0 reject"
    print end
    done=1
  }
  { print }
' "$HBA" > "$TMP"

# escribir sobre el archivo original (preserva inode/ownership/perms)
cat "$TMP" > "$HBA"
rm -f "$TMP"
echo "pg_hba actualizado."
AWK

# Recargar pg_hba (sin restart).
docker exec "$CONTAINER" psql -U "$ADMIN_USER" -d postgres -tAc \
  "SELECT pg_reload_conf();" >/dev/null
echo "pg_hba recargado para '$CONTAINER'."

# Verificar que las reglas quedaron cargadas.
echo "--- reglas iam_login activas (pg_hba_file_rules) ---"
docker exec "$CONTAINER" psql -U "$ADMIN_USER" -d postgres -c \
  "SELECT type, database, user_name, address, auth_method
     FROM pg_hba_file_rules
    WHERE user_name = '{iam_login}'
    ORDER BY line_number;"