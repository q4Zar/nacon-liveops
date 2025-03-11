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
	return jwtauth.Authenticator
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

		if token == nil || !token.Valid() {
			return nil, errors.New("invalid token")
		}

		claims := token.PrivateClaims()
		userType, ok := claims["user_type"].(string)
		if !ok || userType != string(db.UserTypeAdmin) {
			return nil, errors.New("unauthorized: admin access required")
		}

		return handler(ctx, req)
	}
}

// ExtractUserFromContext extracts user information from the JWT token in the context
func ExtractUserFromContext(ctx context.Context) (*db.User, error) {
	token, claims, err := jwtauth.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get token from context: %v", err)
	}

	if token == nil || !token.Valid() {
		return nil, errors.New("invalid token")
	}

	user := &db.User{
		ID:       claims["user_id"].(string),
		Username: claims["username"].(string),
		Type:     db.UserType(claims["user_type"].(string)),
	}

	return user, nil
} 