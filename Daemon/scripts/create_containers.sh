#!/bin/bash
#
# Script ejecutado por el cronjob del daemon SO1 PT2.
# Crea 5 contenedores aleatorios de las 3 categorías:
#   0 → roldyoran/go-client        (alto consumo RAM)
#   1 → alpine stress (bc)         (alto consumo CPU)
#   2 → alpine sleep 240           (bajo consumo)

for i in $(seq 1 5); do
    case $((RANDOM % 3)) in
        0)
            docker run -d roldyoran/go-client
            ;;
        1)
            docker run -d alpine sh -c "while true; do echo '2^20' | bc > /dev/null; sleep 2; done"
            ;;
        2)
            docker run -d alpine sleep 240
            ;;
    esac
done
