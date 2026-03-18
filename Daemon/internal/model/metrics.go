package model

import "time"

/**
* meminfo_pr2_so1_201905884
* MemStats es una estructura que representa las estadísticas de memoria. Contiene tres campos:
* - MemTotal: la cantidad total de memoria disponible en el sistema.
* - MemFree: la cantidad de memoria libre, es decir, la memoria que no está siendo utilizada por ningún proceso.
* - MemUsed: la cantidad de memoria utilizada, que se calcula como MemTotal menos MemFree.
* - Timestamp: la marca de tiempo que indica cuándo se recopilaron estas estadísticas de memoria.
* Se uso unit64 porque el kernel escribe valores en como %llu (unsigned long long).
* Nunca serán negativos.
 */
type MemStats struct {
	MemTotal  uint64    `json:"total_ram_kb"`
	MemFree   uint64    `json:"free_ram_kb"`
	MemUsed   uint64    `json:"used_ram_kb"`
	Timestamp time.Time `json:"timestamp"`
}

/*
  - continfo_pr2_so1_201905884
  - ProcessInfo es una estructura que representa la información de un proceso. Contiene los siguientes campos:
  - - Pid: el identificador del proceso.
  - - Name: el nombre del proceso.
  - - VSZkb: el tamaño virtual del proceso en kilobytes.
  - - RSSkb: el tamaño de la memoria residente del proceso en kilobytes.
  - - MemPct: el porcentaje de memoria que el proceso está utilizando con respecto a la memoria total del sistema.
  - - CPURaw: el tiempo de CPU utilizado por el proceso en unidades de tiempo crudo (por ejemplo, jiffies).
  - - ContainerID: el identificador del contenedor al que pertenece el proceso, si es aplicable.
    Si el proceso no pertenece a ningún contenedor, este campo puede estar vacío o contener un valor predeterminado.
*/
type ProcessInfo struct {
	Pid         int    `json:"pid"`
	Name        string `json:"name"`
	Cmdline     string `json:"cmdline"`
	VSZkb       uint64 `json:"vsz_kb"`
	RSSkb       uint64 `json:"rss_kb"`
	MemPct      uint64 `json:"mem_perc_x100"`
	CPURaw      uint64 `json:"cpu_perc_x100"`
	ContainerID string `json:"container_id"`
}

/*
*
tick N-1:  ContainersActive = 5
tick N:    ContainersActive = 3

	                 ↓
	Eliminados en este tick = 5 - 3 = 2  (si fue positivo)
	Inactivos acumulados    = anterior + 2

- ContainersRemoved int — cuántos se eliminaron en este tick (comparado con el anterior)
- ContainersInactive int — acumulado total de eliminados desde que inició el daemon
*/
type ContainerReport struct {
	FilterID           string        `json:"-"`        // no necesario en Grafana
	Processes          []ProcessInfo `json:"-"`        // se guarda por separado en procinfo
	ContainersActive   int           `json:"containers_active"`
	ContainersExited   int           `json:"containers_exited"`
	ContainersRemoved  int           `json:"containers_removed"`
	ContainersInactive int           `json:"containers_inactive"`
	Timestamp          time.Time     `json:"timestamp"`
}

type JsonContInfo struct {
	Processes    []ProcessInfo `json:"processes"`
	DockerActive int           `json:"docker_active"`
}

type JsonMemInfo struct {
	MemTotal uint64 `json:"total_ram_kb"`
	MemFree  uint64 `json:"free_ram_kb"`
	MemUsed  uint64 `json:"used_ram_kb"`
}
