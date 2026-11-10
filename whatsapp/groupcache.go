package whatsapp

import (
	"context"
	"sync"
	"time"

	"main/http/models"
)

// GroupCache cachea la lista de grupos y newsletters para evitar consultas
// repetidas a WhatsApp. La implementación por defecto es en memoria; se puede
// reemplazar por una basada en Redis sin tocar la lógica de la API.
type GroupCache interface {
	Get(ctx context.Context) ([]models.GroupItem, bool, error)
	Set(ctx context.Context, groups []models.GroupItem) error
	Invalidate(ctx context.Context) error
}

// memoryGroupCache es la implementación por defecto en memoria con TTL.
type memoryGroupCache struct {
	mu      sync.RWMutex
	groups  []models.GroupItem
	expires time.Time
	ttl     time.Duration
}

// NewMemoryGroupCache crea un cache en memoria con el TTL dado.
func NewMemoryGroupCache(ttl time.Duration) *memoryGroupCache {
	return &memoryGroupCache{ttl: ttl}
}

func (c *memoryGroupCache) Get(ctx context.Context) ([]models.GroupItem, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if time.Now().After(c.expires) || len(c.groups) == 0 {
		return nil, false, nil
	}
	out := make([]models.GroupItem, len(c.groups))
	copy(out, c.groups)
	return out, true, nil
}

func (c *memoryGroupCache) Set(ctx context.Context, groups []models.GroupItem) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.groups = groups
	c.expires = time.Now().Add(c.ttl)
	return nil
}

func (c *memoryGroupCache) Invalidate(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.groups = nil
	c.expires = time.Time{}
	return nil
}
