# Enunciado del Proyecto

En la administración de sistemas y el desarrollo de aplicaciones contenerizadas, obtener
información detallada sobre los procesos en ejecución y tomar acciones automatizadas es
un desafío crítico. Aunque herramientas como ps o docker stats ofrecen datos básicos,
carecen de acceso directo a las estructuras del kernel y no permiten una gestión proactiva
de contenedores. Este proyecto propone una solución integral: desarrollar un módulo de
kernel en C que exponga métricas avanzadas de procesos y contenedores (CPU, memoria,
E/S) a través de /proc, junto con un Daemon en GO que no solo presente estos datos de
forma legible, sino que también automatice decisiones (como terminar contenedores que
excedan umbrales de recursos) a su vez que guarda datos importantes en Valkey para su
posterior uso en Grafana. Para validar el sistema, se implementará cronjobs que generen
contenedores de prueba cada minuto, simulando condiciones de carga y permitiendo
evaluar la eficacia de las acciones correctivas. Así, el proyecto combina aprendizaje en
programación a bajo nivel (kernel) y alto nivel (GO), mientras resuelve un problema real en
entornos contenerizados: la monitorización y estabilización autónoma del sistema. A su vez
que de dichos datos se mostraran de manera visual por medio de un dashboard en Grafana.

1.​ Un módulo de kernel desarrollado en C que actuará como sensor de bajo nivel,
accediendo directamente a las estructuras internas del kernel para capturar métricas
detalladas tanto de los procesos asociados a contenedores como de los procesos
generales del sistema, incluyendo consumo de recursos como CPU, memoria y E/S.

2.​ Una Daemon en GO que funcionará como cerebro del sistema, procesando los
datos del kernel en tiempo real para:

    ○​ Tomar decisiones autónomas detener y eliminar contenedores basadas en
    umbrales dinámicos y patrones de comportamiento establecidos
    previamente.
    ○​ Simular y validar la eficacia del sistema bajo condiciones de uso prolongado.
    ○​ Ejecutar scripts de automatización durante la ejecución.
    
3.​ Un Cronjob encargado de ejecutar el script que generara los contenedores de
Docker cada minuto.

4.​ Un Dashboard en Grafana utilizado para mostrar la información que recolecta el
servicio de GO.

## MÓDULO DE KERNEL

Deberá crear un módulo que capture las métricas necesarias para el análisis de la memoria
y los contenedores activos en el sistema.
La información debe ser capturada y guardada en la carpeta /proc:

    1. Capturar en MB o KB (a discreción del estudiante):
    ●​ Total de memoria RAM
    ●​ Memoria RAM libre
    ●​ Memoria RAM en uso​

2. Todos los procesos relacionados a los contenedores generados por el script así
como los procesos generales del sistema deberán contar con:

    ●​ PID
    ●​ Nombre
    ●​ Línea de comando que se ejecutó o ID del contenedor
    ●​ VSZ (Tamaño de la memoria virtual en KB)
    ●​ RSS (Tamaño de memoria física en KB)
    ●​ Porcentaje de Memoria utilizada
    ●​ Porcentaje de CPU utilizado
    Sugerencias:
    ●​ Utilizar la estructura task_struct (del kernel de Linux) para filtrar correctamente los
    procesos relacionados con los contenedores y extraer la información necesaria.
    ●​ En dado caso el porcentaje de CPU sea un número extremadamente grande se
    permite mantenerlo así debido a los cálculos diferenciales que retorna el kernel.​

3. Los datos deberán ser guardados en sus respectivos archivos en /proc
    ●​ Módulo de Procesos de Contenedores: /proc/continfo_pr2_so1_#CARNET

Daemon de GO
Descripción (corazón del proyecto):​
Se requiere desarrollar un gestor de contenedores en Go encargado del análisis,
ejecución y comunicación entre los diferentes componentes del servicio.​
Debe garantizar manejo seguro de memoria y cumplir con las siguientes funcionalidades:
1. Inicio del servicio
●​ Crear un contenedor de Grafana al inicializar el código.
●​ Grafana será el encargado de leer los logs generados por el servicio de Go después
del análisis de los datos.
●​ Se recomienda utilizar un Docker Compose para que el contenedor se pueda
comunicar con la base de datos Valkey
2. Cronjob
●​ El daemon de Go iniciará la implementación y ejecución del cronjob en el sistema
operativo, lo que activará el proceso de creación de contenedores.
3. Ejecución del script para cargar el módulo de kernel
●​ El daemon de Go ejecutará un script encargado de cargar e inicializar el módulo de
kernel.
4. Loop principal (ejecución cada 20 a 60 segundos)
●​ El daemon operará de manera infinita en segundo plano.
●​ En cada iteración (cada 20 a 60 segundos), realiza lo siguiente:
○​ Lectura del archivo en /proc/continfo_pr2_so1_#CARNET
○​ Deserialización del contenido
○​ Análisis para la gestión de contenedores (alta/baja, ajuste de recursos)
■​ detener y eliminar los contenedores según cálculos explicados en la
siguiente sección.
○​ Generación y almacenamiento de registros (logs) en una base de datos
Valkey, diseñado para su posterior visualización en Grafana​
5. Finalización del servicio
●​ Antes de finalizar, el servicio deberá eliminar el cronjob asociado a la creación de
los contenedores para evitar una sobrecarga en el sistema.
d
Me encuentro en la fase de desarrollo del Daemon en Go, en la cual necesito implementar la funcionalidad de cargar el módulo de kernel que he desarrollado. Para esto, quiero crear un nuevo paquete en Go que se encargue de ejecutar un script bash que compile y cargue el módulo de kernel, y luego verificar que las entradas en /proc se hayan creado correctamente para que el daemon pueda leerlas posteriormente.

