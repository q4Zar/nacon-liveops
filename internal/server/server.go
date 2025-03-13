package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"liveops/api"
	"liveops/internal/auth"
	"liveops/internal/cache"
	"liveops/internal/db"
	"liveops/internal/event"
	"liveops/internal/logger"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sony/gobreaker"
	"go.uber.org/zap"
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
	cache      cache.Cache
	logger     *logger.Logger
}

func NewServer() (*Server, error) {
	logger := logger.NewLogger()

	// Initialize database
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/liveops.db"
	}
	database, err := db.NewDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %v", err)
	}

	// Initialize Redis cache
	redisCache, err := cache.NewRedisCache()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Redis cache: %v", err)
	}

	// Initialize repositories
	eventRepo := db.NewEventRepository(database.DB)
	userRepo := db.NewUserRepository(database.DB)

	// Initialize services
	eventSvc := event.NewService(eventRepo, redisCache, logger.Logger)

	// Get circuit breaker configuration from environment
	cbMaxRequests := getEnvInt("CB_MAX_REQUESTS", 5)
	cbInterval := getEnvInt("CB_INTERVAL", 60)
	cbTimeout := getEnvInt("CB_TIMEOUT", 10)
	cbConsecutiveFailures := getEnvInt("CB_CONSECUTIVE_FAILURES", 3)

	// Initialize circuit breaker
	breaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "http-breaker",
		MaxRequests: uint32(cbMaxRequests),
		Interval:    time.Duration(cbInterval) * time.Second,
		Timeout:     time.Duration(cbTimeout) * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures > uint32(cbConsecutiveFailures)
		},
	})

	// Create gRPC server
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(auth.GRPCAuthInterceptor(userRepo)),
		grpc.MaxConcurrentStreams(100),
	)
	api.RegisterLiveOpsServiceServer(grpcServer, &Server{
		eventRepo: eventRepo,
		userRepo:  userRepo,
		eventSvc:  eventSvc,
		breaker:   breaker,
		db:        database,
		cache:     redisCache,
		logger:    logger,
	})
	reflection.Register(grpcServer)

	// Create server instance
	server := &Server{
		grpcServer: grpcServer,
		eventRepo:  eventRepo,
		userRepo:   userRepo,
		eventSvc:   eventSvc,
		breaker:    breaker,
		db:         database,
		cache:      redisCache,
		logger:     logger,
	}

	return server, nil
}

// getEnvInt retrieves an environment variable as integer or returns a default value
func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return intValue
}

func (s *Server) Start() error {
	// Start HTTP server
	httpPort := os.Getenv("HTTP_PORT")
	if httpPort == "" {
		httpPort = "8080"
	}

	// Create HTTP router
	r := chi.NewRouter()
	r.Use(auth.HTTPVerifier())
	r.Use(auth.HTTPAuthenticator())

	// Register HTTP routes
	r.Post("/login", s.handleLogin)
	r.Post("/signup", s.handleSignup)
	r.Get("/events", s.handleGetActiveEvents)
	r.Get("/events/{id}", s.handleGetEventByID)

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%s", httpPort),
		Handler: r,
	}

	// Start HTTP server in a goroutine
	go func() {
		s.logger.Info("Starting HTTP server", zap.String("port", httpPort))
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server failed", zap.Error(err))
		}
	}()

	// Start gRPC server
	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "8081" // Use a different port for gRPC
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	s.logger.Info("Starting gRPC server", zap.String("port", grpcPort))
	if err := s.grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}

	return nil
}

func (s *Server) Stop() {
	if s.httpServer != nil {
		s.httpServer.Close()
	}
	s.grpcServer.GracefulStop()
	if s.db != nil {
		sqlDB, err := s.db.DB.DB()
		if err == nil {
			sqlDB.Close()
		}
	}
	s.cache.Close()
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
		Username string      `json:"username"`
		Password string      `json:"password"`
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
		ID:            req.Id,
		Title:         req.Title,
		Description:   req.Description,
		StartTimeUnix: req.StartTime,
		EndTimeUnix:   req.EndTime,
		Rewards:       req.Rewards,
		CreatedAt:     time.Now().Unix(),
		UpdatedAt:     time.Now().Unix(),
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
		ID:            req.Id,
		Title:         req.Title,
		Description:   req.Description,
		StartTimeUnix: req.StartTime,
		EndTimeUnix:   req.EndTime,
		Rewards:       req.Rewards,
		CreatedAt:     existingEvent.CreatedAt,
		UpdatedAt:     time.Now().Unix(),
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
