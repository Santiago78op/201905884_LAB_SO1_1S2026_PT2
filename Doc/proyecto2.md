# Proyecto 2 — SO1 | 201905884
## Telemetría de Contenedores + Memoria RAM

---

## 1. Arquitectura General

```
┌─────────────────────────────────────────────────────────────┐
│                        KERNEL SPACE                         │
│                                                             │
│  pr2_so1_201905884.ko                                       │
│  ├── /proc/meminfo_pr2_so1_201905884  → RAM total/free/used │
│  └── /proc/continfo_pr2_so1_201905884 → procesos + cgroups  │
└────────────────────────┬────────────────────────────────────┘
                         │ lectura cada 5s
┌────────────────────────▼────────────────────────────────────┐
│                       USER SPACE                            │
│                                                             │
│  daemon (Go)                                                │
│  ├── readMemInfo()   → parsea /proc/meminfo                 │
│  ├── readContInfo()  → parsea /proc/continfo                │
│  ├── json.Marshal()  → serializa Telemetry struct           │
│  ├── writeLog()      → /var/log/proyecto2/telemetria.log    │
│  └── pushToValkey()  → Valkey SET/LPUSH                     │
└────────────┬───────────────────────────┬────────────────────┘
             │                           │
┌────────────▼──────────┐   ┌────────────▼────────────────────┐
│  JSONL Log            │   │  Valkey                         │
│  telemetria.log       │   │  telemetria:latest  (GET)       │
│  {"timestamp":...}    │   │  telemetria:history (LRANGE)    │
│  {"timestamp":...}    │   │                                 │
└───────────────────────┘   └─────────────────────────────────┘
```

---

## 2. Estructura de Carpetas

```
201905884_LAB_SO1_1S2026_PT2/
├── Doc/
│   └── proyecto2.md
├── Enunciado/
│   └── Proyecto2.pdf
├── Kernel/
│   ├── pr2_so1_201905884.c
│   └── Makefile
└── Daemon/
    ├── main.go
    ├── proc.go
    ├── valkey.go
    ├── go.mod
    └── go.sum
```

---

## 3. Módulo Kernel

### 3.1 Entradas /proc creadas

| Entrada | Contenido |
|---|---|
| `/proc/meminfo_pr2_so1_201905884` | RAM total, libre y usada en MB |
| `/proc/continfo_pr2_so1_201905884` | Lista de procesos Docker/containerd + métricas |

### 3.2 Formato de salida

**`/proc/meminfo_pr2_so1_201905884`**
```
RAM_TOTAL_MB=7974
RAM_FREE_MB=3200
RAM_USED_MB=4774
```

**`/proc/continfo_pr2_so1_201905884`**
```
container_id=abc123def456
PID    NAME              VSZ_(KB)   RSS_(KB)   %MEM_PCT   %CPU_RAW   CONTAINER_ID
1234   dockerd           102400     51200      1          9876543    -
5678   containerd        204800     102400     2          1234567    abc123def456
CONTAINERS_ACTIVE=1
```

### 3.3 Parámetros del módulo

```bash
# Cargar sin filtro de contenedor (solo procesos generales)
sudo insmod pr2_so1_201905884.ko

# Cargar filtrando por container ID específico
sudo insmod pr2_so1_201905884.ko container_id="abc123def456"
```

### 3.4 Comandos de build y prueba

```bash
cd Kernel
make                                        # compilar
sudo insmod pr2_so1_201905884.ko            # cargar módulo
cat /proc/meminfo_pr2_so1_201905884        # verificar RAM
cat /proc/continfo_pr2_so1_201905884       # verificar procesos
dmesg | tail -5                             # ver logs del kernel
sudo rmmod pr2_so1_201905884               # descargar módulo
make clean                                  # limpiar binarios
```

### 3.5 Decisiones técnicas del módulo

| Elemento | Decisión | Razón |
|---|---|---|
| `seq_file` + `single_open` | Para exponer datos en `/proc` | API estable del kernel para archivos `/proc` read-only |
| `si_meminfo()` | Para obtener RAM | Función exportada del kernel, segura y precisa |
| `proc_ops` | En lugar de `file_operations` | Requerido en kernels >= 5.6 |
| `GFP_ATOMIC` | En kmalloc dentro del loop | No puede dormirse dentro de `rcu_read_lock` |
| `get_task_mm()` + `mmput()` | Para acceder a `task->mm` | Evita race condition si el proceso termina |
| `rcu_read_lock/unlock` | Alrededor de `for_each_process` | `list_entry_rcu` requiere protección RCU |
| `css_put()` | Después de `task_get_css()` | Libera la referencia y evita memory leak |
| `cgroup_path() == 0` | Check de retorno | En kernels 5.x+ retorna 0 en éxito, no positivo |

