package app

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// RegisterCronjob registra una entrada en el crontab del SO que ejecuta scriptPath
// cada 2 minutos. La ruta se resuelve a absoluta para que cron la encuentre.
func RegisterCronjob(scriptPath string) error {
	abs, err := filepath.Abs(scriptPath)
	if err != nil {
		return fmt.Errorf("cronjob: ruta inválida: %w", err)
	}

	existing, _ := exec.Command("crontab", "-l").Output()

	// Evitar duplicados si el daemon se reinicia
	if strings.Contains(string(existing), abs) {
		return nil
	}

	entry := fmt.Sprintf("*/2 * * * * %s\n", abs)
	newCrontab := string(existing) + entry

	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(newCrontab)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("crontab register: %w", err)
	}
	return nil
}

// RemoveCronjob elimina del crontab la entrada que referencia scriptPath.
// Llamar antes de finalizar el daemon (paso 5 del enunciado).
func RemoveCronjob(scriptPath string) error {
	abs, err := filepath.Abs(scriptPath)
	if err != nil {
		return nil
	}

	out, err := exec.Command("crontab", "-l").Output()
	if err != nil {
		return nil // no hay crontab, nada que eliminar
	}

	var filtered []string
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, abs) {
			filtered = append(filtered, line)
		}
	}

	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(strings.Join(filtered, "\n"))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("crontab remove: %w", err)
	}
	return nil
}
