# Modulo de Kernel

## Descripcion General

Este modulo de kernel en Linux (LKM) se carga de forma dinamica y expone informacion por medio de entradas en `/proc`.

- Archivo 1: `/proc/meminfo_pr2_so1_201905884`
- Archivo 2: `/proc/continfo_pr2_so1_201905884`

Su objetivo es mostrar:

- Estadisticas generales de memoria RAM.
- Procesos relacionados con Docker/contenedores.

## Que es un LKM

Un Linux Kernel Module es codigo que se ejecuta en modo kernel (Ring 0). Puede cargarse y descargarse sin reiniciar el sistema.

```text
USUARIO (Ring 3)          KERNEL (Ring 0)
+----------------+        +----------------+
| Aplicaciones   |<------>| Kernel         |
| normales       |        | (LKM aqui)     |
+----------------+        +----------------+
```

## Ciclo de Vida del Modulo

### 1. Carga (`insmod`)

```bash
sudo insmod pr2_so1_201905884.ko container_id=abc123def456
```

Al cargar, se ejecuta `pr2_module_init()`:

```c
static int __init pr2_module_init(void)
{
    proc_meminfo_entry = proc_create(PROC_MEMINFO_NAME, 0444, NULL, &meminfo_fops);
    proc_continfo_entry = proc_create(PROC_CONTINFO_NAME, 0444, NULL, &continfo_ops);

    pr_info("PR2 SO1 201905884: /proc/meminfo_pr2_so1_201905884 y /proc/continfo_pr2_so1_201905884 creados\n");
    return 0;
}
```

Verificacion:

```bash
ls -la /proc/ | grep pr2
```

### 2. Ejecucion (lectura de `/proc`)

Ejemplo de lectura:

```bash
cat /proc/meminfo_pr2_so1_201905884
```

Secuencia simplificada:

1. El kernel invoca `meminfo_open()`.
2. `single_open()` asocia la funcion `meminfo_show()`.
3. `meminfo_show()` consulta memoria con `si_meminfo()` y escribe al buffer con `seq_printf()`.
4. `seq_read()` entrega el contenido al espacio de usuario.

Ejemplo de implementacion:

```c
static int meminfo_show(struct seq_file *m, void *v)
{
    struct sysinfo i;
    u64 total_kb, free_kb, used_kb;

    si_meminfo(&i);

    total_kb = ((u64)i.totalram * (u64)i.mem_unit) / 1024;
    free_kb  = ((u64)i.freeram  * (u64)i.mem_unit) / 1024;
    used_kb  = total_kb - free_kb;

    seq_printf(m, "RAM_TOTAL_MB=%llu\n", total_kb / 1024);
    seq_printf(m, "RAM_FREE_MB=%llu\n", free_kb / 1024);
    seq_printf(m, "RAM_USED_MB=%llu\n", used_kb / 1024);

    return 0;
}
```

Salida esperada:

```text
RAM_TOTAL_MB=7856
RAM_FREE_MB=4120
RAM_USED_MB=3736
```

### 3. Descarga (`rmmod`)

```bash
sudo rmmod pr2_so1_201905884
```

Al descargar, se ejecuta `pr2_module_exit()`:

```c
static void __exit pr2_module_exit(void)
{
    if (proc_continfo_entry)
        proc_remove(proc_continfo_entry);

    if (proc_meminfo_entry)
        proc_remove(proc_meminfo_entry);

    pr_info("PR2 SO1 201905884: modulo descargado\n");
}
```

## Lectura de Contenedores en `/proc/continfo_pr2_so1_201905884`

Comando:

```bash
cat /proc/continfo_pr2_so1_201905884
```

Flujo en `continfo_show()`:

1. Obtiene memoria total.
2. Recorre procesos con `for_each_process(task)`.
3. Filtra por procesos generales de Docker o por `container_id`.
4. Obtiene `VSZ`, `RSS`, porcentaje de memoria y ticks de CPU.
5. Escribe filas al buffer con `seq_printf()`.

Fragmento representativo:

```c
for_each_process(task) {
    bool include = false;

    if (is_general_process(task))
        include = true;

    if (container_id && task_in_container_by_cgroup2(task, container_id))
        include = true;

    if (!include)
        continue;

    mm = get_task_mm(task);
    if (mm) {
        vsz_kb = mm->total_vm << (PAGE_SHIFT - 10);
        rss_kb = get_mm_rss(mm) << (PAGE_SHIFT - 10);
        mmput(mm);
    }

    mem_pct = (rss_kb * 100) / mem_total_kb;

    seq_printf(m, "%d\t%s\t%llu\t%llu\t%llu\t%llu\t%s\n",
               task->pid, task->comm, vsz_kb, rss_kb,
               mem_pct, task->utime + task->stime, cid_out);
}
```

Ejemplo de salida:

```text
container_id=abc123def456
PID    NAME          VSZ_(KB)  RSS_(KB)  %MEM_PCT  %CPU_RAW  CONTAINER_ID
1234   dockerd       102400    51200     1         1000      -
5678   myapp         204800    102400    2         5000      abc123def456
9012   myapp         204800    102400    2         4500      abc123def456
CONTAINERS_ACTIVE=2
```

## Identificacion de Procesos en Contenedores

### Procesos generales de Docker

```c
static bool is_general_process(const struct task_struct *task)
{
    return (strcmp(task->comm, "dockerd") == 0) ||
           (strcmp(task->comm, "containerd") == 0) ||
           (strcmp(task->comm, "containerd-shim") == 0) ||
           (strcmp(task->comm, "runc") == 0);
}
```

### Filtro por cgroup v2

```c
static bool task_in_container_by_cgroup2(struct task_struct *task, const char *cid)
{
    css = task_get_css(task, memory_cgrp_id);
    cgroup_path(css->cgroup, path, CGROUP_PATH_MAX);

    if (strnstr(path, cid, CGROUP_PATH_MAX))
        return true;

    return false;
}
```

## Seguridad y Buenas Practicas

```c
proc_create(PROC_MEMINFO_NAME, 0444, NULL, &meminfo_fops); // solo lectura

rcu_read_lock();
for_each_process(task) { /* ... */ }
rcu_read_unlock();

path = kmalloc(CGROUP_PATH_MAX, GFP_ATOMIC);
if (!path)
    return false;

css_put(css);
mmput(mm);
kfree(path);
```

## Aclaracion: `/proc` no guarda logs

`/proc` es un sistema de archivos virtual. No persiste datos en disco; genera contenido al momento de leer.

Diferencia principal:

| Aspecto | `/proc/...` | Archivo `.log` |
|---|---|---|
| Ubicacion | Virtual (kernel/RAM) | Disco |
| Actualizacion | En cada lectura | Cuando se escribe |
| Persistencia | No | Si |

Si se desea guardar historico, se debe redirigir salida a un archivo:

```bash
while true; do
  {
    echo "=== $(date '+%Y-%m-%d %H:%M:%S') ==="
    cat /proc/continfo_pr2_so1_201905884
    echo
  } >> /tmp/continfo_pr2.log
  sleep 5
done
```

## Resumen

| Aspecto | Descripcion |
|---|---|
| Tipo | Linux Kernel Module |
| Proposito | Monitoreo de memoria y procesos de contenedores |
| Interfaz | `/proc/meminfo_pr2_so1_201905884` y `/proc/continfo_pr2_so1_201905884` |
| Carga | `sudo insmod pr2_so1_201905884.ko [container_id=...]` |
| Lectura | `cat /proc/meminfo_pr2_so1_201905884` |
| Descarga | `sudo rmmod pr2_so1_201905884` |
