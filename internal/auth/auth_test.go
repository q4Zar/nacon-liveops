package auth

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/assert"
    "google.golang.org/grpc"
    "google.golang.org/grpc/metadata"
)

func TestHTTPAuthMiddleware(t *testing.T) {
    tests := []struct {
        name           string
        apiKey         string
        requiredRole   string
        expectedStatus int
    }{{
        name:           "Valid HTTP User",
        apiKey:         "public-key-123",
        requiredRole:   "http_user",
        expectedStatus: http.StatusOK,
    }, {
        name:           "Invalid API Key",
        apiKey:         "invalid-key",
        requiredRole:   "http_user",
        expectedStatus: http.StatusUnauthorized,
    }, {
        name:           "Wrong Role",
        apiKey:         "admin-key-456",
        requiredRole:   "http_user",
        expectedStatus: http.StatusUnauthorized,
    }}

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            handler := func(w http.ResponseWriter, r *http.Request) {
                w.WriteHeader(http.StatusOK)
            }
            mw := HTTPAuthMiddleware(tt.requiredRole)(http.HandlerFunc(handler))

            req := httptest.NewRequest("GET", "/events", nil)
            req.Header.Set("Authorization", tt.apiKey)
            rr := httptest.NewRecorder()

            mw.ServeHTTP(rr, req)

            assert.Equal(t, tt.expectedStatus, rr.Code)
        })
    }
}

func TestGRPCAuthInterceptor(t *testing.T) {
    tests := []struct {
        name         string
        apiKey       string
        expectError  bool
        expectedErr  string
    }{{
        name:        "Valid gRPC Admin",
        apiKey:      "admin-key-456",
        expectError: false,
    }, {
        name:        "Invalid API Key",
        apiKey:      "invalid-key",
        expectError: true,
        expectedErr: "unauthorized",
    }, {
        name:        "Wrong Role",
        apiKey:      "public-key-123",
        expectError: true,
        expectedErr: "unauthorized",
    }, {
        name:        "No API Key",
        apiKey:      "",
        expectError: true,
        expectedErr: "no API key",
    }}

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ctx := context.Background()
            if tt.apiKey != "" {
                md := metadata.New(map[string]string{"authorization": tt.apiKey})
                ctx = metadata.NewIncomingContext(ctx, md)
            }

            handler := func(ctx context.Context, req interface{}) (interface{}, error) {
                return "success", nil
            }

            resp, err := GRPCAuthInterceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)

            if tt.expectError {
                assert.Error(t, err)
                assert.Equal(t, tt.expectedErr, err.Error())
                assert.Nil(t, resp)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, "success", resp)
            }
        })
    }
}