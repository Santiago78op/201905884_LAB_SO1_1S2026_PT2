package sink

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

/*
*
  - Key define en qué lista de Valkey se guarda (meminfo o continfo)
  - RPush agrega al final de la lista — mantiene el orden cronológico
  - NewValkeyWriter crea el cliente con la dirección del servidor
*/
type ValkeyWriter struct {
	Client *redis.Client
	Key    string
}

func NewValkeyWriter(addr string, key string) *ValkeyWriter {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	return &ValkeyWriter{
		Client: client,
		Key:    key,
	}
}

func (v *ValkeyWriter) Write(data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("valkey: error serializando datos: %w", err)
	}

	if err := v.Client.RPush(context.Background(), v.Key, b).Err(); err != nil {
		return fmt.Errorf("valkey: error escribiendo en %s: %w", v.Key, err)
	}

	return nil
}

// RankEntry es una entrada para el snapshot de ranking en Valkey.
// El member del sorted set codifica PID, ID de contenedor (12 chars) y nombre
// para ser legible como etiqueta en gráficas de pastel de Grafana.
type RankEntry struct {
	Pid      int
	DockerID string  // ID completo del contenedor (se trunca a 12 chars internamente)
	Name     string  // nombre del contenedor Docker
	Score    float64 // valor del score: mem_pct o cpu_pct
}

// ValkeySnapshotWriter reemplaza el sorted set completo en cada tick mediante un
// pipeline atómico (DEL + ZADD). Esto garantiza que no queden entradas obsoletas
// de ticks anteriores, eliminando duplicados en las gráficas de Grafana.
//
// Consulta Grafana: ZREVRANGE <key> 0 4 WITHSCORES → Top 5 sin duplicados.
type ValkeySnapshotWriter struct {
	client *redis.Client
	key    string
}

func NewValkeySnapshotWriter(addr, key string) *ValkeySnapshotWriter {
	return &ValkeySnapshotWriter{
		client: redis.NewClient(&redis.Options{Addr: addr}),
		key:    key,
	}
}

// member construye el string identificador único para el sorted set.
// Formato: "<name> (<cid12>) PID:<pid>" — legible como etiqueta en Grafana.
func rankMember(e RankEntry) string {
	cid := e.DockerID
	if len(cid) > 12 {
		cid = cid[:12]
	}
	return fmt.Sprintf("%s (%s) PID:%d", e.Name, cid, e.Pid)
}

// Replace elimina el sorted set anterior y escribe los entries del tick actual.
// Usar para vistas de "estado actual" donde no se necesita historial.
func (v *ValkeySnapshotWriter) Replace(entries []RankEntry) error {
	pipe := v.client.Pipeline()
	pipe.Del(context.Background(), v.key)
	for _, e := range entries {
		pipe.ZAdd(context.Background(), v.key, redis.Z{
			Score:  e.Score,
			Member: rankMember(e),
		})
	}
	if _, err := pipe.Exec(context.Background()); err != nil {
		return fmt.Errorf("valkey: snapshot %s: %w", v.key, err)
	}
	return nil
}

// Upsert agrega o actualiza entries en el sorted set sin borrar entradas anteriores.
// Usar para rankings históricos: acumula activos e inactivos/eliminados de todos los ticks.
// La unicidad está garantizada por el member que incluye el container ID (no el nombre).
func (v *ValkeySnapshotWriter) Upsert(entries []RankEntry) error {
	if len(entries) == 0 {
		return nil
	}
	pipe := v.client.Pipeline()
	for _, e := range entries {
		pipe.ZAdd(context.Background(), v.key, redis.Z{
			Score:  e.Score,
			Member: rankMember(e),
		})
	}
	if _, err := pipe.Exec(context.Background()); err != nil {
		return fmt.Errorf("valkey: upsert %s: %w", v.key, err)
	}
	return nil
}
