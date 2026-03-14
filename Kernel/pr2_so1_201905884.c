// SPDX-License-Identifier: GPL-2.0
/*
 * PR2 SO1 - 201905884

 * Basado en la documentacion oficial de Linux y el libro 
 * The Linux Kernel Module Programming Guide.
 * 
 * PROC: es un sistema de archivos virtual del kernel de Linux que 
 * proporciona una interfaz para acceder a información del sistema
 * y procesos en tiempo real.

 * Se creo el PROC:
 *   /proc/meminfo_pr2_so1_201905884   -> Total/Free/Used RAM (KB)
 *   /proc/continfo_pr2_so1_201905884  -> Lista procesos con VSZ/RSS/%MEM/%CPU + cmdline

 * Nota:
 * - El %CPU aquí se deja como "ticks" acumulados (puede ser grande).
 * - Para identificar contenedores Docker "del script", 
 * lo usual es filtrar por un marcador en cmdline.
 */

/* 
 ? linux/module.h 
 * Es necesario para crear un módulo de kernel en Linux. 

 ? linux/kernel.h 
 * Se incluye para acceder a las funciones y macros del kernel de Linux.

 ? linux/init.h 
 * Se utiliza para definir las funciones de inicialización y limpieza del módulo. 
*/
#include <linux/module.h>
#include <linux/kernel.h>
#include <linux/init.h>

/*
 ? linux/proc_fs.h 
 * Se incluye para trabajar con el sistema de archivos proc, 
 * que permite crear entradas virtuales para mostrar información del sistema.
 
 ? linux/seq_file.h 
 * Se utiliza para facilitar la creación de archivos en proc 
 * que pueden mostrar información secuencial, como listas de procesos 
 * o estadísticas del sistema. 
*/
#include <linux/proc_fs.h>
#include <linux/seq_file.h>

/*
 ? linux/mmzone.h 
 * Se incluye para acceder a la función si_meminfo, 
 * que proporciona información sobre la memoria del sistema.

 ? El linux/mm.h 
 * Se utiliza para trabajar con la memoria de los procesos, 
 * incluyendo funciones como get_mm_rss y estructuras como mm_struct.
 
 ? linux/sched/signal.h 
 * Se incluye para iterar sobre los procesos en el sistema utilizando for_each_process.
 
 ? El linux/string.h 
 * Se utiliza para funciones de manipulación de cadenas, como strscpy, 
 * que se utiliza para copiar cadenas de manera segura.
*/
#include <linux/mmzone.h>        // si_meminfo
#include <linux/mm.h>            // get_mm_rss, mm_struct
#include <linux/sched/signal.h>  // for_each_process
#include <linux/string.h>        // strscpy

/*
 ? linux/cgroup.h 
 * Se utiliza para trabajar con cgroups, aunque en este módulo no se utiliza directamente, 
 * pero se incluye por si se necesita acceder a información relacionada con cgroups en el futuro.

 ? linux/slab.h 
 * Se incluye para utilizar las funciones de asignación de memoria dinámica del kernel, 
 * como kmalloc y kfree, que se utilizan para gestionar la memoria del buffer de 
 * cmdline en la función continfo_show. 

 ? linux/memcontrol.h 
 * Se incluye para acceder a la función task_get_css, que se utiliza para obtener el estado
 * del subsistema de cgroup de un proceso, aunque esta función puede no estar disponible 
 * en todos los kernels dependiendo de la configuración.
  
 ? linux/jiffies.h
 * Se incluye para acceder a la variable jiffies, que se utiliza para calcular el tiempo 
 * de CPU acumulado por un proceso en la función continfo_show.

 ? linux/mmap_lock.h
 * Se incluye para acceder a la función mmap_read_lock, que se utiliza para proteger el acceso
 * a la memoria de un proceso mientras se obtiene su información de memoria en la función continfo_show.
 */
#include <linux/cgroup.h>       // cgroup_path
#ifdef CONFIG_MEMCG
#include <linux/memcontrol.h>   // memory_cgrp_subsys_id
#endif
#include <linux/slab.h>         // kmalloc, kfree
#include <linux/jiffies.h>     // jiffies
#include <linux/mmap_lock.h>      // mmap_lock
/* 
 * Definiciones de constantes para los nombres de las entradas en /proc, 
 * utilizando el número de carnet para personalizarlo.
 */
#define CARNET "201905884"
#define PROC_MEMINFO_NAME "meminfo_pr2_so1_" CARNET
#define PROC_CONTINFO_NAME "continfo_pr2_so1_" CARNET

