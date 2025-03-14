package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"liveops/internal/db"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisCache() (*RedisCache, error) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}

	ttlStr := os.Getenv("REDIS_TTL")
	ttl := 3600 // default 1 hour
	if ttlStr != "" {
		if val, err := strconv.Atoi(ttlStr); err == nil {
			ttl = val
		}
	}

	client := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %v", err)
	}

	return &RedisCache{
		client: client,
		ttl:    time.Duration(ttl) * time.Second,
	}, nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %v", err)
	}

	return c.client.Set(ctx, key, data, c.ttl).Err()
}

func (c *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("key not found: %s", key)
		}
		return fmt.Errorf("failed to get value: %v", err)
	}

	return json.Unmarshal(data, dest)
}

func (c *RedisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

func (c *RedisCache) SetEvent(ctx context.Context, event *db.Event) error {
	return c.Set(ctx, fmt.Sprintf("event:%s", event.ID), event)
}

func (c *RedisCache) GetEvent(ctx context.Context, id string) (*db.Event, error) {
	var event db.Event
	err := c.Get(ctx, fmt.Sprintf("event:%s", id), &event)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (c *RedisCache) DeleteEvent(ctx context.Context, id string) error {
	return c.Delete(ctx, fmt.Sprintf("event:%s", id))
}

func (c *RedisCache) SetActiveEvents(ctx context.Context, events []db.Event) error {
	return c.Set(ctx, "active_events", events)
}

func (c *RedisCache) GetActiveEvents(ctx context.Context) ([]db.Event, error) {
	var events []db.Event
	err := c.Get(ctx, "active_events", &events)
	if err != nil {
		return nil, err
	}
	return events, nil
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}
