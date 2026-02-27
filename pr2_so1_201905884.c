// SPDX-License-Identifier: GPL-2.0
/*
 * PR2 SO1 - 201905884
 *
 * Basado en la documentacion oficial de Linux y el libro The Linux Kernel Module Programming Guide.
 * 
 * PROC: es un sistema de archivos virtual del kernel de Linux que proporciona una interfaz 
 * para acceder a información del sistema y procesos en tiempo real.
 * 
 * Se creo el PROC:
 *   /proc/meminfo_pr2_so1_201905884   -> Total/Free/Used RAM (KB)
 *   /proc/continfo_pr2_so1_201905884  -> Lista procesos con VSZ/RSS/%MEM/%CPU + cmdline
 *
 * Nota:
 * - El %CPU aquí se deja como "ticks" acumulados (puede ser grande).
 * - Para identificar contenedores Docker "del script", lo usual es filtrar por un marcador en cmdline.
 */

/* 
 * El linux/module.h es necesario para crear un módulo de kernel en Linux. 
 * El linux/kernel.h se incluye para acceder a las funciones y macros del kernel de Linux.
 * El linux/init.h se utiliza para definir las funciones de inicialización y limpieza del módulo. 
*/
#include <linux/module.h>
#include <linux/kernel.h>
#include <linux/init.h>

/*
 * El linux/proc_fs.h se incluye para trabajar con el sistema de archivos proc, que permite crear entradas virtuales 
 *                    para mostrar información del sistema.
 * El linux/seq_file.h se utiliza para facilitar la creación de archivos en proc que pueden mostrar información secuencial, como listas de procesos o estadísticas del sistema. 
*/
#include <linux/proc_fs.h>
#include <linux/seq_file.h>

/*
 * El linux/mmzone.h se incluye para acceder a la función si_meminfo, que proporciona información sobre la memoria del sistema.
 * El linux/mm.h se utiliza para trabajar con la memoria de los procesos, incluyendo funciones como get_mm_rss y 
 *               estructuras como mm_struct.
 * El linux/sched/signal.h se incluye para iterar sobre los procesos en el sistema utilizando for_each_process.
 * El linux/string.h se utiliza para funciones de manipulación de cadenas, como strscpy, que se utiliza para copiar cadenas de manera segura.
*/
#include <linux/mmzone.h>        // si_meminfo
#include <linux/mm.h>            // get_mm_rss, mm_struct
#include <linux/sched/signal.h>  // for_each_process
#include <linux/string.h>        // strscpy

/*
 * El linux/cgroup.h se utiliza para trabajar con cgroups, aunque en este módulo no se utiliza directamente, pero se incluye por si se necesita acceder a información relacionada con cgroups en el futuro.
 * El linux/slab.h se incluye para utilizar las funciones de asignación de memoria dinámica del kernel, como kmalloc y kfree, que se utilizan para gestionar la memoria del buffer de cmdline en la función continfo_show. 
 */
#include <linux/cgroup.h>       // cgroup_path
#include <linux/slab.h>         // kmalloc, kfree

// * Definiciones de constantes para los nombres de las entradas en /proc, utilizando el número de carnet para personalizarlo.
#define CARNET "201905884"
#define PROC_MEMINFO_NAME "meminfo_pr2_so1_" CARNET
#define PROC_CONTINFO_NAME "continfo_pr2_so1_" CARNET

// * La cmdline tiene como función mostrar la línea de comandos con la que se ejecutó cada proceso, lo cual es útil 
// * para identificar procesos específicos, especialmente en un entorno con contenedores Docker.
#define CMDLINE_SIZE 256  // Tamaño máximo para cmdline

// * Declaración de punteros a las entradas del sistema de archivos proc para meminfo y continfo, que se utilizarán 
// * para crear y gestionar estas entradas en el módulo del kernel.
static struct proc_dir_entry *proc_meminfo_entry;
static struct proc_dir_entry *proc_continfo_entry;

/*
 * Parámetro: container_id=<CID>
 * Se pueden pasar 12 chars o el CID completo.
 */
static char *container_id;
module_param(container_id, charp, 0444);
MODULE_PARM_DESC(container_id, "Docker container ID substring used to match processes via cgroup v2 path");

