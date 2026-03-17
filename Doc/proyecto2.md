# Proyecto 2 — SO1 | 201905884
## Telemetría de Contenedores + Memoria RAM

---

## 1. Arquitectura General

```
╔══════════════════════════════════════════════════════════════════════╗
║                          KERNEL SPACE                               ║
║                                                                      ║
║   pr2_so1_201905884.ko                                               ║
║   ├── /proc/meminfo_pr2_so1_201905884   → JSON  { memory_info: … }  ║
║   └── /proc/continfo_pr2_so1_201905884  → JSON  { processes: […] }  ║
╚══════════════════════════════╦═══════════════════════════════════════╝
                               ║  polling aleatorios 20-60s
╔══════════════════════════════╩═══════════════════════════════════════╗
║                       USER SPACE — Go daemon                        ║
║                                                                      ║
║  ARRANQUE (una sola vez):                                            ║
║  ├── godotenv.Load()           → carga variables desde .env         ║
║  ├── docker compose up -d      → levanta Grafana + Valkey           ║
║  ├── kernel.Load()             → insmod via script bash             ║
║  └── app.RegisterCronjob()     → crea entrada crontab (*/2 min)     ║
║                                                                      ║
║  LOOP (cada 20-60 s aleatorios):                                     ║
║  ├── source.FileReader.Read()  → []byte JSON crudo de /proc         ║
║  ├── parser.ParseMemInfo()     → model.MemStats                     ║
║  ├── parser.ParseContInfo()    → model.ContainerReport              ║
║  ├── docker.Manager.Enforce()  → aplica invariantes (3 low + 2 high)║
║  └── sink.ValkeyWriter         → escribe en 6 claves de Valkey      ║
║      ├── RPUSH meminfo         → historial RAM                      ║
║      ├── RPUSH continfo        → historial contenedores             ║
║      ├── RPUSH procinfo        → historial por contenedor           ║
║      ├── ZADD  rss_rank        → ranking RAM actual (sorted set)    ║
║      ├── ZADD  cpu_rank        → ranking CPU actual (sorted set)    ║
║      └── HSET  containers      → estado actual por ID (hash)        ║
╚══════════════════════════════╦═══════════════════════════════════════╝
                               ║
╔══════════════════════════════╩═══════════════════════════════════════╗
║                       DOCKER COMPOSE                                ║
║                                                                      ║
║  valkey_so1  (puerto 6379)                                           ║
║  ├── lista:       meminfo    → entradas JSON de RAM en orden        ║
║  ├── lista:       continfo   → resumen de contenedores activos      ║
║  ├── lista:       procinfo   → métricas por contenedor (historial)  ║
║  ├── sorted-set:  rss_rank   → ranking por RSS (sin duplicados)     ║
║  ├── sorted-set:  cpu_rank   → ranking por CPU (sin duplicados)     ║
║  └── hash:        containers → estado actual por docker_id          ║
║                                                                      ║
║  grafana_so1 (puerto 3000)                                           ║
║  ├── plugin:      redis-datasource  (valkey es protocolo Redis)     ║
║  ├── datasource:  redis://valkey:6379                               ║
║  └── acceso:      http://localhost:3000  (admin / admin)            ║
╚══════════════════════════════════════════════════════════════════════╝
```

---

## 2. Estructura de Carpetas

```
201905884_LAB_SO1_1S2026_PT2/
├── Doc/
│   └── proyecto2.md                        ← este archivo
├── Enunciado/
│   └── Proyecto2.pdf
└── Daemon/
    ├── cmd/daemon/
    │   ├── main.go                         ← entry-point: arranque + señales OS
    │   └── .env                            ← variables de entorno
    ├── internal/
    │   ├── kernel/loader.go                ← Load() / Unload() via script bash
    │   ├── app/service.go                  ← loop principal + tick()
    │   ├── app/cronjob.go                  ← RegisterCronjob / RemoveCronjob
    │   ├── model/metrics.go                ← structs: MemStats, ContainerReport
    │   ├── parser/meminfo.go               ← JSON /proc → model.MemStats
    │   ├── parser/continfo.go              ← JSON /proc → model.ContainerReport
    │   ├── sink/valkey.go                  ← ValkeyWriter / RankWriter / HashWriter
    │   ├── sink/jsonfile.go                ← escritura en archivo (legacy)
    │   ├── source/file_reader.go           ← FileReader: lee /proc con contexto
    │   └── docker/manager.go               ← Manager.Enforce(): ciclo de vida
    ├── docker/
    │   ├── docker-compose.yml              ← Grafana + Valkey en red compartida
    │   └── grafana/provisioning/
    │       └── datasources/valkey.yml      ← datasource Valkey preconfigurado
    ├── kernel/
    │   ├── pr2_so1_201905884.c             ← módulo del kernel
    │   └── Makefile
    ├── scripts/
    │   ├── load_kernel_module.sh           ← compila + insmod + verifica /proc
    │   ├── unload_kernel_module.sh         ← rmmod
    │   └── create_containers.sh            ← ejecutado por el cronjob cada 2 min
    ├── doc/Daemon.md
    ├── go.mod
    └── go.sum
```

