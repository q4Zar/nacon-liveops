package event

import (
    "context"
    "database/sql"
    "encoding/json"
    "liveops/api"
    "liveops/internal/db"
    "net/http"

    "github.com/go-chi/chi/v5"
    "google.golang.org/protobuf/types/known/timestamppb"
)

type Service struct {
    repo *db.EventRepository
}

func NewService(repo *db.EventRepository) *Service {
    return &Service{repo: repo}
}

func (s *Service) GetActiveEvents(w http.ResponseWriter, r *http.Request) {
    events, err := s.repo.GetActiveEvents()
    if err != nil {
        http.Error(w, `{"error": "internal server error"}`, http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    if err := json.NewEncoder(w).Encode(events); err != nil {
        http.Error(w, `{"error": "failed to encode response"}`, http.StatusInternalServerError)
    }
}

func (s *Service) GetEvent(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    if id == "" {
        http.Error(w, `{"error": "missing event id"}`, http.StatusBadRequest)
        return
    }

    event, err := s.repo.GetEvent(id)
    if err == sql.ErrNoRows {
        http.Error(w, `{"error": "event not found"}`, http.StatusNotFound)
        return
    }
    if err != nil {
        http.Error(w, `{"error": "internal server error"}`, http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    if err := json.NewEncoder(w).Encode(event); err != nil {
        http.Error(w, `{"error": "failed to encode response"}`, http.StatusInternalServerError)
    }
}

func (s *Service) CreateEvent(ctx context.Context, req *api.EventRequest) (*api.EventResponse, error) {
    event := db.Event{
        ID:          req.Id,
        Title:       req.Title,
        Description: req.Description,
        StartTime:   req.StartTime,
        EndTime:     req.EndTime,
        Rewards:     req.Rewards,
    }

    if err := s.repo.CreateEvent(event); err != nil {
        return nil, err
    }

    return &api.EventResponse{
        Id:          event.ID,
        Title:       event.Title,
        Description: event.Description,
        StartTime:   event.StartTime,
        EndTime:     event.EndTime,
        Rewards:     event.Rewards,
    }, nil
}

func (s *Service) UpdateEvent(ctx context.Context, req *api.EventRequest) (*api.EventResponse, error) {
    event := db.Event{
        ID:          req.Id,
        Title:       req.Title,
        Description: req.Description,
        StartTime:   req.StartTime,
        EndTime:     req.EndTime,
        Rewards:     req.Rewards,
    }

    if err := s.repo.UpdateEvent(event); err != nil {
        return nil, err
    }

    return &api.EventResponse{
        Id:          event.ID,
        Title:       event.Title,
        Description: event.Description,
        StartTime:   event.StartTime,
        EndTime:     event.EndTime,
        Rewards:     event.Rewards,
    }, nil
}

func (s *Service) DeleteEvent(ctx context.Context, req *api.DeleteRequest) (*api.Empty, error) {
    if err := s.repo.DeleteEvent(req.Id); err != nil {
        return nil, err
    }
    return &api.Empty{}, nil
}

func (s *Service) ListEvents(ctx context.Context, req *api.Empty) (*api.EventsResponse, error) {
    events, err := s.repo.ListEvents()
    if err != nil {
        return nil, err
    }

    var respEvents []*api.EventResponse
    for _, e := range events {
        respEvents = append(respEvents, &api.EventResponse{
            Id:          e.ID,
            Title:       e.Title,
            Description: e.Description,
            StartTime:   e.StartTime,
            EndTime:     e.EndTime,
            Rewards:     e.Rewards,
        })
    }

    return &api.EventsResponse{Events: respEvents}, nil
}