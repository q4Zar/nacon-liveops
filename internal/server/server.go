package server

import (
	"context"
	"database/sql"
	"liveops/api"
	"liveops/internal/auth"
	"liveops/internal/db"
	"liveops/internal/event"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sony/gobreaker"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type Server struct {
    httpServer *http.Server
    grpcServer *grpc.Server
    eventSvc   *event.Service
    breaker    *gobreaker.CircuitBreaker
}

func NewServer(logger *zap.Logger) *Server {
    dbConn, err := sql.Open("sqlite3", "./liveops.db")
    if err != nil {
        logger.Fatal("failed to open database", zap.Error(err))
    }
    dbConn.SetMaxOpenConns(20)
    dbConn.SetMaxIdleConns(10)

    eventRepo := db.NewEventRepository(dbConn)
    eventSvc := event.NewService(eventRepo, logger)

    grpcServer := grpc.NewServer(
        grpc.UnaryInterceptor(auth.GRPCAuthInterceptor(logger)),
        grpc.MaxConcurrentStreams(100),
    )
    api.RegisterLiveOpsServiceServer(grpcServer, eventSvc)
    reflection.Register(grpcServer) // Enable reflection for grpcurl

    breaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
        Name:        "http-breaker",
        MaxRequests: 5,
        Interval:    60 * time.Second,
        Timeout:     10 * time.Second,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures > 3
        },
    })

    httpRouter := chi.NewRouter() // Renamed for clarity
    httpRouter.Use(RateLimit(100, 10))
    httpRouter.Use(TimeoutMiddleware(5 * time.Second))
    httpRouter.With(auth.HTTPAuthMiddleware("http_user", logger)).Get("/events", breakerWrapper(breaker, eventSvc.GetActiveEvents))
    httpRouter.With(auth.HTTPAuthMiddleware("http_user", logger)).Get("/events/{id}", breakerWrapper(breaker, eventSvc.GetEvent))

    // Single handler to multiplex HTTP and gRPC
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.ProtoMajor == 2 && r.Header.Get("Content-Type") == "application/grpc" {
            grpcServer.ServeHTTP(w, r)
        } else {
            httpRouter.ServeHTTP(w, r) // Use the Chi router for HTTP
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
        eventSvc:   eventSvc,
        breaker:    breaker,
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