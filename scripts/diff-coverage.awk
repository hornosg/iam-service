# diff-coverage.awk — calcula la cobertura de las líneas NUEVAS/MODIFICADAS del diff.
#
# Entrada (en orden, dos archivos):
#   1) hunks:  <coverprofile-path>\t<start>\t<end>   por línea  (salida de parse-diff.awk)
#   2) coverprofile de Go:  "mode: set" + bloques
#        <path>:<start>.<col>,<end>.<col> <numStmt> <count>
#
# Un bloque del coverprofile cuenta para el diff si su rango [blockStart,blockEnd]
# se solapa con algún hunk [hStart,hEnd] del MISMO archivo (mismo path del coverprofile,
# prefijo de módulo incluido). Suma numStmt al total y, si count>0, al cubierto.
#
# DEDUP: cuando el coverprofile viene de varios binarios de test (vá accustomed con
# -coverpkg=./src/... y múltiples paquetes test/), el MISMO bloque aparece una vez
# por binario, sin mergear. Se deduplica por (file,start,end) tomando max(count):
# una línea cuenta como cubierta si ALGÚN binario de test la ejercitó.
#
# Salida: "NOSTMTS" si ningún hunk tocó statements medibles, o "<pct> <covered> <total>".
#
# PLAT-E21 T5 — núcleo del gate de cobertura (scripts/coverage-gate.sh).

FNR == NR {
    # Primer archivo: hunks.  Rangos por archivo como "start-end start-end ...".
    if ($0 == "") next
    key = $1
    hunks[key] = (key in hunks ? hunks[key] " " $2 "-" $3 : $2 "-" $3)
    next
}

/^mode:/ { next }
NF < 3 { next }

{
    # Bloque: path:start.col,end.col numStmt count
    colon = index($1, ":")
    file = substr($1, 1, colon - 1)
    if (!(file in hunks)) next            # sólo importan los bloques de archivos cambiados
    range = substr($1, colon + 1)          # start.col,end.col
    comma = index(range, ",")
    startPart = substr(range, 1, comma - 1)
    endPart = substr(range, comma + 1)
    dot = index(startPart, "."); bstart = substr(startPart, 1, dot - 1) + 0
    dot = index(endPart, ".");   bend   = substr(endPart,   1, dot - 1) + 0
    numStmt = $2 + 0
    count = $3 + 0

    # Dedup por (file,start,end): mismo numStmt, count = max(count) sobre los duplicados.
    bkey = file SUBSEP bstart SUBSEP bend
    if (bkey in bidx) {
        i = bidx[bkey]
        if (count > bcount[i]) bcount[i] = count
    } else {
        nb++
        bidx[bkey] = nb
        bfile[nb] = file
        bstartA[nb] = bstart
        bendA[nb] = bend
        bstmt[nb] = numStmt
        bcount[nb] = count
    }
}

END {
    for (i = 1; i <= nb; i++) {
        f = bfile[i]
        if (!(f in hunks)) continue
        n = split(hunks[f], harr, " ")
        for (j = 1; j <= n; j++) {
            dash = index(harr[j], "-")
            hs = substr(harr[j], 1, dash - 1) + 0
            he = substr(harr[j], dash + 1) + 0
            if (bstartA[i] <= he && bendA[i] >= hs) {   # solapa
                total += bstmt[i]
                if (bcount[i] > 0) covered += bstmt[i]
                break
            }
        }
    }
    if (total == 0) { print "NOSTMTS"; exit 0 }
    printf "%.1f %d %d\n", (covered / total) * 100, covered, total
}