/**
 * Función para determinar si un proceso es un proceso "general" relacionado con Docker, como el demonio de Docker 
 * o los procesos de contenedores.
 * @param task: Puntero a la estructura task_struct del proceso que se desea evaluar.
 * @return: Devuelve true si el proceso es un proceso "general" relacionado con Docker y false en caso contrario.
 * 
 * Esta función compara el nombre del proceso (task->comm) con una lista de nombres comunes de procesos relacionados con Docker, 
 * como "dockerd", "containerd", "containerd-shim", "containerd-shim-runc-v2" y "runc".
 */
static bool is_general_process(const struct task_struct *task)
{
	/*
	 * Dependen del proyecto, pero estos son los "generales".
	 * Estos suelen existir en hosts con Docker.
	 */
	return (strcmp(task->comm, "dockerd") == 0) ||
	       (strcmp(task->comm, "containerd") == 0) ||
	       (strcmp(task->comm, "containerd-shim") == 0) ||
	       (strcmp(task->comm, "containerd-shim-runc-v2") == 0) ||
	       (strcmp(task->comm, "runc") == 0);
}

/*
 * Obtiene el cgroup v2 path del task (unified hierarchy) y verifica si contiene el CID.
 *
 * Implementación para cgroup v2:
 * - task_get_css(): obtiene css del task para el subsistema "unificado"
 * - cgroup_path(): convierte el cgroup a string path
 *
 ! NOTA:
 * Algunas funciones de cgroup pueden no estar exportadas para módulos dependiendo del build.
 * En kernels "generic" usualmente están disponibles.
 */

static bool task_in_container_by_cgroup2(struct task_struct *task, const char *cid)
{
    char *path; // Buffer para almacenar el path del cgroup
    struct cgroup_subsys_state *css; // Estructura para almacenar el estado del subsistema de cgroup
    bool mathes = false; // Variable para indicar si el CID coincide con el path del cgroup

    if(!cid || !*cid) {
        return false; // Si el CID es nulo o vacío, no se puede determinar si el proceso está en un contenedor
    }

    path = kmalloc(PATH_MAX, GFP_KERNEL); // Asignar memoria para el buffer del path utilizando kmalloc, que es una función del kernel para asignar memoria dinámica. Se utiliza GFP_KERNEL para indicar que la asignación se realiza en el contexto del kernel.
    if(!path) {
        return false; // Si no se pudo asignar memoria para el buffer del path, no se puede determinar si el proceso está en un contenedor
    }

    /*
	 * En cgroup v2, el "default hierarchy" se trata como subsistema unificado.
	 ? Usamos task_get_css() con &cgroup_subsys[0] NO es correcto/estable.
	 * La forma típica es usar task_get_css() para el subsistema "cpu"/"memory", etc.,
	 * Necesitamos cgroup del task en el árbol unificado. En kernels actuales,
	 * el css está asociado a un subsistema concreto; para obtener un path que incluya
	 * el nombre del cgroup del contenedor, escoger "memory" suele funcionar.
	 *
     ! Nota:
	 * Si el kernel no tiene memory cgroup habilitado, hay que cambiar a "cpu" u otro.
	 */

    #ifdef CONFIG_MEMCG
	css = task_get_css(task, memory_cgrp_subsys_id);
    #else
        css = NULL;
    #endif
        if (!css) {
            kfree(path);
            return false;
        }

        /*
        * cgroup_path() devuelve la longitud escrita o <0 en error.
        * El path típico en cgroup2 se ve como:
        *   /system.slice/docker-<CID>.scope
        * o /docker/<CID>...
        */
        if (cgroup_path(css->cgroup, path, CGROUP_PATH_MAX) > 0) {
            if (strnstr(path, cid, CGROUP_PATH_MAX))
                match = true;
        }

        kfree(path);
        return match; // Devolver true si el CID coincide con el path del cgroup, indicando que el proceso está en un contenedor, o false en caso contrario
}

