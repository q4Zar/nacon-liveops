package main

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"liveops/api"
	"log"
	"math/big"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	defaultDuration        = 60 * time.Second     // 1 minute stress test
	defaultGRPCConcurrency = 10                   // Workers for gRPC operations
	defaultHTTPConcurrency = 2                    // Workers for HTTP operations
	defaultRequestDelay    = 5 * time.Millisecond // Throttle requests
	defaultUsername        = "admin"              // Default admin username
	defaultPassword        = "admin-key-456"      // Default admin password
	defaultDeleteTimeout   = 5 * time.Millisecond // Default timeout for delete operations
	defaultDeleteRetries   = 3                    // Default number of retries for delete operations
)

var (
	eventIDs        []string   // Store successfully created event IDs
	eventIDsMutex   sync.Mutex // Protect access to eventIDs
	httpClient      = &http.Client{Timeout: 5 * time.Second}
	httpAddr        = getServerURL()     // HTTP server URL
	grpcAddr        = getGRPCServerURL() // gRPC server URL
	jwtToken        string               // JWT token for authentication
	duration        time.Duration        // Stress test duration
	grpcConcurrency int                  // Workers for gRPC operations
	httpConcurrency int                  // Workers for HTTP operations
	requestDelay    time.Duration        // Throttle requests
	username        string               // Admin username
	password        string               // Admin password
	deleteTimeout   time.Duration        // Timeout for delete operations
	deleteRetries   int                  // Number of retries for delete operations
)

func getServerURL() string {
	if url := os.Getenv("SERVER_URL"); url != "" {
		return url
	}
	return "http://localhost:8080"
}

func getGRPCServerURL() string {
	if url := os.Getenv("GRPC_SERVER_URL"); url != "" {
		return url
	}
	return "localhost:8080"
}

func getDuration() time.Duration {
	if dur := os.Getenv("STRESS_DURATION"); dur != "" {
		if d, err := time.ParseDuration(dur); err == nil {
			return d
		}
	}
	return defaultDuration
}

func getGRPCConcurrency() int {
	if concurrency := os.Getenv("GRPC_CONCURRENCY"); concurrency != "" {
		if c, err := strconv.Atoi(concurrency); err == nil && c > 0 {
			return c
		}
	}
	return defaultGRPCConcurrency
}

func getHTTPConcurrency() int {
	if concurrency := os.Getenv("HTTP_CONCURRENCY"); concurrency != "" {
		if c, err := strconv.Atoi(concurrency); err == nil && c > 0 {
			return c
		}
	}
	return defaultHTTPConcurrency
}

func getRequestDelay() time.Duration {
	if delay := os.Getenv("REQUEST_DELAY"); delay != "" {
		if d, err := time.ParseDuration(delay); err == nil {
			return d
		}
	}
	return defaultRequestDelay
}

func getUsername() string {
	if user := os.Getenv("ADMIN_USERNAME"); user != "" {
		return user
	}
	return defaultUsername
}

func getPassword() string {
	if pass := os.Getenv("ADMIN_PASSWORD"); pass != "" {
		return pass
	}
	return defaultPassword
}

func getDeleteTimeout() time.Duration {
	if timeout := os.Getenv("DELETE_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			return t
		}
	}
	return defaultDeleteTimeout
}

func getDeleteRetries() int {
	if retries := os.Getenv("DELETE_RETRIES"); retries != "" {
		if r, err := strconv.Atoi(retries); err == nil && r > 0 {
			return r
		}
	}
	return defaultDeleteRetries
}

func init() {
	rand.Seed(time.Now().UnixNano())
	duration = getDuration()
	grpcConcurrency = getGRPCConcurrency()
	httpConcurrency = getHTTPConcurrency()
	requestDelay = getRequestDelay()
	username = getUsername()
	password = getPassword()
	deleteTimeout = getDeleteTimeout()
	deleteRetries = getDeleteRetries()
}

type opStats struct {
	Success     uint64
	Failed      uint64
	RateLimited uint64
}

// randomString generates a random string of given length
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		num, _ := crand.Int(crand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[num.Int64()]
	}
	return string(b)
}

