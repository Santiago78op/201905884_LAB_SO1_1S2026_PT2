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
	"math/rand"
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
// containerRankEntry es una entrada de contenedor para el ranking en Valkey/Grafana.
// Incluye el Docker ID completo, métricas de recursos y estado (activo/eliminado).
type containerRankEntry struct {
	DockerID  string    `json:"docker_id"`
	Pid       int       `json:"pid"`
	Name      string    `json:"container_name"`
	Image     string    `json:"image"`
	Status    string    `json:"status"` // "active" | "removed"
	RSSkb     uint64    `json:"rss_kb"`
	VSZkb     uint64    `json:"vsz_kb"`
	MemPct    uint64    `json:"mem_perc_x100"`
	CPURaw    uint64    `json:"cpu_perc_x100"`
	Timestamp time.Time `json:"timestamp"`
}

type Service struct {
	MemReader     source.Reader
	ContReader    source.Reader
	MemWriter     sink.Writer
	ContWriter    sink.Writer
	ProcWriter    sink.Writer // una entrada por contenedor, para Top Rankings en Grafana
	RssRankWriter *sink.ValkeyRankWriter // sorted set por rss_kb — para ZRANGEBYSCORE sin duplicados
	CpuRankWriter *sink.ValkeyRankWriter // sorted set por cpu_perc_x100 — para ZRANGEBYSCORE sin duplicados
	Docker        *docker.Manager

	totalContainersRemoved int
}

/**
* Run es el método principal del servicio, que inicia un loop que se ejecuta hasta que se cancela el contexto.
* Dentro del loop, se espera a que ocurra un tick del ticker o a que se cancele el contexto.
* Si se cancela el contexto, se detiene el servicio y se devuelve nil.
* Si ocurre un tick, se llama al método tick para realizar la recolección y escritura de datos.
 */
// Run ejecuta el loop principal del daemon con intervalos aleatorios entre 20 y 60 segundos,
// según lo especificado en el enunciado del proyecto.
func (s *Service) Run(ctx context.Context) error {
	for {
		wait := time.Duration(20+rand.Intn(41)) * time.Second
		log.Printf("service: próxima ejecución en %v", wait)
		select {
		case <-ctx.Done():
			log.Println("service: deteniendo...")
			return nil
		case <-time.After(wait):
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

			// Escribir contenedores activos en procinfo para el ranking (incluye Docker ID completo)
			for _, c := range result.ActiveContainers {
				entry := containerRankEntry{
					DockerID:  c.ID,
					Pid:       c.Pid,
					Name:      c.Name,
					Image:     c.Image,
					Status:    "active",
					RSSkb:     c.RSSkb,
					VSZkb:     c.VSZkb,
					MemPct:    c.MemPct,
					CPURaw:    c.CPURaw,
					Timestamp: cont.Timestamp,
				}
				if err := s.ProcWriter.Write(entry); err != nil {
					log.Printf("service: error escribiendo ranking activo %s: %v", c.ID[:12], err)
				}
			}
			// Escribir contenedores eliminados en este tick (incluidos en el ranking)
			for _, c := range result.RemovedContainers {
				entry := containerRankEntry{
					DockerID:  c.ID,
					Pid:       c.Pid,
					Name:      c.Name,
					Image:     c.Image,
					Status:    "removed",
					RSSkb:     c.RSSkb,
					VSZkb:     c.VSZkb,
					MemPct:    c.MemPct,
					CPURaw:    c.CPURaw,
					Timestamp: cont.Timestamp,
				}
				if err := s.ProcWriter.Write(entry); err != nil {
					log.Printf("service: error escribiendo ranking eliminado %s: %v", c.ID[:12], err)
				}
			}

			// Actualizar sorted sets rss_rank y cpu_rank: estado actual sin duplicados
			// Member = container_name (legible en Grafana), Score = métrica
			for _, c := range result.ActiveContainers {
				if s.RssRankWriter != nil {
					if err := s.RssRankWriter.Upsert(float64(c.RSSkb), c.Name); err != nil {
						log.Printf("service: error upsert rss_rank %s: %v", c.ID[:12], err)
					}
				}
				if s.CpuRankWriter != nil {
					if err := s.CpuRankWriter.Upsert(float64(c.CPURaw), c.Name); err != nil {
						log.Printf("service: error upsert cpu_rank %s: %v", c.ID[:12], err)
					}
				}
			}
			for _, c := range result.RemovedContainers {
				if s.RssRankWriter != nil {
					if err := s.RssRankWriter.Remove(c.Name); err != nil {
						log.Printf("service: error remove rss_rank %s: %v", c.ID[:12], err)
					}
				}
				if s.CpuRankWriter != nil {
					if err := s.CpuRankWriter.Remove(c.Name); err != nil {
						log.Printf("service: error remove cpu_rank %s: %v", c.ID[:12], err)
					}
				}
			}
		}
	}
}
