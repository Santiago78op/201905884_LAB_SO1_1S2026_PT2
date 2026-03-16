#!/usr/bin/env bash

# Aborta si cualquier comando falla (-e), si se usa una variable no definida (-u) o si un comando en una tubería falla (-o pipefail).
set -euo pipefail

MODULE_NAME="pr2_so1_201905884"

PROC_MEM="/proc/meminfo_${MODULE_NAME}"
PROC_CONT="/proc/continfo_${MODULE_NAME}"

# 1. Si el módulo NO está cargado, no hay nada que hacer
if ! lsmod | grep -q "^${MODULE_NAME}[[:space:]]"; then
    echo "[kernel-unloader] módulo no estaba cargado"
    exit 0
fi

# 2. Descargar el módulo
sudo rmmod "${MODULE_NAME}"

# 3. Verificar que las entradas /proc desaparecieron
if [ -e "${PROC_MEM}" ] || [ -e "${PROC_CONT}" ]; then
    echo "WARN: entradas /proc aún presentes tras rmmod" >&2
fi

echo "[kernel-unloader] módulo descargado OK"