---
## 3. Módulo de Kernel

### 3.1 ¿Qué hace el módulo?

El módulo de kernel `pr2_so1_201905884.ko` es un LKM (Loadable Kernel Module) escrito en C que:

1. Se registra en `/proc` al cargarse (`insmod`) creando dos entradas virtuales de solo lectura.
2. Lee información de la memoria del sistema via `si_meminfo()` al leer `/proc/meminfo_*`.
3. Itera sobre todos los procesos del sistema via `for_each_process` al leer `/proc/continfo_*`.
4. Para cada proceso, usa `get_task_mm()` para obtener métricas de memoria (VSZ, RSS).
5. Identifica si un proceso pertenece a un contenedor Docker leyendo su cgroup v2 path.
6. Serializa toda la información como JSON y la expone a espacio de usuario.

### 3.2 Entradas /proc creadas

| Entrada | Tipo | Contenido |
|---|---|---|
| `/proc/meminfo_pr2_so1_201905884` | Virtual (read-only) | JSON con RAM total, libre y usada en KB |
| `/proc/continfo_pr2_so1_201905884` | Virtual (read-only) | JSON con lista de procesos + métricas + docker_active |

### 3.3 Formato JSON de salida

**`/proc/meminfo_pr2_so1_201905884`**
```json
{
  "memory_info": {
    "total_ram_kb": 8157520,
    "free_ram_kb":  3200000,
    "used_ram_kb":  4957520
  }
}
```

**`/proc/continfo_pr2_so1_201905884`**
```json
{
  "processes": [
    {
      "pid": 1234,
      "name": "containerd-shim",
      "cmdline": "containerd-shim-runc-v2 -namespace moby -id abc123def456",
      "vsz_kb": 102400,
      "rss_kb": 51200,
      "mem_perc_x100": 63,
      "cpu_perc_x100": 412,
      "container_id": "abc123def456"
    }
  ],
  "docker_active": 5
}
```

> **Nota sobre `*_perc_x100`:** el kernel no puede usar `float`. Los valores son enteros que representan
> el porcentaje multiplicado por 100. Ejemplo: `63` significa `0.63 %`. Los valores se almacenan tal cual en Valkey;
> la conversión a porcentaje real (dividiendo por 100) ocurre al configurar los paneles en Grafana.

### 3.4 Headers del kernel utilizados

| Header | Propósito |
|---|---|
| `linux/proc_fs.h` + `linux/seq_file.h` | API estable para crear archivos `/proc` secuenciales |
| `linux/mmzone.h` | `si_meminfo()` — estadísticas globales de RAM |
| `linux/mm.h` | `get_task_mm()` / `mmput()` — acceso seguro a memoria de proceso |
| `linux/sched/signal.h` | `for_each_process` — iteración sobre tabla de procesos |
| `linux/cgroup.h` | `cgroup_path()` — ruta del cgroup v2 del proceso |
| `linux/slab.h` | `kmalloc(GFP_ATOMIC)` / `kfree` — memoria dinámica del kernel |
| `linux/mmap_lock.h` | `mmap_read_lock/unlock` — protección del espacio de memoria |

### 3.5 Decisiones técnicas clave