/**
 * Función para mostrar la información de memoria en la entrada /proc/meminfo_pr2_so1_201905884.
 * @param m: Puntero a la estructura seq_file utilizada para escribir la salida de la función.
 * @param v: Puntero a un valor que se utiliza para iterar sobre la información secuencial, no se utiliza en esta función.
 * 
 * Esta función obtiene la información de memoria del sistema utilizando si_meminfo, calcula la memoria total, libre y 
 * usada en KB, y luego muestra esta información formateada en la salida del archivo proc.
 * Se asegura de que la memoria usada no sea negativa y formatea la salida para que sea fácil de leer.
 */
static int meminfo_show(struct seq_file *m, void *v)
{
    struct sysinfo i; // Estructura para almacenar la información del sistema
    u64 total_kb, free_kb, used_kb; // Variables para almacenar la memoria total, libre y usada en KB

    si_meminfo(&i); // Obtener la información de memoria del sistema

    total_kb = ((u64)i.totalram * (u64)i.mem_unit) / 1024; // Calcular la memoria total en KB
    free_kb = ((u64)i.freeram * (u64)i.mem_unit) / 1024; // Calcular la memoria libre en KB
    used_kb = (total_kb >= free_kb) ? (total_kb - free_kb) : 0; // Calcular la memoria usada en KB, asegurando que no sea negativa
    
    // Mostrar la información de memoria en KB
    seq_printf(m, "Total RAM: %lu KB\n", (u64)total_kb);
    seq_printf(m, "Free RAM: %lu KB\n", (u64)free_kb);
    seq_printf(m, "Used RAM: %lu KB\n", (u64)used_kb);

    return 0; // Indicar que la función se ejecutó correctamente
}

/**
 * Función para manejar la apertura del archivo proc para meminfo.
 * @param inode: Puntero a la estructura inode del archivo proc que se está abriendo.
 * @param file: Puntero a la estructura file que representa el archivo proc que se está abriendo.
 * 
 * Esta función utiliza single_open para asociar la función meminfo_show con el archivo proc, lo que permite que
 * cada vez que se abra el archivo proc, se ejecute meminfo_show para mostrar la información de memoria actualizada. 
 * El tercer parámetro de single_open se deja como NULL ya que no se necesita pasar datos adicionales a meminfo_show.
 */
static int meminfo_open(struct inode *inode, struct file *file)
{
    return single_open(file, meminfo_show, NULL); // Abrir el archivo proc utilizando single_open y asociar la función de mostrar
}

/**
 * Estructura proc_ops para la entrada /proc/meminfo_pr2_so1_201905884, que define las operaciones que se pueden realizar en esta entrada del sistema de archivos proc.
 * - proc_open: Función para manejar la apertura del archivo proc, que se asigna a meminfo_open para mostrar la información de memoria cada vez que se abra el archivo.
 * - proc_read: Función para manejar la lectura del archivo proc, que se asigna a seq_read para permitir la lectura secuencial de la información mostrada por meminfo_show.
 * - proc_lseek: Función para manejar el desplazamiento en el archivo proc, que se asigna a seq_lseek para permitir el desplazamiento dentro del archivo proc al leerlo.
 * - proc_release: Función para manejar la liberación del archivo proc, que se asigna a single_release para liberar los recursos asociados con la apertura del archivo proc.    
 */
static const struct proc_ops meminfo_fops = {
    .proc_open = meminfo_open, // Función para manejar la apertura del archivo proc
    .proc_read = seq_read, // Función para manejar la lectura del archivo proc utilizando seq_read
    .proc_lseek = seq_lseek, // Función para manejar el desplazamiento en el archivo proc utilizando seq_lseek
    .proc_release = single_release, // Función para manejar la liberación del archivo proc utilizando single_release
};

/**
 * Función para mostrar la información de los procesos en la entrada /proc/continfo_pr2_so1_201905884.
 * @param m: Puntero a la estructura seq_file utilizada para escribir la salida de la función.
 * @param v: Puntero a un valor que se utiliza para iterar sobre la información secuencial, no se utiliza en esta función.
 * 
 * Esta función itera sobre todos los procesos en el sistema utilizando for_each_process, y para cada proceso, verifica si es un proceso "general" relacionado con Docker o si coincide con el container_id especificado en su cgroup v2 path.
 * Si el proceso cumple con los criterios de inclusión, se obtiene su información de memoria (VSZ y RSS), se calcula el porcentaje de memoria utilizada en relación con la memoria total del sistema, y se muestra esta información formateada en la salida del archivo proc.
 * La salida incluye el PID, el nombre del proceso, la memoria virtual (VSZ), la memoria residente (RSS), el porcentaje de memoria utilizada, el tiempo de CPU acumulado (utime + stime) y el container_id si corresponde.
 */
