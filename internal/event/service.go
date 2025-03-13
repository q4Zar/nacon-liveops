package event

import (
	"context"
	"database/sql"
	"encoding/json"
	"liveops/api"
	"liveops/internal/cache"
	"liveops/internal/db"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Service struct {
	repo   db.EventRepository
	cache  cache.Cache
	logger *zap.Logger
}

func NewService(repo db.EventRepository, cache cache.Cache, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		cache:  cache,
		logger: logger,
	}
}

// Convert db.Event to api.EventResponse
func toEventResponse(e db.Event) *api.EventResponse {
	return &api.EventResponse{
		Id:          e.ID,
		Title:       e.Title,
		Description: e.Description,
		StartTime:   e.StartTimeUnix,
		EndTime:     e.EndTimeUnix,
		Rewards:     e.Rewards,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

// Convert api.EventRequest to db.Event
func toDBEvent(req *api.EventRequest) db.Event {
	return db.Event{
		ID:            req.Id,
		Title:         req.Title,
		Description:   req.Description,
		StartTimeUnix: req.StartTime,
		EndTimeUnix:   req.EndTime,
		Rewards:       req.Rewards,
	}
}

// GetActiveEvents returns all active events
func (s *Service) GetActiveEvents(ctx context.Context) (*api.EventsResponse, error) {
	s.logger.Debug("Getting active events")
	events, err := s.repo.GetActiveEvents()
	if err != nil {
		s.logger.Error("Failed to get active events", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get active events")
	}

	apiEvents := make([]*api.EventResponse, len(events))
	for i, event := range events {
		apiEvents[i] = toEventResponse(event)
	}
	return &api.EventsResponse{Events: apiEvents}, nil
}

// GetEvent returns a specific event by ID
func (s *Service) GetEvent(ctx context.Context, req *api.DeleteRequest) (*api.EventResponse, error) {
	s.logger.Debug("Getting event by ID", zap.String("id", req.Id))

	// Try to get from cache first
	event, err := s.cache.GetEvent(ctx, req.Id)
	if err != nil {
		s.logger.Error("Failed to get event from cache", zap.Error(err))
		// Continue with database lookup
	} else if event != nil {
		return event, nil
	}

	// If not in cache, get from database
	dbEvent, err := s.repo.GetEvent(req.Id)
	if err == sql.ErrNoRows {
		s.logger.Info("Event not found", zap.String("id", req.Id))
		return nil, status.Error(codes.NotFound, "event not found")
	}
	if err != nil {
		s.logger.Error("Failed to fetch event", zap.String("id", req.Id), zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get event")
	}

	eventResponse := toEventResponse(dbEvent)

	// Cache the event for future requests
	if err := s.cache.SetEvent(ctx, eventResponse); err != nil {
		s.logger.Error("Failed to cache event", zap.Error(err))
		// Don't fail the request if caching fails
	}

	return eventResponse, nil
}

// CreateEvent creates a new event
func (s *Service) CreateEvent(ctx context.Context, req *api.EventRequest) (*api.EventResponse, error) {
	s.logger.Debug("Creating new event", zap.Any("request", req))

	dbEvent := toDBEvent(req)
	if err := s.repo.CreateEvent(dbEvent); err != nil {
		s.logger.Error("Failed to create event", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create event")
	}

	eventResponse := toEventResponse(dbEvent)

	// Cache the event
	if err := s.cache.SetEvent(ctx, eventResponse); err != nil {
		s.logger.Error("Failed to cache event", zap.Error(err))
		// Don't fail the request if caching fails
	}

	return eventResponse, nil
}

// UpdateEvent updates an existing event
func (s *Service) UpdateEvent(ctx context.Context, req *api.EventRequest) (*api.EventResponse, error) {
	s.logger.Debug("Updating event", zap.String("id", req.Id), zap.Any("request", req))

	dbEvent := toDBEvent(req)
	if err := s.repo.UpdateEvent(dbEvent); err != nil {
		s.logger.Error("Failed to update event", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to update event")
	}

	eventResponse := toEventResponse(dbEvent)

	// Update cache
	if err := s.cache.SetEvent(ctx, eventResponse); err != nil {
		s.logger.Error("Failed to update event in cache", zap.Error(err))
		// Don't fail the request if caching fails
	}

	return eventResponse, nil
}

// DeleteEvent deletes an event by ID
func (s *Service) DeleteEvent(ctx context.Context, req *api.DeleteRequest) error {
	s.logger.Debug("Deleting event", zap.String("id", req.Id))

	if err := s.repo.DeleteEvent(req.Id); err != nil {
		s.logger.Error("Failed to delete event", zap.Error(err))
		return status.Error(codes.Internal, "failed to delete event")
	}

	// Delete from cache
	if err := s.cache.DeleteEvent(ctx, req.Id); err != nil {
		s.logger.Error("Failed to delete event from cache", zap.Error(err))
		// Don't fail the request if cache deletion fails
	}

	return nil
}

// ListEvents returns all events
func (s *Service) ListEvents(ctx context.Context, req *api.Empty) (*api.EventsResponse, error) {
	s.logger.Debug("Listing all events")

	// Try to get from cache first
	events, err := s.cache.GetEvents(ctx)
	if err != nil {
		s.logger.Error("Failed to get events from cache", zap.Error(err))
		// Continue with database lookup
	} else if len(events) > 0 {
		return &api.EventsResponse{Events: events}, nil
	}

	// If not in cache, get from database
	dbEvents, err := s.repo.ListEvents()
	if err != nil {
		s.logger.Error("Failed to list events", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list events")
	}

	// Convert database events to API responses
	apiEvents := make([]*api.EventResponse, len(dbEvents))
	for i, dbEvent := range dbEvents {
		apiEvents[i] = toEventResponse(dbEvent)
	}

	// Cache each event for future requests
	for _, event := range apiEvents {
		if err := s.cache.SetEvent(ctx, event); err != nil {
			s.logger.Error("Failed to cache event", zap.Error(err))
			// Don't fail the request if caching fails
		}
	}

	return &api.EventsResponse{Events: apiEvents}, nil
}

// HandleGetActiveEvents handles HTTP GET /events
func (s *Service) HandleGetActiveEvents(w http.ResponseWriter, r *http.Request) {
	s.logger.Info("Received GET /events request", zap.String("method", r.Method), zap.String("path", r.URL.Path))
	events, err := s.repo.GetActiveEvents()
	if err != nil {
		s.logger.Error("Failed to fetch active events", zap.Error(err))
		http.Error(w, `{"error": "internal server error"}`, http.StatusInternalServerError)
		return
	}

	var g errgroup.Group
	g.SetLimit(4)
	result := make([][]byte, len(events))

	for i, e := range events {
		i, e := i, e
		g.Go(func() error {
			apiEvent := toEventResponse(e)
			data, err := json.Marshal(apiEvent)
			if err != nil {
				return err
			}
			result[i] = data
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		s.logger.Error("Failed to encode events", zap.Error(err))
		http.Error(w, `{"error": "failed to encode response"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("["))
	for i, data := range result {
		if i > 0 {
			w.Write([]byte(","))
		}
		w.Write(data)
	}
	w.Write([]byte("]"))
	s.logger.Info("Successfully returned active events", zap.Int("count", len(events)))
}

// HandleGetEvent handles HTTP GET /events/{id}
func (s *Service) HandleGetEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.logger.Info("Received GET /events/{id} request", zap.String("method", r.Method), zap.String("path", r.URL.Path), zap.String("id", id))
	if id == "" {
		s.logger.Warn("Missing event ID in request")
		http.Error(w, `{"error": "missing event id"}`, http.StatusBadRequest)
		return
	}

	event, err := s.repo.GetEvent(id)
	if err == sql.ErrNoRows {
		s.logger.Info("Event not found", zap.String("id", id))
		http.Error(w, `{"error": "event not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		s.logger.Error("Failed to fetch event", zap.String("id", id), zap.Error(err))
		http.Error(w, `{"error": "internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	apiEvent := toEventResponse(event)
	if err := json.NewEncoder(w).Encode(apiEvent); err != nil {
		s.logger.Error("Failed to encode event", zap.String("id", id), zap.Error(err))
		http.Error(w, `{"error": "failed to encode response"}`, http.StatusInternalServerError)
		return
	}
	s.logger.Info("Successfully returned event", zap.String("id", id))
}
