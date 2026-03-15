package parser

/*
* fmt: para envolver errores con mensaje informativo.
* strconv: para convertir cadenas a números, como parsear el valor de memoria.
* strings: para manipular cadenas, como dividir líneas y extraer campos.
* time: para manejar timestamps, si es necesario agregar un timestamp a los datos parseados.
* model: para usar las estructuras definidas en el paquete model, como MemInfoData.
 */
import (
	"encoding/json"
	"fmt"
	"time"

	model "daemon/internal/model"
)

/**
El archivo tiene este formato:
  RAM_TOTAL_MB=8192
  RAM_FREE_MB=4096
  RAM_USED_MB=4096
*/

/*
 *Recibe el texto crudo (lo que devolvió FileReader.Read()), retorna el struct listo o un error.
 */
func ParseMemInfo(raw string) (model.MemStats, error) {
	var parsed model.JsonMemInfo

	// Deserializar el JSON directamente
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return model.MemStats{}, fmt.Errorf("error parsing JSON: %v", err)
	}

	// Mapear los datos parseados a MemStats
	memStats := model.MemStats{
		MemTotal:  parsed.MemTotal,
		MemFree:   parsed.MemFree,
		MemUsed:   parsed.MemUsed,
		Timestamp: time.Now(), // Agregar un timestamp actual
	}

	return memStats, nil
}