Ya se creo la logica del módulo de kernel el cual se encuentra en la carpeta Kernel, ahora necesito que me muestres como crear el script bash para cargar el módulo y luego como crear el paquete en Go para ejecutarlo desde el main.go del daemon. 

Tambien te indico que la logica que lee los archivos en /proc ya esta implementada, por lo que solo necesito la parte de cargar el módulo y verificar que las entradas en /proc se hayan creado correctamente.

Recuerda que puedes apoyarte de los archivos .md que se encuentran en el repositorio para guiarte en la estructura del proyecto y en la implementación de las funcionalidades, pero quiero que me muestres como crear el script bash y el paquete en Go paso por paso para poder implementarlo y aprender como funciona cada parte.

Anteriormente me indicaste esto en tu terminal, pero no pude seguir asi que te lo vuelvo a mostrar:

❯ Necesito que me muestres como crearlo paso por paso, para poder implementarlo y aprender como funciona  

● Perfecto, te lo explico paso a paso para que lo implementes tú mismo.

  ---
  Visión General

  El flujo que vamos a construir es:

  main.go
    └── kernel.Load()          ← nuevo paquete Go
          └── ejecuta bash     ← nuevo script .sh
                ├── make        (compila el .ko si no existe)
                └── insmod      (carga el módulo)
                      └── crea /proc/meminfo_... y /proc/continfo_...
                                ↑ el daemon ya puede leerlos

  ---
  Paso 1 — Crear el script bash

  Crea la carpeta y el archivo:

  mkdir -p scripts
  touch scripts/load_kernel_module.sh
  chmod +x scripts/load_kernel_module.sh

  El script tiene 5 responsabilidades. Escríbelo así:

  #!/usr/bin/env bash
  # Uso: ./scripts/load_kernel_module.sh [CONTAINER_ID]
  # Requiere: ejecutar como root (sudo) para insmod

  set -euo pipefail

  MODULE_NAME="pr2_so1_201905884"
  CONTAINER_ID="${1:-}"   # $1 si se pasó, vacío si no

  # ── Calcular rutas ─────────────────────────────────────────
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
  KERNEL_DIR="${PROJECT_ROOT}/Kernel"
  KO_FILE="${KERNEL_DIR}/${MODULE_NAME}.ko"

  PROC_MEM="/proc/meminfo_${MODULE_NAME}"
  PROC_CONT="/proc/continfo_${MODULE_NAME}"

  # ── 1. ¿Ya está cargado? ───────────────────────────────────
  if lsmod | grep -q "^${MODULE_NAME}[[:space:]]"; then
      echo "[kernel-loader] módulo ya cargado"
      exit 0
  fi

  # ── 2. Compilar si el .ko no existe ───────────────────────
  if [ ! -f "${KO_FILE}" ]; then
      echo "[kernel-loader] compilando módulo..."
      make -C "${KERNEL_DIR}"
  fi

  # ── 3. Cargar con insmod ───────────────────────────────────
  if [ -n "${CONTAINER_ID}" ]; then
      insmod "${KO_FILE}" container_id="${CONTAINER_ID}"
  else
      insmod "${KO_FILE}"
  fi

  # ── 4. Verificar /proc ────────────────────────────────────
  [ -r "${PROC_MEM}"  ] || { echo "ERROR: ${PROC_MEM} no existe"  >&2; exit 1; }
  [ -r "${PROC_CONT}" ] || { echo "ERROR: ${PROC_CONT} no existe" >&2; exit 1; }

  echo "[kernel-loader] módulo cargado OK"

  ¿Por qué set -euo pipefail? — El script aborta en cualquier error (-e), falla si usas variables no definidas (-u), y propaga errores en pipes (-pipefail).

  ¿Por qué BASH_SOURCE[0]? — Obtiene la ruta del propio script, sin importar desde dónde lo ejecutes.

  ---
  Paso 2 — Crear el paquete kernel en Go

  Crea la carpeta:

  mkdir -p Daemon/internal/kernel
  touch Daemon/internal/kernel/loader.go

  Escribe el archivo:

  package kernel

  import (
      "fmt"
      "log"
      "os"
      "os/exec"
      "strings"
  )

  // LoadOpts contiene los parámetros para cargar el módulo.
  type LoadOpts struct {
      ScriptPath  string // ruta al script .sh
      ContainerID string // opcional: ID del contenedor Docker
  }

  // Load ejecuta el script y retorna error si falla.
  func Load(opts LoadOpts) error {
      // Verificar que el script existe antes de ejecutar
      if _, err := os.Stat(opts.ScriptPath); err != nil {
          return fmt.Errorf("kernel: script no encontrado %q: %w", opts.ScriptPath, err)
      }

      // Construir los argumentos: bash <script> [container_id]
      args := []string{opts.ScriptPath}
      if opts.ContainerID != "" {
          args = append(args, opts.ContainerID)
      }

      // Ejecutar el script capturando stdout + stderr combinados
      cmd := exec.Command("/bin/bash", args...)
      out, err := cmd.CombinedOutput()

      // Loguear cada línea de salida del script
      for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
          if line != "" {
              log.Printf("%s", line)
          }
      }

      if err != nil {
          return fmt.Errorf("kernel: script falló: %w", err)
      }
      return nil
  }

  Conceptos clave:
  - exec.Command("/bin/bash", args...) — ejecuta el script con bash explícito
  - CombinedOutput() — captura stdout y stderr en un solo slice de bytes
  - fmt.Errorf(..., %w, err) — wrapping de errores, permite usar errors.Is/As después

  ---
  Paso 3 — Actualizar main.go

  Necesitas hacer 3 cambios en el main.go existente:

  A) Agregar los imports que faltan:

  import (
      "context"
      "flag"          // ← nuevo
      "log"
      "os"
      "os/signal"
      "syscall"
      "time"

      "daemon/internal/app"
      "daemon/internal/kernel" // ← nuevo
      "daemon/internal/sink"
      "daemon/internal/source"
  )

  B) Agregar los flags y llamar al loader al inicio de main():

  func main() {
      // ── Flags de configuración ─────────────────────────────────────
      kernelScript := flag.String(
          "kernel-script",
          "scripts/load_kernel_module.sh",   // default: relativo al CWD
          "Ruta al script que carga el módulo de kernel",
      )
      containerID := flag.String(
          "container-id",
          "",
          "ID del contenedor Docker para filtrar en el módulo",
      )
      flag.Parse()

      // ── Cargar módulo de kernel ────────────────────────────────────
      log.Println("main: cargando módulo de kernel...")
      if err := kernel.Load(kernel.LoadOpts{
          ScriptPath:  *kernelScript,
          ContainerID: *containerID,
      }); err != nil {
          log.Fatalf("main: no se pudo cargar el módulo: %v", err)
      }
      log.Println("main: módulo de kernel listo")

      // ... resto del main sin cambios ...

  ---
  Estructura final resultante

  201905884_LAB_SO1_1S2026_PT2/
  ├── scripts/
  │   └── load_kernel_module.sh          ← NUEVO
  ├── Kernel/
  │   ├── pr2_so1_201905884.c
  │   └── Makefile
  └── Daemon/
      ├── cmd/daemon/main.go             ← MODIFICADO
      └── internal/
          ├── kernel/
          │   └── loader.go              ← NUEVO
          ├── app/service.go
          └── ...

  ---
  Paso 4 — Compilar y probar

  # Desde la raíz del proyecto
  cd Daemon
  go build -o ../daemon_pr2 ./cmd/daemon/

  # Ejecutar con el módulo sin container_id
  cd ..
  sudo ./daemon_pr2 --kernel-script=scripts/load_kernel_module.sh

  # O con container_id
  sudo ./daemon_pr2 \
    --kernel-script=scripts/load_kernel_module.sh \
    --container-id=abc123def456

  ---
  Resumen del flujo

  ┌────────────────────────┬────────────────────────────────────────────────────────────────┐
  │          Paso          │                            Qué pasa                            │
  ├────────────────────────┼────────────────────────────────────────────────────────────────┤
  │ flag.Parse()           │ Lee los args --kernel-script y --container-id                  │
  ├────────────────────────┼────────────────────────────────────────────────────────────────┤
  │ kernel.Load()          │ Go llama al script con exec.Command                            │
  ├────────────────────────┼────────────────────────────────────────────────────────────────┤
  │ Script: lsmod          │ Verifica si ya está cargado → si sí, exit 0                    │
  ├────────────────────────┼────────────────────────────────────────────────────────────────┤
  │ Script: make           │ Compila el .ko solo si no existe                               │
  ├────────────────────────┼────────────────────────────────────────────────────────────────┤
  │ Script: insmod         │ Carga el módulo (con o sin container_id)                       │
  ├────────────────────────┼────────────────────────────────────────────────────────────────┤
  │ Script: verifica /proc │ Confirma que las entradas existen                              │
  ├────────────────────────┼────────────────────────────────────────────────────────────────┤
  │ svc.Run()              │ El daemon ya puede leer /proc/meminfo_... y /proc/continfo_... │
  └────────────────────────┴────────────────────────────────────────────────────────────────┘

  ¿Por cuál paso quieres empezar?

✻ Baked for 35s

  3 tasks (0 done, 1 in progress, 2 open)
  ◼ Crear script de carga del módulo de kernel
  ◻ Crear paquete kernel/loader.go en el daemon
  ◻ Actualizar main.go para invocar el kernel loader
