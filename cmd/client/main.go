package main

import (
    "context"
    "liveops/api"
    "log"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    "google.golang.org/grpc/metadata"
)

func main() {
    // Dial the gRPC server (plaintext since server uses h2c)
    conn, err := grpc.Dial(
        "localhost:8080",
        grpc.WithTransportCredentials(insecure.NewCredentials()), // No TLS for h2c
    )
    if err != nil {
        log.Fatalf("Failed to dial server: %v", err)
    }
    defer conn.Close()

    // Create a client for LiveOpsService
    client := api.NewLiveOpsServiceClient(conn)

    // Set up context with timeout and authorization header
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    // Add Authorization metadata
    ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "admin-key-456")

    // Get current time and 1 hour from now as Unix timestamps
    now := time.Now()
    startTime := now.Unix()
    endTime := now.Add(time.Hour).Unix()

    // Prepare the EventRequest
    req := &api.EventRequest{
        Id:          "evt3",
        Title:       "New Event",
        Description: "A freshly created event",
        StartTime:   startTime, // int64 Unix timestamp
        EndTime:     endTime,   // int64 Unix timestamp
        Rewards:     `{"gold": 500}`,
    }

    // Call CreateEvent
    resp, err := client.CreateEvent(ctx, req)
    if err != nil {
        log.Fatalf("Failed to create event: %v", err)
    }

    // Print the response
    log.Printf("Event created successfully:")
    log.Printf("ID: %s", resp.Id)
    log.Printf("Title: %s", resp.Title)
    log.Printf("Description: %s", resp.Description)
    log.Printf("StartTime: %d", resp.StartTime)
    log.Printf("EndTime: %d", resp.EndTime)
    log.Printf("Rewards: %s", resp.Rewards)
}