/* 
 * La cmdline muestra la línea de comandos con la que se ejecutó cada proceso,
 * esta para identificar procesos específicos, especialmente en un entorno 
 * con contenedores Docker.
 
 * CMDLINE se define como 256 para establecer la longitud del buffer.
*/
#define CMDLINE 256

/*
 * El container_id muesta el identidicador del contenedor que esta
 * siendo ejecutado.
*/
#define CONTAINER_ID 64

/* 
 * Declaración de punteros a las entradas del sistema de archivos proc 
 * para meminfo y continfo, que se utilizarán para crear y gestionar 
 * estas entradas en el módulo del kernel.
*/
static struct proc_dir_entry *proc_meminfo_entry;
static struct proc_dir_entry *proc_continfo_entry;

/*
 * Parámetro: container_id=<CID>
 * Se pueden pasar 12 chars o el CID completo.
 
static char *container_id;
module_param(container_id, charp, 0444);
MODULE_PARM_DESC(container_id, "Docker container ID substring used to match processes via cgroup v2 path");
*/

/**
 * Función para determinar si un proceso es un proceso "general" relacionado con Docker, 
 * como el demonio de Docker o los procesos de contenedores.
 * 
 * @param task: Puntero a la estructura task_struct del proceso que se desea evaluar.
 * @return: Devuelve true si el proceso es un proceso "general" relacionado con Docker 
 * y false en caso contrario.
 * 
 * Esta función compara el nombre del proceso (task->comm) con una lista de nombres 
 * comunes de procesos relacionados con Docker, como "dockerd", "containerd", 
 * "containerd-shim", "containerd-shim-runc-v2" y "runc".
 */
static bool is_general_process(const struct task_struct *task)
{
	/*
	 * Procesos "generales".
	 * Estos suelen existir en hosts con Docker.
	 */
	return (strcmp(task->comm, "dockerd") == 0) ||
	       (strcmp(task->comm, "containerd") == 0) ||
	       (strcmp(task->comm, "containerd-shim") == 0) ||
	       (strcmp(task->comm, "containerd-shim-runc-v2") == 0) ||
	       (strcmp(task->comm, "runc") == 0);
}

/**
 * Función para mostrar la información de memoria en la entrada /proc/meminfo_pr2_so1_201905884.
 * 
 * @param m: Puntero a la estructura seq_file utilizada para escribir la salida de la función.
 * @param v: Puntero a un valor que se utiliza para iterar sobre la información 
 * secuencial, no se utiliza en esta función.
 * 
 * Esta función obtiene la información de memoria del sistema utilizando si_meminfo,
 * calcula la memoria total, libre y usada en KB, y luego muestra esta información 
 * formateada en la salida del archivo proc.
 * 
 * Se asegura de que la memoria usada no sea negativa y formatea la salida para que sea fácil de leer.
 */
static int meminfo_show(struct seq_file *m, void *v)
{
    struct sysinfo i; // Estructura para almacenar la información del sistema
    unsigned long total_ram, 
                  free_ram, 
                  used_ram; // Variables para almacenar la memoria total, libre y usada en KB

    char buf[256]; // Buffer para almacenar la salida formateada de la información de memoria

    si_meminfo(&i); // Obtener la información de memoria del sistema

    total_ram = (i.totalram * i.mem_unit) / 1024; // Calcular la memoria total en KB
    free_ram = (i.freeram * i.mem_unit) / 1024; // Calcular la memoria libre en KB
    used_ram = (total_ram >= free_ram) ? (total_ram - free_ram) : 0; // Calcular la memoria usada en KB, asegurando que no sea negativa

    //Crear Json con la información de memoria formateada
    snprintf(buf, sizeof(buf),
            "{\n"
                "memory_info: {\n"
                "  \"total_ram_kb\": %lu,\n"
                "  \"free_ram_kb\": %lu,\n"
                "  \"used_ram_kb\": %lu\n"
            "   }\n"
            "}\n",
            total_ram, free_ram, used_ram);
    
    seq_printf(m, "%s", buf); // Imprimir el buffer formateado en la salida del archivo proc

    return 0; // Indicar que la función se ejecutó correctamente
}

