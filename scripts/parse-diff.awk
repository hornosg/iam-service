# parse-diff.awk — lee `git diff -U0` por stdin y emite los hunks del lado NUEVO
# (HEAD), uno por línea:  <module>/<path>\t<start>\t<end>
#
# Archivos borrados (+++ /dev/null) se ignoran: no hay líneas nuevas que medir.
# Los rangos son inclusivos [start,end]. Uso:
#   git diff -U0 <base>...<head> -- '*.go' | awk -v mod=iam -f parse-diff.awk
#
# PLAT-E21 T5 — insumo del gate de cobertura (scripts/coverage-gate.sh).

/^diff --git/ { file = "" }
/^\+\+\+ / {
    if ($0 ~ /\/dev\/null/) { file = ""; next }
    # "+++ b/<path>" -> el path arranca en la posición 7 (después de "+++ b/")
    file = substr($0, 7)
    next
}
/^@@/ {
    if (file == "") next
    # Campos: $1=@@  $2=-a,b  $3=+c,d  $4=@@   (git diff -U0 sin context lines)
    plus = $3
    sub(/^\+/, "", plus)
    nf = split(plus, p, ",")
    start = p[1] + 0
    cnt = (nf >= 2 ? p[2] + 0 : 1)
    if (cnt == 0) next
    end = start + cnt - 1
    printf "%s/%s\t%s\t%s\n", mod, file, start, end
}