package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"liveops/api"
	"liveops/internal/auth"
	"liveops/internal/db"
	"liveops/internal/event"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sony/gobreaker"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

type Server struct {
	api.UnimplementedLiveOpsServiceServer
	httpServer *http.Server
	grpcServer *grpc.Server
	eventRepo  db.EventRepository
	userRepo   db.UserRepository
	eventSvc   *event.Service
	breaker    *gobreaker.CircuitBreaker
	db         *db.DB
	logger     *zap.Logger
}

func NewServer(logger *zap.Logger) *Server {
	// Initialize database
	database, err := db.NewDB("./liveops.db")
	if err != nil {
		logger.Fatal("failed to initialize database", zap.Error(err))
	}

	// Initialize repositories
	eventRepo := db.NewEventRepository(database.DB)
	userRepo := db.NewUserRepository(database.DB)

	// Initialize services
	eventSvc := event.NewService(eventRepo, logger)

	// Initialize circuit breaker
	breaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "http-breaker",
		MaxRequests: 5,
		Interval:    60 * time.Second,
		Timeout:     10 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures > 3
		},
	})

	// Initialize server
	server := &Server{
		eventRepo: eventRepo,
		userRepo:  userRepo,
		eventSvc:  eventSvc,
		breaker:   breaker,
		db:        database,
		logger:    logger,
	}

	// Initialize gRPC server
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(auth.GRPCAuthInterceptor(userRepo)),
		grpc.MaxConcurrentStreams(100),
	)
	api.RegisterLiveOpsServiceServer(grpcServer, server)
	reflection.Register(grpcServer)
	server.grpcServer = grpcServer

	return server
}

func (s *Server) Start(addr string) error {
	// Initialize JWT
	auth.InitJWT([]byte("your-secret-key-here"))

	// Create router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(RateLimit(100, 10))
	r.Use(TimeoutMiddleware(5 * time.Second))

	// Public routes
	r.Group(func(r chi.Router) {
		r.Post("/login", s.handleLogin)
		r.Post("/signup", s.handleSignup)
	})

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(auth.HTTPVerifier())
		r.Use(auth.HTTPAuthenticator())
		r.Get("/events", breakerWrapper(s.breaker, s.handleGetActiveEvents))
		r.Get("/events/{id}", breakerWrapper(s.breaker, s.handleGetEventByID))
	})

	// Create HTTP/2 server with multiplexing
	h2s := &http2.Server{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.ProtoMajor == 2 && req.Header.Get("Content-Type") == "application/grpc" {
			s.grpcServer.ServeHTTP(w, req)
		} else {
			r.ServeHTTP(w, req)
		}
	})

	// Create server with h2c handler
	h2cHandler := h2c.NewHandler(handler, h2s)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      h2cHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	s.logger.Info("Starting server", zap.String("addr", addr))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to create listener: %v", err)
	}

	return s.httpServer.Serve(listener)
}

func RateLimit(limit float64, burst int) func(next http.Handler) http.Handler {
	limiter := rate.NewLimiter(rate.Limit(limit), burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				http.Error(w, `{"error": "rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func TimeoutMiddleware(timeout time.Duration) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func breakerWrapper(cb *gobreaker.CircuitBreaker, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := cb.Execute(func() (interface{}, error) {
			handler(w, r)
			return nil, nil
		})
		if err != nil {
			if err == gobreaker.ErrOpenState {
				http.Error(w, `{"error": "service unavailable"}`, http.StatusServiceUnavailable)
			} else {
				http.Error(w, `{"error": "internal server error"}`, http.StatusInternalServerError)
			}
		}
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := s.userRepo.ValidateUser(creds.Username, creds.Password)
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := auth.GenerateToken(user)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"token": token,
	})
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string    `json:"username"`
		Password string    `json:"password"`
		UserType db.UserType `json:"user_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.userRepo.CreateUser(req.Username, req.Password, req.UserType); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create user: %v", err), http.StatusBadRequest)
		return
	}

	user, err := s.userRepo.GetUser(req.Username)
	if err != nil {
		http.Error(w, "User created but failed to retrieve", http.StatusInternalServerError)
		return
	}

	token, err := auth.GenerateToken(user)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"token": token,
	})
}

