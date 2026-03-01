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
	MemTotal  uint64
	MemFree   uint64
	MemUsed   uint64
	Timestamp time.Time
}

/*
*
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
	Pid         int
	Name        string
	VSZkb       uint64
	RSSkb       uint64
	MemPct      uint64
	CPURaw      uint64
	ContainerID string
}

/*
*
  - !Nota: Esto es importante: el parser va a leer el archivo de arriba hacia abajo y necesita un lugar donde acumular los procesos mientras
  - !lee línea por línea. ContainerReport es ese lugar.
  - ContainerReport es una estructura que representa un informe de contenedor. Contiene los siguientes campos:
  - - FilterID: un identificador que se utiliza para filtrar los procesos que pertenecen a un contenedor específico.
  - - Processes: un slice de ProcessInfo que contiene la información de los procesos que pertenecen al contenedor identificado por FilterID.
  - - ContainersActive: un contador que indica la cantidad de contenedores activos en el sistema.
  - - Timestamp: la marca de tiempo que indica cuándo se recopiló este informe de contenedor.
    Este campo se puede utilizar para monitorear el estado general del sistema y
    detectar posibles problemas relacionados con la cantidad de contenedores en ejecución.
*/
type ContainerReport struct {
	FilterID         string
	Processes        []ProcessInfo
	ContainersActive int
	Timestamp        time.Time
}
