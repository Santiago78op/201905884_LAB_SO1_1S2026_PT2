package docker

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"os/exec"
	"sort"
	"strings"

	"daemon/internal/model"
)

// Category clasifica el tipo de contenedor gestionado.
type Category int

const (
	CategorySystem         Category = iota // grafana, valkey — nunca tocar
	CategoryLowConsumption                  // alpine sleep 240
	CategoryHighConsumption                 // go-client o alpine stress
	CategoryUnknown
)

// Invariantes: cuántos contenedores de cada tipo deben existir siempre.
const (
	TargetLow  = 3
	TargetHigh = 2
)

// Container representa un contenedor en ejecución con sus métricas enriquecidas.
type Container struct {
	ID       string
	Image    string
	Command  string
	Name     string
	Category Category
	// Métricas desde /proc/continfo (enriquecidas en Enforce)
	RSSkb  uint64
	VSZkb  uint64
	MemPct uint64
	CPURaw uint64
}

// EnforceResult resume el resultado de aplicar los invariantes en un tick.
type EnforceResult struct {
	Removed           int         // contenedores eliminados en este tick
	ActiveLow         int         // bajo consumo activos tras enforce
	ActiveHigh        int         // alto consumo activos tras enforce
	Exited            int         // contenedores en estado Exited (terminaron solos, no removidos)
	ActiveContainers  []Container // contenedores activos tras enforce (para ranking)
	RemovedContainers []Container // contenedores eliminados en este tick (para ranking)
}

// Manager gestiona el ciclo de vida de los contenedores Docker del daemon.
type Manager struct{}

// NewManager crea un nuevo Manager.
func NewManager() *Manager {
	return &Manager{}
}

// list obtiene los contenedores actualmente en ejecución vía docker ps.
func (m *Manager) list() ([]Container, error) {
	out, err := exec.Command(
		"docker", "ps", "--no-trunc",
		"--format", "{{.ID}}\t{{.Image}}\t{{.Command}}\t{{.Names}}",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}

	var containers []Container
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "\t", 4)
		if len(parts) < 4 {
			continue
		}
		c := Container{
			ID:      parts[0],
			Image:   parts[1],
			Command: parts[2],
			Name:    strings.TrimPrefix(parts[3], "/"),
		}
		c.Category = classify(c)
		containers = append(containers, c)
	}
	return containers, nil
}

// classify determina la categoría de un contenedor por imagen y comando.
func classify(c Container) Category {
	image := strings.ToLower(c.Image)
	cmd := strings.ToLower(c.Command)
	name := strings.ToLower(c.Name)

	// Contenedores de sistema — nunca eliminar
	if strings.Contains(image, "grafana") ||
		strings.Contains(image, "valkey") ||
		strings.Contains(image, "redis") ||
		strings.Contains(name, "grafana") ||
		strings.Contains(name, "valkey") {
		return CategorySystem
	}

	// Alto consumo RAM: go-client
	if strings.Contains(image, "go-client") {
		return CategoryHighConsumption
	}

	// Alpine: distinguir por comando
	if strings.Contains(image, "alpine") {
		if strings.Contains(cmd, "sleep") {
			return CategoryLowConsumption
		}
		// bc / while → stress test → alto consumo CPU
		return CategoryHighConsumption
	}

	return CategoryUnknown
}

// resourceScore combina RSS y CPU como métrica de ordenamiento.
func resourceScore(c Container) uint64 {
	return c.RSSkb + c.CPURaw
}

// sortDesc ordena contenedores de mayor a menor consumo de recursos.
func sortDesc(containers []Container) {
	sort.Slice(containers, func(i, j int) bool {
		return resourceScore(containers[i]) > resourceScore(containers[j])
	})
}

// enrichWithMetrics vincula métricas de /proc/continfo a cada contenedor Docker.
// El kernel retorna los primeros 12 chars del container ID en el campo container_id.
func enrichWithMetrics(containers []Container, processes []model.ProcessInfo) []Container {
	type agg struct{ rss, vsz, memPct, cpu uint64 }
	metrics := make(map[string]agg)

	for _, p := range processes {
		cid := p.ContainerID
		if cid == "" || cid == "-" {
			continue
		}
		a := metrics[cid]
		a.rss += p.RSSkb
		a.vsz += p.VSZkb
		a.memPct += p.MemPct
		a.cpu += p.CPURaw
		metrics[cid] = a
	}

	for i, c := range containers {
		prefix := c.ID
		if len(prefix) > 12 {
			prefix = prefix[:12]
		}
		if a, ok := metrics[prefix]; ok {
			containers[i].RSSkb = a.rss
			containers[i].VSZkb = a.vsz
			containers[i].MemPct = a.memPct
			containers[i].CPURaw = a.cpu
		}
	}
	return containers
}

// countExited cuenta contenedores gestionados (no sistema) en estado Exited.
// Estos son contenedores que terminaron solos (ej: alpine sleep 240 tras 4 minutos).
func (m *Manager) countExited() int {
	out, err := exec.Command(
		"docker", "ps", "-a", "--no-trunc",
		"--filter", "status=exited",
		"--format", "{{.ID}}\t{{.Image}}\t{{.Command}}\t{{.Names}}",
	).Output()
	if err != nil {
		return 0
	}

	count := 0
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "\t", 4)
		if len(parts) < 4 {
			continue
		}
		c := Container{ID: parts[0], Image: parts[1], Command: parts[2], Name: strings.TrimPrefix(parts[3], "/")}
		if cat := classify(c); cat == CategoryLowConsumption || cat == CategoryHighConsumption {
			count++
		}
	}
	return count
}