// stressOperation runs a function continuously until the context expires
func stressOperation(ctx context.Context, name string, fn func(context.Context, *opStats), stats *opStats) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			fn(ctx, stats)
			time.Sleep(requestDelay)
		}
	}
}

// createEvent sends a gRPC CreateEvent request
func createEvent(client api.LiveOpsServiceClient) func(context.Context, *opStats) {
	return func(ctx context.Context, stats *opStats) {
		id := uuid.New().String()
		now := time.Now()
		req := &api.EventRequest{
			Id:          id,
			Title:       "Event-" + randomString(8),
			Description: randomString(20),
			StartTime:   now.Unix(),
			EndTime:     now.Add(2 * time.Hour).Unix(),
			Rewards:     fmt.Sprintf(`{"gold": %d}`, rand.Intn(1000)),
		}

		resp, err := client.CreateEvent(ctx, req)
		if err != nil {
			log.Printf("Failed to create event %s: %v", id, err)
			atomic.AddUint64(&stats.Failed, 1)
			return
		}
		eventIDsMutex.Lock()
		eventIDs = append(eventIDs, resp.Id)
		eventIDsMutex.Unlock()
		atomic.AddUint64(&stats.Success, 1)
	}
}

// updateEvent sends a gRPC UpdateEvent request
func updateEvent(client api.LiveOpsServiceClient) func(context.Context, *opStats) {
	return func(ctx context.Context, stats *opStats) {
		eventIDsMutex.Lock()
		if len(eventIDs) == 0 {
			eventIDsMutex.Unlock()
			return
		}
		id := eventIDs[rand.Intn(len(eventIDs))]
		eventIDsMutex.Unlock()

		now := time.Now()
		req := &api.EventRequest{
			Id:          id,
			Title:       "Updated-" + randomString(8),
			Description: randomString(20),
			StartTime:   now.Unix(),
			EndTime:     now.Add(2 * time.Hour).Unix(),
			Rewards:     fmt.Sprintf(`{"gold": %d}`, rand.Intn(1000)),
		}

		_, err := client.UpdateEvent(ctx, req)
		if err != nil {
			log.Printf("Failed to update event %s: %v", id, err)
			atomic.AddUint64(&stats.Failed, 1)
			return
		}
		atomic.AddUint64(&stats.Success, 1)
	}
}

// listEvents sends a gRPC ListEvents request
func listEvents(client api.LiveOpsServiceClient) func(context.Context, *opStats) {
	return func(ctx context.Context, stats *opStats) {
		resp, err := client.ListEvents(ctx, &api.Empty{})
		if err != nil {
			if st, ok := status.FromError(err); ok && st.Code() == codes.ResourceExhausted {
				atomic.AddUint64(&stats.RateLimited, 1)
			} else {
				log.Printf("Failed to list events: %v", err)
				atomic.AddUint64(&stats.Failed, 1)
			}
			return
		}
		atomic.AddUint64(&stats.Success, 1)
		eventIDsMutex.Lock()
		eventIDs = make([]string, 0, len(resp.Events))
		for _, e := range resp.Events {
			eventIDs = append(eventIDs, e.Id)
		}
		eventIDsMutex.Unlock()
	}
}

// signIn performs admin authentication and gets JWT token
func signIn() error {
	// Create login request
	reqBody := map[string]interface{}{
		"username": username,
		"password": password,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	// Send login request
	resp, err := http.Post(httpAddr+"/login", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("login request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed: %s", string(body))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}

	jwtToken = result.Token
	return nil
}

// fetchActiveEvents makes an HTTP GET /events request
func fetchActiveEvents() func(context.Context, *opStats) {
	return func(ctx context.Context, stats *opStats) {
		req, err := http.NewRequestWithContext(ctx, "GET", httpAddr+"/events", nil)
		if err != nil {
			log.Printf("Failed to create GET /events request: %v", err)
			atomic.AddUint64(&stats.Failed, 1)
			return
		}
		req.Header.Set("Authorization", "Bearer "+jwtToken)

		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("Failed to fetch active events: %v", err)
			atomic.AddUint64(&stats.Failed, 1)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			atomic.AddUint64(&stats.RateLimited, 1)
		} else if resp.StatusCode != http.StatusOK {
			log.Printf("GET /events failed with status %d", resp.StatusCode)
			atomic.AddUint64(&stats.Failed, 1)
		} else {
			atomic.AddUint64(&stats.Success, 1)
		}
	}
}

