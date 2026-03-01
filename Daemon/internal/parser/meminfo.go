package parser

/*
* fmt: para envolver errores con mensaje informativo.
* strconv: para convertir cadenas a números, como parsear el valor de memoria.
* strings: para manipular cadenas, como dividir líneas y extraer campos.
* time: para manejar timestamps, si es necesario agregar un timestamp a los datos parseados.
* model: para usar las estructuras definidas en el paquete model, como MemInfoData.
 */
import (
	"fmt"
	"strconv"
	"strings"
	"time"

	model "Daemon/internal/model"
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
	// Se inicializa una variable MemStats para almacenar los datos parseados.
	var memStats model.MemStats

	// Se divide el texto crudo en líneas usando strings.Split.
	lines := strings.Split(raw, "\n")

	// Para cada línea, se usa strings.SplitN(linea, "=", 2) — el 2 limita a máximo 2 partes, importante si algún valor pudiera contener =
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // Si la línea no tiene el formato esperado, se ignora.
		}

		// Se aplica strings.TrimSpace a cada parte para eliminar espacios en blanco alrededor de la clave y el valor.
		key := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])

		// Switch sobre la clave, parsea cada valor con strconv.ParseUint(valor, 10, 64) y asígnalo al campo correcto
		switch key {
		case "RAM_TOTAL_MB":
			value, err := strconv.ParseUint(valueStr, 10, 64)
			if err != nil {
				return memStats, fmt.Errorf("error parsing RAM_TOTAL_MB: %w", err)
			}
			memStats.MemTotal = value
		case "RAM_FREE_MB":
			value, err := strconv.ParseUint(valueStr, 10, 64)
			if err != nil {
				return memStats, fmt.Errorf("error parsing RAM_FREE_MB: %w", err)
			}
			memStats.MemFree = value
		case "RAM_USED_MB":
			value, err := strconv.ParseUint(valueStr, 10, 64)
			if err != nil {
				return memStats, fmt.Errorf("error parsing RAM_USED_MB: %w", err)
			}
			memStats.MemUsed = value
		}
	}

	// Se agrega un timestamp al struct, si es necesario.
	memStats.Timestamp = time.Now()

	// Valida que MemTotal != 0 para evitar divisiones por cero o datos inválidos.
	if memStats.MemTotal == 0 {
		return memStats, fmt.Errorf("RAM_TOTAL_MB cannot be zero")
	}

	return memStats, nil

}
