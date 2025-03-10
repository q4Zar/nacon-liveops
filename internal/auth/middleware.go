package auth

import (
	"context"
	"encoding/base64"
	"liveops/internal/db"
	"net/http"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type contextKey string

const (
	UserContextKey contextKey = "user"
)

type Authenticator struct {
	userRepo db.UserRepository
	logger   *zap.Logger
}

func NewAuthenticator(userRepo db.UserRepository, logger *zap.Logger) *Authenticator {
	return &Authenticator{
		userRepo: userRepo,
		logger:   logger,
	}
}

func (a *Authenticator) HTTPAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := a.authenticateHTTP(r)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// For HTTP endpoints, check if user has HTTP access
		if user.Type != db.UserTypeHTTP && user.Type != db.UserTypeAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *Authenticator) GRPCAuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	user, err := a.authenticateGRPC(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid credentials")
	}

	// For gRPC endpoints, check if user is admin
	if user.Type != db.UserTypeAdmin {
		return nil, status.Errorf(codes.PermissionDenied, "admin access required")
	}

	newCtx := context.WithValue(ctx, UserContextKey, user)
	return handler(newCtx, req)
}

func (a *Authenticator) authenticateHTTP(r *http.Request) (*db.User, error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return nil, status.Error(codes.Unauthenticated, "no credentials provided")
	}

	const prefix = "Basic "
	if !strings.HasPrefix(auth, prefix) {
		return nil, status.Error(codes.Unauthenticated, "invalid auth type")
	}

	payload, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid base64 in credentials")
	}

	pair := strings.SplitN(string(payload), ":", 2)
	if len(pair) != 2 {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials format")
	}

	user, err := a.userRepo.ValidateUser(pair[0], pair[1])
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	return user, nil
}

func (a *Authenticator) authenticateGRPC(ctx context.Context) (*db.User, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		a.logger.Debug("no metadata in context")
		return nil, status.Error(codes.Unauthenticated, "no metadata provided")
	}

	auth := md.Get("authorization")
	if len(auth) == 0 {
		a.logger.Debug("no authorization header")
		return nil, status.Error(codes.Unauthenticated, "no credentials provided")
	}

	a.logger.Debug("received auth header", zap.String("auth", auth[0]))

	const prefix = "Basic "
	if !strings.HasPrefix(auth[0], prefix) {
		a.logger.Debug("invalid auth type", zap.String("auth", auth[0]))
		return nil, status.Error(codes.Unauthenticated, "invalid auth type")
	}

	payload, err := base64.StdEncoding.DecodeString(auth[0][len(prefix):])
	if err != nil {
		a.logger.Debug("invalid base64", zap.Error(err))
		return nil, status.Error(codes.Unauthenticated, "invalid base64 in credentials")
	}

	pair := strings.SplitN(string(payload), ":", 2)
	if len(pair) != 2 {
		a.logger.Debug("invalid credential format")
		return nil, status.Error(codes.Unauthenticated, "invalid credentials format")
	}

	a.logger.Debug("attempting to validate user", zap.String("username", pair[0]))
	user, err := a.userRepo.ValidateUser(pair[0], pair[1])
	if err != nil {
		a.logger.Debug("user validation failed", zap.Error(err))
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	a.logger.Debug("user authenticated successfully", 
		zap.String("username", user.Username),
		zap.String("type", string(user.Type)))
	return user, nil
} 