// SPDX-License-Identifier: GPL-2.0
/*
 * PR2 SO1 - 201905884
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
 * El linux/uaccess.h se utiliza para acceder a la memoria de los procesos desde el espacio del kernel utilizando 
 *                    access_process_vm.
 * El linux/jiffies.h se incluye para trabajar con jiffies, que son unidades de tiempo utilizadas en el kernel de Linux.
 * El linux/slab.h se utiliza para gestionar la memoria dinámica en el kernel, incluyendo funciones como kmalloc y kfree.
*/
#include <linux/mmzone.h>        // si_meminfo
#include <linux/mm.h>            // get_mm_rss, mm_struct
#include <linux/sched/signal.h>  // for_each_process
#include <linux/uaccess.h>       // access_process_vm
#include <linux/jiffies.h>       // jiffies helpers
#include <linux/slab.h>          // kmalloc/kfree

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


/**
 * Función para leer la línea de comandos de un proceso específico.
 * @param task: Puntero a la estructura task_struct del proceso del cual se desea obtener la línea de comandos.
 * @param buf: Buffer donde se almacenará la línea de comandos leída.
 * @param buflen: Tamaño del buffer proporcionado para almacenar la línea de comandos.
 * 
 * Esta función accede a la memoria del proceso para leer la línea de comandos, manejando casos especiales 
 * como procesos sin memoria (kernel threads) y asegurando que el buffer esté correctamente formateado 
 * para su uso posterior.
 */
