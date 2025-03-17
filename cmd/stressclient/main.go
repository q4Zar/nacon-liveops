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
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	defaultDuration        = 60 * time.Second       // 1 minute stress test
	defaultGRPCConcurrency = 10                     // Workers for gRPC operations
	defaultHTTPConcurrency = 2                      // Workers for HTTP operations
	defaultRequestDelay    = 5 * time.Millisecond   // Throttle requests
	defaultUsername        = "admin"                // Default admin username
	defaultPassword        = "admin-key-456"        // Default admin password
	defaultDeleteTimeout   = 500 * time.Millisecond // Default timeout for delete operations
	defaultDeleteRetries   = 0                      // Default number of retries for delete operations
	defaultHTTPTimeout     = 5 * time.Second        // Default HTTP client timeout
	defaultMaxIdleConns    = 100                    // Default max idle connections
	defaultIdleConnTimeout = 90 * time.Second       // Default idle connection timeout
)

var (
	eventIDs      []string   // Store successfully created event IDs
	eventIDsMutex sync.Mutex // Protect access to eventIDs
	httpClient    = &http.Client{
		Timeout: getHTTPTimeout(),
		Transport: &http.Transport{
			MaxIdleConns:        getMaxIdleConns(),
			IdleConnTimeout:     getIdleConnTimeout(),
			DisableCompression:  true,
			MaxIdleConnsPerHost: getMaxIdleConns(),
		},
	}
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

func getHTTPTimeout() time.Duration {
	if timeout := os.Getenv("HTTP_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			return t
		}
	}
	return defaultHTTPTimeout
}

func getMaxIdleConns() int {
	if conns := os.Getenv("MAX_IDLE_CONNS"); conns != "" {
		if c, err := strconv.Atoi(conns); err == nil && c > 0 {
			return c
		}
	}
	return defaultMaxIdleConns
}

func getIdleConnTimeout() time.Duration {
	if timeout := os.Getenv("IDLE_CONN_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			return t
		}
	}
	return defaultIdleConnTimeout
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
	Success      uint64
	Failed       uint64
	RateLimited  uint64
	TotalLatency time.Duration // Total latency for successful requests
	MaxLatency   time.Duration // Maximum latency observed
	MinLatency   time.Duration // Minimum latency observed
	Count        uint64        // Number of latency measurements
}