### 3.6 Bugs corregidos

| # | Bug | Fix |
|---|---|---|
| 1 | `css_put()` faltante → reference leak | Agregado después de usar `css` |
| 2 | `cgroup_path() > 0` nunca verdadero | Cambiado a `== 0` |
| 3 | `kmalloc(GFP_KERNEL)` dentro de RCU | Cambiado a `GFP_ATOMIC` |
| 4 | `for_each_process` sin RCU lock | Agregado `rcu_read_lock/unlock` |
| 5 | `task->mm` sin locking | Reemplazado por `get_task_mm()` + `mmput()` |
| 6 | Doble llamada a `task_in_container_by_cgroup2` | Variable `in_cgroup` reutilizada |
| 7 | Formato incorrecto (KB, texto libre) | Ahora `RAM_*_MB=` en MB |
| 8 | `containers_active` dentro del loop | Movido al inicio de la función |
| 9 | `CONTAINERS_ACTIVE` impreso antes de contar | Movido después de `rcu_read_unlock()` |
| 10 | `rcu_read_lock()` duplicado al final | Corregido a `rcu_read_unlock()` |

---

## 4. Daemon (Go)

### 4.1 Dependencias

```bash
# Instalar Go (si no está instalado)
sudo apt install golang-go       # Debian/Ubuntu
sudo pacman -S go                # Arch

# Inicializar módulo e instalar go-redis
cd Daemon
go mod init daemon_pr2
go get github.com/redis/go-redis/v9
```

### 4.2 Estructura JSON generada

```json
{
  "timestamp": "2026-02-28T12:34:56Z",
  "ram": {
    "total_mb": 7974,
    "free_mb": 3200,
    "used_mb": 4774
  },
  "containers_active": 1,
  "container_id": "abc123def456",
  "processes": [
    {
      "pid": 1234,
      "name": "dockerd",
      "vsz_kb": 102400,
      "rss_kb": 51200,
      "mem_pct": 1,
      "cpu_ticks": 9876543,
      "in_container": false
    },
    {
      "pid": 5678,
      "name": "containerd",
      "vsz_kb": 204800,
      "rss_kb": 102400,
      "mem_pct": 2,
      "cpu_ticks": 1234567,
      "in_container": true
    }
  ]
}
```

### 4.3 Structs Go

```go
type MemInfo struct {
    TotalMB uint64 `json:"total_mb"`
    FreeMB  uint64 `json:"free_mb"`
    UsedMB  uint64 `json:"used_mb"`
}

type ProcessInfo struct {
    PID         int    `json:"pid"`
    Name        string `json:"name"`
    VSZKB       uint64 `json:"vsz_kb"`
    RSSKB       uint64 `json:"rss_kb"`
    MemPct      uint64 `json:"mem_pct"`
    CPUTicks    uint64 `json:"cpu_ticks"`
    InContainer bool   `json:"in_container"`
}

type ContInfo struct {
    ContainersActive int
    ContainerID      string
    Processes        []ProcessInfo
}

type Telemetry struct {
    Timestamp        string        `json:"timestamp"`
    RAM              MemInfo       `json:"ram"`
    ContainersActive int           `json:"containers_active"`
    ContainerID      string        `json:"container_id"`
    Processes        []ProcessInfo `json:"processes"`
}
```

### 4.4 Archivos del daemon

#### `main.go` — Entry point y loop principal

