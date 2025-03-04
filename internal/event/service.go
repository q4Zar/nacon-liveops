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
    "google.golang.org/protobuf/types/known/timestamppb"
)

type Service struct {
    api.UnimplementedLiveOpsServiceServer
    repo   db.EventRepository
    logger *zap.Logger
}

func NewService(repo db.EventRepository, logger *zap.Logger) *Service {
    return &Service{
        repo:   repo,
        logger: logger,
    }
}

func (s *Service) GetActiveEvents(w http.ResponseWriter, r *http.Request) {
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
            data, err := json.Marshal(e)
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

func (s *Service) GetEvent(w http.ResponseWriter, r *http.Request) {
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
    if err := json.NewEncoder(w).Encode(event); err != nil {
        s.logger.Error("Failed to encode event", zap.String("id", id), zap.Error(err))
        http.Error(w, `{"error": "failed to encode response"}`, http.StatusInternalServerError)
        return
    }
    s.logger.Info("Successfully returned event", zap.String("id", id))
}

func (s *Service) CreateEvent(ctx context.Context, req *api.EventRequest) (*api.EventResponse, error) {
    s.logger.Info("Received CreateEvent request", zap.String("id", req.Id))
    event := db.Event{
        ID:          req.Id,
        Title:       req.Title,
        Description: req.Description,
        StartTime:   timestamppb.New(time.Unix(req.GetStartTime(), 0)),
        EndTime:     timestamppb.New(time.Unix(req.GetEndTime(), 0)),
        Rewards:     req.Rewards,
    }

    if err := s.repo.CreateEvent(event); err != nil {
        s.logger.Error("Failed to create event", zap.String("id", req.Id), zap.Error(err))
        return nil, err
    }

    resp := &api.EventResponse{
        Id:          event.ID,
        Title:       event.Title,
        Description: event.Description,
        StartTime:   event.StartTime.AsTime().Unix(),
        EndTime:     event.EndTime.AsTime().Unix(),
        Rewards:     event.Rewards,
    }
    s.logger.Info("Successfully created event", zap.String("id", req.Id))
    return resp, nil
}

func (s *Service) UpdateEvent(ctx context.Context, req *api.EventRequest) (*api.EventResponse, error) {
    s.logger.Info("Received UpdateEvent request", zap.String("id", req.Id))
    event := db.Event{
        ID:          req.Id,
        Title:       req.Title,
        Description: req.Description,
        StartTime:   timestamppb.New(time.Unix(req.GetStartTime(), 0)),
        EndTime:     timestamppb.New(time.Unix(req.GetEndTime(), 0)),
        Rewards:     req.Rewards,
    }

    if err := s.repo.UpdateEvent(event); err != nil {
        s.logger.Error("Failed to update event", zap.String("id", req.Id), zap.Error(err))
        return nil, err
    }

    resp := &api.EventResponse{
        Id:          event.ID,
        Title:       event.Title,
        Description: event.Description,
        StartTime:   event.StartTime.AsTime().Unix(),
        EndTime:     event.EndTime.AsTime().Unix(),
        Rewards:     event.Rewards,
    }
    s.logger.Info("Successfully updated event", zap.String("id", req.Id))
    return resp, nil
}

func (s *Service) DeleteEvent(ctx context.Context, req *api.DeleteRequest) (*api.Empty, error) {
    s.logger.Info("Received DeleteEvent request", zap.String("id", req.Id))
    if err := s.repo.DeleteEvent(req.Id); err != nil {
        s.logger.Error("Failed to delete event", zap.String("id", req.Id), zap.Error(err))
        return nil, err
    }
    s.logger.Info("Successfully deleted event", zap.String("id", req.Id))
    return &api.Empty{}, nil
}

func (s *Service) ListEvents(ctx context.Context, req *api.Empty) (*api.EventsResponse, error) {
    s.logger.Info("Received ListEvents request")
    events, err := s.repo.ListEvents()
    if err != nil {
        s.logger.Error("Failed to list events", zap.Error(err))
        return nil, err
    }

    var respEvents []*api.EventResponse
    for _, e := range events {
        respEvents = append(respEvents, &api.EventResponse{
            Id:          e.ID,
            Title:       e.Title,
            Description: e.Description,
            StartTime:   e.StartTime.AsTime().Unix(),
            EndTime:     e.EndTime.AsTime().Unix(),
            Rewards:     e.Rewards,
        })
    }
    s.logger.Info("Successfully returned events", zap.Int("count", len(events)))
    return &api.EventsResponse{Events: respEvents}, nil
}