// recordLatency safely records latency metrics
func (s *opStats) recordLatency(d time.Duration) {
	atomic.AddUint64(&s.Count, 1)
	atomic.AddInt64((*int64)(&s.TotalLatency), int64(d))

	for {
		current := atomic.LoadInt64((*int64)(&s.MaxLatency))
		if d <= time.Duration(current) {
			break
		}
		if atomic.CompareAndSwapInt64((*int64)(&s.MaxLatency), current, int64(d)) {
			break
		}
	}

	for {
		current := atomic.LoadInt64((*int64)(&s.MinLatency))
		if current != 0 && d >= time.Duration(current) {
			break
		}
		if atomic.CompareAndSwapInt64((*int64)(&s.MinLatency), current, int64(d)) {
			break
		}
	}
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
			start := time.Now()
			fn(ctx, stats)
			if stats.Count > 0 { // Only record latency for successful operations
				stats.recordLatency(time.Since(start))
			}
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

// retryableHTTPRequest performs an HTTP request with retries
func retryableHTTPRequest(ctx context.Context, req *http.Request, maxRetries int) (*http.Response, error) {
	var lastErr error
	for retry := 0; retry <= maxRetries; retry++ {
		if retry > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(retry) * time.Second):
				// Exponential backoff
			}
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			if isRetryableError(err) {
				lastErr = err
				continue
			}
			return nil, err
		}

		// Check if the status code is retryable
		if resp.StatusCode >= 500 && resp.StatusCode != http.StatusTooManyRequests {
			resp.Body.Close()
			lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
			continue
		}

		return resp, nil
	}
	return nil, fmt.Errorf("max retries reached: %v", lastErr)
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for network errors
	if netErr, ok := err.(net.Error); ok {
		return netErr.Temporary() || netErr.Timeout()
	}

	// Check for specific error strings
	errStr := err.Error()
	retryableErrors := []string{
		"connection reset by peer",
		"broken pipe",
		"connection refused",
		"no such host",
	}

	for _, rerr := range retryableErrors {
		if strings.Contains(errStr, rerr) {
			return true
		}
	}

	return false
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

		resp, err := retryableHTTPRequest(ctx, req, 3)
		if err != nil {
			if ctx.Err() != nil {
				// Context cancelled or deadline exceeded
				return
			}
			log.Printf("Failed to fetch active events after retries: %v", err)
			atomic.AddUint64(&stats.Failed, 1)
			return
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
			atomic.AddUint64(&stats.Success, 1)
		case http.StatusTooManyRequests:
			atomic.AddUint64(&stats.RateLimited, 1)
		default:
			log.Printf("GET /events failed with status %d", resp.StatusCode)
			atomic.AddUint64(&stats.Failed, 1)
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

		// Retry loop for delete operation
		var err error
		success := false
		for retry := 0; retry <= deleteRetries; retry++ {
			req := &api.DeleteRequest{Id: ids[i]}
			_, err = client.DeleteEvent(deleteCtx, req)
			if err == nil {
				atomic.AddUint64(&stats.Success, 1)
				success = true
				break
			}

			// Check if error is retryable
			if st, ok := status.FromError(err); ok {
				switch st.Code() {
				case codes.DeadlineExceeded, codes.Unavailable, codes.Internal:
					if retry < deleteRetries {
						log.Printf("Retry %d/%d: Failed to delete event %s: %v", retry+1, deleteRetries, ids[i], err)
						time.Sleep(time.Second * time.Duration(retry+1)) // Exponential backoff
						continue
					}
				case codes.NotFound:
					// If the event is not found, consider it a success since it's already deleted
					atomic.AddUint64(&stats.Success, 1)
					success = true
					break
				}
			}
			break // Non-retryable error or max retries reached
		}

		cancel() // Cancel the context right after the operation

		if !success {
			log.Printf("Failed to delete event %s: %v", ids[i], err)
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

	// Configure gRPC connection with retry and backoff
	conn, err := grpc.Dial(
		grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  100 * time.Millisecond,
				Multiplier: 1.6,
				Jitter:     0.2,
				MaxDelay:   3 * time.Second,
			},
			MinConnectTimeout: 5 * time.Second,
		}),
		grpc.WithDefaultServiceConfig(`{
			"loadBalancingPolicy": "round_robin",
			"retryPolicy": {
				"MaxAttempts": 4,
				"InitialBackoff": "0.1s",
				"MaxBackoff": "3s",
				"BackoffMultiplier": 2.0,
				"RetryableStatusCodes": [ "UNAVAILABLE" ]
			}
		}`),
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
	deleteEvents(ctx, grpcClient, &deleteStats, createSuccess)

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

		// Add latency statistics if we have measurements
		if op.stats.Count > 0 {
			avgLatency := time.Duration(int64(op.stats.TotalLatency) / int64(op.stats.Count))
			log.Printf("  Latency Statistics:")
			log.Printf("    Average: %v", avgLatency)
			log.Printf("    Min: %v", op.stats.MinLatency)
			log.Printf("    Max: %v", op.stats.MaxLatency)
		}
	}

	// Add DeleteEvent stats with latency
	total := deleteStats.Success + deleteStats.Failed + deleteStats.RateLimited
	totalRequests += total
	totalSuccess += deleteStats.Success
	log.Printf("DeleteEvent:")
	log.Printf("  Total Requests: %d", total)
	log.Printf("  Success: %d", deleteStats.Success)
	log.Printf("  Failed: %d", deleteStats.Failed)
	log.Printf("  Rate Limited: %d", deleteStats.RateLimited)
	log.Printf("  Getter (Reads): 0")

	if deleteStats.Count > 0 {
		avgLatency := time.Duration(int64(deleteStats.TotalLatency) / int64(deleteStats.Count))
		log.Printf("  Latency Statistics:")
		log.Printf("    Average: %v", avgLatency)
		log.Printf("    Min: %v", deleteStats.MinLatency)
		log.Printf("    Max: %v", deleteStats.MaxLatency)
	}

	successRate := float64(totalSuccess) / float64(totalRequests) * 100
	log.Printf("Summary:")
	log.Printf("  Total Requests: %d", totalRequests)
	log.Printf("  Total Success: %d (%.2f%%)", totalSuccess, successRate)
	log.Printf("  Events in Database: %d", eventCount)
	log.Println("===================")
}
