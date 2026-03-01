package source

import (
	"context"
	"time"
)

/**
 * Reader es una interface que define el comportamiento de un lector de archivos.
 * Cualquier tipo que implemente esta interfaz debe proporcionar una implementación del método Read,
 * que lee datos de una fuente específica y devuelve un slice de bytes junto con un error
 * si ocurre algún problema durante la lectura. Además, debe implementar el método Name,
 * que devuelve el nombre del lector.
 */
type Reader interface {
	Read(ctx context.Context) ([]byte, error)
	Name() string
}

/**
 * FileReader es una estructura que implementa la interfaz Reader para leer datos de un archivo.
 * Contiene el campo path, que especifica la ruta del archivo a leer, y el campo Timeout,
 * que define el tiempo máximo permitido para realizar la lectura antes de que se considere un error por tiempo de espera.
 */
type FileReader struct {
	path    string
	Timeout time.Duration
}