static void read_task_cmdline(struct task_struct *task, char *buf, size_t buflen)
{
    struct mm_struct *mm; // Estructura de memoria del proceso
    unsigned long arg_start, arg_end; // Direcciones de inicio y fin de los argumentos
    int nread; // Número de bytes leídos

    if(!buf || buflen == 0) {
        return; // Si el buffer es nulo o el tamaño es cero, no se puede leer
    }

    buf[0] = '\0'; // Inicializar el buffer con una cadena vacía

    /**
     * Kernel threads normalmente no tienen mm (mm == NULL), por lo que se maneja ese caso devolviendo una cadena vacía.
    */

    if (!task->mm) {
        strscpy(buf, "[kernel_thread]", buflen);
        return;
    }

    mm = task->mm;

    arg_start = mm->arg_start; // Obtener la dirección de inicio de los argumentos
    arg_end = mm->arg_end; // Obtener la dirección de fin de los argumentos

    if(arg_end <= arg_start) {
        strscpy(buf, "[no_cmdline]", buflen); // Si no hay argumentos, se copia una cadena indicativa
        return; // Si el rango de argumentos no es válido, no se puede leer
    }

    /**
     * Lee memoria del proceso (user space) desde el kernel utilizando access_process_vm, que permite acceder 
     * a la memoria de un proceso específico.
     * Se leen los argumentos del proceso y se almacenan en el buffer proporcionado.
     */

    nread = access_process_vm(task, arg_start, buf, 
                    min_t(size_t, buflen -1, (size_t)(arg_end - arg_start)),
                    0); // El último parámetro es para flags, se deja en 0 para lectura normal

    if(nread <= 0) {
        strscpy(buf, "[unreadable_cmdline]", buflen); // Si no se pudieron leer los argumentos, se copia una cadena indicativa de error
        return; // Si no se pudieron leer los argumentos, no se puede continuar
    }

    buf[nread] = '\0'; // Asegurar que el buffer esté null-terminated

    /**
     * CMDLINE en linux viene separado por '\0', por lo que se reemplazan los caracteres nulos por espacios para una mejor legibilidad.
     */
    size_t i;
    for(i = 0; i < buflen; i++) {
        if(buf[i] == '\0') {
            buf[i] = ' '; // Reemplazar caracteres nulos por espacios
        }
    }
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
    seq_printf(m, "Total RAM: %lu KB\n", total_kb);
    seq_printf(m, "Free RAM: %lu KB\n", free_kb);
    seq_printf(m, "Used RAM: %lu KB\n", used_kb);

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

static int continfo_show(struct seq_file *m, void *v)
{
    struct task_struct *task; // Estructura para iterar sobre los procesos
    struct sysinfo i; // Estructura para almacenar la información del sistema
    u64 total_kb; // Variable para almacenar la memoria total en KB

    si_meminfo(&i); // Obtener la información de memoria del sistema
    total_kb = ((u64)i.totalram * (u64)i.mem_unit) / 1024; // Calcular la memoria total en KB
    
    if(!mem_total_kb)
    {
        mem_total_kb = 1; // Evitar división por cero, aunque esto no debería ocurrir en un sistema con memoria
    }

    seq_printf(m, "PID\tNAME\tVSZ(KB)\tRSS(KB)\t%%MEM_PCT\t%%CPU_RAW\tCMDLINE\n"); // Imprimir encabezado de la tabla

    // Iterar sobre todos los procesos en el sistema utilizando for_each_process, que es una macro que permite recorrer la lista de procesos.
    for_each_process(task) {
		struct mm_struct *mm = task->mm; // Estructura de memoria del proceso, que se utiliza para obtener información sobre la memoria utilizada por el proceso
		u64 vsz_kb = 0, rss_kb = 0; // Variables para almacenar el tamaño virtual (VSZ) y el tamaño residente (RSS) en KB
		u64 mem_pct = 0; // Variable para almacenar el porcentaje de memoria utilizada por el proceso

		char *cmdline; // Buffer para almacenar la línea de comandos del proceso, que se utilizará para mostrar la información del proceso en la salida del archivo proc

		if (mm) {
			vsz_kb = (u64)mm->total_vm << (PAGE_SHIFT - 10); // Calcular el tamaño virtual (VSZ) en KB, utilizando total_vm que representa el número de páginas virtuales utilizadas por el proceso y ajustando por el tamaño de página
			rss_kb = (u64)get_mm_rss(mm) << (PAGE_SHIFT - 10); // Calcular el tamaño residente (RSS) en KB, utilizando get_mm_rss para obtener el número de páginas residentes en memoria y ajustando por el tamaño de página
		}

		mem_pct = (rss_kb * 100) / mem_total_kb; // Calcular el porcentaje de memoria utilizada por el proceso, dividiendo el RSS del proceso por la memoria total del sistema y multiplicando por 100 para obtener un porcentaje

		cmdline = kmalloc(CMDLINE_MAX, GFP_KERNEL); // Asignar memoria para el buffer de la línea de comandos utilizando kmalloc, que es una función del kernel para asignar memoria dinámica. Se utiliza GFP_KERNEL para indicar que la asignación se realiza en el contexto del kernel.
		
        if (cmdline) {
			/* versión simple: intenta leer cmdline */
			/* (ver nota: la función de arriba puede ajustarse) */
			read_task_cmdline(task, cmdline, CMDLINE_MAX);
		}

		/*
		 * CPU_RAW: tiempo acumulado "crudo". Puede ser grande; el enunciado lo permite.
		 * En kernels modernos, utime/stime pueden no ser accesibles directamente según config,
		 * pero en muchos builds sí. Si da problemas, se debe cambiar a 0 o a otro helper.
		 */
#if defined(CONFIG_VIRT_CPU_ACCOUNTING_GEN) || defined(CONFIG_VIRT_CPU_ACCOUNTING_NATIVE) || 1
		seq_printf(m, "%d\t%s\t%llu\t%llu\t%llu\t%llu\t%s\n",
			   task->pid,
			   task->comm,
			   vsz_kb,
			   rss_kb,
			   mem_pct,
			   (u64)(task->utime + task->stime),
			   cmdline ? cmdline : "[no_mem]");
#else
		seq_printf(m, "%d\t%s\t%llu\t%llu\t%llu\t%u\t%s\n",
			   task->pid,
			   task->comm,
			   vsz_kb,
			   rss_kb,
			   mem_pct,
			   0u,
			   cmdline ? cmdline : "[no_mem]");
#endif

		kfree(cmdline); // Liberar la memoria asignada para el buffer de la línea de comandos utilizando kfree, que es una función del kernel para liberar memoria dinámica
	}

	return 0; // Indicar que la función se ejecutó correctamente
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