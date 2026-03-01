package source

/*
* context: para poder cancerlar la lectura del archivo si el demon se detiene.
* fmt: para envolver errores con mensaje informativo.
* os: para abrir el archivo y leer su contenido, con os.ReadFile, que es una función
*	  conveniente para leer todo el contenido de un archivo en una sola llamada.
 */
import (
	"context"
	"fmt"
	"os"
)

/**
* Reader es una interfaz que define los métodos que debe implementar cualquier tipo de lector de archivos.
 */
type Reader interface {
	// Name retorna el nombre del reader, que en este caso es la ruta del archivo.
	Name() string
	// Read lee el contenido del archivo y retorna los datos como un slice de bytes.
	Read(ctx context.Context) ([]byte, error)
}

/**
* Esta línea no hace nada en runtime. Solo le dice al compilador: "
* si FileReader no implementa Reader, falla aquí con un mensaje claro".
 */
var _ Reader = FileReader{} // Asegura que FileReader implementa la interfaz Reader

/**
* FileReader es una estructura que representa un lector de archivos. Contiene un campo Path,
* que es la ruta del archivo que se va a leer.
 */
type FileReader struct {
	// Path es la ruta del archivo que se va a leer.
	Path string
}

/**
* Metodo
* Retorna la ruta. Sirve para identificar el lector en logs.
* Ejemplo: "error en reader /proc/meminfo_pr2_so1_201905884".
 */
func (r FileReader) Name() string {
	return r.Path
}

/**
* Metodo
* Lee el contenido del archivo especificado en el campo Path de FileReader.
* Utiliza el contexto para permitir la cancelación de la lectura si el demon se detiene.
* Retorna los datos leídos como un slice de bytes y un error si ocurre algún problema durante la lectura.
 */
func (r FileReader) Read(ctx context.Context) ([]byte, error) {
	// Revisar si el contexto ha sido cancelado antes de intentar leer el archivo.
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("lectura cancelada para el archivo %s: %w", r.Path, ctx.Err())
	default:
	}

	// Lee todo el archivo en memoria
	data, err := os.ReadFile(r.Path)
	if err != nil {
		// Envuelve el error con un mensaje informativo que incluye la ruta del archivo.
		return nil, fmt.Errorf("error al leer el archivo %s: %w", r.Path, err)
	}

	// Retorna los datos leídos y nil para indicar que no hubo error.
	return data, nil
}
