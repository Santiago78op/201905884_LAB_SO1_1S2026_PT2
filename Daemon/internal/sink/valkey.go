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

// ValkeyRankWriter mantiene un Sorted Set con score=rss_kb y member=docker_id.
// Permite queries por rango de RAM sin duplicados: ZRANGEBYSCORE rss_rank <min> <max>.
type ValkeyRankWriter struct {
	Client *redis.Client
	Key    string
}

func NewValkeyRankWriter(addr string, key string) *ValkeyRankWriter {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	return &ValkeyRankWriter{
		Client: client,
		Key:    key,
	}
}

// Upsert agrega o actualiza el contenedor en el sorted set con su RSS como score.
func (v *ValkeyRankWriter) Upsert(score float64, member string) error {
	if err := v.Client.ZAdd(context.Background(), v.Key, redis.Z{
		Score:  score,
		Member: member,
	}).Err(); err != nil {
		return fmt.Errorf("valkey: zadd %s: %w", v.Key, err)
	}
	return nil
}

// Remove elimina un contenedor del sorted set (cuando es eliminado).
func (v *ValkeyRankWriter) Remove(member string) error {
	if err := v.Client.ZRem(context.Background(), v.Key, member).Err(); err != nil {
		return fmt.Errorf("valkey: zrem %s: %w", v.Key, err)
	}
	return nil
}