// fetchEventByID makes an HTTP GET /events/{id} request
func fetchEventByID() func(context.Context, *opStats) {
	return func(ctx context.Context, stats *opStats) {
		eventIDsMutex.Lock()
		if len(eventIDs) == 0 {
			eventIDsMutex.Unlock()
			return
		}
		id := eventIDs[rand.Intn(len(eventIDs))]
		eventIDsMutex.Unlock()

		url := fmt.Sprintf("%s/events/%s", httpAddr, id)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			log.Printf("Failed to create GET /events/%s request: %v", id, err)
			atomic.AddUint64(&stats.Failed, 1)
			return
		}
		req.Header.Set("Authorization", "Bearer "+jwtToken)

		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("Failed to fetch event %s: %v", id, err)
			atomic.AddUint64(&stats.Failed, 1)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			atomic.AddUint64(&stats.RateLimited, 1)
		} else if resp.StatusCode != http.StatusOK {
			log.Printf("GET /events/%s failed with status %d", id, resp.StatusCode)
			atomic.AddUint64(&stats.Failed, 1)
		} else {
			atomic.AddUint64(&stats.Success, 1)
		}
	}
}

// deleteEvents deletes exactly the number of successfully created events
func deleteEvents(ctx context.Context, client api.LiveOpsServiceClient, stats *opStats, createSuccess uint64) {
	eventIDsMutex.Lock()
	ids := make([]string, len(eventIDs))
	copy(ids, eventIDs)
	eventIDsMutex.Unlock()

	// Limit deletions to the number of successful creations
	deleteCount := int(min(uint64(len(ids)), createSuccess))
	for i := 0; i < deleteCount; i++ {
		// Create a new context with timeout for each delete operation
		deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
		defer cancel()

		// Retry loop for delete operation
		var err error
		for retry := 0; retry < deleteRetries; retry++ {
			req := &api.DeleteRequest{Id: ids[i]}
			_, err = client.DeleteEvent(deleteCtx, req)
			if err == nil {
				atomic.AddUint64(&stats.Success, 1)
				break
			}

			// Check if error is retryable
			if st, ok := status.FromError(err); ok {
				switch st.Code() {
				case codes.DeadlineExceeded, codes.Unavailable, codes.Internal:
					log.Printf("Retry %d/%d: Failed to delete event %s: %v", retry+1, deleteRetries, ids[i], err)
					time.Sleep(time.Second * time.Duration(retry+1)) // Exponential backoff
					continue
				}
			}
			break // Non-retryable error
		}

		if err != nil {
			log.Printf("Failed to delete event %s after %d retries: %v", ids[i], deleteRetries, err)
			atomic.AddUint64(&stats.Failed, 1)
		}
	}
}

// min returns the smaller of two uint64 values
func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// getEventCount fetches the current number of events
func getEventCount(client api.LiveOpsServiceClient, ctx context.Context) (int, error) {
	resp, err := client.ListEvents(ctx, &api.Empty{})
	if err != nil {
		return 0, fmt.Errorf("failed to fetch event count: %v", err)
	}
	return len(resp.Events), nil
}

