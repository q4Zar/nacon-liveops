package main

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"liveops/api"
	"log"
	"math/big"
	"math/rand"
	"net/http"
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
    grpcAddr         = "localhost:8080"
    httpAddr         = "http://localhost:8080"
    duration         = 60 * time.Second // 1 minute stress test
    grpcConcurrency  = 10              // Keep high for gRPC
    httpConcurrency  = 2               // Reduce for HTTP to hit rate limit moderately
    grpcAuthKey      = "admin-key-456" // For gRPC
    httpAuthKey      = "public-key-123" // For HTTP
    requestDelay     = 5 * time.Millisecond // Slight delay to avoid resource exhaustion
)

var (
    eventIDs      []string      // Store created event IDs
    eventIDsMutex sync.Mutex    // Protect access to eventIDs
    httpClient    = &http.Client{Timeout: 5 * time.Second}
)

type opStats struct {
    Success     uint64
    Failed      uint64
    RateLimited uint64
}

func init() {
    rand.Seed(time.Now().UnixNano())
}

// randomString generates a random string of given length using crypto/rand
func randomString(length int) string {
    const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    b := make([]byte, length)
    for i := range b {
        num, _ := crand.Int(crand.Reader, big.NewInt(int64(len(charset))))
        b[i] = charset[num.Int64()]
    }
    return string(b)
}

// randomEventID returns a random event ID from the created list
func randomEventID() string {
    eventIDsMutex.Lock()
    defer eventIDsMutex.Unlock()
    if len(eventIDs) == 0 {
        return ""
    }
    return eventIDs[rand.Intn(len(eventIDs))]
}

// stressOperation runs a function continuously until the context expires
func stressOperation(ctx context.Context, name string, fn func(context.Context, *opStats), stats *opStats) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            fn(ctx, stats)
            time.Sleep(requestDelay) // Throttle requests
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
        id := randomEventID()
        if id == "" {
            return
        }
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

// deleteEvent sends a gRPC DeleteEvent request
func deleteEvent(client api.LiveOpsServiceClient) func(context.Context, *opStats) {
    return func(ctx context.Context, stats *opStats) {
        id := randomEventID()
        if id == "" {
            return
        }
        req := &api.DeleteRequest{Id: id}

        _, err := client.DeleteEvent(ctx, req)
        if err != nil {
            log.Printf("Failed to delete event %s: %v", id, err)
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

// fetchActiveEvents makes an HTTP GET /events request
func fetchActiveEvents() func(context.Context, *opStats) {
    return func(ctx context.Context, stats *opStats) {
        req, err := http.NewRequestWithContext(ctx, "GET", httpAddr+"/events", nil)
        if err != nil {
            log.Printf("Failed to create GET /events request: %v", err)
            atomic.AddUint64(&stats.Failed, 1)
            return
        }
        req.Header.Set("Authorization", httpAuthKey)

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
        id := randomEventID()
        if id == "" {
            return
        }
        url := fmt.Sprintf("%s/events/%s", httpAddr, id)
        req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
        if err != nil {
            log.Printf("Failed to create GET /events/%s request: %v", id, err)
            atomic.AddUint64(&stats.Failed, 1)
            return
        }
        req.Header.Set("Authorization", httpAuthKey)

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

// getEventCount fetches the current number of events via ListEvents
func getEventCount(client api.LiveOpsServiceClient, ctx context.Context) (int, error) {
    resp, err := client.ListEvents(ctx, &api.Empty{})
    if err != nil {
        return 0, fmt.Errorf("failed to fetch event count: %v", err)
    }
    return len(resp.Events), nil
}

func main() {
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

    // Context with 1-minute timeout for stress test
    ctx, cancel := context.WithTimeout(context.Background(), duration)
    defer cancel()

    // Add gRPC authorization header
    ctx = metadata.AppendToOutgoingContext(ctx, "authorization", grpcAuthKey)

    // Operations to stress test
    operations := []struct {
        name    string
        fn      func(api.LiveOpsServiceClient) func(context.Context, *opStats) // gRPC
        httpFn  func() func(context.Context, *opStats)                         // HTTP
        stats   opStats
        workers int
    }{
        {"CreateEvent", createEvent, nil, opStats{}, grpcConcurrency},
        {"UpdateEvent", updateEvent, nil, opStats{}, grpcConcurrency},
        {"DeleteEvent", deleteEvent, nil, opStats{}, grpcConcurrency},
        {"ListEvents", listEvents, nil, opStats{}, grpcConcurrency},
        {"FetchActiveEvents", nil, fetchActiveEvents, opStats{}, httpConcurrency},
        {"FetchEventByID", nil, fetchEventByID, opStats{}, httpConcurrency},
    }

    var wg sync.WaitGroup
    log.Printf("Starting continuous stress test for %v with %d gRPC workers and %d HTTP workers per operation", duration, grpcConcurrency, httpConcurrency)

    // Start workers for each operation
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

    // Wait for the duration to complete
    wg.Wait()

    // Get final event count
    finalCtx, finalCancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer finalCancel()
    finalCtx = metadata.AppendToOutgoingContext(finalCtx, "authorization", grpcAuthKey)
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
    successRate := float64(totalSuccess) / float64(totalRequests) * 100
    log.Printf("Summary:")
    log.Printf("  Total Requests: %d", totalRequests)
    log.Printf("  Total Success: %d (%.2f%%)", totalSuccess, successRate)
    log.Printf("  Events in Database: %d", eventCount)
    log.Println("===================")
}