package main

/**
* context: para manejar la cancelación de goroutines y operaciones asíncronas.
* log: para registrar mensajes de error o información.
* os: para interactuar con el sistema operativo, como manejar señales de interrupción.
* os/signal: para capturar señales del sistema, como SIGINT o SIGTERM, y permitir una terminación limpia del programa.
* syscall: para acceder a constantes de señales específicas del sistema operativo.
* time: para manejar temporizadores o timestamps, si es necesario.
* app: para usar la lógica principal de la aplicación, como la función Run que ejecuta el ciclo principal del daemon.
* sink: para usar la funcionalidad relacionada con el almacenamiento o envío de datos, como la función SendData que envía los datos parseados a un destino específico.
* source: para usar la funcionalidad relacionada con la obtención de datos, como la función GetMemInfo que obtiene la información de memoria del sistema.
 */
import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"daemon/internal/app"
	"daemon/internal/docker"
	"daemon/internal/kernel"
	"daemon/internal/sink"
	"daemon/internal/source"

	"github.com/joho/godotenv"
)

/**
* El programa principal del daemon se encarga de configurar y ejecutar el servicio.
* Se crea un contexto con cancelación para manejar la terminación del programa de manera limpia.
* Se configura la captura de señales del sistema para permitir que el programa responda a interrupciones como SIGINT o SIGTERM.
* Se inicia el ciclo principal del daemon en una goroutine separada, que espera a que llegue una señal para iniciar la terminación del programa.
* Se crea una instancia del servicio con los lectores y escritores necesarios para obtener y almacenar los datos de memoria y contenedores.
* Finalmente, se ejecuta el servicio y se maneja cualquier error que pueda ocurrir durante su ejecución.
 */
func main() {

	// Carga de Variables de Entorno env
	err := godotenv.Load()
	if err != nil {
		log.Printf("main: no se pudo cargar el archivo .env: %v", err)
	} else {
		log.Println("main: archivo .env cargado exitosamente")
	}

	// Kernel script flag: permite especificar la ruta al script que carga el módulo del kernel, con un valor por defecto.
	kernelScript := flag.String(
		"kernel-script",
		os.Getenv("KERNEL_SCRIPT_PATH"),
		"Ruta al script que carga el módulo de kernel",
	)

	// Container ID flag: permite especificar el ID del contenedor Docker, con un valor por defecto vacío.
	containerID := flag.String(
		"container-id",
		"",
		"ID del contenedor Docker",
	)

	// log para iniciar la carga del módulo del kernel
	log.Println("main: cargando módulo de Kernel ...")

	/*
	* flag.String() retorna un *string (puntero).
	* El * lo desreferencia para obtener el valor string
	 */

	// Si el módulo no carga, las entradas /proc no existen. No inicia el Daemon rerpota log.Fatalf.
	if err := kernel.Load(kernel.LoadOpts{
		ScriptPath:  *kernelScript,
		ContainerID: *containerID,
	}); err != nil {
		log.Fatalf("main: error al cargar el módulo de kernel: %v", err)
	}

	log.Println("main: módulo de Kernel cargado exitosamente")

	// Al salir del daemon, descargar el módulo de kernel
	unloadScript := os.Getenv("KERNEL_UNLOAD_SCRIPT_PATH")
	defer func() {
		log.Println("main: descargando módulo de kernel...")
		if err := kernel.Unload(unloadScript); err != nil {
			log.Printf("main: error descargando módulo de kernel: %v", err)
		} else {
			log.Println("main: módulo de kernel descargado exitosamente")
		}
	}()

	// Levantar contenedores de Grafana y Valkey
	log.Println("main: levantando contenedores de Grafana y Valkey ...")
	composeFile := os.Getenv("COMPOSE_FILE_PATH")

	cmdCompose := exec.Command("sudo", "docker", "compose", "-f", composeFile, "up", "-d")
	cmdCompose.Stdout = os.Stdout
	cmdCompose.Stderr = os.Stderr

	if err := cmdCompose.Run(); err != nil {
		log.Fatalf("main: error al levantar contenedores con Docker Compose: %v", err)
	}

	log.Println("main: contenedores de Grafana y Valkey levantados exitosamente")

	// Registrar cronjob (paso 2): crea 5 contenedores aleatorios cada 2 minutos
	cronScript := os.Getenv("CRON_SCRIPT_PATH")
	if err := app.RegisterCronjob(cronScript); err != nil {
		log.Fatalf("main: error registrando cronjob: %v", err)
	}
	log.Println("main: cronjob registrado (cada 2 minutos)")

	// Se parsean los flags para que estén disponibles en el programa.
	flag.Parse()

	// Se crea un contexto con cancelación para manejar la terminación del programa.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Asegura que el contexto se cancele al finalizar main.

	/*
		<sigs es una lectura bloqueante: espera hasta que llegue una señal. No se colaca en el hilo principal,
		sino el daemon nunca llegaría a svc.Run(). La goroutine la espera en paralelo mientras

			el daemon trabaja.

			Qué señales capturar:
			- SIGTERM → la que manda systemd, kill <pid>, o Docker al detener un contenedor
			- SIGINT → la que produce Ctrl+C en terminal

			Cuando llega cualquiera de las dos, se llama cancel(), lo que hace que ctx.Done() se cierre,
			lo que hace que service.Run() retorne nil limpiamente.
	*/
	// Captura de señales del Os para permitir una terminación limpia.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Se ejecuta el ciclo principal del daemon en una goroutine separada.
	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %v, shutting down...", sig)
		cancel() // Cancela el contexto para iniciar la terminación del programa.
	}()

	// Conector de todas las piezas del servicio
	svc := &app.Service{
		MemReader:  source.FileReader{Path: os.Getenv("FILE_READER_SERVICE_MEM_PATH")},
		ContReader: source.FileReader{Path: os.Getenv("FILE_READER_SERVICE_CONT_PATH")},
		MemWriter:  sink.NewValkeyWriter(os.Getenv("VALKEY_ADDR"), os.Getenv("VALKEY_KEY_MEM")),
		ContWriter: sink.NewValkeyWriter(os.Getenv("VALKEY_ADDR"), os.Getenv("VALKEY_KEY_CONT")),
		ProcWriter: sink.NewValkeyWriter(os.Getenv("VALKEY_ADDR"), os.Getenv("VALKEY_KEY_PROC")),
		Docker:     docker.NewManager(),
	}

	log.Println("main: daemon iniciado")

	if err := svc.Run(ctx); err != nil {
		log.Fatalf("main: error fatal: %v", err)
	}

	// Paso 5: eliminar el cronjob antes de finalizar
	if err := app.RemoveCronjob(cronScript); err != nil {
		log.Printf("main: error eliminando cronjob: %v", err)
	} else {
		log.Println("main: cronjob eliminado")
	}

	log.Println("main: daemon detenido")
}
