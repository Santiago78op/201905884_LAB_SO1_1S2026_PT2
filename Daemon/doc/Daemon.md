# Documentacion Tecnica — PR2 SO1 | 201905884

Documentacion tecnica completa del modulo de kernel en C y del daemon en Go que conforman el sistema de monitoreo de contenedores Docker.

---

## Indice

1. [Vision general del sistema](#1-vision-general-del-sistema)
2. [Modulo de kernel](#2-modulo-de-kernel)
3. [Scripts bash](#3-scripts-bash)
4. [Daemon Go — arquitectura](#4-daemon-go--arquitectura)
5. [Paquete kernel — carga del modulo](#5-paquete-kernel--carga-del-modulo)
6. [Paquete source — lectura de /proc](#6-paquete-source--lectura-de-proc)
7. [Paquete model — structs de dominio](#7-paquete-model--structs-de-dominio)
8. [Paquete parser — deserializacion](#8-paquete-parser--deserializacion)
9. [Paquete docker — gestion de contenedores](#9-paquete-docker--gestion-de-contenedores)
10. [Paquete sink — persistencia en Valkey](#10-paquete-sink--persistencia-en-valkey)
11. [Paquete app — orquestador](#11-paquete-app--orquestador)
12. [cmd/daemon/main.go — entrada](#12-cmddaemonmaingo--entrada)
13. [Infraestructura Docker](#13-infraestructura-docker)
14. [Integracion con systemd](#14-integracion-con-systemd)
15. [Flujo de datos completo](#15-flujo-de-datos-completo)
16. [Estructuras de datos en Valkey](#16-estructuras-de-datos-en-valkey)
17. [Configuracion (.env)](#17-configuracion-env)
18. [Decisiones de diseno](#18-decisiones-de-diseno)
19. [Referencia de errores](#19-referencia-de-errores)

---

## 1. Vision general del sistema

```
┌─────────────────────────────────────────────────────────────────────┐
│                          KERNEL SPACE                               │
│                                                                     │
│  pr2_so1_201905884.ko                                               │
│    __init: proc_create() x2                                         │
│    meminfo_show()  -> JSON RAM (total/free/used en KB)              │
│    continfo_show() -> JSON procesos Docker (PID/VSZ/RSS/CPU/cmdline)│
│                                                                     │
│  /proc/meminfo_pr2_so1_201905884                                    │
│  /proc/continfo_pr2_so1_201905884                                   │
└───────────────┬──────────────────────────┬──────────────────────────┘
                │ insmod (al arrancar)      │ os.ReadFile() cada 20-60s
┌───────────────▼──────────────────────────▼──────────────────────────┐
│                        USER SPACE — Daemon Go                       │
│                                                                     │
│  main.go                                                            │
│    godotenv.Load()     <- carga .env                                │
│    kernel.Load()       <- insmod via script bash                    │
│    docker compose up   <- levanta Grafana + Valkey                  │
│    RegisterCronjob()   <- crea entrada en crontab del usuario       │
│    svc.Run(ctx)        <- loop principal (20-60s aleatorio)         │
│      FileReader.Read() <- []byte JSON crudo desde /proc             │
│      parser.*          <- JSON crudo -> structs tipados             │
│      docker.Enforce()  <- aplica invariantes (3 low + 2 high)       │
│      sink.*            <- persiste en Valkey (LIST/ZSET/HASH)       │
│    kernel.Unload()     <- rmmod al salir                            │
│    RemoveCronjob()     <- limpia crontab al salir                   │
└───────────────────────────────┬─────────────────────────────────────┘
                                │ protocolo Redis (go-redis v9)
┌───────────────────────────────▼─────────────────────────────────────┐
│                    DOCKER COMPOSE (red red_pr2_so1)                 │
│                                                                     │
│  valkey_so1  :6379   LIST meminfo / continfo / procinfo             │
│                      ZSET rss_rank / cpu_rank                       │
│                      HASH containers                                │
│                                                                     │
│  grafana_so1 :3000   plugin redis-datasource -> consulta Valkey     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 2. Modulo de kernel

**Archivo:** `Daemon/kernel/pr2_so1_201905884.c`

### Proposito

Exponer dos pseudo-archivos en el sistema de archivos virtual `/proc` que el daemon Go puede leer con una simple llamada a `os.ReadFile()`. Los datos se generan en cada lectura directamente desde las estructuras internas del kernel.

### Headers utilizados

| Header | Uso |
|---|---|
| `linux/proc_fs.h` | `proc_create()`, `proc_remove()` para gestionar entradas `/proc` |
| `linux/seq_file.h` | `seq_printf()`, `single_open()` para escritura secuencial |
| `linux/mmzone.h` | `si_meminfo()` para obtener estadisticas globales de RAM |
| `linux/mm.h` | `get_task_mm()`, `get_mm_rss()`, `mmput()` para memoria por proceso |
| `linux/sched/signal.h` | `for_each_process()` para iterar sobre todos los procesos |
| `linux/cgroup.h` | `cgroup_path()` para obtener la ruta del cgroup de un proceso |
| `linux/memcontrol.h` | `task_get_css()`, `memory_cgrp_id` para acceder al cgroup de memoria |
| `linux/slab.h` | `kmalloc()`, `kfree()` para buffers dinamicos en espacio kernel |
| `linux/jiffies.h` | `jiffies` para calcular uso de CPU acumulado |
| `linux/mmap_lock.h` | `mmap_read_lock()` para acceso seguro a `mm_struct` |

### Constantes

```c
#define CARNET           "201905884"
#define PROC_MEMINFO_NAME "meminfo_pr2_so1_201905884"
#define PROC_CONTINFO_NAME "continfo_pr2_so1_201905884"
#define CMDLINE          256   // longitud maxima del buffer de cmdline
#define CONTAINER_ID     128   // longitud maxima del path de cgroup
```

### Identificacion de procesos Docker

**`is_docker_infra(task)`** — retorna `true` para procesos de infraestructura Docker por nombre exacto: `dockerd`, `containerd`, `containerd-shim`, `containerd-shim-runc-v2`, `runc`.

**`is_docker_process(task)`** — combina dos criterios:
1. Es infraestructura Docker (por nombre)
2. Su cgroup path contiene la cadena `"docker"` (workloads reales: `sleep`, `sh`, binarios Go, etc.)

La deteccion por cgroup requiere `CONFIG_MEMCG` habilitado en el kernel. Si no esta habilitado, solo se detecta infraestructura por nombre.

```c
css = task_get_css(task, memory_cgrp_id);
cgroup_path(css->cgroup, path, sizeof(path));
in_docker = (strstr(path, "docker") != NULL);
```

### `meminfo_show()` — entrada /proc/meminfo

Lee el estado global de RAM con `si_meminfo()` y escribe JSON:

```json
{
  "memory_info": {
    "total_ram_kb": 16384000,
    "free_ram_kb": 8192000,
    "used_ram_kb": 8192000
  }
}
```

Calculo:
```c
total_ram = (i.totalram * i.mem_unit) / 1024;
free_ram  = (i.freeram  * i.mem_unit) / 1024;
used_ram  = (total_ram >= free_ram) ? (total_ram - free_ram) : 0;
```

`i.mem_unit` es el tamano de pagina en bytes. La division entre 1024 convierte bytes a KB.

### `continfo_show()` — entrada /proc/continfo

Itera sobre todos los procesos con `for_each_process(task)` dentro de `rcu_read_lock()` y escribe un array JSON de procesos Docker.

Para cada proceso incluido:

| Campo | Calculo |
|---|---|
| `vsz_kb` | `mm->total_vm << (PAGE_SHIFT - 10)` — paginas virtuales a KB |
| `rss_kb` | `get_mm_rss(mm) << (PAGE_SHIFT - 10)` — paginas residentes a KB |
| `mem_perc_x100` | `(rss * 10000) / mem_total_ram` — porcentaje x100 para evitar floats |
| `cpu_perc_x100` | `(task->utime + task->stime) * 10000 / jiffies` — ticks acumulados x100 |
| `cmdline` | `get_process_cmdline(task)` — lee `mm->arg_start..arg_end` via `access_process_vm()` |
| `container_id` | `get_container_id(task)` — extrae 12 chars despues de `"docker-"` en el cgroup path |

Salida JSON:
```json
{
  "system_metrics": { "total_ram_kb": 16384000, "free_ram_kb": 8000000, "used_ram_kb": 8384000 },
  "processes": [
    {
      "pid": 1234,
      "name": "dockerd",
      "cmdline": "/usr/bin/dockerd -H fd://",
      "vsz_kb": 1024000,
      "rss_kb": 51200,
      "mem_perc_x100": 312,
      "cpu_perc_x100": 5423,
      "container_id": "-"
    },
    {
      "pid": 5678,
      "name": "sh",
      "cmdline": "sh -c while true; do echo '2^20' | bc > /dev/null; sleep 2; done",
      "vsz_kb": 4096,
      "rss_kb": 2048,
      "mem_perc_x100": 12,
      "cpu_perc_x100": 98700,
      "container_id": "abc123def456"
    }
  ],
  "docker_active": 7
}
```

### `get_process_cmdline(task)`

1. `get_task_mm(task)` — obtiene la estructura de memoria del proceso
2. `mmap_read_lock(mm)` / `mmap_read_unlock(mm)` — acceso seguro a `arg_start` y `arg_end`
3. `access_process_vm(task, arg_start, buf, len, 0)` — copia los argumentos del espacio de usuario
4. Reemplaza `\0` entre argumentos por espacios para legibilidad
5. `mmput(mm)` — libera la referencia al `mm_struct`

### `get_container_id(task)`

1. `task_get_css(task, memory_cgrp_id)` — obtiene el cgroup subsystem state de memoria
2. `cgroup_path(css->cgroup, buf, size)` — obtiene la ruta completa del cgroup
   - Ejemplo: `/system.slice/docker-abc123def456...scope`
3. `strstr(path, "docker-")` — localiza el prefijo del ID
4. `strscpy(container_id, p, 13)` — copia los primeros 12 caracteres del container ID

### Inicializacion y limpieza

```c
static int __init pr2_module_init(void) {
    proc_meminfo_entry  = proc_create(PROC_MEMINFO_NAME,  0444, NULL, &meminfo_fops);
    proc_continfo_entry = proc_create(PROC_CONTINFO_NAME, 0444, NULL, &continfo_ops);
    // Si falla continfo, elimina meminfo antes de retornar -ENOMEM
}

static void __exit pr2_module_exit(void) {
    proc_remove(proc_continfo_entry);
    proc_remove(proc_meminfo_entry);
}
```

El permiso `0444` hace las entradas de solo lectura para todos los usuarios.

---

## 3. Scripts bash

### `scripts/load_kernel_module.sh`

| Paso | Accion | Por que |
|---|---|---|
| 1 | `lsmod \| grep MODULE_NAME` | Idempotente: si ya esta cargado, sale con exit 0 sin error |
| 2 | `mktemp -d` + `cp` + `make` | Compila en directorio temporal porque `make` del kernel no soporta rutas con espacios |
| 3 | `cp .ko` al directorio kernel | Guarda el binario compilado para el kernel en ejecucion |
| 4 | `sudo insmod .ko` | Carga el modulo; requiere root |
| 5 | `[ -r /proc/... ]` | Verifica que `__init` creo las entradas `/proc` correctamente |

`set -euo pipefail` garantiza que cualquier fallo aborta el script completo.

`BASH_SOURCE[0]` calcula la ruta absoluta del script independientemente del directorio desde donde se llame.

### `scripts/unload_kernel_module.sh`

1. Verifica que el modulo este cargado con `lsmod`
2. `sudo rmmod MODULE_NAME` — descarga el modulo
3. Advierte si las entradas `/proc` siguen presentes tras el `rmmod`

### `scripts/create_containers.sh`

Ejecutado por el cronjob cada 2 minutos. Crea 5 contenedores aleatorios:

| Caso `RANDOM % 3` | Contenedor | Categoria |
|---|---|---|
| 0 | `roldyoran/go-client` | Alto consumo RAM |
| 1 | `alpine sh -c "while true; do echo '2^20' \| bc > /dev/null; sleep 2; done"` | Alto consumo CPU |
| 2 | `alpine sleep 240` | Bajo consumo |

---

## 4. Daemon Go — arquitectura

```
cmd/daemon/main.go          <- punto de entrada, conecta todas las piezas
    |
    +-- internal/kernel/    <- carga/descarga modulo via script bash
    +-- internal/source/    <- lee /proc (interfaz Reader)
    +-- internal/parser/    <- JSON crudo -> structs Go
    +-- internal/model/     <- definicion de structs de dominio
    +-- internal/docker/    <- gestiona contenedores con docker ps/stop/rm/run
    +-- internal/sink/      <- escribe en Valkey (LIST/ZSET/HASH)
    +-- internal/app/       <- orquesta el loop principal y el cronjob
```

**Regla de capas:** cada paquete solo conoce a los paquetes que importa explicitamente. `app/service.go` no sabe como se lee ni como se escribe; solo orquesta quien llama a quien.

---

## 5. Paquete kernel — carga del modulo

**Archivo:** `internal/kernel/loader.go`

```go
type LoadOpts struct {
    ScriptPath  string   // ruta al script bash de carga
    ContainerID string   // ID opcional de contenedor para filtrar
}

func Load(opts LoadOpts) error
func Unload(scriptPath string) error
```

**Flujo de `Load()`:**
1. `os.Stat(ScriptPath)` — verifica existencia del script antes de ejecutar
2. Construye `args = [ScriptPath, ContainerID?]`
3. `exec.Command("/bin/bash", args...)` — invoca bash explicitamente, no depende del shebang
4. `cmd.CombinedOutput()` — ejecuta y captura stdout+stderr; bloquea hasta terminar
5. Loguea cada linea del script en el log del daemon
6. Si exit code != 0, retorna `fmt.Errorf("kernel: error ...: %w", err)`

**Por que `CombinedOutput()` y no `Output()`:** el script puede escribir errores en stderr (mensajes de `insmod`). `CombinedOutput()` los mezcla en orden cronologico en el log del daemon.

**Por que `%w` en `fmt.Errorf`:** envuelve el `*exec.ExitError` original. El caller puede inspeccionarlo con `errors.Is()` o `errors.As()` sin perder informacion.

---

## 6. Paquete source — lectura de /proc

**Archivo:** `internal/source/file_reader.go`

```go
type Reader interface {
    Read(ctx context.Context) ([]byte, error)
}

type FileReader struct {
    Path string
}
```

**Flujo de `Read()`:**
1. Verifica `ctx.Done()` — si el daemon esta apagandose, no inicia la lectura
2. `os.ReadFile(r.Path)` — lee el pseudo-archivo completo en un buffer
3. Envuelve el error con la ruta para diagnostico

**Por que polling y no inotify:** los archivos en `/proc` son virtuales, se generan en cada llamada a `read()`. Los watchers de inotify no detectan cambios en pseudo-archivos del kernel. El polling periodico es el metodo correcto.

**Por que `os.ReadFile()` y no `bufio.Scanner()`:** `/proc` garantiza atomicidad en la lectura completa. Leer linea a linea podria ver un snapshot inconsistente si el kernel actualiza los datos entre lecturas parciales.

---

## 7. Paquete model — structs de dominio

**Archivo:** `internal/model/metrics.go`

```go
// Datos de memoria del sistema (de /proc/meminfo_*)
type MemStats struct {
    MemTotal  uint64    `json:"total_ram_kb"`
    MemFree   uint64    `json:"free_ram_kb"`
    MemUsed   uint64    `json:"used_ram_kb"`
    Timestamp time.Time `json:"timestamp"`
}

// Un proceso Docker con sus metricas de recursos
type ProcessInfo struct {
    Pid         int    `json:"pid"`
    Name        string `json:"name"`
    Cmdline     string `json:"cmdline"`
    VSZkb       uint64 `json:"vsz_kb"`
    RSSkb       uint64 `json:"rss_kb"`
    MemPct      uint64 `json:"mem_perc_x100"`   // porcentaje x100
    CPURaw      uint64 `json:"cpu_perc_x100"`   // porcentaje x100
    ContainerID string `json:"container_id"`
}

// Resumen del estado de contenedores en un tick
type ContainerReport struct {
    Processes          []ProcessInfo `json:"-"`           // se guarda por separado en procinfo
    ContainersActive   int           `json:"containers_active"`
    ContainersExited   int           `json:"containers_exited"`
    ContainersRemoved  int           `json:"containers_removed"`
    ContainersInactive int           `json:"containers_inactive"`
    Timestamp          time.Time     `json:"timestamp"`
}

// Wrapper del JSON que emite el modulo de kernel para continfo
type JsonContInfo struct {
    Processes    []ProcessInfo `json:"processes"`
    DockerActive int           `json:"docker_active"`
}
```

**Por que `uint64`:** el kernel emite estos valores con `%llu` (unsigned long long). Nunca son negativos.

**Por que `x100` en `MemPct` y `CPURaw`:** el kernel evita floats multiplicando x100. `735` representa `7.35%`. El daemon convierte con `toPerc()` al escribir en los ZSET.

**Por que `json:"-"` en `Processes`:** los procesos se guardan individualmente en la lista `procinfo` de Valkey, no dentro del `ContainerReport`. Si se incluyeran, `continfo` tendria un JSON anidado enorme en cada entrada.

---

## 8. Paquete parser — deserializacion

### `parser/meminfo.go`

```go
type kernelMemInfoWrapper struct {
    MemoryInfo model.JsonMemInfo `json:"memory_info"`
}

func ParseMemInfo(raw string) (model.MemStats, error)
```

1. `json.Unmarshal([]byte(raw), &wrapper)` — deserializa el JSON anidado del kernel
2. Mapea `wrapper.MemoryInfo` a `model.MemStats`
3. Asigna `Timestamp = time.Now()`

### `parser/continfo.go`

```go
func ParserContInfo(raw string) (model.ContainerReport, error)
```

1. `json.Unmarshal([]byte(raw), &parsed)` — deserializa el JSON del kernel a `JsonContInfo`
2. Mapea cada `ProcessInfo` del JSON al slice `report.Processes`
3. Asigna `ContainersActive = parsed.DockerActive`
4. Asigna `Timestamp = time.Now()`

---

## 9. Paquete docker — gestion de contenedores

**Archivo:** `internal/docker/manager.go`

### Categorias de contenedores

```go
const (
    CategorySystem         // grafana, valkey — nunca eliminar
    CategoryLowConsumption // alpine sleep 240
    CategoryHighConsumption // go-client o alpine stress
    CategoryUnknown
)
```

### Invariantes

```go
const (
    TargetLow  = 3   // siempre 3 contenedores de bajo consumo
    TargetHigh = 2   // siempre 2 contenedores de alto consumo
)
```

### `Enforce(processes []model.ProcessInfo) (EnforceResult, error)`

Secuencia en cada tick:

1. `list()` — `docker ps --no-trunc --format "{{.ID}}\t{{.Image}}\t{{.Command}}\t{{.Names}}"`
2. `classify(c)` — determina categoria por imagen y comando
3. `enrichWithMetrics(containers, processes)` — vincula metricas de `/proc/continfo` a cada contenedor
   - El kernel usa los primeros 12 chars del container ID en `container_id`
   - Los valores se **suman** si el contenedor tiene multiples procesos
4. Aplica invariantes:
   - **Bajo consumo en exceso:** `sortDesc()` y elimina los de **mayor consumo** (anomalos)
   - **Alto consumo en exceso:** `sortDesc()` y elimina los de **menor consumo** (mantiene los mas activos)
5. Crea contenedores si hay deficit (`createLow()` / `createHigh()`)
6. `countExited()` — cuenta contenedores gestionados en estado `Exited`

### `enrichWithMetrics()`

```go
// Agrega metricas de /proc al struct Container
// El kernel retorna los primeros 12 chars del ID en container_id
for _, p := range processes {
    cid := p.ContainerID            // "abc123def456" (12 chars)
    a.rss    += p.RSSkb             // suma todos los procesos del contenedor
    a.vsz    += p.VSZkb
    a.memPct += p.MemPct            // suma de valores x100
    a.cpu    += p.CPURaw            // suma de valores x100
}
for i, c := range containers {
    prefix := c.ID[:12]             // primeros 12 chars del ID completo
    if a, ok := metrics[prefix]; ok {
        containers[i].RSSkb  = a.rss
        containers[i].MemPct = a.memPct
        containers[i].CPURaw = a.cpu
    }
}
```

### `resourceScore(c Container) uint64`

```go
return c.RSSkb + c.CPURaw
```

Metrica combinada para ordenar contenedores. Al sumar RAM y CPU raw, prioriza los que consumen mas en ambas dimensiones.

---

## 10. Paquete sink — persistencia en Valkey

**Archivo:** `internal/sink/valkey.go`

El paquete define tres writers especializados, cada uno para un tipo de dato diferente en Valkey.

### `ValkeyWriter` — listas (LIST)

```go
type ValkeyWriter struct {
    Client *redis.Client
    Key    string
}

func (v *ValkeyWriter) Write(data any) error {
    b, _ := json.Marshal(data)
    v.Client.RPush(ctx, v.Key, b)   // RPUSH key <json>
}
```

Comando Valkey: `RPUSH key value` — agrega al final de la lista en orden cronologico.

Usado para: `meminfo`, `continfo`, `procinfo`.

### `ValkeyRankWriter` — sorted sets (ZSET)

```go
type ValkeyRankWriter struct {
    Client *redis.Client
    Key    string
}

func (v *ValkeyRankWriter) Upsert(score float64, member string) error {
    v.Client.ZAdd(ctx, v.Key, redis.Z{Score: score, Member: member})
}

func (v *ValkeyRankWriter) Remove(member string) error {
    v.Client.ZRem(ctx, v.Key, member)
}
```

Comandos Valkey: `ZADD key score member` / `ZREM key member`.

- `member` = nombre del contenedor (garantiza unicidad — no hay duplicados por contenedor)
- `score` = porcentaje real convertido por `toPerc()` (ej: `7.35`)

Usado para: `rss_rank`, `cpu_rank`.

### `ValkeyHashWriter` — hashes (HASH)

```go
type ValkeyHashWriter struct {
    Client *redis.Client
    Key    string
}

func (v *ValkeyHashWriter) HSet(field string, value any) error {
    b, _ := json.Marshal(value)
    v.Client.HSet(ctx, v.Key, field, b)   // HSET key field <json>
}

func (v *ValkeyHashWriter) HDel(field string) error {
    v.Client.HDel(ctx, v.Key, field)      // HDEL key field
}
```

- `field` = docker ID completo (64 chars)
- `value` = JSON completo del contenedor (`containerRankEntry`)

Siempre refleja el estado **actual** de los contenedores activos. Cuando un contenedor se elimina, `HDel` lo quita del hash. Grafana hace `HGETALL containers` y obtiene solo los vivos.

Usado para: `containers`.

---

## 11. Paquete app — orquestador

**Archivos:** `internal/app/service.go`, `internal/app/cronjob.go`

### `Service`

```go
type Service struct {
    MemReader           source.Reader
    ContReader          source.Reader
    MemWriter           sink.Writer
    ContWriter          sink.Writer
    ProcWriter          sink.Writer
    RssRankWriter       *sink.ValkeyRankWriter
    CpuRankWriter       *sink.ValkeyRankWriter
    ContainerHashWriter *sink.ValkeyHashWriter
    Docker              *docker.Manager
    totalContainersRemoved int
}
```

### `Run(ctx context.Context) error`

Loop principal con intervalo aleatorio entre 20 y 60 segundos:

```go
for {
    wait := time.Duration(20+rand.Intn(41)) * time.Second
    select {
    case <-ctx.Done():        // SIGTERM/SIGINT -> salir limpiamente
        return nil
    case <-time.After(wait):  // intervalo cumplido -> ejecutar tick
        s.tick(ctx)
    }
}
```

El `select` escucha dos canales simultaneamente. El intervalo aleatorio evita patrones predecibles de carga.

### `tick(ctx context.Context)`

Secuencia de operaciones en cada ciclo:

```
1. MemReader.Read()       -> JSON crudo de /proc/meminfo_*
2. parser.ParseMemInfo()  -> model.MemStats
3. MemWriter.Write()      -> RPUSH meminfo <JSON>

4. ContReader.Read()      -> JSON crudo de /proc/continfo_*
5. parser.ParserContInfo() -> model.ContainerReport
6. docker.Enforce()       -> aplica invariantes, clasifica contenedores

7. ContWriter.Write()     -> RPUSH continfo <JSON del ContainerReport>

8. Para cada contenedor activo:
   ProcWriter.Write()        -> RPUSH procinfo <JSON del containerRankEntry>
   RssRankWriter.Upsert()    -> ZADD rss_rank toPerc(MemPct) nombre
   CpuRankWriter.Upsert()    -> ZADD cpu_rank toPerc(CPURaw) nombre
   ContainerHashWriter.HSet() -> HSET containers dockerID <JSON completo>

9. Para cada contenedor eliminado:
   ProcWriter.Write()        -> RPUSH procinfo <JSON con status="removed">
   RssRankWriter.Remove()    -> ZREM rss_rank nombre
   CpuRankWriter.Remove()    -> ZREM cpu_rank nombre
   ContainerHashWriter.HDel() -> HDEL containers dockerID
```

**Patron de error en cascada:** si un paso falla, se loguea y se salta al siguiente. El daemon nunca se detiene por un error en un tick individual.

### `containerRankEntry`

Struct serializado a JSON para `procinfo` y `containers`:

```go
type containerRankEntry struct {
    DockerID    string    `json:"docker_id"`
    Pid         int       `json:"pid"`
    Name        string    `json:"container_name"`
    Image       string    `json:"image"`
    Status      string    `json:"status"`      // "active" | "removed"
    RSSkb       uint64    `json:"rss_kb"`
    VSZkb       uint64    `json:"vsz_kb"`
    MemPctX100  uint64    `json:"mem_perc_x100"` // valor x100 del kernel (sin convertir)
    CPURawX100  uint64    `json:"cpu_perc_x100"` // valor x100 del kernel (sin convertir)
    Timestamp   time.Time `json:"timestamp"`
}
```

**Diferencia entre hash y ZSET:** el hash guarda el valor `x100` crudo (para inspeccion detallada), los ZSET guardan el score convertido a porcentaje real con `toPerc()` (para queries de rango).

### `toPerc(x100 uint64) float64`

```go
v := math.Round(float64(x100)/100.0*100) / 100
return math.Min(v, 100.00)
```

- Divide entre 100 para obtener el porcentaje real
- Redondea a 2 decimales
- Tapa en 100.00 (suma de procesos puede superar 100%)

### Cronjob (`cronjob.go`)

**`RegisterCronjob(scriptPath string)`:**
1. `crontab -l` — lee el crontab actual del usuario
2. Agrega la entrada `*/2 * * * * <ruta_absoluta>/create_containers.sh`
3. `crontab -` — escribe el nuevo crontab

**`RemoveCronjob(scriptPath string)`:**
1. `crontab -l` — lee el crontab actual
2. Filtra las lineas que contengan la ruta del script
3. `crontab -` — escribe el crontab sin la entrada del daemon

---

## 12. cmd/daemon/main.go — entrada

### Secuencia de arranque

```
1. godotenv.Load()              <- carga .env desde el working directory
2. flag.Parse()                 <- procesa --kernel-script y --container-id
3. kernel.Load()                <- insmod via script bash (fatal si falla)
4. defer kernel.Unload()        <- rmmod al salir (garantizado por defer)
5. docker compose up -d         <- levanta Grafana + Valkey
6. app.RegisterCronjob()        <- registra crontab
7. signal.Notify(SIGINT,SIGTERM) <- captura señales de apagado
8. go func() { <-sigChan; cancel() } <- goroutine que espera señales
9. svc.Run(ctx)                 <- loop principal (bloquea hasta cancel)
10. app.RemoveCronjob()         <- elimina entrada del crontab
```

### Por que `kernel.Load()` antes del compose

Si el modulo no carga, las entradas `/proc` no existen. Fallar rapido con `log.Fatalf` evita arrancar Grafana y Valkey para luego no tener datos que mostrar.

### Por que goroutine para señales

```go
go func() {
    sig := <-sigChan     // bloquea hasta recibir señal
    cancel()             // cancela el contexto -> svc.Run() retorna
}()
svc.Run(ctx)             // bloquea en el loop principal
```

`<-sigChan` es bloqueante. Si estuviera en el hilo principal, el daemon nunca llegaria a `svc.Run()`. La goroutine espera la señal en paralelo.

### Señales capturadas

| Señal | Origen tipico |
|---|---|
| `SIGTERM` | `systemctl stop`, `kill <pid>`, Docker al detener el contenedor |
| `SIGINT` | `Ctrl+C` en terminal |

---

## 13. Infraestructura Docker

### `docker/docker-compose.yml`

```yaml
networks:
  red_pr2_so1:
    driver: bridge    # red privada entre servicios

services:
  valkey:
    image: valkey/valkey:latest
    container_name: valkey_so1
    ports:
      - "${VALKEY_PORT:-6379}:6379"
    networks: [red_pr2_so1]
    volumes:
      - valkey_data:/data   # persistencia entre reinicios

  grafana:
    image: grafana/grafana:latest
    container_name: grafana_so1
    ports:
      - "${GRAFANA_PORT:-3000}:3000"
    environment:
      - GF_INSTALL_PLUGINS=redis-datasource   # instala plugin al arrancar
    networks: [red_pr2_so1]
    volumes:
      - ./grafana/provisioning:/etc/grafana/provisioning
    depends_on: [valkey]
```

**Por que una red bridge propia:** los contenedores se resuelven por nombre de servicio (`valkey`, `grafana`) dentro de la red. Sin red compartida, Grafana no puede conectarse a `redis://valkey:6379`.

**Por que `GF_INSTALL_PLUGINS=redis-datasource`:** Valkey implementa el protocolo Redis. El plugin `redis-datasource` de Grafana es compatible y se instala automaticamente en cada arranque del contenedor.

### `grafana/provisioning/datasources/valkey.yml`

```yaml
apiVersion: 1
datasources:
  - name: Valkey
    type: redis-datasource
    access: proxy
    url: redis://valkey:6379    # 'valkey' = nombre del servicio en la red docker
    isDefault: true
    editable: true
```

`access: proxy` significa que Grafana hace las queries desde el servidor, no desde el navegador del usuario. Esto funciona porque Grafana y Valkey estan en la misma red Docker.

---

## 14. Integracion con systemd

El daemon ya esta preparado para systemd:
- Captura `SIGTERM` (la señal que manda `systemctl stop`)
- Realiza limpieza ordenada en los `defer` de `main()`
- Escribe logs en stdout/stderr (redirigidos a `journald`)

### Unit file

```ini
[Unit]
Description=Daemon SO1 PT2 - Monitor de contenedores
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=julian
WorkingDirectory=/home/julian/Julian/201905884_LAB_SO1_1S2026_PT2/Daemon/cmd/daemon
ExecStart=/home/julian/Julian/201905884_LAB_SO1_1S2026_PT2/Daemon/daemon_so1
Restart=no
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

**Por que `WorkingDirectory` apunta a `cmd/daemon/`:** `godotenv.Load()` busca `.env` en el directorio de trabajo. Las rutas relativas del `.env` (`../../scripts/...`) se resuelven correctamente desde `cmd/daemon/`.

**Por que `Restart=no`:** el daemon descarga el modulo de kernel al salir. Si systemd lo reiniciara automaticamente despues de una falla, reintentaria `insmod` sin haber hecho `rmmod`, lo que causaria un error de "module already in use". Para reinicio automatico, primero se debe verificar el estado del modulo.

### Comandos systemd

```bash
sudo systemctl daemon-reload          # recargar tras cambiar el unit file
sudo systemctl start daemon-so1       # iniciar
sudo systemctl stop daemon-so1        # detener (manda SIGTERM)
sudo systemctl status daemon-so1      # estado y ultimas lineas de log
sudo journalctl -u daemon-so1 -f      # logs en tiempo real
sudo systemctl enable daemon-so1      # arrancar al boot
```

---

## 15. Flujo de datos completo

```
/proc/meminfo_pr2_so1_201905884
        |
        | os.ReadFile() [source.FileReader]
        v
  []byte JSON crudo:
  { "memory_info": { "total_ram_kb": 16384000, ... } }
        |
        | json.Unmarshal() [parser.ParseMemInfo]
        v
  model.MemStats { MemTotal: 16384000, MemFree: 8000000, Timestamp: now }
        |
        | json.Marshal() + RPUSH [sink.ValkeyWriter]
        v
  Valkey LIST "meminfo"
  -> {"total_ram_kb":16384000,"free_ram_kb":8000000,"used_ram_kb":8384000,"timestamp":"2026-03-17T..."}


/proc/continfo_pr2_so1_201905884
        |
        | os.ReadFile() [source.FileReader]
        v
  []byte JSON crudo:
  { "processes": [{...}, {...}], "docker_active": 7 }
        |
        | json.Unmarshal() [parser.ParserContInfo]
        v
  model.ContainerReport { Processes: [...], ContainersActive: 7 }
        |
        | docker.Enforce() [docker.Manager]
        v
  EnforceResult {
    ActiveContainers:  [c1, c2, c3, c4, c5],
    RemovedContainers: [c6],
    ActiveLow: 3, ActiveHigh: 2, Removed: 1
  }
        |
        +---> RPUSH continfo <ContainerReport JSON>          [sink.ValkeyWriter]
        |
        +---> Para cada contenedor activo:
        |       RPUSH procinfo <containerRankEntry JSON>     [sink.ValkeyWriter]
        |       ZADD  rss_rank toPerc(MemPct) nombre        [sink.ValkeyRankWriter]
        |       ZADD  cpu_rank toPerc(CPURaw) nombre        [sink.ValkeyRankWriter]
        |       HSET  containers dockerID <JSON completo>   [sink.ValkeyHashWriter]
        |
        +---> Para cada contenedor eliminado:
                RPUSH procinfo <containerRankEntry con status="removed">
                ZREM  rss_rank nombre
                ZREM  cpu_rank nombre
                HDEL  containers dockerID
```

---

## 16. Estructuras de datos en Valkey

### LISTs — historial cronologico

| Key | Comando escritura | Contenido por entrada |
|---|---|---|
| `meminfo` | `RPUSH` | `{"total_ram_kb":N,"free_ram_kb":N,"used_ram_kb":N,"timestamp":"..."}` |
| `continfo` | `RPUSH` | `{"containers_active":N,"containers_exited":N,"containers_removed":N,"containers_inactive":N,"timestamp":"..."}` |
| `procinfo` | `RPUSH` | `{"docker_id":"...","pid":N,"container_name":"...","image":"...","status":"active\|removed","rss_kb":N,"vsz_kb":N,"mem_perc_x100":N,"cpu_perc_x100":N,"timestamp":"..."}` |

Queries de lectura:
```
LLEN    meminfo              # cantidad de entradas
LINDEX  meminfo -1           # ultima entrada
LRANGE  meminfo 0 -1         # historial completo
LRANGE  procinfo 0 49        # ultimos 50 registros
```

### ZSETs — ranking sin duplicados

| Key | Score | Member |
|---|---|---|
| `rss_rank` | `toPerc(MemPct)` = porcentaje real de RAM (ej: `7.35`) | nombre del contenedor |
| `cpu_rank` | `toPerc(CPURaw)` = porcentaje real de CPU (ej: `98.70`) | nombre del contenedor |

Queries de lectura:
```
ZRANGEBYSCORE rss_rank -inf +inf WITHSCORES    # todos ordenados de menor a mayor RAM
ZREVRANGEBYSCORE rss_rank +inf -inf WITHSCORES # todos ordenados de mayor a menor RAM
ZRANGEBYSCORE rss_rank 50 +inf WITHSCORES      # contenedores con mas del 50% de RAM
```

### HASH — estado actual

| Key | Field | Value |
|---|---|---|
| `containers` | docker ID (64 chars) | JSON completo del `containerRankEntry` |

Siempre refleja el estado actual. `HDel` elimina el field cuando el contenedor es removido.

Queries de lectura:
```
HGETALL  containers           # estado de todos los contenedores activos
HGET     containers <dockerID> # estado de un contenedor especifico
HKEYS    containers           # lista de docker IDs activos
HLEN     containers           # cuantos contenedores hay activos
```

---

## 17. Configuracion (.env)

| Variable | Valor por defecto | Descripcion |
|---|---|---|
| `KERNEL_SCRIPT_PATH` | `../../scripts/load_kernel_module.sh` | Script de carga del modulo |
| `KERNEL_UNLOAD_SCRIPT_PATH` | `../../scripts/unload_kernel_module.sh` | Script de descarga del modulo |
| `CRON_SCRIPT_PATH` | `../../scripts/create_containers.sh` | Script ejecutado por el cronjob |
| `FILE_READER_SERVICE_MEM_PATH` | `/proc/meminfo_pr2_so1_201905884` | Entrada /proc de memoria |
| `FILE_READER_SERVICE_CONT_PATH` | `/proc/continfo_pr2_so1_201905884` | Entrada /proc de contenedores |
| `COMPOSE_FILE_PATH` | `../../docker/docker-compose.yml` | docker-compose.yml |
| `GRAFANA_PORT` | `3000` | Puerto expuesto de Grafana |
| `GRAFANA_USER` | `admin` | Usuario inicial de Grafana |
| `GRAFANA_PASSWORD` | `admin` | Contrasena inicial de Grafana |
| `VALKEY_ADDR` | `localhost:6379` | Direccion de Valkey desde el host |
| `VALKEY_KEY_MEM` | `meminfo` | Key de la lista de memoria en Valkey |
| `VALKEY_KEY_CONT` | `continfo` | Key de la lista de contenedores en Valkey |
| `VALKEY_KEY_PROC` | `procinfo` | Key de la lista de procesos en Valkey |
| `VALKEY_KEY_RSS_RANK` | `rss_rank` | Key del ZSET de ranking por RAM |
| `VALKEY_KEY_CPU_RANK` | `cpu_rank` | Key del ZSET de ranking por CPU |
| `VALKEY_KEY_CONTAINERS` | `containers` | Key del HASH de estado actual |

---

## 18. Decisiones de diseno

| Decision | Razon |
|---|---|
| Script bash separado para insmod | El script puede ejecutarse independientemente para diagnosticar el modulo sin correr el daemon entero |
| `set -euo pipefail` en los scripts | Impide que `insmod` falle silenciosamente y el daemon arranque sin modulo |
| Verificacion de `/proc` en el script | Prueba de aceptacion: confirma que el modulo ejecuto su `__init` correctamente |
| `lsmod` al inicio del script de carga | Hace el daemon idempotente: puede reiniciarse sin error aunque el modulo ya este activo |
| `kernel.Load()` antes del contexto | Falla rapido: no tiene sentido iniciar si las entradas `/proc` no estan disponibles |
| Intervalo aleatorio 20-60s | Evita patrones predecibles de carga en el sistema |
| Valores `x100` del kernel | El kernel evita floats multiplicando x100; `toPerc()` los convierte al escribir en ZSET |
| ZSET con `member=nombre` y no ID | El nombre es estable; garantiza unicidad sin duplicados por reconexion de contenedor |
| HASH con `field=dockerID` | El ID es inmutable; unicidad garantizada aunque cambien el nombre |
| `RPush` en lugar de `SET` | Acumula historial cronologico en lugar de sobreescribir el ultimo valor |
| `Processes json:"-"` en ContainerReport | Los procesos se guardan individualmente en `procinfo`; incluirlos en `continfo` crearia JSON enorme |
| Goroutine para señales | `<-sigChan` es bloqueante; sin goroutine el daemon no llegaria a `svc.Run()` |
| `defer kernel.Unload()` en main | Garantiza la descarga del modulo incluso si hay panic o error inesperado |
| `WorkingDirectory=cmd/daemon/` en systemd | `godotenv.Load()` busca `.env` en el CWD; las rutas relativas del .env resuelven desde ahi |

---

## 19. Referencia de errores

| Error en log | Causa | Solucion |
|---|---|---|
| `kernel: script no encontrado "..."` | Ruta incorrecta en `KERNEL_SCRIPT_PATH` o CWD incorrecto | Verificar `.env` y ejecutar desde `cmd/daemon/` |
| `kernel: error al ejecutar el script` | `insmod` rechazo el modulo | `dmesg \| tail -20` para el error del kernel |
| `ERROR: /proc/meminfo_... no existe` | El modulo cargo pero `__init` fallo al crear la entrada | `dmesg \| tail -20` |
| `error al leer el archivo /proc/...` | Modulo no cargado | `lsmod \| grep pr2_so1_201905884` |
| `main: error al levantar contenedores` | Docker no esta corriendo o usuario sin permisos | `systemctl start docker` y verificar grupo `docker` |
| `valkey: error escribiendo en meminfo` | Valkey no esta corriendo | `docker ps \| grep valkey` |
| `service: docker enforce` | Error ejecutando `docker ps` o `docker stop` | Verificar que docker esta corriendo y el usuario esta en el grupo `docker` |
| `status=203/EXEC` en systemd | El binario no existe o la ruta en el unit file es incorrecta | Compilar: `go build -o daemon_so1 ./cmd/daemon` |
| `crontab register: exit status 1` | El usuario no tiene permiso para modificar su crontab | Verificar que `crontab` esta disponible: `which crontab` |
