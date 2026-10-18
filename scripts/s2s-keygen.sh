#!/usr/bin/env bash
# ACC-E09 T2 (ADR-006 «Registro») — genera el par RSA de un EMISOR de tokens de servicio.
#
#   scripts/s2s-keygen.sh <servicio>
#
# - Pública  → ${S2S_KEYS_DIR:-<lab>/infra/s2s-keys}/<servicio>/<kid>.pem  (registro de emisores
#   de iam, S2S_ISSUER_KEYS_DIR; versionable: es material público).
# - Privada  → ${S2S_PRIVATE_KEY_DIR:-$HOME/.s2s-keys}/<servicio>/<kid>.pem, 0600. Se monta SÓLO en
#   el contenedor del emisor; nunca en iam, nunca en infra/, nunca en git.
#
# El kid es el de ADR-003 §c (thumbprint RFC 7638, "acc-<12hex>"), calculado con el mismo código
# que el runtime (scripts/signing-kid), así el registro de iam y el header kid de la aserción
# coinciden. Rotación: correr de nuevo deja dos claves para el emisor (el registro admite ≤ 2);
# borrar la vieja de infra/s2s-keys/<servicio>/ cierra la ventana.
set -euo pipefail

SERVICE="${1:-}"
if [[ ! "$SERVICE" =~ ^[a-z][a-z0-9-]*$ ]]; then
  echo "uso: $0 <servicio>   (minúsculas, dígitos y guiones; ej. catalog-bff-service)" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LAB="$(cd "$ROOT/../.." && pwd)"
PUB_DIR="${S2S_KEYS_DIR:-$LAB/infra/s2s-keys}/$SERVICE"
PRIV_DIR="${S2S_PRIVATE_KEY_DIR:-$HOME/.s2s-keys}/$SERVICE"

mkdir -p "$PUB_DIR" "$PRIV_DIR"
PUB_REAL="$(cd "$PUB_DIR/.." && pwd -P)"
PRIV_REAL="$(cd "$PRIV_DIR" && pwd -P)"
case "$PRIV_REAL/" in
  "$PUB_REAL"/*) echo "la privada no puede quedar dentro del registro público ($PUB_REAL)" >&2; exit 1 ;;
esac
chmod 700 "$PRIV_DIR"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
umask 077
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$TMP/priv.pem" 2>/dev/null
KID="$(cd "$ROOT" && go run ./scripts/signing-kid "$TMP/priv.pem")"
if [[ ! "$KID" =~ ^acc-[0-9a-f]{12}$ ]]; then
  echo "kid inesperado: $KID" >&2
  exit 1
fi
openssl pkey -in "$TMP/priv.pem" -pubout -out "$TMP/pub.pem"

install -m 600 "$TMP/priv.pem" "$PRIV_DIR/$KID.pem"
install -m 644 "$TMP/pub.pem" "$PUB_DIR/$KID.pem"

echo "kid:     $KID"
echo "pública: $PUB_DIR/$KID.pem"
echo "privada: $PRIV_DIR/$KID.pem (0600 — montar sólo en $SERVICE)"
