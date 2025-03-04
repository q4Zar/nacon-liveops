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
    "github.com/sony/gobreaker"
    "go.uber.org/zap"
    "golang.org/x/time/rate"
    "google.golang.org/grpc"
    _ "github.com/mattn/go-sqlite3"
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

    breaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
        Name:        "http-breaker",
        MaxRequests: 5,
        Interval:    60 * time.Second,
        Timeout:     10 * time.Second,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures > 3
        },
    })

    r := chi.NewRouter()
    r.Use(RateLimit(100, 10))
    r.Use(TimeoutMiddleware(5 * time.Second))
    r.With(auth.HTTPAuthMiddleware("http_user", logger)).Get("/events", breakerWrapper(breaker, eventSvc.GetActiveEvents))
    r.With(auth.HTTPAuthMiddleware("http_user", logger)).Get("/events/{id}", breakerWrapper(breaker, eventSvc.GetEvent))
    r.Handle("/", grpcServer)

    return &Server{
        httpServer: &http.Server{
            Handler:      h2cHandler(r, grpcServer),
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

func h2cHandler(httpHandler http.Handler, grpcServer *grpc.Server) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.ProtoMajor == 2 && r.Header.Get("Content-Type") == "application/grpc" {
            grpcServer.ServeHTTP(w, r)
        } else {
            httpHandler.ServeHTTP(w, r)
        }
    })
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