// stopAndRemove detiene y elimina un contenedor por ID.
// -t 2: espera máximo 2s antes de forzar SIGKILL (evita bloqueos de 10s por tick).
func (m *Manager) stopAndRemove(id string) error {
	if err := exec.Command("docker", "stop", "-t", "2", id).Run(); err != nil {
		return fmt.Errorf("docker stop: %w", err)
	}
	if err := exec.Command("docker", "rm", id).Run(); err != nil {
		return fmt.Errorf("docker rm: %w", err)
	}
	return nil
}

// createLow crea un contenedor de bajo consumo (alpine sleep 240).
func (m *Manager) createLow() error {
	return exec.Command("docker", "run", "-d", "alpine", "sleep", "240").Run()
}

// createHigh crea un contenedor de alto consumo aleatorio:
// 50% go-client (RAM) y 50% alpine stress (CPU).
func (m *Manager) createHigh() error {
	if rand.Intn(2) == 0 {
		return exec.Command("docker", "run", "-d", "roldyoran/go-client").Run()
	}
	return exec.Command(
		"docker", "run", "-d", "alpine", "sh", "-c",
		"while true; do echo '2^20' | bc > /dev/null; sleep 2; done",
	).Run()
}

// Enforce aplica los invariantes de contenedores:
//   - Siempre 3 contenedores de bajo consumo
//   - Siempre 2 contenedores de alto consumo
//   - Nunca elimina Grafana ni Valkey
//
// Estrategia de selección al haber exceso:
//   - Bajo consumo:  elimina los de MAYOR consumo (están fuera de su categoría)
//   - Alto consumo:  elimina los de MENOR consumo (mantiene los más activos)
func (m *Manager) Enforce(processes []model.ProcessInfo) (EnforceResult, error) {
	containers, err := m.list()
	if err != nil {
		return EnforceResult{}, err
	}
	containers = enrichWithMetrics(containers, processes)

	var low, high []Container
	for _, c := range containers {
		switch c.Category {
		case CategoryLowConsumption:
			low = append(low, c)
		case CategoryHighConsumption:
			high = append(high, c)
		}
	}

	result := EnforceResult{}

	// --- Bajo consumo: mantener los TargetLow de MENOR consumo ---
	if len(low) > TargetLow {
		sortDesc(low) // [mayor consumo ... menor consumo]
		for _, c := range low[:len(low)-TargetLow] {
			if err := m.stopAndRemove(c.ID); err != nil {
				log.Printf("docker: error eliminando [bajo] %s: %v", c.ID[:12], err)
				continue
			}
			log.Printf("docker: eliminado [bajo] %s (%s)", c.ID[:12], c.Image)
			result.Removed++
			result.RemovedContainers = append(result.RemovedContainers, c)
		}
	}

	// --- Alto consumo: mantener los TargetHigh de MAYOR consumo ---
	if len(high) > TargetHigh {
		sortDesc(high) // [mayor consumo ... menor consumo]
		for _, c := range high[TargetHigh:] {
			if err := m.stopAndRemove(c.ID); err != nil {
				log.Printf("docker: error eliminando [alto] %s: %v", c.ID[:12], err)
				continue
			}
			log.Printf("docker: eliminado [alto] %s (%s)", c.ID[:12], c.Image)
			result.Removed++
			result.RemovedContainers = append(result.RemovedContainers, c)
		}
	}

	// Activos reales tras las eliminaciones
	lowActive := len(low)
	if lowActive > TargetLow {
		lowActive = TargetLow
	}
	highActive := len(high)
	if highActive > TargetHigh {
		highActive = TargetHigh
	}

	// Registrar contenedores activos que sobreviven (para ranking)
	if len(low) > TargetLow {
		// low está ordenado desc; los últimos lowActive son los de menor consumo (los que se mantienen)
		result.ActiveContainers = append(result.ActiveContainers, low[len(low)-lowActive:]...)
	} else {
		result.ActiveContainers = append(result.ActiveContainers, low...)
	}
	if len(high) > TargetHigh {
		// high está ordenado desc; los primeros highActive son los de mayor consumo (los que se mantienen)
		result.ActiveContainers = append(result.ActiveContainers, high[:highActive]...)
	} else {
		result.ActiveContainers = append(result.ActiveContainers, high...)
	}

	// --- Crear contenedores faltantes para cumplir mínimos ---
	for i := lowActive; i < TargetLow; i++ {
		if err := m.createLow(); err != nil {
			log.Printf("docker: error creando [bajo]: %v", err)
		} else {
			log.Println("docker: creado [bajo] alpine sleep 240")
			lowActive++
		}
	}

	for i := highActive; i < TargetHigh; i++ {
		if err := m.createHigh(); err != nil {
			log.Printf("docker: error creando [alto]: %v", err)
		} else {
			log.Println("docker: creado [alto]")
			highActive++
		}
	}

	result.ActiveLow = lowActive
	result.ActiveHigh = highActive
	result.Exited = m.countExited()
	return result, nil
}
