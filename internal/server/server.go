package server

import (
    "database/sql"
    "liveops/api"
    "liveops/internal/auth"
    "liveops/internal/db"
    "liveops/internal/event"
    "log"
    "net"
    "net/http"

    "github.com/go-chi/chi/v5"
    "golang.org/x/time/rate"
    "google.golang.org/grpc"
    _ "github.com/mattn/go-sqlite3"
)

type Server struct {
    httpServer *http.Server
    grpcServer *grpc.Server
    eventSvc   *event.Service
}

func NewServer() *Server {
    dbConn, err := sql.Open("sqlite3", "./liveops.db")
    if err != nil {
        log.Fatal(err)
    }
    dbConn.SetMaxOpenConns(10) // Tune based on load
    dbConn.SetMaxIdleConns(5)

    eventRepo := db.NewEventRepository(dbConn)
    eventSvc := event.NewService(eventRepo)

    grpcServer := grpc.NewServer(
        grpc.UnaryInterceptor(auth.GRPCAuthInterceptor),
        grpc.MaxConcurrentStreams(100), // Limit gRPC streams
    )
    api.RegisterLiveOpsServiceServer(grpcServer, eventSvc)

    r := chi.NewRouter()
    r.Use(RateLimit(100, 10)) // 100 req/s, burst of 10
    r.With(auth.HTTPAuthMiddleware("http_user")).Get("/events", eventSvc.GetActiveEvents)
    r.With(auth.HTTPAuthMiddleware("http_user")).Get("/events/{id}", eventSvc.GetEvent)
    r.Handle("/", grpcServer)

    return &Server{
        httpServer: &http.Server{
            Handler: h2cHandler(r, grpcServer),
        },
        grpcServer: grpcServer,
        eventSvc:   eventSvc,
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