func (s *Server) handleGetActiveEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.eventRepo.GetActiveEvents()
	if err != nil {
		s.logger.Error("Failed to get active events", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to get active events: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(events); err != nil {
		s.logger.Error("Failed to encode response", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleGetEventByID(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "id")
	if eventID == "" {
		http.Error(w, "Event ID is required", http.StatusBadRequest)
		return
	}

	event, err := s.eventRepo.GetEvent(eventID)
	if err == sql.ErrNoRows {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}
	if err != nil {
		s.logger.Error("Failed to get event", zap.String("id", eventID), zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to get event: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(event); err != nil {
		s.logger.Error("Failed to encode response", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// gRPC methods
func (s *Server) CreateEvent(ctx context.Context, req *api.EventRequest) (*api.EventResponse, error) {
	user, err := auth.ExtractUserFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid authentication: %v", err)
	}

	if user.Type != db.UserTypeAdmin {
		return nil, status.Error(codes.PermissionDenied, "admin access required")
	}

	event := db.Event{
		ID:           req.Id,
		Title:        req.Title,
		Description:  req.Description,
		StartTimeUnix: req.StartTime,
		EndTimeUnix:   req.EndTime,
		Rewards:      req.Rewards,
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	}

	if err := s.eventRepo.CreateEvent(event); err != nil {
		s.logger.Error("Failed to create event", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to create event: %v", err)
	}

	return &api.EventResponse{
		Id:          event.ID,
		Title:       event.Title,
		Description: event.Description,
		StartTime:   event.StartTimeUnix,
		EndTime:     event.EndTimeUnix,
		Rewards:     event.Rewards,
		CreatedAt:   event.CreatedAt,
		UpdatedAt:   event.UpdatedAt,
	}, nil
}

// ListEvents returns a list of all events
func (s *Server) ListEvents(ctx context.Context, req *api.Empty) (*api.EventsResponse, error) {
	user, err := auth.ExtractUserFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid authentication: %v", err)
	}

	if user.Type != db.UserTypeAdmin {
		return nil, status.Error(codes.PermissionDenied, "admin access required")
	}

	events, err := s.eventRepo.ListEvents()
	if err != nil {
		s.logger.Error("Failed to get events", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get events: %v", err)
	}

	var protoEvents []*api.EventResponse
	for _, event := range events {
		protoEvents = append(protoEvents, &api.EventResponse{
			Id:          event.ID,
			Title:       event.Title,
			Description: event.Description,
			StartTime:   event.StartTimeUnix,
			EndTime:     event.EndTimeUnix,
			Rewards:     event.Rewards,
			CreatedAt:   event.CreatedAt,
			UpdatedAt:   event.UpdatedAt,
		})
	}

	return &api.EventsResponse{
		Events: protoEvents,
	}, nil
}

// UpdateEvent updates an existing event
func (s *Server) UpdateEvent(ctx context.Context, req *api.EventRequest) (*api.EventResponse, error) {
	user, err := auth.ExtractUserFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid authentication: %v", err)
	}

	if user.Type != db.UserTypeAdmin {
		return nil, status.Error(codes.PermissionDenied, "admin access required")
	}

	// Check if event exists
	existingEvent, err := s.eventRepo.GetEvent(req.Id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, status.Errorf(codes.NotFound, "event not found: %s", req.Id)
		}
		s.logger.Error("Failed to get event", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get event: %v", err)
	}

	// Update event fields
	event := db.Event{
		ID:           req.Id,
		Title:        req.Title,
		Description:  req.Description,
		StartTimeUnix: req.StartTime,
		EndTimeUnix:   req.EndTime,
		Rewards:      req.Rewards,
		CreatedAt:    existingEvent.CreatedAt,
		UpdatedAt:    time.Now().Unix(),
	}

	if err := s.eventRepo.UpdateEvent(event); err != nil {
		s.logger.Error("Failed to update event", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to update event: %v", err)
	}

	return &api.EventResponse{
		Id:          event.ID,
		Title:       event.Title,
		Description: event.Description,
		StartTime:   event.StartTimeUnix,
		EndTime:     event.EndTimeUnix,
		Rewards:     event.Rewards,
		CreatedAt:   event.CreatedAt,
		UpdatedAt:   event.UpdatedAt,
	}, nil
}

// DeleteEvent deletes an existing event
func (s *Server) DeleteEvent(ctx context.Context, req *api.DeleteRequest) (*api.Empty, error) {
	user, err := auth.ExtractUserFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid authentication: %v", err)
	}

	if user.Type != db.UserTypeAdmin {
		return nil, status.Error(codes.PermissionDenied, "admin access required")
	}

	// Check if event exists
	_, err = s.eventRepo.GetEvent(req.Id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, status.Errorf(codes.NotFound, "event not found: %s", req.Id)
		}
		s.logger.Error("Failed to get event", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to get event: %v", err)
	}

	if err := s.eventRepo.DeleteEvent(req.Id); err != nil {
		s.logger.Error("Failed to delete event", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to delete event: %v", err)
	}

	return &api.Empty{}, nil
}

// ... other gRPC methods with similar authentication checks ...