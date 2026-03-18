# PR2 SO1 — Monitor de Contenedores Docker
**Carnet:** 201905884 | **Curso:** Sistemas Operativos 1 | **Semestre:** 1S2026

Sistema de monitoreo en tiempo real de contenedores Docker. Un módulo de kernel en C expone métricas de memoria y procesos via `/proc`. Un daemon en Go las lee, aplica reglas de gestión de contenedores y persiste los datos en Valkey para visualizarlos en Grafana.

---

## Requisitos

| Herramienta | Version minima | Verificar |
|---|---|---|
| Linux kernel | 5.15+ | `uname -r` |
| Go | 1.21+ | `go version` |
| Docker + Docker Compose | 24+ | `docker version` |
| make, gcc, kernel-headers | según kernel | `apt install build-essential linux-headers-$(uname -r)` |

El usuario debe pertenecer al grupo `docker`:
```bash
sudo usermod -aG docker $USER
# Cerrar sesión y volver a entrar para que tenga efecto
```

`sudo` sin contraseña para `insmod` y `rmmod`:
```bash
echo "$USER ALL=(ALL) NOPASSWD: /usr/sbin/insmod, /usr/sbin/rmmod" | sudo tee /etc/sudoers.d/daemon-so1
```

---

## Estructura del repositorio

```
201905884_LAB_SO1_1S2026_PT2/
├── README.md                          <- Este archivo (manual de usuario)
├── Daemon/
│   ├── cmd/daemon/
│   │   ├── main.go                    <- Entrada del daemon
│   │   └── .env                       <- Configuracion (variables de entorno)
│   ├── internal/
│   │   ├── app/         service.go, cronjob.go
│   │   ├── docker/      manager.go
│   │   ├── kernel/      loader.go
│   │   ├── model/       metrics.go
│   │   ├── parser/      meminfo.go, continfo.go
│   │   ├── sink/        valkey.go, jsonfile.go
│   │   └── source/      file_reader.go
│   ├── kernel/
│   │   ├── pr2_so1_201905884.c        <- Modulo de kernel
│   │   └── Makefile
│   ├── scripts/
│   │   ├── load_kernel_module.sh
│   │   ├── unload_kernel_module.sh
│   │   └── create_containers.sh
│   ├── docker/
│   │   ├── docker-compose.yml
│   │   └── grafana/provisioning/datasources/valkey.yml
│   └── doc/
│       └── Daemon.md                  <- Documentacion tecnica
└── Doc/
    └── proyecto2.md                   <- Enunciado del proyecto
```

---

## Configuracion (.env)

El archivo `.env` se encuentra en `Daemon/cmd/daemon/.env`. Contiene todas las variables que el daemon necesita al arrancar.

```env
# Rutas a los scripts del modulo de kernel
KERNEL_SCRIPT_PATH=../../scripts/load_kernel_module.sh
KERNEL_UNLOAD_SCRIPT_PATH=../../scripts/unload_kernel_module.sh

# Script del cronjob (crea 5 contenedores aleatorios cada 2 minutos)
CRON_SCRIPT_PATH=../../scripts/create_containers.sh

# Entradas /proc generadas por el modulo de kernel
FILE_READER_SERVICE_MEM_PATH=/proc/meminfo_pr2_so1_201905884
FILE_READER_SERVICE_CONT_PATH=/proc/continfo_pr2_so1_201905884

# Docker Compose
COMPOSE_FILE_PATH=../../docker/docker-compose.yml

# Grafana
GRAFANA_PORT=3000
GRAFANA_USER=admin
GRAFANA_PASSWORD=admin

# Valkey (base de datos)
VALKEY_ADDR=localhost:6379
VALKEY_KEY_MEM=meminfo
VALKEY_KEY_CONT=continfo
VALKEY_KEY_PROC=procinfo
VALKEY_KEY_RSS_RANK=rss_rank
VALKEY_KEY_CPU_RANK=cpu_rank
VALKEY_KEY_CONTAINERS=containers
```

> Las rutas de scripts son relativas al directorio de trabajo del daemon (`Daemon/cmd/daemon/`).

