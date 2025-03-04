package auth

import (
    "context"
    "errors"
    "net/http"

    "github.com/go-chi/chi/v5/middleware"
    "google.golang.org/grpc"
    "google.golang.org/grpc/metadata"
)

var apiKeys = map[string]string{
    "public-key-123": "http_user",
    "admin-key-456":  "grpc_admin",
}

// HTTPAuthMiddleware adapts to Chi's middleware style
func HTTPAuthMiddleware(requiredRole string) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            apiKey := r.Header.Get("Authorization")
            role, exists := apiKeys[apiKey]
            if !exists || role != requiredRole {
                http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

func GRPCAuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
    md, ok := metadata.FromIncomingContext(ctx)
    if !ok {
        return nil, errors.New("no metadata")
    }

    apiKeysArr := md.Get("authorization")
    if len(apiKeysArr) == 0 {
        return nil, errors.New("no API key")
    }

    role, exists := apiKeys[apiKeysArr[0]]
    if !exists || role != "grpc_admin" {
        return nil, errors.New("unauthorized")
    }

    return handler(ctx, req)
}