static int continfo_show(struct seq_file *m, void *v)
{
    struct task_struct *task; // Estructura para iterar sobre los procesos
    struct sysinfo i; // Estructura para almacenar la información del sistema
    u64 mem_total_kb; // Variable para almacenar la memoria total en KB

    si_meminfo(&i); // Obtener la información de memoria del sistema
    total_kb = ((u64)i.totalram * (u64)i.mem_unit) / 1024; // Calcular la memoria total en KB
    
    if(!mem_total_kb)
    {
        mem_total_kb = 1; // Evitar división por cero, aunque esto no debería ocurrir en un sistema con memoria
    }

    seq_printf(m, "container_id=%s\n", container_id ? container_id : "(none)"); // Imprimir el container_id que se está utilizando para filtrar los procesos, o "(none)" si no se ha especificado un container_id
    seq_printf(m, "PID\tNAME\tVSZ_(KB)\tRSS_(KB)\t%%MEM_PCT\t%%CPU_RAW\tCONTAINER_ID\n"); // Imprimir encabezado de la tabla

    // Iterar sobre todos los procesos en el sistema utilizando for_each_process, que es una macro que permite recorrer la lista de procesos.
    for_each_process(task) {
        bool include = false; // Variable para determinar si se debe incluir el proceso en la salida

        if(is_general_process(task)) {
            include = true; // Incluir procesos generales relacionados con Docker
        }

        if(!include && container_id && *container_id) {
           if(task_in_container_by_cgroup2(task, container_id)) {
                include = true; // Incluir procesos que coincidan con el container_id en su cgroup v2 path
            }
        }

        if(!include) {
            continue; // Si el proceso no se debe incluir, pasar al siguiente proceso
        }

        // * Metricas de memoria del proceso
        {
            struct mm_struct *mm = task->mm;
			u64 vsz_kb = 0, rss_kb = 0, mem_pct = 0;

			if (mm) {
				vsz_kb = (u64)mm->total_vm << (PAGE_SHIFT - 10);
				rss_kb = (u64)get_mm_rss(mm) << (PAGE_SHIFT - 10);
			}

			mem_pct = (rss_kb * 100) / mem_total_kb;

			seq_printf(m, "%d\t%s\t%llu\t%llu\t%llu\t%llu\t%s\n",
				   task->pid,
				   task->comm,
				   vsz_kb,
				   rss_kb,
				   mem_pct,
				   (u64)(task->utime + task->stime),
				   (container_id && *container_id) ? container_id : "-");
		}

        return 0; // Indicar que la función se ejecutó correctamente
    }
}

/**
 * Función para manejar la apertura del archivo proc para continfo.
 * @param inode: Puntero a la estructura inode del archivo proc que se está abriendo.
 * @param file: Puntero a la estructura file que representa el archivo proc que se está abriendo.
 * 
 * Esta función utiliza single_open para asociar la función continfo_show con el archivo proc, lo que permite que
 * cada vez que se abra el archivo proc, se ejecute continfo_show para mostrar la información de los procesos actualizada. 
 * El tercer parámetro de single_open se deja como NULL ya que no se necesita pasar datos adicionales a continfo_show.
 */
static int continfo_open(struct inode *inode, struct file *file)
{
	return single_open(file, continfo_show, NULL);
}

/**
 * Estructura proc_ops para la entrada /proc/continfo_pr2_so1_201905884, que define las operaciones que se pueden realizar en esta entrada del sistema de archivos proc.
 * - proc_open: Función para manejar la apertura del archivo proc, que se asigna a continfo_open para mostrar la información de los procesos cada vez que se abra el archivo.
 * - proc_read: Función para manejar la lectura del archivo proc, que se asigna a seq_read para permitir la lectura secuencial de la información mostrada por continfo_show.
 * - proc_lseek: Función para manejar el desplazamiento en el archivo proc, que se asigna a seq_lseek para permitir el desplazamiento dentro del archivo proc al leerlo.
 * - proc_release: Función para manejar la liberación del archivo proc, que se asigna a single_release para liberar los recursos asociados con la apertura del archivo proc.        
 */
