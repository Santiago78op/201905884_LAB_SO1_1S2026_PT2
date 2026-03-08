# Le dice al sistema que este archivo es un script bash.
#!/usr/bin/env bash

# Aborta si cualquier comando falla (-e), si se usa una variable no definida (-u) o si un comando en una tubería falla (-o pipefail).
set -euo pipefail

# Nombre del módulo (igual que el .c y el .ko)
MODULE_NAME="pr2_so1_201905884"
# Toma el primer argumento $1; si no se pasó, queda vacio en lugar de causar un error por -u.
MODULE_PATH="${1:-}"

# Bloque de rutas

# SCRIPT_DIR obtiene el directorio donde se encuentra este script, sin importar desde dónde se ejecute.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# PROJECT_ROOT es el directorio padre de SCRIPT_DIR, asumiendo que este script está en un subdirectorio del proyecto.
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
# KERNEL_DIR es el subdirectorio "Kernel" dentro del proyecto, donde se espera que esté el archivo .ko del módulo.
KERNEL_DIR="${PROJECT_ROOT}/Kernel"
# KO_FILE es la ruta completa al archivo .ko del módulo, construida a partir de KERNEL_DIR y MODULE_NAME.
KO_FILE="${KERNEL_DIR}/${MODULE_NAME}.ko"

# Rutas a los archivos en /proc que el módulo creará, usando el nombre del módulo para diferenciarlos.
PROC_MEM="/proc/meminfo_${MODULE_NAME}"
PROC_CONT="/proc/continfo_${MODULE_NAME}"

# Bloque lógica

# 1. Si el módulo ya está cargado, no hacer nada
# lsmod | grep — lista módulos activos y busca el nombre del modulo; si ya está, sale sin error
if lsmod | grep -q "^${MODULE_NAME}[[:space:]]"; then
    echo "[kernel-loader] módulo ya cargado"
    exit 0
fi

# 2. Compilar solo si el .ko no existe
# [ ! -f "${KO_FILE}" ] — si el .ko no existe todavía, lo compila con make
if [ ! -f "${KO_FILE}" ]; then
    echo "[kernel-loader] compilando..."
    make -C "${KERNEL_DIR}"
fi

# 3. Cargar el módulo
# insmod — carga el .ko en el kernel; si hay CONTAINER_ID, se lo pasa como parámetro al módulo
if [ -n "${CONTAINER_ID}" ]; then
    insmod "${KO_FILE}" container_id="${CONTAINER_ID}"
else
    insmod "${KO_FILE}"
fi

# 4. Verificar que /proc fue creado
# [ -r ... ] — verifica que las entradas /proc existen y son legibles; si no, el módulo falló al iniciarse
[ -r "${PROC_MEM}"  ] || { echo "ERROR: ${PROC_MEM} no existe"  >&2; exit 1; }
[ -r "${PROC_CONT}" ] || { echo "ERROR: ${PROC_CONT} no existe" >&2; exit 1; }

echo "[kernel-loader] módulo cargado OK"