```go
package main

import (
    "encoding/json"
    "fmt"
    "log"
    "os"
    "time"
)

const (
    procMeminfo  = "/proc/meminfo_pr2_so1_201905884"
    procContinfo = "/proc/continfo_pr2_so1_201905884"
    logDir       = "/var/log/proyecto2"
    logFile      = logDir + "/telemetria.log"
    intervalSec  = 5
)

type Telemetry struct {
    Timestamp        string        `json:"timestamp"`
    RAM              MemInfo       `json:"ram"`
    ContainersActive int           `json:"containers_active"`
    ContainerID      string        `json:"container_id"`
    Processes        []ProcessInfo `json:"processes"`
}

func main() {
    ensureLogDir()
    fmt.Printf("[daemon] iniciado — intervalo %ds\n", intervalSec)

    for {
        ts := time.Now().UTC().Format(time.RFC3339)

        mem, err := readMemInfo()
        if err != nil {
            log.Printf("[daemon] error leyendo meminfo: %v", err)
        }

        cont, err := readContInfo()
        if err != nil {
            log.Printf("[daemon] error leyendo continfo: %v", err)
        }

        t := Telemetry{
            Timestamp:        ts,
            RAM:              mem,
            ContainersActive: cont.ContainersActive,
            ContainerID:      cont.ContainerID,
            Processes:        cont.Processes,
        }

        data, err := json.Marshal(t)
        if err != nil {
            log.Printf("[daemon] error serializando JSON: %v", err)
            time.Sleep(intervalSec * time.Second)
            continue
        }

        writeLog(string(data))
        pushToValkey(string(data))

        fmt.Printf("[daemon] %s — RAM usada: %dMB — containers: %d\n",
            ts, mem.UsedMB, cont.ContainersActive)

        time.Sleep(intervalSec * time.Second)
    }
}

func ensureLogDir() {
    if err := os.MkdirAll(logDir, 0755); err != nil {
        log.Fatalf("[daemon] no se pudo crear %s: %v", logDir, err)
    }
}

func writeLog(jsonStr string) {
    f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        log.Printf("[daemon] no se pudo abrir log: %v", err)
        return
    }
    defer f.Close()
    fmt.Fprintln(f, jsonStr)
}
```

#### `proc.go` — Parseo de /proc

```go
package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// ── Structs ──────────────────────────────────────────────────

type MemInfo struct {
	TotalMB uint64 `json:"total_mb"`
	FreeMB  uint64 `json:"free_mb"`
	UsedMB  uint64 `json:"used_mb"`
}

type ProcessInfo struct {
	PID         int    `json:"pid"`
	Name        string `json:"name"`
	VSZKB       uint64 `json:"vsz_kb"`
	RSSKB       uint64 `json:"rss_kb"`
	MemPct      uint64 `json:"mem_pct"`
	CPUTicks    uint64 `json:"cpu_ticks"`
	InContainer bool   `json:"in_container"`
}

type ContInfo struct {
	ContainersActive int
	ContainerID      string
	Processes        []ProcessInfo
}

// ── readMemInfo ──────────────────────────────────────────────

func readMemInfo() (MemInfo, error) {
	var mem MemInfo

	f, err := os.Open(procMeminfo)
	if err != nil {
		return mem, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Cada línea es KEY=VALUE — dividir por el primer "="
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			continue
		}

		switch key {
		case "RAM_TOTAL_MB":
			mem.TotalMB = val
		case "RAM_FREE_MB":
			mem.FreeMB = val
		case "RAM_USED_MB":
			mem.UsedMB = val
		}
	}

	return mem, scanner.Err()
}

// ── readContInfo ─────────────────────────────────────────────

func readContInfo() (ContInfo, error) {
	cont := ContInfo{
		Processes: []ProcessInfo{},
	}

	f, err := os.Open(procContinfo)
	if err != nil {
		return cont, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// 1) CONTAINERS_ACTIVE=N  (al final del archivo)
		if strings.HasPrefix(line, "CONTAINERS_ACTIVE=") {
			val, err := strconv.Atoi(strings.TrimPrefix(line, "CONTAINERS_ACTIVE="))
			if err == nil {
				cont.ContainersActive = val
			}
			continue
		}

		// 2) container_id=...
		if strings.HasPrefix(line, "container_id=") {
			cont.ContainerID = strings.TrimPrefix(line, "container_id=")
			continue
		}

		// 3) Encabezado — ignorar
		if strings.HasPrefix(line, "PID") {
			continue
		}

		// 4) Línea de proceso — separada por whitespace (tabs)
		//    Formato: PID NAME VSZ RSS MEM_PCT CPU_TICKS CID
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		vsz, _      := strconv.ParseUint(fields[2], 10, 64)
		rss, _      := strconv.ParseUint(fields[3], 10, 64)
		memPct, _   := strconv.ParseUint(fields[4], 10, 64)
		cpuTicks, _ := strconv.ParseUint(fields[5], 10, 64)
		inContainer := fields[6] != "-"

		cont.Processes = append(cont.Processes, ProcessInfo{
			PID:         pid,
			Name:        fields[1],
			VSZKB:       vsz,
			RSSKB:       rss,
			MemPct:      memPct,
			CPUTicks:    cpuTicks,
			InContainer: inContainer,
		})
	}

	return cont, scanner.Err()
}
```

**Por qué funciona así:**

| Decisión | Razón |
|---|---|
| `bufio.Scanner` | Lee línea a línea sin cargar todo el archivo en memoria |
| `strings.SplitN(line, "=", 2)` | Divide solo en el primer `=`, seguro si el valor contiene `=` |
| `strings.Fields(line)` | Divide por cualquier whitespace (tabs y espacios), compatible con el TSV del kernel |
| `fields[6] != "-"` | El módulo escribe `"-"` cuando el proceso no pertenece al contenedor |
| `Processes: []ProcessInfo{}` | Inicializar slice vacío garantiza que el JSON serializa `[]` en lugar de `null` |

