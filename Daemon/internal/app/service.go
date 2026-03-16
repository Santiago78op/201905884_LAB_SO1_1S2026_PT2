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
	"math"
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
// Mantiene el formato original del kernel: mem_perc_x100 y cpu_perc_x100 como enteros.
type containerRankEntry struct {
	DockerID    string    `json:"docker_id"`
	Pid         int       `json:"pid"`
	Name        string    `json:"container_name"`
	Image       string    `json:"image"`
	Status      string    `json:"status"` // "active" | "removed"
	RSSkb       uint64    `json:"rss_kb"`
	VSZkb       uint64    `json:"vsz_kb"`
	MemPctX100  uint64    `json:"mem_perc_x100"`
	CPURawX100  uint64    `json:"cpu_perc_x100"`
	Timestamp   time.Time `json:"timestamp"`
}

// toPerc convierte un valor x100 del kernel a porcentaje real, nunca mayor a 100.00.
func toPerc(x100 uint64) float64 {
	v := math.Round(float64(x100)/100.0*100) / 100 // redondear a 2 decimales
	return math.Min(v, 100.00)
}

type Service struct {
	MemReader     source.Reader
	ContReader    source.Reader
	MemWriter     sink.Writer
	ContWriter    sink.Writer
	ProcWriter    sink.Writer // una entrada por contenedor, para Top Rankings en Grafana
	RssRankWriter     *sink.ValkeyRankWriter // sorted set por rss_kb — para ZRANGEBYSCORE sin duplicados
	CpuRankWriter     *sink.ValkeyRankWriter // sorted set por cpu_perc_x100 — para ZRANGEBYSCORE sin duplicados
	ContainerHashWriter *sink.ValkeyHashWriter // hash field=name value=JSON — estado actual completo para Grafana
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

			// Escribir contenedores activos en procinfo (historial)
			for _, c := range result.ActiveContainers {
				entry := containerRankEntry{
					DockerID:   c.ID,
					Pid:        c.Pid,
					Name:       c.Name,
					Image:      c.Image,
					Status:     "active",
					RSSkb:      c.RSSkb,
					VSZkb:      c.VSZkb,
					MemPctX100: c.MemPct,
					CPURawX100: c.CPURaw,
					Timestamp:  cont.Timestamp,
				}
				if err := s.ProcWriter.Write(entry); err != nil {
					log.Printf("service: error escribiendo ranking activo %s: %v", c.ID[:12], err)
				}
			}
			// Escribir contenedores eliminados en este tick (historial)
			for _, c := range result.RemovedContainers {
				entry := containerRankEntry{
					DockerID:   c.ID,
					Pid:        c.Pid,
					Name:       c.Name,
					Image:      c.Image,
					Status:     "removed",
					RSSkb:      c.RSSkb,
					VSZkb:      c.VSZkb,
					MemPctX100: c.MemPct,
					CPURawX100: c.CPURaw,
					Timestamp:  cont.Timestamp,
				}
				if err := s.ProcWriter.Write(entry); err != nil {
					log.Printf("service: error escribiendo ranking eliminado %s: %v", c.ID[:12], err)
				}
			}

			// Actualizar sorted sets y hash: estado actual sin duplicados
			// Hash keyed por docker_id → unicidad garantizada
			// Sorted sets usan container_name como member y toPerc() como score
			for _, c := range result.ActiveContainers {
				if s.RssRankWriter != nil {
					if err := s.RssRankWriter.Upsert(toPerc(c.MemPct), c.Name); err != nil {
						log.Printf("service: error upsert rss_rank %s: %v", c.ID[:12], err)
					}
				}
				if s.CpuRankWriter != nil {
					if err := s.CpuRankWriter.Upsert(toPerc(c.CPURaw), c.Name); err != nil {
						log.Printf("service: error upsert cpu_rank %s: %v", c.ID[:12], err)
					}
				}
				if s.ContainerHashWriter != nil {
					entry := containerRankEntry{
						DockerID:   c.ID,
						Pid:        c.Pid,
						Name:       c.Name,
						Image:      c.Image,
						Status:     "active",
						RSSkb:      c.RSSkb,
						VSZkb:      c.VSZkb,
						MemPctX100: c.MemPct,
						CPURawX100: c.CPURaw,
						Timestamp:  cont.Timestamp,
					}
					if err := s.ContainerHashWriter.HSet(c.ID, entry); err != nil {
						log.Printf("service: error hset containers %s: %v", c.ID[:12], err)
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
				if s.ContainerHashWriter != nil {
					if err := s.ContainerHashWriter.HDel(c.ID); err != nil {
						log.Printf("service: error hdel containers %s: %v", c.ID[:12], err)
					}
				}
			}
		}
	}
}
