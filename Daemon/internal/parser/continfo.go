package parser

/*
* fmt: para envolver errores con mensaje informativo.
* strconv: para convertir cadenas a números, como parsear el valor de memoria.
* strings: para manipular cadenas, como dividir líneas y extraer campos.
* time: para manejar timestamps, si es necesario agregar un timestamp a los datos parseados.
* model: para usar las estructuras definidas en el paquete model, como ContainerReport.
 */
import (
	"encoding/json"
	"fmt"
	"time"

	model "daemon/internal/model"
)

func ParserContInfo(raw string) (model.ContainerReport, error) {
	var report model.ContainerReport
	var parsed model.JsonContInfo

	// Deserializar el JSON directamente
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return report, fmt.Errorf("error parsing JSON: %v", err)
	}

	// Mapear los datos parseados a ContainerReport
	for _, proc := range parsed.Processes {
		report.Processes = append(report.Processes, model.ProcessInfo{
			Pid:         proc.Pid,
			Name:        proc.Name,
			Cmdline:     proc.Cmdline,
			VSZkb:       proc.VSZkb,
			RSSkb:       proc.RSSkb,
			MemPct:      proc.MemPct,
			CPURaw:      proc.CPURaw,
			ContainerID: proc.ContainerID,
		})
	}

	report.ContainersActive = parsed.DockerActive
	report.Timestamp = time.Now() // Agregar un timestamp actual

	return report, nil
}
