package server

import (
	"context"
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
	"github.com/go-chi/jwtauth/v5"
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

	// Initialize gRPC server with auth interceptor
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(auth.GRPCAuthInterceptor(userRepo)),
		grpc.MaxConcurrentStreams(100),
	)
	api.RegisterLiveOpsServiceServer(grpcServer, eventSvc)
	reflection.Register(grpcServer)

	breaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "http-breaker",
		MaxRequests: 5,
		Interval:    60 * time.Second,
		Timeout:     10 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures > 3
		},
	})

	httpRouter := chi.NewRouter()
	httpRouter.Use(RateLimit(100, 10))
	httpRouter.Use(TimeoutMiddleware(5 * time.Second))

	// Protected routes with authentication
	httpRouter.Group(func(r chi.Router) {
		r.Use(auth.HTTPVerifier())
		r.Use(auth.HTTPAuthenticator())
		r.Get("/events", breakerWrapper(breaker, eventSvc.GetActiveEvents))
		r.Get("/events/{id}", breakerWrapper(breaker, eventSvc.GetEvent))
	})

	// Single handler to multiplex HTTP and gRPC
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && r.Header.Get("Content-Type") == "application/grpc" {
			grpcServer.ServeHTTP(w, r)
		} else {
			httpRouter.ServeHTTP(w, r)
		}
	})

	// Wrap with h2c to support HTTP/2 cleartext
	h2s := &http2.Server{}
	h2cHandler := h2c.NewHandler(handler, h2s)

	return &Server{
		httpServer: &http.Server{
			Handler:      h2cHandler,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		grpcServer: grpcServer,
		eventRepo:  eventRepo,
		userRepo:   userRepo,
		eventSvc:   eventSvc,
		breaker:    breaker,
		db:         database,
	}
}

func (s *Server) Serve(l net.Listener) error {
	return s.httpServer.Serve(l)
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

func (s *Server) Start(httpAddr, grpcAddr string) error {
	// Initialize JWT with a secret key (in production, use a secure key management system)
	auth.InitJWT([]byte("your-secret-key-here"))

	// Start HTTP server
	go func() {
		if err := s.startHTTP(httpAddr); err != nil {
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	// Start gRPC server
	return s.startGRPC(grpcAddr)
}

func (s *Server) startHTTP(addr string) error {
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

	// Public routes
	r.Group(func(r chi.Router) {
		r.Post("/login", s.handleLogin)
		r.Post("/signup", s.handleSignup)
	})

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(auth.HTTPVerifier())
		r.Use(auth.HTTPAuthenticator())

		r.Get("/events", s.handleGetActiveEvents)
		r.Get("/events/{id}", s.handleGetEventByID)
	})

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: r,
	}

	fmt.Printf("HTTP server listening on %s\n", addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) startGRPC(addr string) error {
	fmt.Printf("gRPC server listening on %s\n", addr)
	return nil // Return the listener setup and serve code here
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

// ... existing event handler methods ...

// gRPC methods
func (s *Server) CreateEvent(ctx context.Context, req *api.EventRequest) (*api.EventResponse, error) {
	user, err := auth.ExtractUserFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid authentication: %v", err)
	}

	if user.Type != db.UserTypeAdmin {
		return nil, status.Error(codes.PermissionDenied, "admin access required")
	}

	// ... rest of the create event logic ...
	return &api.EventResponse{}, nil
}

// ... other gRPC methods with similar authentication checks ...