| Elemento | Decisión | Razón |
|---|---|---|
| `seq_file` + `single_open` | Exponer datos en `/proc` | API estable para archivos read-only del kernel |
| `si_meminfo()` | Obtener estadísticas de RAM | Función pública del kernel, precisa y segura |
| `proc_ops` | En vez de `file_operations` | Obligatorio en kernels >= 5.6 |
| `GFP_ATOMIC` en `kmalloc` | Dentro del loop de procesos | No puede dormirse dentro de `rcu_read_lock` |
| `get_task_mm()` + `mmput()` | Acceder a `task->mm` | Evita race condition si el proceso termina |
| `rcu_read_lock/unlock` | Alrededor de `for_each_process` | `list_entry_rcu` requiere protección RCU |
| `*_perc_x100` como `uint64` | En vez de float | El kernel prohíbe float en espacio kernel |
| JSON como formato de salida | En vez de texto plano | Elimina la necesidad de un parser personalizado en Go |

### 3.6 Comandos de build y prueba

```bash
cd Daemon/kernel

# Compilar (requiere linux-headers instalados)
make

# Cargar módulo en el kernel
sudo insmod pr2_so1_201905884.ko

# Verificar las entradas /proc
cat /proc/meminfo_pr2_so1_201905884
cat /proc/continfo_pr2_so1_201905884

# Ver logs del kernel
dmesg | tail -10

# Descargar módulo
sudo rmmod pr2_so1_201905884

# Limpiar binarios
make clean
```

---
## 4. El Daemon — Proceso Completo

### 4.1 Descripción General

El daemon es un proceso en segundo plano (background) escrito en Go que:

- **No tiene interfaz de usuario**: se ejecuta con `sudo` y corre indefinidamente.
- **Orquesta todo el sistema**: carga el kernel, levanta Docker, registra el cron y hace polling de `/proc`.
- **Responde a señales del OS**: `SIGINT` (Ctrl+C) y `SIGTERM` inician un shutdown limpio.
- **Intervalo aleatorio**: espera entre 20 y 60 segundos entre ciclos.

### 4.2 Arranque paso a paso

```
main()
  │
  ├─ 1. godotenv.Load()
  │      Lee Daemon/cmd/daemon/.env
  │      Carga variables: rutas de scripts, claves Valkey, puertos, etc.
  │
  ├─ 2. kernel.Load(LoadOpts{ScriptPath: ...})
  │      Ejecuta scripts/load_kernel_module.sh via /bin/bash
  │      El script: compila el .ko → insmod → verifica /proc
  │      Si falla → log.Fatalf (el daemon no arranca sin el módulo)
  │      defer kernel.Unload() se ejecutará al terminar el daemon
  │
  ├─ 3. exec.Command(docker compose up -d)
  │      Levanta valkey_so1 y grafana_so1 en red compartida red_pr2_so1
  │      Si falla → log.Fatalf
  │
  ├─ 4. app.RegisterCronjob(cronScript)
  │      Agrega '*/2 * * * * /abs/path/create_containers.sh' al crontab
  │      El script crea 5 contenedores aleatorios cada 2 minutos
  │      Idempotente: no duplica si ya existe la entrada
  │
  ├─ 5. signal.Notify(sigChan, SIGINT, SIGTERM)
  │      goroutine: espera señal → cancel() → ctx.Done() se cierra
  │
  └─ 6. svc.Run(ctx)
         Loop infinito (ver sección 4.3)
```

### 4.3 El Loop Principal — `service.Run()`

```
Run(ctx):
  loop:
    wait = random(20s … 60s)
    select:
      case ctx.Done():        → retorna nil (shutdown limpio)
      case time.After(wait):  → tick(ctx)
```

Cada tick ejecuta:

