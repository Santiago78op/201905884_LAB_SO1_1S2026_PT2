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
