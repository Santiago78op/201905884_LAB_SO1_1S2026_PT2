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

❯ Ayudame a realizar esto Inicio del servicio                                                                                                                                                 
● Crear un contenedor de Grafana al inicializar el código.                                                                                                                                    
● Grafana será el encargado de leer los logs generados por el servicio de Go después                                                                                                          
del análisis de los datos.                                                                                                                                                                    
● Se recomienda utilizar un Docker Compose para que el contenedor se pueda                                                                                                                    
comunicar con la base de datos Valkey                                                                                                                                                         

● Primero leo la documentación del proyecto para entender el contexto completo.

● Read 2 files (ctrl+o to expand)                                                                                                                                                             
  ⎿  Interrupted · What should Claude do instead?
                                                                                                                                                                                              
❯ Inicio del servicio                                                                                                                                                                         
● Crear un contenedor de Grafana al inicializar el código.                                                                                                                                    
● Grafana será el encargado de leer los logs generados por el servicio de Go después                                                                                                          
del análisis de los datos.                                                                                                                                                                    
● Se recomienda utilizar un Docker Compose para que el contenedor se pueda                                                                                                                    
comunicar con la base de datos Valkey, siempre indicame como hacerlo paso a paso                                                                                                              

● Entendido. Aquí tienes el plan completo paso a paso para que lo implementes tú.

  ---                                                                                                                                                                                         
  Visión general de lo que vamos a construir
                                                                                                                                                                                              
  Host (con sudo):                                          
    Go daemon → pushToValkey() → localhost:6379
                                        │
                      ┌─────────────────▼──────────────────┐
                      │         Docker Compose              │
                      │                                     │
                      │  ┌─────────────┐  ┌─────────────┐  │
                      │  │   valkey    │  │   grafana   │  │
                      │  │  :6379      │◄─┤  :3000      │  │
                      │  └─────────────┘  └─────────────┘  │
                      └─────────────────────────────────────┘

  El daemon arranca el compose al iniciar. Grafana lee de Valkey usando el plugin de Redis (Valkey es 100% compatible con Redis).

  ---
  Paso 1 — Crear la estructura de carpetas

  En la raíz del proyecto ejecuta:

  mkdir -p grafana/provisioning/datasources
  mkdir -p Daemon/internal/compose
  touch docker-compose.yml
  touch grafana/provisioning/datasources/valkey.yml
  touch Daemon/internal/compose/manager.go

  Tu árbol quedará así:

  201905884_LAB_SO1_1S2026_PT2/
  ├── docker-compose.yml                          ← NUEVO
  ├── grafana/
  │   └── provisioning/
  │       └── datasources/
  │           └── valkey.yml                      ← NUEVO
  ├── scripts/
  │   └── load_kernel_module.sh
  ├── Kernel/
  └── Daemon/
      └── internal/
          └── compose/
              └── manager.go                      ← NUEVO

  Avísame cuando lo tengas para el Paso 2.

  ---
  Paso 2 — Escribir el docker-compose.yml

  Abre docker-compose.yml y escribe esto:

  services:

    valkey:
      image: valkey/valkey:8-alpine
      container_name: valkey_pr2
      ports:
        - "6379:6379"
      restart: unless-stopped

    grafana:
      image: grafana/grafana:latest
      container_name: grafana_pr2
      ports:
        - "3000:3000"
      environment:
        - GF_SECURITY_ADMIN_PASSWORD=admin
        - GF_INSTALL_PLUGINS=redis-datasource
      volumes:
        - ./grafana/provisioning:/etc/grafana/provisioning
      depends_on:
        - valkey
      restart: unless-stopped

  ¿Qué hace cada parte?

  ┌─────────────────────────────────────┬──────────────────────────────────────────────────────────────────────────────────────────┐
  │               Bloque                │                                       Explicación                                        │
  ├─────────────────────────────────────┼──────────────────────────────────────────────────────────────────────────────────────────┤
  │ valkey/valkey:8-alpine              │ Imagen oficial de Valkey (Alpine = imagen mínima)                                        │
  ├─────────────────────────────────────┼──────────────────────────────────────────────────────────────────────────────────────────┤
  │ ports: "6379:6379"                  │ Expone Valkey al host → el daemon Go puede conectarse en localhost:6379                  │
  ├─────────────────────────────────────┼──────────────────────────────────────────────────────────────────────────────────────────┤
  │ GF_INSTALL_PLUGINS=redis-datasource │ Instala el plugin de Redis en Grafana al arrancar (Valkey es compatible con este plugin) │
  ├─────────────────────────────────────┼──────────────────────────────────────────────────────────────────────────────────────────┤
  │ volumes: ./grafana/provisioning     │ Monta la carpeta local en Grafana para configuración automática                          │
  ├─────────────────────────────────────┼──────────────────────────────────────────────────────────────────────────────────────────┤
  │ depends_on: valkey                  │ Grafana espera a que Valkey esté listo antes de arrancar                                 │
  └─────────────────────────────────────┴──────────────────────────────────────────────────────────────────────────────────────────┘

  Avísame cuando lo tengas para el Paso 3.

  ---
  Paso 3 — Configurar el datasource de Valkey en Grafana

  Abre grafana/provisioning/datasources/valkey.yml y escribe:

  apiVersion: 1

  datasources:
    - name: Valkey
      type: redis-datasource
      access: proxy
      url: redis://valkey:6379
      isDefault: true
      editable: true

  ¿Qué hace cada campo?

  ┌──────────────────────────┬────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
  │          Campo           │                                                  Explicación                                                   │
  ├──────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ type: redis-datasource   │ Usa el plugin que instalamos con GF_INSTALL_PLUGINS                                                            │
  ├──────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ url: redis://valkey:6379 │ valkey es el nombre del servicio en Docker Compose — los contenedores se resuelven por nombre dentro de la red │
  ├──────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ access: proxy            │ Grafana hace las consultas desde el servidor (no desde el browser)                                             │
  ├──────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ isDefault: true          │ Este datasource se selecciona por defecto al crear panels                                                      │
  └──────────────────────────┴────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘

  ¿Por qué valkey:6379 y no localhost:6379?
  Dentro de Docker Compose, cada servicio tiene su propio hostname igual al nombre del servicio. localhost dentro del contenedor de Grafana apuntaría a sí mismo, no a Valkey.

  Avísame cuando lo tengas para el Paso 4.

  ---
  Paso 4 — Crear el paquete Go compose/manager.go

  Abre Daemon/internal/compose/manager.go y escribe esto:

  package compose

  import (
      "fmt"
      "log"
      "os"
      "os/exec"
      "strings"
  )

  type Opts struct {
      // ComposeFile es la ruta al docker-compose.yml
      ComposeFile string
  }

  // Up ejecuta "docker compose up -d" con el archivo indicado.
  // Retorna nil si los contenedores arrancaron correctamente.
  func Up(opts Opts) error {
      if _, err := os.Stat(opts.ComposeFile); err != nil {
          return fmt.Errorf("compose: archivo no encontrado %q: %w", opts.ComposeFile, err)
      }

      cmd := exec.Command("docker", "compose", "-f", opts.ComposeFile, "up", "-d")
      out, err := cmd.CombinedOutput()

      for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
          if line != "" {
              log.Printf("[compose] %s", line)
          }
      }

      if err != nil {
          return fmt.Errorf("compose: docker compose up falló: %w", err)
      }
      return nil
  }

  Conceptos clave:

  ┌────────────────────────────────────┬────────────────────────────────────────────────────────────────────┐
  │              Elemento              │                            Explicación                             │
  ├────────────────────────────────────┼────────────────────────────────────────────────────────────────────┤
  │ docker compose -f archivo up -d    │ -f especifica el archivo; -d arranca en modo detached (background) │
  ├────────────────────────────────────┼────────────────────────────────────────────────────────────────────┤
  │ os.Stat() antes de ejecutar        │ Verifica que el archivo existe y da error claro si no              │
  ├────────────────────────────────────┼────────────────────────────────────────────────────────────────────┤
  │ CombinedOutput()                   │ Captura stdout + stderr del proceso docker en un solo buffer       │
  ├────────────────────────────────────┼────────────────────────────────────────────────────────────────────┤
  │ Misma estructura que kernel.Load() │ Patrón consistente en todo el proyecto                             │
  └────────────────────────────────────┴────────────────────────────────────────────────────────────────────┘

  Avísame cuando lo tengas para el Paso 5.

  ---
  Paso 5 — Actualizar main.go

  Abre Daemon/cmd/daemon/main.go. Necesitas dos cambios:

  Cambio A — Agregar el import:

  import (
      // ... imports existentes ...
      "daemon/internal/compose"  // ← agregar
      "daemon/internal/kernel"
      // ...
  )

  Cambio B — Llamar a compose.Up() justo después de kernel.Load():

  // Después de kernel.Load() exitoso:
  log.Println("main: iniciando contenedores (Grafana + Valkey)...")
  if err := compose.Up(compose.Opts{
      ComposeFile: *composeFile,
  }); err != nil {
      log.Fatalf("main: no se pudo iniciar los contenedores: %v", err)
  }
  log.Println("main: contenedores listos — Grafana en http://localhost:3000")

  Y agrega el flag en la sección de flags:

  composeFile := flag.String(
      "compose-file",
      "docker-compose.yml",
      "Ruta al docker-compose.yml con Grafana y Valkey",
  )

  ¿Por qué compose.Up() va después de kernel.Load()?
  El orden importa: primero el módulo (fuente de datos), luego la infraestructura de visualización. Si el módulo falla, no tiene sentido levantar Grafana.

  Cuando termines, compila para verificar:

  cd Daemon
  go build ./...

  Avísame cuando lo tengas para el Paso 6.

  ---
  Paso 6 — Probar el flujo completo

  Desde la raíz del proyecto:

  # 1. Compilar el daemon
  cd Daemon && go build -o ../daemon_pr2 ./cmd/daemon/ && cd ..

  # 2. Ejecutar (sudo necesario para insmod)
  sudo ./daemon_pr2 \
    --kernel-script=scripts/load_kernel_module.sh \
    --compose-file=docker-compose.yml

  Salida esperada:

  main: cargando módulo de kernel...
  [kernel-loader] módulo cargado OK
  main: módulo de kernel listo
  main: iniciando contenedores (Grafana + Valkey)...
  [compose] Container valkey_pr2  Started
  [compose] Container grafana_pr2 Started
  main: contenedores listos — Grafana en http://localhost:3000
  main: daemon iniciado

  Luego abre el browser en http://localhost:3000 (usuario: admin, contraseña: admin).

  ---
  Resumen del flujo completo

  sudo ./daemon_pr2
      │
      ├── kernel.Load()     → insmod pr2_so1_201905884.ko
      │                       crea /proc/meminfo_... y /proc/continfo_...
      │
      ├── compose.Up()      → docker compose up -d
      │                       arranca valkey:6379 y grafana:3000
      │
      └── svc.Run()         → cada 5s:
                                lee /proc → parsea → escribe JSONL
                                pushToValkey() → Valkey
                                                    ↑
                                              Grafana lo lee
                                              y lo visualiza

  ¿En qué paso estás? Avísame cuando quieras continuar o si tienes alguna duda.
