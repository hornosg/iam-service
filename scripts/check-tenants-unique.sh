#!/usr/bin/env bash
# check-tenants-unique.sh — ACC-E04.T10: la tabla `tenants` existe en EXACTAMENTE una
# base del lab. Falla el build si aparece una segunda definición `CREATE TABLE tenants`
# en las migraciones de account-service (este repo) o de los servicios de
# mercado-cercano. Absorbe PLAT-E34 T7.
#
# Alcance del escaneo (contrato de la épica, "account-service + servicios MC"):
#   - migrations/ de este repo (siempre presente);
#   - migrations/ de cada servicio bajo MC_SERVICES_DIR. Local se resuelve al layout
#     del lab (active/mercado-cercano/services); el CI clona los repos de la org
#     mercadocercano y apunta MC_SERVICES_DIR ahí (workflow ci.yml, job tenants-unique).
#   NO escanea other/ ni pocs/: son proyectos heredados/referencia fuera del lab vivo.
#     (flexibility-pay, en other/, tiene su propia tabla `tenants` y NO es una
#     violación — el scope deliberado es lo que permite exit 0 hoy.)
#
# Fail-closed: si no puede armar el alcance completo (sin MC_SERVICES_DIR y sin el
# layout del lab), corta con exit 2 — un check que escanea menos de lo prometido no
# distingue "1 tabla" de "no miré".
#
# Uso:
#   bash scripts/check-tenants-unique.sh                          # local (layout del lab)
#   MC_SERVICES_DIR=/tmp/mc-services ./scripts/check-tenants-unique.sh   # CI
#
# Exit: 0 = exactamente 1 definición y es la de este repo; 1 = 0 o ≥2 definiciones
# (nombra el intruso); 2 = alcance incompleto.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PATTERN='CREATE TABLE.*\btenants\b'

is_own() {
  case "$1" in "$REPO_ROOT"/*) return 0 ;; *) return 1 ;; esac
}

label() {
  # Etiqueta cada match con su origen: account-service o mc/<servicio>.
  # Los paths se canonicalizan ANTES de comparar: el layout del lab produce rutas con
  # ../.. que un match por prefijo clasificaría mal (intruso leído como propio).
  local f
  f="$(realpath "$1")"
  if is_own "$f"; then
    echo "account-service: ${f#"$REPO_ROOT"/}"
  elif [[ -n "${mc_dir:-}" && "$f" == "$mc_dir"/* ]]; then
    echo "mc/${f#"$mc_dir"/}"
  else
    echo "mc: $f"
  fi
}

is_intruder() {
  local f
  f="$(realpath "$1")"
  ! is_own "$f"
}

# --- alcance ---
mc_dir="${MC_SERVICES_DIR:-}"
if [[ -z "$mc_dir" ]]; then
  candidate="$REPO_ROOT/../../active/mercado-cercano/services"
  if [[ -d "$candidate" ]]; then
    mc_dir="$(realpath "$candidate")"
    echo "(MC_SERVICES_DIR no seteado — usando el layout del lab: $mc_dir)"
  fi
fi

shopt -s nullglob
mc_migs=()
if [[ -n "$mc_dir" ]]; then
  for d in "$mc_dir"/*/migrations; do
    mc_migs+=("$d")
  done
fi
n_services="${#mc_migs[@]}"
if (( n_services == 0 )); then
  echo "ERROR (fail-closed): alcance incompleto — no hay migraciones de servicios MC." >&2
  echo "  El check promete escanear account-service + servicios MC; con menos alcance no" >&2
  echo "  distingue '1 tabla' de 'no miré'. Seteá MC_SERVICES_DIR (CI) o corré desde el lab." >&2
  exit 2
fi

roots=("$REPO_ROOT/migrations" "${mc_migs[@]}")

echo "== check-tenants-unique — ACC-E04.T10 (absorbe PLAT-E34 T7) =="
echo "Patrón: grep -rln --include='*.sql' '$PATTERN'"
echo "Raíces: account-service/migrations + $n_services servicios MC"

# grep exit 1 (sin matches) es un estado válido, no un error: || true y el conteo decide.
matches="$(grep -rln --include='*.sql' "$PATTERN" "${roots[@]}" || true)"

declared=()
count=0
if [[ -n "$matches" ]]; then
  while IFS= read -r f; do
    declared+=("$f")
    count=$((count + 1))
  done <<<"$matches"
fi

echo "Definiciones encontradas: $count"
if (( count > 0 )); then
  for f in "${declared[@]}"; do
    echo "  - $(label "$f")"
  done
fi

if (( count != 1 )); then
  echo "RESULTADO: FALLA — $count definiciones de la tabla \`tenants\` en el lab (esperado exactamente 1)."
  for f in "${declared[@]}"; do
    if is_intruder "$f"; then
      echo "🔴 INTRUSO: $(label "$f")"
    fi
  done
  exit 1
fi

if ! is_own "$(realpath "${declared[0]}")"; then
  echo "RESULTADO: FALLA — la única definición de \`tenants\` está fuera de account-service ($(label "${declared[0]}"))."
  echo "  La identidad del tenant vive en iam_db (account-service); si se movió, el check"
  echo "  y la épica se re-escriben juntos, no uno detrás del otro."
  exit 1
fi

echo "RESULTADO: OK — una sola tabla \`tenants\` en el lab: $(label "${declared[0]}")"
exit 0