---

## Instalacion y ejecucion

### Opcion A — Ejecucion directa (desarrollo)

**1. Compilar el daemon:**
```bash
cd ~/Julian/201905884_LAB_SO1_1S2026_PT2/Daemon
go build -o daemon_so1 ./cmd/daemon
```

**2. Ejecutar desde el directorio correcto:**
```bash
cd cmd/daemon
../../daemon_so1
```

O con `go run` directamente:
```bash
cd Daemon/cmd/daemon
go run main.go
```

Salida esperada al arrancar:
```
2026/03/17 10:00:00 main: archivo .env cargado exitosamente
2026/03/17 10:00:00 main: cargando modulo de Kernel ...
2026/03/17 10:00:01 kernel: [kernel-loader] compilando para kernel 6.17.9...
2026/03/17 10:00:05 kernel: [kernel-loader] modulo cargado OK
2026/03/17 10:00:05 main: modulo de Kernel cargado exitosamente
2026/03/17 10:00:05 main: levantando contenedores de Grafana y Valkey ...
2026/03/17 10:00:08 main: contenedores de Grafana y Valkey levantados exitosamente
2026/03/17 10:00:08 main: cronjob registrado (cada 2 minutos)
2026/03/17 10:00:08 main: daemon iniciado
2026/03/17 10:00:08 service: proxima ejecucion en 35s
```

---

### Opcion B — Servicio systemd (produccion)

**1. Compilar el binario:**
```bash
cd ~/Julian/201905884_LAB_SO1_1S2026_PT2/Daemon
go build -o daemon_so1 ./cmd/daemon
```

**2. Crear el unit file:**
```bash
sudo nano /etc/systemd/system/daemon-so1.service
```

Contenido:
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

**3. Registrar y arrancar:**
```bash
sudo systemctl daemon-reload
sudo systemctl start daemon-so1
```

**4. Ver estado:**
```bash
sudo systemctl status daemon-so1
```

---

## Comandos de operacion

### Iniciar y detener

```bash
# Con systemd
sudo systemctl start daemon-so1
sudo systemctl stop daemon-so1

# Ver logs en tiempo real
sudo journalctl -u daemon-so1 -f

# Desde terminal (Ctrl+C para detener)
cd Daemon/cmd/daemon && go run main.go
```

### Verificar el modulo de kernel

```bash
# Ver si el modulo esta cargado
lsmod | grep pr2_so1_201905884

# Ver entradas /proc creadas
ls /proc/ | grep pr2

# Leer datos de memoria en crudo
cat /proc/meminfo_pr2_so1_201905884

# Leer datos de contenedores en crudo
cat /proc/continfo_pr2_so1_201905884

# Descargar el modulo manualmente
sudo rmmod pr2_so1_201905884
```

### Verificar datos en Valkey

```bash
# Entrar al CLI de Valkey
docker exec -it valkey_so1 valkey-cli

# Dentro del CLI:
KEYS *                          # ver todas las keys
LLEN meminfo                    # cuantas lecturas de RAM hay
LLEN continfo                   # cuantas lecturas de contenedores hay
LINDEX meminfo -1               # ultima lectura de RAM
LINDEX continfo -1              # ultima lectura de contenedores
LRANGE procinfo 0 4             # ultimos 5 registros de procesos
ZRANGEBYSCORE rss_rank -inf +inf WITHSCORES   # ranking de RAM
ZRANGEBYSCORE cpu_rank -inf +inf WITHSCORES   # ranking de CPU
HGETALL containers              # estado actual de todos los contenedores
```

### Contenedores Docker

```bash
# Ver contenedores activos
docker ps

# Ver estado de Grafana y Valkey
docker ps | grep -E "grafana|valkey"

# Ver logs de Grafana
docker logs grafana_so1

# Detener la infraestructura manualmente
cd Daemon/docker && docker compose down
```

---

## Acceso a Grafana

1. Abrir el navegador en `http://localhost:3000`
2. Usuario: `admin` | Contrasena: `admin`
3. El datasource **Valkey** ya esta configurado automaticamente
4. Ir a **Explore** y usar las siguientes queries:

