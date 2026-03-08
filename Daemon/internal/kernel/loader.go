package kernel

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

type LoadOpts struct {
	ScriptPath  string
	ContainerID string
}

func Load(opts LoadOpts) error {
	// Verifica que el script exista
	if _, err := os.Stat(opts.ScriptPath); os.IsNotExist(err) {
		return fmt.Errorf("kernel: script no encontrado %q: %w", opts.ScriptPath, err)
	}

	// Ejecuta el script con los argumentos necesarios
	args := []string{opts.ScriptPath}
	if opts.ContainerID != "" {
		args = append(args, opts.ContainerID)
	}

	// Ejecuta bash explícitamente, no depende del shebang
	cmd := exec.Command("/bin/bash", args...)
	// Captura stdout y stderr juntos en un slice de bytes; el script termina antes de que esta línea retorne
	out, err := cmd.CombinedOutput()

	// Divide la salida en líneas para que cada echo del script aparezca en el log del daemon
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		// Solo loguea líneas no vacías para evitar logs innecesarios
		if line != "" {
			log.Printf("kernel: %s", line)
		}
	}

	// Si el comando falló, devuelve un error con la salida del script para facilitar el diagnóstico
	if err != nil {
		return fmt.Errorf("kernel: error al ejecutar el script %q: %w\nSalida del script:\n%s", opts.ScriptPath, err, string(out))
	}

	// Si todo salió bien, devuelve nil
	return nil
}
