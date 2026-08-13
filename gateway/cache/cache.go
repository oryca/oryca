package cache

import "github.com/redis/go-redis/v9"

type Cache struct {
	client *redis.Client
}

func New(client *redis.Client) *Cache {
	return &Cache{client: client}
}