/**
 * Función para manejar la apertura del archivo proc para meminfo.
 * 
 * @param inode: Puntero a la estructura inode del archivo proc que se está abriendo.
 * @param file: Puntero a la estructura file que representa el archivo proc que se está abriendo.
 * 
 * Esta función utiliza single_open para asociar la función meminfo_show con el archivo proc, 
 * lo que permite que cada vez que se abra el archivo proc, se ejecute meminfo_show para 
 * mostrar la información de memoria actualizada. 
 * 
 * El tercer parámetro de single_open se deja como NULL ya que no 
 * se necesita pasar datos adicionales a meminfo_show.
 */
static int meminfo_open(struct inode *inode, struct file *file)
{
    // Abrir el archivo proc utilizando single_open y asociar la función de mostrar
    return single_open(file, meminfo_show, NULL);
}

/**
 * Estructura proc_ops para la entrada /proc/meminfo_pr2_so1_201905884, que define las operaciones 
 * que se pueden realizar en esta entrada del sistema de archivos proc.
 */
static const struct proc_ops meminfo_fops = {
    .proc_open = meminfo_open, // Función para manejar la apertura del archivo proc
    .proc_read = seq_read, // Función para manejar la lectura del archivo proc utilizando seq_read
    .proc_lseek = seq_lseek, // Función para manejar el desplazamiento en el archivo proc utilizando seq_lseek
    .proc_release = single_release, // Función para manejar la liberación del archivo proc utilizando single_release
};

/**
 * @brief Get the process cmdline object    
 * 
 * @param task 
 * @return char* 
 */
static char *get_process_cmdline(struct task_struct *task) {
    struct mm_struct *mm = get_task_mm(task); // Obtener la estructura de memoria del proceso
    char *cmdline;
    unsigned long   arg_start = 0, 
                    arg_end = 0,
                    arg_len = 0;

    if (!mm)
        return NULL; // Si no se puede obtener la estructura de memoria, retornar NULL

    cmdline = kmalloc(CMDLINE, GFP_KERNEL); // Asignar memoria para el buffer de cmdline
    if (!cmdline) {
        mmput(mm); // Liberar la estructura de memoria si no se pudo asignar el buffer
        return NULL; // Retornar NULL si no se pudo asignar memoria
    }

    mmap_read_lock(mm); // Bloquear la memoria del proceso para lectura
    arg_start = mm->arg_start; // Obtener el inicio de los argumentos del proceso
    arg_end = mm->arg_end; // Obtener el final de los argumentos del proceso
    mmap_read_unlock(mm); // Desbloquear la memoria del proceso

    // Valida si arg_start y arg_end son válidos y si arg_end es mayor que arg_start para evitar lecturas inválidas
    if(arg_start && arg_end > arg_start) {
        int bytes_read;
        int k;

        arg_len = arg_end - arg_start;
        if(arg_len > CMDLINE - 1){
            arg_len = CMDLINE - 1; // Limitar la longitud de cmdline para evitar desbordamientos
        }

        bytes_read = access_process_vm(task, arg_start, cmdline, arg_len, 0); // Leer la línea de comandos del proceso

        for(k = 0; k < bytes_read - 1; k++) {
            if(cmdline[k] == '\0') {
                cmdline[k] = ' '; // Reemplazar los caracteres nulos por espacios para mejorar la legibilidad
            }
        }

        cmdline[bytes_read] = '\0'; // Asegurar que el buffer de cmdline esté terminado en nulo
    }

    mmput(mm); // Liberar la estructura de memoria del proceso
    return cmdline; // Retornar el buffer de cmdline con la línea de comandos del proceso
}

/**
 * @brief Get the container id object
 * 
 * @param task 
 * @return char* 
 */
#ifdef CONFIG_MEMCG
static char *get_container_id(struct task_struct *task) {
    struct cgroup_subsys_state *css;
    char *container_id, *p;

    container_id = kmalloc(CONTAINER_ID, GFP_KERNEL);
    if (!container_id)
        return NULL;
    container_id[0] = '\0';
    css = task_get_css(task, memory_cgrp_id);
    if (css) {
        cgroup_path(css->cgroup, container_id, CONTAINER_ID);
        css_put(css); // Liberar la referencia obtenida por task_get_css
    }

    // Extraer el container_id de la ruta del cgroup, asumiendo que el container_id es el último componente de la ruta
    p = strstr(container_id, "docker-"); // Buscar el prefijo "docker-" en la ruta del cgroup
    if (p){
        p += strlen("docker-"); // Mover el puntero para apuntar al inicio del container_id después del prefijo
        strscpy(container_id, p, 13); // Copiar el container_id al buffer de container_id
        container_id[12] = '\0'; // Asegurar que el buffer de container_id esté terminado en nulo
    }

    return container_id;
}
#endif /* CONFIG_MEMCG */

