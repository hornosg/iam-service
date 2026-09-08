#!/usr/bin/env bash
# ACC-E03 T2 — genera el par de claves de firma RS256 (ADR-003 §b) sin versionar
# NINGÚN byte de material privado.
#
#   scripts/generate-signing-keys.sh [dir-salida] [--force]
#
# Produce en <dir-salida> (default: keys/, ignorado por git):
#   - jwt_signing_private.pem   (PKCS#8, RSA-2048, permisos 0600)
#   - jwt_signing_public.pem    (PEM público — para Kong en T5 y deploys)
#   - signing-key.json          (kid registrado + metadatos)
#
# El kid es determinístico (thumbprint RFC 7638 del material público,
# "acc-<12hex>"): dos ejecuciones sobre la misma clave dan el mismo kid.
# Para rotar (T8), generá un par NUEVO con un nombre distinto:
#   scripts/generate-signing-keys.sh keys --name rotacion-2027-01 --force
set -euo pipefail

cd "$(dirname "$0")/.."

OUT_DIR="keys"
NAME="jwt_signing"
FORCE=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --force) FORCE=1 ;;
    --name) NAME="$2"; shift ;;
    *) OUT_DIR="$1" ;;
  esac
  shift
done

PRIV="$OUT_DIR/${NAME}_private.pem"
PUB="$OUT_DIR/${NAME}_public.pem"
META="$OUT_DIR/${NAME}.json"

if [[ -e "$PRIV" && "$FORCE" -ne 1 ]]; then
  echo "REFUSADO: $PRIV ya existe. Sobreescribir la clave viva invalida todos los"
  echo "tokens emitidos con ella. Para rotar, generá un par NUEVO (--name) o pasá"
  echo "--force si realmente querés reemplazarla (entorno descartable)."
  exit 1
fi

mkdir -p "$OUT_DIR"

# RSA-2048 en PKCS#8 (cabecera "BEGIN" de clave privada) — genpkey emite PKCS#8 por defecto.
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$PRIV"
chmod 0600 "$PRIV"

# El par es válido si la pública se deriva de la privada sin error
# (criterio (b) de la tarea) — y la pública es la que T5 templatiza en Kong.
openssl pkey -in "$PRIV" -pubout -out "$PUB"
chmod 0644 "$PUB"

# kid determinístico, computado con la MISMA derivación del runtime.
KID="$(go run ./scripts/signing-kid "$PRIV")"

CREATED="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
cat > "$META" <<EOF
{
  "kid": "$KID",
  "algorithm": "RS256",
  "private_key_file": "$PRIV",
  "public_key_file": "$PUB",
  "created_at": "$CREATED",
  "note": "Material generado por ACC-E03 T2. La privada NO se versiona (keys/ en .gitignore). El kid es thumbprint RFC 7638 del material publico."
}
EOF

echo "Par generado:"
echo "  privada : $PRIV (0600, NO versionada)"
echo "  pública : $PUB"
echo "  kid     : $KID"
echo "Para inyectarla en dev: JWT_PRIVATE_KEY_FILE=$PRIV"