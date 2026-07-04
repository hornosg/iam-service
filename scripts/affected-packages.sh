#!/usr/bin/env bash
set -euo pipefail

# affected-packages: resuelve qué paquetes Go corrieron riesgo con un diff.
# Uso: ./scripts/affected-packages.sh <base-ref> [head-ref]
# Salida: import paths afectados, uno por línea (ninguno si el diff no tocó .go/go.mod/go.sum).
#
# "Afectado" = el paquete cuyo directorio tiene un archivo .go modificado, MÁS sus
# dependientes DIRECTOS (paquetes que lo importan) — un solo salto, no transitivo.
# Los paquetes de test/ externos (test/<dominio>/... que importan iam/src/<dominio>/...)
# quedan cubiertos por esta misma regla: son dependientes directos del paquete src/ que
# testean, sin necesitar mapeo especial src/ -> test/.
#
# Si go.mod o go.sum cambiaron, el grafo de dependencias pudo moverse entero: no hay
# subconjunto seguro, se listan todos los paquetes del módulo (mismo criterio que un
# cambio en cualquier paquete raíz ampliamente importado).
#
# Requiere: go, jq.

BASE_REF="${1:?Uso: $0 <base-ref> [head-ref]}"
HEAD_REF="${2:-HEAD}"

for dep in go jq git; do
  command -v "${dep}" >/dev/null 2>&1 || { echo "ERROR: ${dep} no está en PATH" >&2; exit 1; }
done

ROOT="$(git rev-parse --show-toplevel)"
cd "${ROOT}"

MODULE="$(go list -m)"

changed_files="$(git diff --name-only "${BASE_REF}...${HEAD_REF}" -- '*.go' 'go.mod' 'go.sum')"

if [ -z "${changed_files}" ]; then
  exit 0
fi

if echo "${changed_files}" | grep -qE '^go\.(mod|sum)$'; then
  go list ./...
  exit 0
fi

# --- Mapa dir-relativo -> import path (una pasada de `go list`, sin JSON) ---
pkg_map="$(go list -f '{{.ImportPath}}|{{.Dir}}' ./...)"

declare -A DIR_TO_PKG
while IFS='|' read -r import_path abs_dir; do
  rel_dir="${abs_dir#"${ROOT}"}"
  rel_dir="${rel_dir#/}"
  [ -z "${rel_dir}" ] && rel_dir="."
  DIR_TO_PKG["${rel_dir}"]="${import_path}"
done <<< "${pkg_map}"

# --- Paquetes con un archivo modificado en su directorio ---
declare -A AFFECTED
while IFS= read -r dir; do
  pkg="${DIR_TO_PKG[${dir}]:-}"
  [ -n "${pkg}" ] && AFFECTED["${pkg}"]=1
done < <(echo "${changed_files}" | xargs -n1 dirname | sort -u)

if [ "${#AFFECTED[@]}" -eq 0 ]; then
  # Los .go modificados no caen en ningún paquete conocido (no debería pasar salvo
  # archivos generados/ignorados por `go list`).
  exit 0
fi

# Snapshot de lo directamente modificado — los dependientes se calculan contra ESTE
# set fijo, un solo salto, no contra el set creciente (eso sería transitivo).
declare -A CHANGED
for pkg in "${!AFFECTED[@]}"; do CHANGED["${pkg}"]=1; done

# --- Dependientes directos: pares "import_path_importado <TAB> paquete_que_importa" ---
# Unión de Imports (GoFiles) + TestImports (archivos _test.go internos, package X) +
# XTestImports (archivos _test.go externos, package X_test) — un directorio de test/
# que solo tiene _test.go (sin archivo regular) reporta sus imports en TestImports/
# XTestImports, NUNCA en Imports (que queda null). Mirar solo Imports pierde en
# silencio la mayoría de los paquetes reales de test/ de este repo.
edges="$(go list -json ./... | jq -r --arg mod "${MODULE}" '
  .ImportPath as $p
  | ((.Imports // []) + (.TestImports // []) + (.XTestImports // []))[]
  | select(startswith($mod))
  | "\(.)\t\($p)"
')"

while IFS=$'\t' read -r imported importer; do
  [ -z "${imported}" ] && continue
  [ -n "${CHANGED[${imported}]:-}" ] && AFFECTED["${importer}"]=1
done <<< "${edges}"

printf '%s\n' "${!AFFECTED[@]}" | sort -u
