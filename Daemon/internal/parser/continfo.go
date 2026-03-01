package parser

/*
* fmt: para envolver errores con mensaje informativo.
* strconv: para convertir cadenas a números, como parsear el valor de memoria.
* strings: para manipular cadenas, como dividir líneas y extraer campos.
* time: para manejar timestamps, si es necesario agregar un timestamp a los datos parseados.
* model: para usar las estructuras definidas en el paquete model, como ContainerReport.
 */
import (
	"fmt"
	"strconv"
	"strings"
	"time"

	model "daemon/internal/model"
)

/**
* Estructura de datos para almacenar la información de los contenedores
  container_id=abc123          ← tipo A: metadata inicial
  PID   NAME    VSZ_(KB)...    ← tipo B: header (ignorar)
  142   dockerd 102400...      ← tipo C: datos de proceso
  CONTAINERS_ACTIVE=1          ← tipo D: metadata final
*/

func ParserContInfo(raw string) (model.ContainerReport, error) {
	// Se inicializa una variable ContainerReport para almacenar los datos parseados.
	var report model.ContainerReport

	// Se divide el texto crudo en líneas usando strings.Split.
	lines := strings.Split(raw, "\n")

	/*
		* El loop debe realizar if / continue:

		1. ¿Línea vacía?                     → continue
		2. ¿HasPrefix "container_id="?       → parsear FilterID, continue
		3. ¿HasPrefix "PID\t"?               → continue  (header)
		4. ¿HasPrefix "CONTAINERS_ACTIVE="?  → parsear int, continue
		5. Todo lo demás                     → Split("\t"), verificar 7 campos, parsear ProcessInfo
	*/
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "container_id=") {
			report.FilterID = strings.TrimPrefix(line, "container_id=")
			continue
		}
		if strings.HasPrefix(line, "PID\t") {
			continue // header
		}
		if strings.HasPrefix(line, "CONTAINERS_ACTIVE=") {
			valueStr := strings.TrimPrefix(line, "CONTAINERS_ACTIVE=")
			value, err := strconv.ParseInt(valueStr, 10, 64)
			if err != nil {
				return report, fmt.Errorf("error parsing CONTAINERS_ACTIVE: %w", err)
			}
			report.ContainersActive = int(value)
			continue
		}

		// * función auxiliar para parsear la línea de proceso
		processInfo, err := ParserProcessLine(line)
		if err != nil {
			continue // Si hay un error de parseo, se ignora esta línea y se continúa con la siguiente
		}

		// * Si se parseó correctamente, se agrega el ProcessInfo al slice de procesos del reporte.
		report.Processes = append(report.Processes, processInfo)

	}

	// Se agrega un timestamp al struct, para registrar cuándo se obtuvieron los datos.
	report.Timestamp = time.Now()

	// Se retorna el ContainerReport con los datos parseados.
	return report, nil
}

/*
* ParserProcessLine es una función auxiliar que parsea una línea de
* datos de proceso y devuelve un ProcessInfo.
* - linea: la línea de texto que contiene los datos del proceso, con campos separados por tabulaciones.
* - devuelve: un ProcessInfo con los datos parseados, o un error si el formato es incorrecto.
* - error handling: si la línea no tiene exactamente 7 campos, se devuelve un error indicando que el formato es inválido.
 */
func ParserProcessLine(line string) (model.ProcessInfo, error) {
	fields := strings.Split(line, "\t")

	if len(fields) != 7 {
		return model.ProcessInfo{}, fmt.Errorf("Numero invalido de campos: esperaba 7, obtuvo %d", len(fields))
	}

	// Tabla de conversión de campos:
	// PID: fields[0] → int
	// NAME: fields[1] → string
	// VSZ_(KB): fields[2] → int
	// RSS_(KB): fields[3] → int
	// CPU_%: fields[4] → uint64.
	// MEM_%: fields[5] → uint64.
	// CONTAINER_ID: fields[6] → string (si es aplicable, puede estar vacío)
	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return model.ProcessInfo{}, fmt.Errorf("error parsing PID: %w", err)
	}

	name := fields[1]

	vszKB, err := strconv.ParseUint(fields[2], 10, 64)
	if err != nil {
		return model.ProcessInfo{}, fmt.Errorf("error parsing VSZ_(KB): %w", err)
	}

	rssKB, err := strconv.ParseUint(fields[3], 10, 64)
	if err != nil {
		return model.ProcessInfo{}, fmt.Errorf("error parsing RSS_(KB): %w", err)
	}

	memPercent, err := strconv.ParseUint(fields[4], 10, 64)
	if err != nil {
		return model.ProcessInfo{}, fmt.Errorf("error parsing MEM_%%: %w", err)
	}

	cpuPercent, err := strconv.ParseUint(fields[5], 10, 64)
	if err != nil {
		return model.ProcessInfo{}, fmt.Errorf("error parsing CPU_%%: %w", err)
	}

	containerId := fields[6]

	return model.ProcessInfo{
		Pid:         pid,
		Name:        name,
		VSZkb:       vszKB,
		RSSkb:       rssKB,
		MemPct:      memPercent,
		CPURaw:      cpuPercent,
		ContainerID: containerId,
	}, nil
}