```
tick(ctx):
  │
  ├─ A. Leer memoria
  │      FileReader.Read('/proc/meminfo_pr2_so1_201905884')
  │        → []byte (JSON crudo del kernel)
  │      parser.ParseMemInfo(raw)
  │        → model.MemStats{ MemTotal, MemFree, MemUsed, Timestamp }
  │      ValkeyWriter.Write(mem)
  │        → RPUSH meminfo <json>
  │
  ├─ B. Leer contenedores
  │      FileReader.Read('/proc/continfo_pr2_so1_201905884')
  │        → []byte (JSON crudo del kernel)
  │      parser.ParseContInfo(raw)
  │        → model.ContainerReport{ Processes[], ContainersActive }
  │
  ├─ C. Aplicar invariantes (Docker.Enforce)
  │      list() → docker ps (contenedores activos)
  │      enrichWithMetrics() → vincula métricas del kernel a cada contenedor
  │      Si hay exceso de bajo consumo → stopAndRemove los de mayor consumo
  │      Si hay exceso de alto consumo → stopAndRemove los de menor consumo
  │      Si faltan bajos → createLow() (alpine sleep 240)
  │      Si faltan altos → createHigh() (go-client o alpine stress)
  │      Retorna EnforceResult{ Active[], Removed[], Counts }
  │
  └─ D. Persistir en Valkey (6 claves)
         ContWriter.Write(cont)       → RPUSH continfo <json>
         ProcWriter.Write(entry)      → RPUSH procinfo <json> (uno por contenedor)
         RssRankWriter.Upsert()       → ZADD rss_rank score=memPct% member=name
         CpuRankWriter.Upsert()       → ZADD cpu_rank score=cpuPct% member=name
         ContainerHashWriter.HSet()   → HSET containers <docker_id> <json>
         (cuando se elimina un contenedor)
         RssRankWriter.Remove()       → ZREM rss_rank member
         CpuRankWriter.Remove()       → ZREM cpu_rank member
         ContainerHashWriter.HDel()   → HDEL containers <docker_id>
```

### 4.4 Shutdown Limpio

Cuando el daemon recibe `SIGINT` o `SIGTERM`:

1. La goroutine de señales llama a `cancel()`.
2. `ctx.Done()` se cierra → `svc.Run()` retorna `nil` en la próxima iteración del select.
3. `app.RemoveCronjob()` elimina la entrada del crontab del sistema.
4. `defer kernel.Unload()` ejecuta `scripts/unload_kernel_module.sh` → `rmmod`.
5. `main()` retorna y el proceso termina limpiamente.

### 4.5 Variables de Entorno (.env)

| Variable | Valor por defecto | Descripción |
|---|---|---|
| `KERNEL_SCRIPT_PATH` | `../../scripts/load_kernel_module.sh` | Ruta al script de carga del módulo |
| `KERNEL_UNLOAD_SCRIPT_PATH` | `../../scripts/unload_kernel_module.sh` | Script de descarga |
| `CRON_SCRIPT_PATH` | `../../scripts/create_containers.sh` | Script del cronjob |
| `FILE_READER_SERVICE_MEM_PATH` | `/proc/meminfo_pr2_so1_201905884` | Entrada /proc de RAM |
| `FILE_READER_SERVICE_CONT_PATH` | `/proc/continfo_pr2_so1_201905884` | Entrada /proc de contenedores |
| `COMPOSE_FILE_PATH` | `../../docker/docker-compose.yml` | Archivo docker-compose |
| `VALKEY_ADDR` | `localhost:6379` | Dirección del servidor Valkey |
| `VALKEY_KEY_MEM` | `meminfo` | Clave lista de RAM |
| `VALKEY_KEY_CONT` | `continfo` | Clave lista de contenedores |
| `VALKEY_KEY_PROC` | `procinfo` | Clave lista de procesos por contenedor |
| `VALKEY_KEY_RSS_RANK` | `rss_rank` | Clave sorted-set de ranking RAM |
| `VALKEY_KEY_CPU_RANK` | `cpu_rank` | Clave sorted-set de ranking CPU |
| `VALKEY_KEY_CONTAINERS` | `containers` | Clave hash de estado actual |

---
## 5. Proceso de Lectura/Escritura en Valkey

### 5.1 ¿Qué es Valkey?

Valkey es un fork open-source de Redis, 100% compatible con el protocolo Redis. El daemon lo usa como
base de datos en memoria para almacenar métricas en tiempo real. Grafana lo lee a través del plugin `redis-datasource`.

### 5.2 Tres tipos de escritores (sink/valkey.go)

#### ValkeyWriter — Listas (historial cronológico)

```go
type ValkeyWriter struct {
    Client *redis.Client
    Key    string
}

func (v *ValkeyWriter) Write(data any) error {
    b, _ := json.Marshal(data)
    v.Client.RPush(ctx, v.Key, b)   // Agrega al FINAL de la lista
    return nil
}
```

