package main

import (
	"context"
	crand "crypto/rand" // Renamed to avoid conflict
	"encoding/json"
	"fmt"
	"io"
	"liveops/api"
	"log"
	"math/big"
	"math/rand" // For rand.Intn and rand.Seed
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
    grpcAddr      = "localhost:8080"
    httpAddr      = "http://localhost:8080/events"
    numRequests   = 100  // Number of events to create and fetch attempts
    concurrency   = 20   // Number of concurrent goroutines
    grpcAuthKey   = "admin-key-456"  // For gRPC CreateEvent
    httpAuthKey   = "public-key-123" // For HTTP GET /events
)

func init() {
    // Seed math/rand with a unique value
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

// createEvent sends a gRPC CreateEvent request with random data
func createEvent(client api.LiveOpsServiceClient, ctx context.Context, wg *sync.WaitGroup) {
    defer wg.Done()

    id := uuid.New().String() // Random UUID for event ID
    now := time.Now()
    req := &api.EventRequest{
        Id:          id,
        Title:       "Event-" + randomString(8),
        Description: randomString(20),
        StartTime:   now.Unix(),
        EndTime:     now.Add(2 * time.Hour).Unix(), // Active for 2 hours
        Rewards:     fmt.Sprintf(`{"gold": %d}`, rand.Intn(1000)), // Random gold 0-999
    }

    resp, err := client.CreateEvent(ctx, req)
    if err != nil {
        log.Printf("Failed to create event %s: %v", id, err)
        return
    }
    log.Printf("Created event %s: %s", resp.Id, resp.Title)
}

// fetchEvents makes an HTTP GET /events request
func fetchEvents(wg *sync.WaitGroup) {
    defer wg.Done()

    req, err := http.NewRequest("GET", httpAddr, nil)
    if err != nil {
        log.Printf("Failed to create HTTP request: %v", err)
        return
    }
    req.Header.Set("Authorization", httpAuthKey)

    client := &http.Client{Timeout: 5 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        log.Printf("Failed to fetch events: %v", err)
        return
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        log.Printf("Failed to read response: %v", err)
        return
    }

    if resp.StatusCode != http.StatusOK {
        log.Printf("HTTP request failed with status %d: %s", resp.StatusCode, string(body))
        return
    }

    var events []map[string]interface{} // Simple parsing for logging
    if err := json.Unmarshal(body, &events); err != nil {
        log.Printf("Failed to unmarshal events: %v", err)
        return
    }
    log.Printf("Fetched %d active events", len(events))
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

    // Context with timeout for all requests
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Add gRPC authorization header
    ctx = metadata.AppendToOutgoingContext(ctx, "authorization", grpcAuthKey)

    // WaitGroup for synchronization
    var wg sync.WaitGroup

    // Burst create events
    log.Printf("Starting burst: creating %d events with %d concurrent requests", numRequests, concurrency)
    start := time.Now()
    for i := 0; i < numRequests; i++ {
        wg.Add(1)
        go createEvent(grpcClient, ctx, &wg)
        if i%concurrency == concurrency-1 {
            // Wait briefly to avoid overwhelming the server too quickly
            time.Sleep(50 * time.Millisecond)
        }
    }
    wg.Wait()
    log.Printf("Event creation burst completed in %v", time.Since(start))

    // Burst fetch events
    log.Printf("Starting burst: fetching events %d times with %d concurrent requests", numRequests, concurrency)
    start = time.Now()
    for i := 0; i < numRequests; i++ {
        wg.Add(1)
        go fetchEvents(&wg)
        if i%concurrency == concurrency-1 {
            time.Sleep(50 * time.Millisecond)
        }
    }
    wg.Wait()
    log.Printf("Event fetch burst completed in %v", time.Since(start))

    log.Println("Stress test completed")
}