#### `valkey.go` — Inserción en Valkey

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
)

const (
	valkeyHost    = "127.0.0.1"
	valkeyPort    = 6379
	valkeyLatest  = "telemetria:latest"
	valkeyHistory = "telemetria:history"
	historyMax    = 1000
)

func pushToValkey(jsonStr string) {
	ctx := context.Background()

	client := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", valkeyHost, valkeyPort),
	})
	defer client.Close()

	// Verificar conexión
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("[valkey] no se pudo conectar a %s:%d — %v", valkeyHost, valkeyPort, err)
		return
	}

	// Última lectura siempre disponible en O(1)
	if err := client.Set(ctx, valkeyLatest, jsonStr, 0).Err(); err != nil {
		log.Printf("[valkey] error en SET %s: %v", valkeyLatest, err)
	}

	// Insertar al frente del historial
	if err := client.LPush(ctx, valkeyHistory, jsonStr).Err(); err != nil {
		log.Printf("[valkey] error en LPUSH %s: %v", valkeyHistory, err)
	}

	// Mantener solo las últimas historyMax entradas
	if err := client.LTrim(ctx, valkeyHistory, 0, historyMax-1).Err(); err != nil {
		log.Printf("[valkey] error en LTRIM %s: %v", valkeyHistory, err)
	}
}
```

**Por qué funciona así:**

| Decisión | Razón |
|---|---|
| `client.Ping()` antes de operar | Detecta fallo de conexión temprano y loguea sin crashear el daemon |
| `SET telemetria:latest` sin TTL (`0`) | La lectura más reciente siempre disponible hasta ser sobreescrita |
| `LPUSH` + `LTRIM 0 999` | Implementa ring buffer: inserta al frente, recorta el fondo — O(1) amortizado |
| `defer client.Close()` | Garantiza que la conexión TCP se cierra aunque algún comando falle |
| `context.Background()` | Contexto sin timeout para operaciones simples; se puede cambiar a `WithTimeout` si se necesita |

---

## 5. Valkey — Keys utilizadas

| Key | Comando | Descripción |
|---|---|---|
| `telemetria:latest` | `SET` | Última lectura siempre disponible |
| `telemetria:history` | `LPUSH` + `LTRIM 0 999` | Ring buffer de últimas 1000 lecturas |

### Comandos de verificación

```bash
# Última lectura formateada
valkey-cli GET telemetria:latest | jq .

# Últimas 3 lecturas del historial
valkey-cli LRANGE telemetria:history 0 2 | jq .

# Seguir el log en tiempo real
tail -f /var/log/proyecto2/telemetria.log | jq .
```

---

## 6. Errores comunes y solución

| Error | Causa | Solución |
|---|---|---|
| `ERROR: could not insert module: Unknown symbol` | Símbolo del kernel no exportado | Verificar que `CONFIG_MEMCG=y` en el kernel actual |
| `make: Nothing to be done` | Archivos ya compilados | `make clean && make` |
| `insmod: ERROR: could not insert module: Operation not permitted` | Secure Boot activo | Deshabilitar Secure Boot en BIOS o firmar el módulo |
| `cat: /proc/meminfo_pr2_so1_201905884: No such file or directory` | Módulo no cargado | `sudo insmod pr2_so1_201905884.ko` |
| `dial tcp 127.0.0.1:6379: connect: connection refused` | Valkey no corre | `sudo systemctl start valkey` |
| `permission denied` al escribir log | Falta `/var/log/proyecto2` | `sudo mkdir -p /var/log/proyecto2 && sudo chmod 777 /var/log/proyecto2` |

---

## 7. Comandos rápidos de referencia

```bash
# ── Kernel ──────────────────────────────────────────────
cd Kernel && make
sudo insmod pr2_so1_201905884.ko
cat /proc/meminfo_pr2_so1_201905884
cat /proc/continfo_pr2_so1_201905884
sudo rmmod pr2_so1_201905884

# ── Daemon ──────────────────────────────────────────────
cd Daemon
go build -o daemon_pr2 .
sudo ./daemon_pr2

# ── Valkey ──────────────────────────────────────────────
valkey-cli GET telemetria:latest | jq .
valkey-cli LRANGE telemetria:history 0 9 | jq .

# ── Log ─────────────────────────────────────────────────
tail -f /var/log/proyecto2/telemetria.log | jq .
```
