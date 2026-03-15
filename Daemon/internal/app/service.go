package app

/**
* context: para manejar la cancelación de operaciones y el paso de datos entre goroutines.
* log: para registrar errores y eventos importantes durante la ejecución del servicio.
* time: para manejar intervalos de tiempo, como el intervalo de recolección de datos.
* parser: para usar las funciones de parseo definidas en el paquete parser, como ParserContInfo.
* sink: para usar las interfaces y estructuras de sink, como Writer y JSONLineFile.
* source: para usar las interfaces y estructuras de source, como Reader y CommandSource.
 */
import (
	"context"
	"log"
	"time"

	"daemon/internal/docker"
	"daemon/internal/parser"
	"daemon/internal/sink"
	"daemon/internal/source"
)

/*
*
  - Service es la estructura principal del servicio, que contiene los lectores y escritores para memoria y contenedores,
    así como el intervalo de recolección de datos.
  - MemReader: es un source.Reader que se encarga de leer datos relacionados con la memoria del sistema.
  - ContReader: es un source.Reader que se encarga de leer datos relacionados con los contenedores.
  - MemWriter: es un sink.Writer que se encarga de escribir los datos de memoria en un destino, como un archivo JSON.
  - ContWriter: es un sink.Writer que se encarga de escribir los datos de contenedores en un destino, como un archivo JSON.
  - Interval: es un time.Duration que define el intervalo de tiempo entre cada recolección de datos.
  - prevContainersActive: es un contador que almacena el número de contenedores activos en el tick anterior.
  - totalContainersRemoved: es un contador acumulativo que almacena el total de contenedores eliminados desde que inició el servicio.
*/
type Service struct {
	MemReader  source.Reader
	ContReader source.Reader
	MemWriter  sink.Writer
	ContWriter sink.Writer
	Interval   time.Duration
	Docker     *docker.Manager

	totalContainersRemoved int
}

/**
* Run es el método principal del servicio, que inicia un loop que se ejecuta hasta que se cancela el contexto.
* Dentro del loop, se espera a que ocurra un tick del ticker o a que se cancele el contexto.
* Si se cancela el contexto, se detiene el servicio y se devuelve nil.
* Si ocurre un tick, se llama al método tick para realizar la recolección y escritura de datos.
 */
func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("service: deteniendo...")
			return nil
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

/**
* tick es el método que se ejecuta en cada tick del ticker, y se encarga de realizar la recolección y escritura de datos.
* En este método, se realizan las siguientes acciones:
* 1. Se lee el dato crudo de memoria usando MemReader.Read. Si ocurre un error, se registra en el log.
* 2. Si la lectura de memoria es exitosa, se parsea el dato usando parser.ParseMemInfo. Si ocurre un error, se registra en el log.
* 3. Si el parseo de memoria es exitoso, se escribe el dato parseado usando MemWriter.Write. Si ocurre un error, se registra en el log.
* 4. Se repiten los mismos pasos para la información de contenedores, usando ContReader, parser.ParserContInfo y ContWriter.
 */
func (s *Service) tick(ctx context.Context) {
	// --- meminfo ---
	rawMem, err := s.MemReader.Read(ctx)
	if err != nil {
		log.Printf("service: error leyendo meminfo: %v", err)
	} else {
		mem, err := parser.ParseMemInfo(string(rawMem))
		if err != nil {
			log.Printf("service: error parseando meminfo: %v", err)
		} else if err := s.MemWriter.Write(mem); err != nil {
			log.Printf("service: error escribiendo meminfo: %v", err)
		}
	}

	// --- continfo ---
	rawCont, err := s.ContReader.Read(ctx)
	if err != nil {
		log.Printf("service: error leyendo continfo: %v", err)
	} else {
		cont, err := parser.ParserContInfo(string(rawCont))
		if err != nil {
			log.Printf("service: error parseando continfo: %v", err)
		} else {
			// Aplicar invariantes y gestionar contenedores
			result, dockerErr := s.Docker.Enforce(cont.Processes)
			if dockerErr != nil {
				log.Printf("service: docker enforce: %v", dockerErr)
			} else {
				s.totalContainersRemoved += result.Removed
				cont.ContainersRemoved = result.Removed
				cont.ContainersInactive = s.totalContainersRemoved
				cont.ContainersActive = result.ActiveLow + result.ActiveHigh
				cont.ContainersExited = result.Exited
			}

			if err := s.ContWriter.Write(cont); err != nil {
				log.Printf("service: error escribiendo continfo: %v", err)
			}
		}
	}
}