Comando Valkey: `RPUSH <key> <json_bytes>`

- Cada llamada agrega un elemento al final de la lista.
- La lista crece indefinidamente guardando el historial completo.
- El elemento más nuevo está al final: `LINDEX key -1`
- El elemento más viejo está al inicio: `LINDEX key 0`

#### ValkeyRankWriter — Sorted Sets (rankings sin duplicados)

```go
type ValkeyRankWriter struct {
    Client *redis.Client
    Key    string
}

func (v *ValkeyRankWriter) Upsert(score float64, member string) error {
    v.Client.ZAdd(ctx, v.Key, redis.Z{Score: score, Member: member})
    return nil
}

func (v *ValkeyRankWriter) Remove(member string) error {
    v.Client.ZRem(ctx, v.Key, member)
    return nil
}
```

Comandos Valkey: `ZADD <key> <score> <member>` / `ZREM <key> <member>`

- `member` = nombre del contenedor (único → no hay duplicados).
- `score` = porcentaje de RAM o CPU como float64.
- Si el contenedor ya existe, `ZADD` actualiza su score (upsert automático).
- Si el contenedor se elimina, `ZREM` lo quita del ranking.
- Permite consultas por rango: `ZRANGEBYSCORE rss_rank 0 5` = contenedores con menos del 5% RAM.

#### ValkeyHashWriter — Hash (estado actual completo)

```go
type ValkeyHashWriter struct {
    Client *redis.Client
    Key    string
}

func (v *ValkeyHashWriter) HSet(field string, value any) error {
    b, _ := json.Marshal(value)
    v.Client.HSet(ctx, v.Key, field, b)
    return nil
}

func (v *ValkeyHashWriter) HDel(field string) error {
    v.Client.HDel(ctx, v.Key, field)
    return nil
}
```

Comandos Valkey: `HSET <key> <docker_id> <json>` / `HDEL <key> <docker_id>`

- `field` = Docker ID completo (único por contenedor).
- `value` = JSON completo con todas las métricas actuales del contenedor.
- `HGETALL containers` devuelve el estado actual de todos los contenedores activos.
- Al eliminar un contenedor, `HDEL` lo quita del hash.

### 5.3 Las 6 Claves en Valkey

| Clave | Tipo Redis | Escritura | Lectura en Grafana | Contenido |
|---|---|---|---|---|
| `meminfo` | List | `RPUSH` | `LRANGE meminfo 0 -1` | JSON de RAM por tick |
| `continfo` | List | `RPUSH` | `LRANGE continfo 0 -1` | JSON de resumen de contenedores |
| `procinfo` | List | `RPUSH` | `LRANGE procinfo 0 -1` | JSON de métricas por contenedor |
| `rss_rank` | Sorted Set | `ZADD` | `ZRANGEBYSCORE rss_rank -inf +inf WITHSCORES` | Ranking por % RAM |
| `cpu_rank` | Sorted Set | `ZADD` | `ZRANGEBYSCORE cpu_rank -inf +inf WITHSCORES` | Ranking por % CPU |
| `containers` | Hash | `HSET` | `HGETALL containers` | Estado actual por docker_id |

### 5.4 Formato JSON almacenado

**Clave `meminfo`** (una entrada por tick):
```json
{
  "total_ram_kb": 8157520,
  "free_ram_kb":  3200000,
  "used_ram_kb":  4957520,
  "timestamp":    "2026-03-17T01:30:00Z"
}
```

**Clave `continfo`** (una entrada por tick):
```json
{
  "containers_active":   5,
  "containers_exited":   2,
  "containers_removed":  1,
  "containers_inactive": 3,
  "timestamp": "2026-03-17T01:30:00Z"
}
```

**Clave `procinfo`** (una entrada por contenedor por tick):
```json
{
  "docker_id":      "abc123def456...",
  "pid":            5678,
  "container_name": "my_alpine",
  "image":          "alpine",
  "status":         "active",
  "rss_kb":         51200,
  "vsz_kb":         102400,
  "mem_perc_x100":  63,
  "cpu_perc_x100":  412,
  "timestamp":      "2026-03-17T01:30:00Z"
}
```