| Query | Que muestra |
|---|---|
| `LRANGE meminfo 0 -1` | Historial completo de RAM |
| `LRANGE continfo 0 -1` | Historial de estado de contenedores |
| `LRANGE procinfo 0 49` | Ultimos 50 registros de procesos |
| `ZRANGEBYSCORE rss_rank -inf +inf WITHSCORES` | Ranking por memoria |
| `ZRANGEBYSCORE cpu_rank -inf +inf WITHSCORES` | Ranking por CPU |
| `HGETALL containers` | Estado actual de contenedores |

---

## Logica de gestion de contenedores

El daemon mantiene en todo momento:
- **3 contenedores de bajo consumo** (`alpine sleep 240`)
- **2 contenedores de alto consumo** (`roldyoran/go-client` o `alpine` con stress de CPU)

Si hay exceso:
- Bajo consumo: elimina los que **mas recursos consumen** (los anomalos)
- Alto consumo: elimina los que **menos recursos consumen** (mantiene los mas activos)

Si hay deficit, crea nuevos automaticamente.

El **cronjob** agrega 5 contenedores aleatorios cada 2 minutos para simular carga. Al detener el daemon, el cronjob se elimina automaticamente.

---

## Detener el sistema

```bash
# Con systemd — apagado limpio
sudo systemctl stop daemon-so1

# Desde terminal — Ctrl+C o:
kill $(pgrep daemon_so1)
```

Al detenerse el daemon:
1. Cancela el loop principal
2. Descarga el modulo de kernel (`rmmod`)
3. Elimina el cronjob del sistema

Los contenedores de Grafana y Valkey **siguen corriendo** tras detener el daemon (tienen `restart: unless-stopped`). Para detenerlos:
```bash
cd Daemon/docker && docker compose down
```

---

## Solucion de problemas

| Error | Causa probable | Solucion |
|---|---|---|
| `kernel: script no encontrado` | Ruta incorrecta en `.env` o se ejecuta desde directorio equivocado | Ejecutar desde `Daemon/cmd/daemon/` |
| `main: error al cargar el modulo` | `insmod` fallo | Ver `dmesg \| tail -20` para el error del kernel |
| `/proc/meminfo_... no existe` | El modulo cargo pero `__init` fallo | `dmesg \| tail -20` |
| `error al levantar contenedores` | Docker no esta corriendo o usuario no tiene permisos | `sudo systemctl start docker` y verificar grupo `docker` |
| `valkey: error escribiendo` | Valkey no esta corriendo | `docker ps \| grep valkey` y verificar que levanto |
| `status=203/EXEC` en systemd | El binario no existe o ruta incorrecta en unit file | Compilar primero: `go build -o daemon_so1 ./cmd/daemon` |
| El daemon no responde a `Ctrl+C` | Problema con captura de senales | Usar `kill -SIGTERM $(pgrep daemon_so1)` |

---

## Flujo completo del sistema

```
Kernel Module (.ko)
  /proc/meminfo_pr2_so1_201905884   <- RAM total/libre/usada en KB
  /proc/continfo_pr2_so1_201905884  <- Procesos Docker con PID/VSZ/RSS/CPU/cmdline
           |
           | os.ReadFile() cada 20-60s (aleatorio)
           v
Daemon Go
  parser   -> convierte JSON crudo a structs tipados
  docker   -> aplica invariantes (3 low + 2 high)
  sink     -> escribe en Valkey (LIST, ZSET, HASH)
           |
           | protocolo Redis (go-redis)
           v
Valkey :6379
  LIST  meminfo     <- historial cronologico de RAM
  LIST  continfo    <- historial de estado de contenedores
  LIST  procinfo    <- historial por contenedor individual
  ZSET  rss_rank    <- ranking por % de RAM (sin duplicados)
  ZSET  cpu_rank    <- ranking por % de CPU (sin duplicados)
  HASH  containers  <- estado actual de cada contenedor
           |
           | redis-datasource plugin
           v
Grafana :3000
  Dashboard con graficas en tiempo real
```