static const struct proc_ops continfo_ops = {
	.proc_open    = continfo_open,
	.proc_read    = seq_read,
	.proc_lseek   = seq_lseek,
	.proc_release = single_release,
};

/**
 * Función de inicialización del módulo, que se ejecuta cuando el módulo es cargado en el kernel.
 * Esta función crea las entradas en el sistema de archivos proc para meminfo y continfo, y maneja los errores en caso de que no se puedan crear las entradas.
 * Si la creación de la entrada para meminfo falla, se devuelve un error de memoria. Si la creación de la entrada para continfo falla, se elimina la entrada de meminfo creada previamente y se devuelve un error de memoria.
 * Si ambas entradas se crean correctamente, se imprime un mensaje de información en el log del kernel indicando que las entradas se han creado exitosamente.   
 */
static int __init pr2_module_init(void)
{
	proc_meminfo_entry = proc_create(PROC_MEMINFO_NAME, 0444, NULL, &meminfo_ops);
	if (!proc_meminfo_entry)
		return -ENOMEM;

	proc_continfo_entry = proc_create(PROC_CONTINFO_NAME, 0444, NULL, &continfo_ops);
	if (!proc_continfo_entry) {
		proc_remove(proc_meminfo_entry);
		return -ENOMEM;
	}

	pr_info("PR2 SO1 %s: /proc/%s y /proc/%s creados\n",
		CARNET, PROC_MEMINFO_NAME, PROC_CONTINFO_NAME);
	
    // Advertencia si no se especificó container_id, ya que solo se listarán procesos generales relacionados con Docker.
        if (!container_id || !*container_id)
	pr_warn("PR2 SO1 %s: container_id no especificado; solo se listaran procesos generales\n", CARNET);

	return 0;
}

/**
 * Función de limpieza del módulo, que se ejecuta cuando el módulo es descargado del kernel.
 * Esta función elimina las entradas en el sistema de archivos proc para meminfo y continfo si
 * existen, y luego imprime un mensaje de información en el log del kernel indicando que el módulo ha sido descargado.   
 * Si las entradas existen, se eliminan utilizando proc_remove, que es una función del kernel para eliminar entradas del sistema de archivos proc.
 * Después de eliminar las entradas, se imprime un mensaje de información utilizando pr_info para indicar que el módulo ha sido descargado exitosamente.   
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

/**
 * Las macros module_init y module_exit se utilizan para registrar las funciones de inicialización y limpieza del módulo, respectivamente.
 * - module_init(pr2_module_init): Registra la función pr2_module_init como la
 * función de inicialización del módulo, que se ejecutará cuando el módulo sea cargado en el kernel.
 * - module_exit(pr2_module_exit): Registra la función pr2_module_exit como la
 *  función de limpieza del módulo, que se ejecutará cuando el módulo sea descargado del kernel.
 * Estas macros son esenciales para que el kernel sepa qué funciones ejecutar en los momentos adecuados durante el ciclo de vida del módulo.
 */
module_init(pr2_module_init);
module_exit(pr2_module_exit);

/**
 * Las macros MODULE_LICENSE, MODULE_AUTHOR y MODULE_DESCRIPTION se utilizan para proporcionar información sobre el módulo.
 * - MODULE_LICENSE("GPL"): Indica que el módulo está licenciado bajo la Licencia Pública General de GNU (GPL), lo que permite su uso y distribución bajo los términos de esta licencia.
 * - MODULE_AUTHOR("201905884"): Proporciona el nombre del autor del módulo, en este caso, se utiliza el número de carnet para identificar al autor.
 * - MODULE_DESCRIPTION("Modulo PR2 SO1: meminfo + continfo en /proc"): Proporciona una descripción breve del módulo, indicando que se trata de un módulo para el PR2 de Sistemas Operativos 1 que crea entradas en /proc para mostrar información de memoria y procesos.   
 */
MODULE_LICENSE("GPL");
MODULE_AUTHOR("201905884");
MODULE_DESCRIPTION("Modulo PR2 SO1: meminfo + continfo en /proc");