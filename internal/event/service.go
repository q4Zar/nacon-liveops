package event

import (
	"context"
	"database/sql"
	"encoding/json"
	"liveops/api"
	"liveops/internal/db"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Service struct {
	repo   db.EventRepository
	logger *zap.Logger
}

func NewService(repo db.EventRepository, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// Convert db.Event to api.EventResponse
func toEventResponse(e db.Event) *api.EventResponse {
	return &api.EventResponse{
		Id:          e.ID,
		Title:       e.Title,
		Description: e.Description,
		StartTime:   e.StartTime.AsTime().Unix(),
		EndTime:     e.EndTime.AsTime().Unix(),
		Rewards:     e.Rewards,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

// Convert api.EventRequest to db.Event
func toDBEvent(req *api.EventRequest) db.Event {
	return db.Event{
		ID:          req.Id,
		Title:       req.Title,
		Description: req.Description,
		StartTime:   timestamppb.New(time.Unix(req.StartTime, 0)),
		EndTime:     timestamppb.New(time.Unix(req.EndTime, 0)),
		Rewards:     req.Rewards,
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
func (s *Service) GetEvent(ctx context.Context, id string) (*api.EventResponse, error) {
	s.logger.Debug("Getting event by ID", zap.String("id", id))
	event, err := s.repo.GetEvent(id)
	if err != nil {
		s.logger.Error("Failed to get event", zap.String("id", id), zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get event")
	}
	return toEventResponse(event), nil
}

// CreateEvent creates a new event
func (s *Service) CreateEvent(ctx context.Context, req *api.EventRequest) (*api.EventResponse, error) {
	s.logger.Debug("Creating event")
	dbEvent := toDBEvent(req)

	if err := s.repo.CreateEvent(dbEvent); err != nil {
		s.logger.Error("Failed to create event", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create event")
	}

	return toEventResponse(dbEvent), nil
}

// UpdateEvent updates an existing event
func (s *Service) UpdateEvent(ctx context.Context, req *api.EventRequest) (*api.EventResponse, error) {
	s.logger.Debug("Updating event", zap.String("id", req.Id))
	dbEvent := toDBEvent(req)

	if err := s.repo.UpdateEvent(dbEvent); err != nil {
		s.logger.Error("Failed to update event", zap.String("id", req.Id), zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to update event")
	}

	return toEventResponse(dbEvent), nil
}

// DeleteEvent deletes an event by ID
func (s *Service) DeleteEvent(ctx context.Context, req *api.DeleteRequest) (*api.Empty, error) {
	s.logger.Debug("Deleting event", zap.String("id", req.Id))
	if err := s.repo.DeleteEvent(req.Id); err != nil {
		s.logger.Error("Failed to delete event", zap.String("id", req.Id), zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to delete event")
	}

	return &api.Empty{}, nil
}

// ListEvents returns all events
func (s *Service) ListEvents(ctx context.Context, req *api.Empty) (*api.EventsResponse, error) {
	s.logger.Debug("Listing all events")
	events, err := s.repo.ListEvents()
	if err != nil {
		s.logger.Error("Failed to list events", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list events")
	}

	apiEvents := make([]*api.EventResponse, len(events))
	for i, event := range events {
		apiEvents[i] = toEventResponse(event)
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