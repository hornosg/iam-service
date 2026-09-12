#!/usr/bin/env bash
# coverage-gate.sh — wrapper de iam-service (PLAT-E08.T5).
#
# La herramienta compartida vive en management/scripts/coverage-gate.sh
# (D-a: gobernanza del lab, cross-project — un gate por repo diverge y deja
# de comparar). Este wrapper mantiene el contrato histórico de iam-service:
#
#   --help                                 ayuda de ambos modos
#   --servicio <path> --umbral <N> --comando '<go test>'
#                                          → delega a la herramienta compartida
#                                            (medición total, dimensión
#                                            `cobertura` del scorecard)
#   [--coverprofile <f>] <base-ref> [head] → delega a coverage-gate-diff.sh
#                                            (diff-coverage de CI, PLAT-E21 T5;
#                                            implementación local porque usa
#                                            affected-packages.sh y los awk
#                                            de este directorio)
#
# Asume el layout del lab ($DEVY_PATH con management/ al lado de platform/).

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPARTIDO="$DIR/../../../management/scripts/coverage-gate.sh"

case "${1:-}" in
  --help|-h)
    echo "coverage-gate.sh — wrapper de iam-service (PLAT-E08.T5)"
    echo ""
    echo "Modos:"
    echo "  --servicio <path> --umbral <N> --comando '<go test>'"
    echo "      Medición total de cobertura del servicio (compartida,"
    echo "      management/scripts/coverage-gate.sh — dimensión cobertura del scorecard)."
    echo "  [--coverprofile <f>] <base-ref> [head-ref]"
    echo "      Diff-coverage de CI contra coverage-baseline.json (coverage-gate-diff.sh)."
    exit 0
    ;;
  --servicio)
    exec bash "${COMPARTIDO}" "$@"
    ;;
  *)
    exec bash "$DIR/coverage-gate-diff.sh" "$@"
    ;;
esac