**Sorted set `rss_rank`**:
```
member="my_alpine"       score=0.63   (0.63% RAM)
member="go-client_abc"   score=2.15   (2.15% RAM)
member="stress_xyz"      score=0.08   (0.08% RAM)
```

**Hash `containers`**:
```
field="abc123def456..."  →  value=<JSON completo del contenedor>
field="def789ghi012..."  →  value=<JSON completo del contenedor>
```

### 5.5 Verificación directa en Valkey

```bash
# Conectar al cliente Valkey dentro del contenedor
sudo docker exec -it valkey_so1 valkey-cli

# Contar entradas en las listas
LLEN meminfo
LLEN continfo
LLEN procinfo

# Ver la última entrada de RAM (elemento -1 = último agregado)
LINDEX meminfo -1

# Ver los últimos 5 resúmenes de contenedores
LRANGE continfo -5 -1

# Ver ranking por RAM de menor a mayor consumo
ZRANGEBYSCORE rss_rank -inf +inf WITHSCORES

# Ver ranking por CPU de mayor a menor consumo
ZREVRANGEBYSCORE cpu_rank +inf -inf WITHSCORES

# Ver estado actual de todos los contenedores
HGETALL containers

# Ver todas las claves activas
KEYS *
```

---
## 6. Grafana — Visualización de Datos

### 6.1 ¿Cómo se conecta Grafana a Valkey?

Grafana usa el plugin `redis-datasource` que implementa el protocolo Redis (Valkey es 100% compatible).
La conexión está preconfigurada en `docker/grafana/provisioning/datasources/valkey.yml`:

```yaml
apiVersion: 1

datasources:
  - name: Valkey
    type: redis-datasource
    access: proxy
    url: redis://valkey:6379   # 'valkey' = nombre del servicio en Docker Compose
    isDefault: true
    editable: true
```

**Puntos clave:**

- `access: proxy` → Grafana hace las queries desde el servidor, no desde el browser.
- `url: redis://valkey:6379` → `valkey` se resuelve por DNS interno de la red Docker (`red_pr2_so1`).
- El archivo se monta via volumen Docker en `/etc/grafana/provisioning/datasources/`.
- Al arrancar Grafana, lee automáticamente este archivo y configura el datasource sin intervención manual.

### 6.2 Queries disponibles en Grafana por tipo de panel

| Panel | Tipo de query | Comando Valkey |
|---|---|---|
| Línea de tiempo RAM usada | Time series desde lista | `LRANGE meminfo 0 -1` |
| RAM libre vs usada | Time series desde lista | `LRANGE meminfo 0 -1` |
| Contenedores activos en el tiempo | Time series desde lista | `LRANGE continfo 0 -1` |
| Historial de eliminaciones | Time series desde lista | `LRANGE continfo 0 -1` |
| Ranking actual por RAM | Bar/Table desde sorted set | `ZRANGEBYSCORE rss_rank -inf +inf WITHSCORES` |
| Ranking actual por CPU | Bar/Table desde sorted set | `ZRANGEBYSCORE cpu_rank -inf +inf WITHSCORES` |
| Estado actual de contenedores | Table desde hash | `HGETALL containers` |

### 6.3 Flujo completo de un dato hasta Grafana

```
1. El kernel escribe en /proc/meminfo_pr2_so1_201905884 al ser leído:
   { "memory_info": { "total_ram_kb": 8157520, "free_ram_kb": 3200000, "used_ram_kb": 4957520 } }

2. El daemon (FileReader.Read) lee el archivo virtual:
   rawMem = []byte(JSON crudo del kernel)

3. parser.ParseMemInfo(rawMem) convierte el JSON a struct Go:
   mem = model.MemStats{
     MemTotal: 8157520, MemFree: 3200000, MemUsed: 4957520, Timestamp: now()
   }

4. ValkeyWriter.Write(mem) serializa y persiste en Valkey:
   RPUSH meminfo '{"total_ram_kb":8157520,"free_ram_kb":3200000,"used_ram_kb":4957520,"timestamp":"..."}'

5. Valkey mantiene la lista 'meminfo' con todas las lecturas históricas.

6. Grafana (al abrir el dashboard o en auto-refresh):
   redis-datasource ejecuta: LRANGE meminfo 0 -1
   Recibe array de strings JSON

7. Grafana parsea cada JSON, extrae los campos del panel configurado:
   campo 'used_ram_kb' como valor Y, campo 'timestamp' como eje X

8. El panel renderiza la línea de tiempo con la evolución de la RAM.
```

