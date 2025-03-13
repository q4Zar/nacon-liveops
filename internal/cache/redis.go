package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"liveops/api"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	eventKeyPrefix = "event:"
	eventsListKey  = "events:list"
	eventTTL       = 24 * time.Hour
)

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache() (*RedisCache, error) {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "localhost"
	}

	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}

	password := os.Getenv("REDIS_PASSWORD")

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", host, port),
		Password: password,
		DB:       0,
	})

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %v", err)
	}

	return &RedisCache{client: client}, nil
}

func (c *RedisCache) SetEvent(ctx context.Context, event *api.EventResponse) error {
	key := eventKeyPrefix + event.Id
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %v", err)
	}

	if err := c.client.Set(ctx, key, data, eventTTL).Err(); err != nil {
		return fmt.Errorf("failed to set event in cache: %v", err)
	}

	// Update events list
	if err := c.client.SAdd(ctx, eventsListKey, event.Id).Err(); err != nil {
		return fmt.Errorf("failed to update events list: %v", err)
	}

	return nil
}

func (c *RedisCache) GetEvent(ctx context.Context, id string) (*api.EventResponse, error) {
	key := eventKeyPrefix + id
	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get event from cache: %v", err)
	}

	var event api.EventResponse
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event: %v", err)
	}

	return &event, nil
}

func (c *RedisCache) GetEvents(ctx context.Context) ([]*api.EventResponse, error) {
	// Get all event IDs from the set
	ids, err := c.client.SMembers(ctx, eventsListKey).Result()
	if err == redis.Nil {
		return []*api.EventResponse{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get event IDs from cache: %v", err)
	}

	// Get all events
	var events []*api.EventResponse
	for _, id := range ids {
		event, err := c.GetEvent(ctx, id)
		if err != nil {
			return nil, err
		}
		if event != nil {
			events = append(events, event)
		}
	}

	return events, nil
}

func (c *RedisCache) DeleteEvent(ctx context.Context, id string) error {
	key := eventKeyPrefix + id
	if err := c.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete event from cache: %v", err)
	}

	// Remove from events list
	if err := c.client.SRem(ctx, eventsListKey, id).Err(); err != nil {
		return fmt.Errorf("failed to remove event from events list: %v", err)
	}

	return nil
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}