/**
 * Función para mostrar la información de los procesos en la entrada /proc/continfo_pr2_so1_201905884.
 * 
 * @param m: Puntero a la estructura seq_file utilizada para escribir la salida de la función.
 * @param v: Puntero a un valor que se utiliza para iterar sobre la información secuencial, no se utiliza en esta función.
 * 
 * Esta función itera sobre todos los procesos en el sistema utilizando for_each_process,
 * y para cada proceso, verifica si es un proceso "general" relacionado con Docker o 
 * si coincide con el container_id especificado en su cgroup v2 path.
 * 
 * Si el proceso cumple, se obtiene su información de memoria (VSZ y RSS), se calcula el 
 * porcentaje de memoria utilizada en relación con la memoria total del sistema, 
 * y se muestra esta información formateada en la salida del archivo proc.
 * 
 * La salida incluye el PID, el nombre del proceso, la memoria virtual (VSZ), 
 * la memoria residente (RSS), el porcentaje de memoria utilizada, 
 * el tiempo de CPU acumulado (utime + stime) y el container_id si corresponde.
 * 
 * Se agrega el cmdline para identificar procesos específicos, relacionados con
 * entorno con contenedores Docker, se cuenta el número de procesos activos 
 * en el contenedor para mostrarlo al final de la salida.
 */