func main() {
	// Sign in first to get JWT token
	log.Println("Signing in as admin...")
	if err := signIn(); err != nil {
		log.Fatalf("Failed to sign in: %v", err)
	}
	log.Println("Successfully signed in")

	// Connect to gRPC server
	conn, err := grpc.Dial(
		grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("Failed to dial gRPC server: %v", err)
	}
	defer conn.Close()

	grpcClient := api.NewLiveOpsServiceClient(conn)

	// Context for the stress test duration
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	// Add JWT token to context
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+jwtToken)

	// Operations to run during the test (excluding DeleteEvent)
	operations := []struct {
		name    string
		fn      func(api.LiveOpsServiceClient) func(context.Context, *opStats)
		httpFn  func() func(context.Context, *opStats)
		stats   opStats
		workers int
	}{
		{"CreateEvent", createEvent, nil, opStats{}, grpcConcurrency},
		{"UpdateEvent", updateEvent, nil, opStats{}, grpcConcurrency},
		{"ListEvents", listEvents, nil, opStats{}, grpcConcurrency},
		{"FetchActiveEvents", nil, fetchActiveEvents, opStats{}, httpConcurrency},
		{"FetchEventByID", nil, fetchEventByID, opStats{}, httpConcurrency},
	}

	// Delete operation stats (run at the end)
	deleteStats := opStats{}

	var wg sync.WaitGroup
	log.Printf("Starting stress test for %v with %d gRPC workers and %d HTTP workers, targeting HTTP: %s, gRPC: %s", duration, grpcConcurrency, httpConcurrency, httpAddr, grpcAddr)

	// Start workers for all operations except delete
	for i := range operations {
		op := &operations[i]
		for j := 0; j < op.workers; j++ {
			wg.Add(1)
			go func(op *struct {
				name    string
				fn      func(api.LiveOpsServiceClient) func(context.Context, *opStats)
				httpFn  func() func(context.Context, *opStats)
				stats   opStats
				workers int
			}) {
				defer wg.Done()
				if op.fn != nil {
					stressOperation(ctx, op.name, op.fn(grpcClient), &op.stats)
				} else {
					stressOperation(ctx, op.name, op.httpFn(), &op.stats)
				}
			}(op)
		}
	}

	// Wait for the stress test duration
	wg.Wait()

	// Perform deletions equal to successful creations
	createSuccess := operations[0].stats.Success // CreateEvent is index 0
	log.Printf("Cleaning up: Deleting %d events (equal to successful creations)...", createSuccess)
	// deleteEvents(ctx, grpcClient, &deleteStats, createSuccess)

	// Get final event count
	finalCtx, finalCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finalCancel()
	finalCtx = metadata.AppendToOutgoingContext(finalCtx, "authorization", "Bearer "+jwtToken)
	eventCount, err := getEventCount(grpcClient, finalCtx)
	if err != nil {
		log.Printf("Error fetching final event count: %v", err)
		eventCount = -1
	}

	// Generate report
	log.Println("Stress Test Report:")
	log.Println("===================")
	totalRequests := uint64(0)
	totalSuccess := uint64(0)
	for _, op := range operations {
		total := op.stats.Success + op.stats.Failed + op.stats.RateLimited
		totalRequests += total
		totalSuccess += op.stats.Success
		getterCount := op.stats.Success
		if op.name != "ListEvents" && op.name != "FetchActiveEvents" && op.name != "FetchEventByID" {
			getterCount = 0
		}
		log.Printf("%s:", op.name)
		log.Printf("  Total Requests: %d", total)
		log.Printf("  Success: %d", op.stats.Success)
		log.Printf("  Failed: %d", op.stats.Failed)
		log.Printf("  Rate Limited: %d", op.stats.RateLimited)
		log.Printf("  Getter (Reads): %d", getterCount)
	}
	// Add DeleteEvent stats
	total := deleteStats.Success + deleteStats.Failed + deleteStats.RateLimited
	totalRequests += total
	totalSuccess += deleteStats.Success
	log.Printf("DeleteEvent:")
	log.Printf("  Total Requests: %d", total)
	log.Printf("  Success: %d", deleteStats.Success)
	log.Printf("  Failed: %d", deleteStats.Failed)
	log.Printf("  Rate Limited: %d", deleteStats.RateLimited)
	log.Printf("  Getter (Reads): 0")

	successRate := float64(totalSuccess) / float64(totalRequests) * 100
	log.Printf("Summary:")
	log.Printf("  Total Requests: %d", totalRequests)
	log.Printf("  Total Success: %d (%.2f%%)", totalSuccess, successRate)
	log.Printf("  Events in Database: %d", eventCount)
	log.Println("===================")
}
