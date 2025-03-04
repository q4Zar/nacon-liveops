package auth

import (
    "context"
    "errors"
    "net/http"

    "go.uber.org/zap"
    "google.golang.org/grpc"
    "google.golang.org/grpc/metadata"
)

var apiKeys = map[string]string{
    "public-key-123": "http_user",
    "admin-key-456":  "grpc_admin",
}

func HTTPAuthMiddleware(requiredRole string, logger *zap.Logger) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            logger.Info("Authenticating REST request",
                zap.String("method", r.Method),
                zap.String("path", r.URL.Path))
            apiKey := r.Header.Get("Authorization")
            role, exists := apiKeys[apiKey]
            if !exists || role != requiredRole {
                logger.Warn("Unauthorized REST access attempt",
                    zap.String("api_key", apiKey),
                    zap.String("required_role", requiredRole))
                http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
                return
            }
            logger.Info("REST authentication successful",
                zap.String("role", role))
            next.ServeHTTP(w, r)
        })
    }
}

func GRPCAuthInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
        logger.Info("Authenticating gRPC request",
            zap.String("method", info.FullMethod))
        md, ok := metadata.FromIncomingContext(ctx)
        if !ok {
            logger.Warn("No metadata in gRPC request")
            return nil, errors.New("no metadata")
        }

        apiKeysArr := md.Get("authorization")
        if len(apiKeysArr) == 0 {
            logger.Warn("No API key in gRPC request")
            return nil, errors.New("no API key")
        }

        role, exists := apiKeys[apiKeysArr[0]]
        if !exists || role != "grpc_admin" {
            logger.Warn("Unauthorized gRPC access attempt",
                zap.String("api_key", apiKeysArr[0]))
            return nil, errors.New("unauthorized")
        }
        logger.Info("gRPC authentication successful",
            zap.String("role", role))
        return handler(ctx, req)
    }
}