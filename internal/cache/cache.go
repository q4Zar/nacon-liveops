package cache

import (
	"context"
	"liveops/api"
)

// Cache defines the interface for event caching operations
type Cache interface {
	// SetEvent stores an event in the cache
	SetEvent(ctx context.Context, event *api.EventResponse) error

	// GetEvent retrieves an event from the cache by ID
	GetEvent(ctx context.Context, id string) (*api.EventResponse, error)

	// GetEvents retrieves all events from the cache
	GetEvents(ctx context.Context) ([]*api.EventResponse, error)

	// DeleteEvent removes an event from the cache
	DeleteEvent(ctx context.Context, id string) error

	// Close closes the cache connection
	Close() error
}
