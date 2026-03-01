package sink

/**
* encoding/json: para convertir estructuras de datos a formato JSON antes de escribir en el archivo.
* fmt: para envolver errores con mensaje informativo.
* os: para manejar la creación y escritura de archivos.
 */
import (
	"encoding/json"
	"fmt"
	"os"
)

/*
*
  - El metodo Write de JSONLineFile es responsable de escribir un dato en formato JSON en un archivo.
  - Cada dato se escribe como una línea JSON, lo que facilita la lectura posterior del archivo.
  - El método realiza los siguientes pasos:
  - 1. Abre el archivo en modo append, para agregar cada nuevo dato al final del archivo sin sobrescribir lo anterior.
    Si el archivo no existe, se crea automáticamente.
  - 2. Convierte el dato a formato JSON usando json.Marshal.
    Si ocurre un error durante la conversión, se envuelve con un mensaje informativo.
  - 3. Escribe la línea JSON en el archivo, seguida de un salto de línea.
    Si ocurre un error durante la escritura, se devuelve el error.
*/
func (s JSONLineFile) Write(v any) error {
	// Se abre el archivo en modo append, para agregar cada nuevo dato al final del archivo sin sobrescribir lo anterior.
	f, err := os.OpenFile(s.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open sink file %s: %w", s.Path, err)
	}
	defer f.Close()

	// Se convierte el dato a formato JSON usando json.Marshal. Si ocurre un error durante la conversión, se envuelve con un mensaje informativo.
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}

	// Se escribe la línea JSON en el archivo, seguida de un salto de línea. Si ocurre un error durante la escritura, se devuelve el error.
	_, err = f.Write(append(b, '\n'))
	return err
}

/**
* Writer es una interfaz que define un método Write(v any) error, que acepta cualquier tipo de dato y
* devuelve un error si ocurre algún problema durante la escritura.
* Any: es un alias para interface{}, lo que significa que el método Write puede aceptar cualquier tipo de dato.
 */
type Writer interface {
	Write(v any) error
}

// JSONLineFile es una estructura que implementa la interfaz Writer, escribiendo cada dato como una línea JSON en un archivo.
var _ Writer = JSONLineFile{}

type JSONLineFile struct {
	Path string
}