### 6.4 Acceso inicial a Grafana

```bash
# Abrir en el browser
http://localhost:3000

# Credenciales por defecto (configuradas en docker-compose.yml)
# Usuario:    admin
# Contraseña: admin

# Navegar a Explore → seleccionar datasource 'Valkey'
# Probar query: LRANGE meminfo 0 -1
```

---

## 7. Gestión de Contenedores Docker

### 7.1 El Cronjob

Al iniciar, el daemon registra en el crontab del sistema:

```cron
*/2 * * * * /ruta/absoluta/create_containers.sh
```

El script `create_containers.sh` crea 5 contenedores aleatorios cada 2 minutos:

| Tipo | Imagen | Comando | Categoría |
|---|---|---|---|
| Go-client | `roldyoran/go-client` | (default) | Alto consumo RAM |
| Stress CPU | `alpine` | `while true; do echo '2^20' | bc; sleep 2; done` | Alto consumo CPU |
| Sleep | `alpine` | `sleep 240` | Bajo consumo |

### 7.2 Invariantes del Docker Manager

El daemon mantiene siempre estas cantidades de contenedores activos:

| Categoría | Target | Criterio al eliminar exceso |
|---|---|---|
| Bajo consumo (`alpine sleep`) | **3** | Elimina los de MAYOR consumo |
| Alto consumo (`go-client` o `stress`) | **2** | Elimina los de MENOR consumo |
| Sistema (`grafana`, `valkey`) | — | Nunca se tocan |

### 7.3 Flujo de Enforce() en cada tick

```
Enforce(processes):
  │
  ├─ 1. docker ps → lista todos los contenedores activos
  │
  ├─ 2. classify() → categoriza cada contenedor:
  │      grafana/valkey → System  (nunca eliminar)
  │      alpine + sleep → Low     (bajo consumo)
  │      go-client      → High    (alto consumo RAM)
  │      alpine + bc    → High    (alto consumo CPU)
  │
  ├─ 3. enrichWithMetrics() → vincula métricas del kernel
  │      Busca prefijo de 12 chars del container_id en los procesos de /proc
  │      Suma RSS, VSZ, MemPct, CPURaw de todos los procesos del contenedor
  │
  ├─ 4. Aplicar invariantes:
  │      Low > 3  → ordenar desc → eliminar los de mayor consumo
  │      High > 2 → ordenar desc → eliminar los de menor consumo
  │
  ├─ 5. Crear faltantes:
  │      Low < 3  → createLow()  → docker run -d alpine sleep 240
  │      High < 2 → createHigh() → docker run -d go-client  (50% de probabilidad)
  │                              → docker run -d alpine stress (50% de probabilidad)
  │
  └─ 6. Retornar EnforceResult:
         ActiveContainers[]  → los que sobreviven (para rankings en Valkey)
         RemovedContainers[] → los eliminados (para historial en Valkey)
         Counts: Removed, ActiveLow, ActiveHigh, Exited
```

### 7.4 Contenedores en estado Exited

Los contenedores `alpine sleep 240` terminan solos después de 4 minutos.
El manager los detecta via `docker ps -a --filter status=exited` y los cuenta en `ContainersExited`.
El cronjob repone los contenedores perdidos en la siguiente ejecución (cada 2 minutos).

---

## 8. Infraestructura Docker Compose

### 8.1 Servicios

| Servicio | Imagen | Puerto | Descripción |
|---|---|---|---|
| `valkey_so1` | `valkey/valkey:latest` | `6379` | Base de datos en memoria donde el daemon escribe métricas |
| `grafana_so1` | `grafana/grafana:latest` | `3000` | Visualización via plugin redis-datasource |

### 8.2 Comunicación entre componentes

```
Host (daemon Go):                    Red red_pr2_so1 (bridge interna Docker):
  localhost:6379 ──────────────────► valkey_so1:6379    (Valkey)
  localhost:3000                     grafana_so1:3000   (Grafana)
                                           │
                              redis://valkey:6379 (DNS interno Docker)
                                           ▼
                                     valkey_so1:6379    (misma instancia)
```