static int continfo_show(struct seq_file *m, void *v)
{
    // Variables sobre procesos y memoria
    struct task_struct *task; // Puntero para iterar sobre los procesos
    struct sysinfo si; // Estructura para almacenar la información del sistema
    unsigned long mem_total_ram; // Variable para almacenar la memoria total del sistema en KB
    int containers_active = 0; // Contador para el número de procesos activos en el contenedor
    char json_str [];
    char buf[512];

    si_meminfo(&si); // Obtener la información de memoria del sistema
    mem_total_ram = (si.totalram * si.mem_unit) / 1024; //

    if (!mem_total_ram) {
        mem_total_ram = 1; // Evitar división por cero en caso de que la memoria total sea cero
    }

    // Imprimir el encabezado del JSON para la lista de procesos
    snprintf(buf, sizeof(buf),
            "{\n"
                "\"system_process_metrics\": {\n"
                "   \"total_ram_kb\": %lu,\n"
                "   \"free_ram_kb\": %lu,\n"
                "   \"used_ram_kb\": %lu\n"
                " },\n",
            mem_total_ram, 
            si.freeram << (PAGE_SHIFT - 10), 
            (mem_total_ram - si.freeram * si.mem_unit / 1024)
        );

    // Agregar a json_str para imprimir el encabezado del JSON para la lista de procesos
    json_str [0] = '\0';
    strncat(json_str, buf, sizeof(buf) - 1);

    /*
    * Se llama a rcu_read_lock() para proteger la sección de código que accede a la lista de procesos, 
    * ya que for_each_process puede acceder a estructuras de datos que pueden ser modificadas por otros hilos o procesos. 
    * Esto asegura que la información de los procesos sea consistente durante la iteración.
    */
    rcu_read_lock();
    
    /*
    * Iterar sobre todos los procesos en el sistema utilizando for_each_process, 
    * que es una macro que permite recorrer la lista de procesos.
    */
    for_each_process(task) {
        bool include = is_general_process(task); // Variable para determinar si se debe incluir el proceso en la salida
        unsigned long total_jiffies = jiffies; // Variable para almacenar el tiempo de CPU acumulado por el proceso
        char *cmdline = NULL; // Buffer para almacenar la línea de comandos del proceso
        char *container_id = NULL; // Buffer para almacenar el container_id del proceso

        if(!include) {
            continue; // Si el proceso no se debe incluir, pasar al siguiente proceso
        }

        // * Metricas de memoria del proceso
        {
            // Variables procesos relacionados con  procesos system, se restringe a solo procesos de Docker
            /*
             * pid: Identificador el proceso
             * name: nombre del proceso
             * cmdline: línea de comandos con la que se ejecutó el proceso
             * vsz: tamaño de la memoria virtual del proceso en KB
             * rss: tamaño de la memoria residente del proceso en KB
             * mem_perc: porcentaje de memoria utilizada por el proceso en relación con la memoria total del sistema
             * cpu_ticks: tiempo de CPU acumulado por el proceso en jiffies (utime + stime)
             * container_id: identificador del contenedor al que pertenece el proceso, si corresponde
            */

            unsigned long   vsz = 0, 
                            rss = 0,
                            cpu_usage = 0;
            unsigned long mem_perc = 0;
            struct mm_struct *mm = get_task_mm(task); // Obtener la estructura de memoria del proceso
            
            if (mm) {
                vsz = mm->total_vm << (PAGE_SHIFT - 10); // Calcular el tamaño de la memoria virtual en KB
                rss = get_mm_rss(mm) << (PAGE_SHIFT - 10); // Calcular el tamaño de la memoria residente en KB
                mem_perc = (mem_total_ram > 0) ? (rss * 10000) / mem_total_ram : 0; // Calcular el porcentaje de memoria utilizada por el proceso
                mmput(mm); // Liberar la estructura de memoria del proceso
            }

            // Calcular el tiempo de CPU acumulado por el proceso en jiffies (utime + stime)
            unsigned long total_time = task->utime + task->stime;
            cpu_usage = (total_time > 0) ? (total_time * 10000) / total_jiffies : 0;

            // Asinacion del valor cmdline
            cmdline = get_process_cmdline(task);

            // Asignacion del container_id
#ifdef CONFIG_MEMCG
            container_id = get_container_id(task);
#endif
            
            // Impresion del Body del JSON con la información del proceso formateada
            snprintf(buf, sizeof(buf),
                    "\n \"system_process_docker\": {\n"
                        "  \"PID\": %d,\n"
                        "  \"Name\": \"%s\",\n"
                        "  \"Cmdline\": \"%s\",\n"
                        "  \"Vsz\": %lu,\n"
                        "  \"Rss\": %lu,\n"
                        "  \"MemPerc\": %lu,\n"
                        "  \"CpuUsage\": %lu,\n"
                        "  \"ContainerId\": \"%s\"\n"
                    "     },\n",
                    task->pid,
                    task->comm,
                    cmdline ? cmdline : "",
                    vsz,
                    rss,
                    mem_perc,
                    cpu_usage,
                    container_id ? container_id : "-"
                );
            
            // Agregrar al str el Body del JSON con la información del proceso formateada
            strlcat(json_str, buf, sizeof(json_str)); // Agregar el buffer formateado al json_str
            seq_printf(m, "%s", buf); // Imprimir la información del proceso en la salida del archivo proc

            kfree(cmdline);
            kfree(container_id);
            containers_active++;
		}
    }

    /*
    * Se llama a rcu_read_unlock() para liberar el bloqueo de lectura de RCU después de haber terminado de acceder a la lista de procesos.
    * Esto permite que otros hilos o procesos puedan modificar la lista de procesos nuevamente.
    */
    rcu_read_unlock();

    // Imprimir Pie del JSON con la información de contenedores activos
    snprintf(buf, sizeof(buf),
        "\n \"docker_process_Active\": {\n"
            "  \"dockers_processes\": %d\n"
        "   }\n"
        "}\n",
        containers_active
    );

    // Agregar al json_str el Pie del JSON con la información de contenedores activos
    strlcat(json_str, buf, sizeof(json_str)); // Agregar el buffer formateado al json_str

    seq_printf(m, "%s", buf); 

    return 0; // Indicar que la función se ejecutó correctamente
}

/**
 * Función para manejar la apertura del archivo proc para continfo.
 * 
 * @param inode: Puntero a la estructura inode del archivo proc que se está abriendo.
 * @param file: Puntero a la estructura file que representa el archivo proc que se está abriendo.
 * 
 * Esta función utiliza single_open para asociar la función continfo_show con el archivo proc, 
 * lo que permite que cada vez que se abra el archivo proc, se ejecute continfo_show para 
 * mostrar la información de los procesos actualizada. 
 * 
 * El tercer parámetro de single_open se deja como NULL ya que no 
 * se necesita pasar datos adicionales a continfo_show.
 */
static int continfo_open(struct inode *inode, struct file *file)
{
	return single_open(file, continfo_show, NULL);
}

/**
 * Estructura proc_ops para la entrada /proc/continfo_pr2_so1_201905884, que define las operaciones
 * que se pueden realizar en esta entrada del sistema de archivos proc. 
 */
static const struct proc_ops continfo_ops = {
	.proc_open    = continfo_open, // Función para manejar la apertura del archivo proc
	.proc_read    = seq_read, // Función para manejar la lectura del archivo proc utilizando seq_read
	.proc_lseek   = seq_lseek, // Función para manejar el desplazamiento en el archivo proc utilizando seq_lseek
	.proc_release = single_release, // Función para manejar la liberación del archivo proc utilizando single_release
};

