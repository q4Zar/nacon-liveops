package auth

import (
	"context"
	"errors"
	"fmt"
	"liveops/internal/db"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/jwtauth/v5"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var (
	tokenAuth *jwtauth.JWTAuth
	// Token expiration time (24 hours)
	tokenExpiration = 24 * time.Hour
)

// Initialize JWT authentication with the given secret key
func InitJWT(secretKey []byte) {
	tokenAuth = jwtauth.New("HS256", secretKey, nil)
}

// GenerateToken creates a new JWT token for a user
func GenerateToken(user *db.User) (string, error) {
	claims := map[string]interface{}{
		"user_id":   user.ID,
		"username":  user.Username,
		"user_type": string(user.Type),
		"exp":       time.Now().Add(tokenExpiration).Unix(),
	}

	_, tokenString, err := tokenAuth.Encode(claims)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %v", err)
	}

	return tokenString, nil
}

// Verifier middleware for HTTP requests
func HTTPVerifier() func(http.Handler) http.Handler {
	return jwtauth.Verifier(tokenAuth)
}

// Authenticator middleware for HTTP requests
func HTTPAuthenticator() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, claims, err := jwtauth.FromContext(r.Context())

			if err != nil || token == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if err := jwt.Validate(token); err != nil {
				http.Error(w, "Unauthorized - Invalid token", http.StatusUnauthorized)
				return
			}

			// Token is valid, continue
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), "jwt_claims", claims)))
		})
	}
}

// GRPCAuthInterceptor creates an interceptor for gRPC authentication
func GRPCAuthInterceptor(userRepo db.UserRepository) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, errors.New("missing metadata")
		}

		authHeader := md.Get("authorization")
		if len(authHeader) == 0 {
			return nil, errors.New("missing authorization header")
		}

		tokenString := strings.TrimPrefix(authHeader[0], "Bearer ")
		token, err := tokenAuth.Decode(tokenString)
		if err != nil {
			return nil, fmt.Errorf("invalid token: %v", err)
		}

		if err := jwt.Validate(token); err != nil {
			return nil, errors.New("invalid token")
		}

		claims, err := token.AsMap(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get claims: %v", err)
		}

		userType, ok := claims["user_type"].(string)
		if !ok || userType != string(db.UserTypeAdmin) {
			return nil, errors.New("unauthorized: admin access required")
		}

		return handler(ctx, req)
	}
}

// ExtractUserFromContext extracts user information from the JWT token in the context
func ExtractUserFromContext(ctx context.Context) (*db.User, error) {
	// Try to get token from gRPC metadata first
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		authHeader := md.Get("authorization")
		if len(authHeader) == 0 {
			return nil, errors.New("no authorization header found in metadata")
		}

		tokenString := strings.TrimPrefix(authHeader[0], "Bearer ")
		token, err := tokenAuth.Decode(tokenString)
		if err != nil {
			return nil, fmt.Errorf("invalid token: %v", err)
		}

		if err := jwt.Validate(token); err != nil {
			return nil, errors.New("invalid token")
		}

		claims, err := token.AsMap(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get claims: %v", err)
		}

		user := &db.User{
			ID:       claims["user_id"].(string),
			Username: claims["username"].(string),
			Type:     db.UserType(claims["user_type"].(string)),
		}

		return user, nil
	}

	// Fallback to HTTP context
	token, claims, err := jwtauth.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get token from context: %v", err)
	}

	if token == nil {
		return nil, errors.New("no token found in context")
	}

	if err := jwt.Validate(token); err != nil {
		return nil, errors.New("invalid token")
	}

	user := &db.User{
		ID:       claims["user_id"].(string),
		Username: claims["username"].(string),
		Type:     db.UserType(claims["user_type"].(string)),
	}

	return user, nil
} 