- El daemon Go corre en el **host**, accede a Valkey via `localhost:6379`.
- Grafana corre en un **contenedor**, accede a Valkey via `redis://valkey:6379`.
- Ambos acceden a la **misma instancia** de Valkey.

### 8.3 Comandos de gestión

```bash
# Levantar servicios
sudo docker compose -f Daemon/docker/docker-compose.yml up -d

# Ver estado
sudo docker compose -f Daemon/docker/docker-compose.yml ps

# Ver logs
sudo docker compose -f Daemon/docker/docker-compose.yml logs -f

# Detener (mantiene datos en volúmenes)
sudo docker compose -f Daemon/docker/docker-compose.yml stop

# Detener y eliminar contenedores
sudo docker compose -f Daemon/docker/docker-compose.yml down

# Detener y eliminar todo incluyendo datos
sudo docker compose -f Daemon/docker/docker-compose.yml down -v
```

---

## 9. Ejecución Completa del Proyecto

### 9.1 Prerrequisitos

```bash
# Headers del kernel (para compilar el módulo)
sudo apt install linux-headers-$(uname -r)

# Go (para compilar el daemon)
sudo apt install golang-go

# Docker y Docker Compose v2
sudo apt install docker.io docker-compose-v2

# Dependencias Go del proyecto
cd Daemon && go mod download
```

### 9.2 Ejecución

```bash
cd Daemon/cmd/daemon
sudo go run .
```

### 9.3 Salida esperada

```
main: archivo .env cargado exitosamente
main: cargando módulo de Kernel ...
kernel: [kernel-loader] compilando para kernel 6.x.x-generic...
kernel: [kernel-loader] compilación OK
kernel: [kernel-loader] módulo cargado OK
main: módulo de Kernel cargado exitosamente
main: levantando contenedores de Grafana y Valkey ...
[+] Running 2/2
 Container valkey_so1   Started
 Container grafana_so1  Started
main: contenedores de Grafana y Valkey levantados exitosamente
main: cronjob registrado (cada 2 minutos)
main: daemon iniciado
service: próxima ejecución en 34s
```

---

## 10. Errores Comunes y Solución

| Error | Causa | Solución |
|---|---|---|
| `no se pudo cargar el archivo .env` | No se ejecuta desde `cmd/daemon/` | `cd Daemon/cmd/daemon && sudo go run .` |
| `kernel: script no encontrado` | Ruta incorrecta en `.env` | Verificar `KERNEL_SCRIPT_PATH` en `.env` |
| `insmod: Operation not permitted` | Secure Boot activo | Deshabilitar Secure Boot en BIOS |
| `Unknown symbol` al hacer insmod | `CONFIG_MEMCG=n` en el kernel | Verificar: `grep CONFIG_MEMCG /boot/config-$(uname -r)` |
| `docker compose: command not found` | Docker Compose v2 no instalado | `sudo apt install docker-compose-v2` |
| `connection refused` en Valkey | Contenedor no levantado | `sudo docker compose up -d` |
| `/proc/meminfo_*: No such file` | Módulo no cargado | `sudo insmod Daemon/kernel/pr2_so1_201905884.ko` |
| Grafana sin datos en el dashboard | Datasource no conectado | Verificar en `http://localhost:3000/datasources` |

---

## 11. Comandos Rápidos de Referencia

```bash
# Kernel
cd Daemon/kernel && make
sudo insmod pr2_so1_201905884.ko
cat /proc/meminfo_pr2_so1_201905884
cat /proc/continfo_pr2_so1_201905884
dmesg | tail -10
sudo rmmod pr2_so1_201905884
make clean

# Daemon
cd Daemon/cmd/daemon && sudo go run .

# Docker
sudo docker compose -f Daemon/docker/docker-compose.yml up -d
sudo docker compose -f Daemon/docker/docker-compose.yml ps
sudo docker compose -f Daemon/docker/docker-compose.yml down

# Valkey (dentro del contenedor)
sudo docker exec -it valkey_so1 valkey-cli
LLEN meminfo
LINDEX meminfo -1
LRANGE continfo 0 -1
ZRANGEBYSCORE rss_rank -inf +inf WITHSCORES
HGETALL containers
KEYS *

# Grafana: http://localhost:3000  (admin / admin)
```