/**
 * Función de inicialización del módulo, que se ejecuta cuando el módulo es cargado en el kernel.
 * 
 * Esta función crea las entradas en el sistema de archivos proc para meminfo y continfo 
 * utilizando proc_create, y luego imprime un mensaje de información en el log del kernel 
 * indicando que las entradas han sido creadas exitosamente.
 * 
 * Si la creación de alguna de las entradas falla, se limpian las entradas creadas previamente 
 * y se devuelve un error de memoria (-ENOMEM).
 * 
 * Además, se verifica si el parámetro container_id ha sido especificado y se imprime 
 * una advertencia si no lo ha sido, indicando que solo se listarán procesos generales. 
 * 
 * También se verifica si la configuración CONFIG_MEMCG está habilitada y se imprime 
 * una advertencia si no lo está, indicando que no se podrán filtrar procesos por cgroup (solo generales).   
 */
static int __init pr2_module_init(void)
{
	proc_meminfo_entry = proc_create(PROC_MEMINFO_NAME, 0444, NULL, &meminfo_fops);
	if (!proc_meminfo_entry)
		return -ENOMEM;

	proc_continfo_entry = proc_create(PROC_CONTINFO_NAME, 0444, NULL, &continfo_ops);
	if (!proc_continfo_entry) {
		proc_remove(proc_meminfo_entry);
		return -ENOMEM;
	}

	pr_info("PR2 SO1 %s: /proc/%s y /proc/%s creados\n",
		CARNET, PROC_MEMINFO_NAME, PROC_CONTINFO_NAME);

    /*
	if (!container_id || !*container_id)
		pr_warn("PR2 SO1 %s: container_id no especificado; solo se listaran procesos generales\n", CARNET);
    */

#ifndef CONFIG_MEMCG
	pr_warn("PR2 SO1 %s: CONFIG_MEMCG deshabilitado; no se podran filtrar procesos por cgroup (solo generales)\n", CARNET);
#endif

	return 0;
}

/**
 * Función de limpieza del módulo, que se ejecuta cuando el módulo es descargado del kernel.
 * 
 * Esta función elimina las entradas en el sistema de archivos proc para meminfo y continfo si
 * existen, y luego imprime un mensaje de información en el log del kernel indicando
 * que el módulo ha sido descargado. 
 *   
 * Si las entradas existen, se eliminan utilizando proc_remove, que es una función del kernel
 * para eliminar entradas del sistema de archivos proc.
 * 
 * Después de eliminar las entradas, se imprime un mensaje de información utilizando pr_info 
 * para indicar que el módulo ha sido descargado exitosamente.   
 * 
 * El mensaje incluye el número de carnet para identificar el módulo específico que ha sido descargado.
 */
static void __exit pr2_module_exit(void)
{
	if (proc_continfo_entry)
		proc_remove(proc_continfo_entry);
	if (proc_meminfo_entry)
		proc_remove(proc_meminfo_entry);

	pr_info("PR2 SO1 %s: modulo descargado\n", CARNET);
}

/*
 * Las macros module_init y module_exit se utilizan para registrar 
 * las funciones de inicialización y limpieza del módulo.

 * - module_init(pr2_module_init): Registra la función pr2_module_init como la
 * función de inicialización del módulo, que se ejecutará cuando el módulo 
 * sea cargado en el kernel.
 
 * - module_exit(pr2_module_exit): Registra la función pr2_module_exit como la
 *  función de limpieza del módulo, que se ejecutará cuando el módulo 
 * sea descargado "Eliminado" del kernel.
 * 
 */
module_init(pr2_module_init);
module_exit(pr2_module_exit);

/*
 * Las macros MODULE_LICENSE, MODULE_AUTHOR y MODULE_DESCRIPTION información sobre el módulo.

 * - MODULE_LICENSE("GPL"): Indica que el módulo está licenciado bajo la Licencia 
 * Pública General de GNU (GPL), lo que permite su uso y distribución bajo los términos de esta licencia.

 * - MODULE_AUTHOR("201905884"): Proporciona el nombre del autor del módulo.

 * - MODULE_DESCRIPTION("Modulo PR2 SO1: meminfo + continfo en /proc"): Proporciona una descripción breve del módulo.
 */
MODULE_LICENSE("GPL");
MODULE_AUTHOR("201905884");
MODULE_DESCRIPTION("Modulo PR2 SO1: meminfo + continfo en /proc");