package main

import (
	"context"
	"encoding/json"
	"fmt"
	"liveops/api" // Your proto-generated package
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/exp/rand"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
    defaultServerURL     = "http://go-nacon:8080" // HTTP default
    defaultGRPCServerURL = "go-nacon:8080"        // gRPC default
    grpcAuthKey          = "admin-key-456"        // gRPC auth
    httpAuthKey          = "public-key-123"       // HTTP auth
)

var (
    serverURL     = getEnv("SERVER_URL", defaultServerURL)
    grpcServerURL = getEnv("GRPC_SERVER_URL", defaultGRPCServerURL)
    httpClient    = &http.Client{Timeout: 5 * time.Second}
)

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
    if value, exists := os.LookupEnv(key); exists {
        return value
    }
    return defaultValue
}

// rootCmd represents the base command
var rootCmd = &cobra.Command{
    Use:   "liveops-cli",
    Short: "CLI for interacting with liveops server (HTTP and gRPC)",
    Long:  `A custom CLI tool to interact with the liveops server via HTTP and gRPC, with auto-populated data options.`,
}

// interactCmd triggers the interactive mode
var interactCmd = &cobra.Command{
    Use:   "interact",
    Short: "Interact with the server interactively",
    Run: func(cmd *cobra.Command, args []string) {
        runInteractive()
    },
}

func init() {
    rootCmd.AddCommand(interactCmd)
}

func main() {
    if err := rootCmd.Execute(); err != nil {
        log.Fatalf("Failed to execute CLI: %v", err)
    }
}

// runInteractive handles the interactive CLI flow
func runInteractive() {
    // Connect to gRPC server
    conn, err := grpc.Dial(
        grpcServerURL,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    if err != nil {
        log.Fatalf("Failed to connect to gRPC server at %s: %v", grpcServerURL, err)
    }
    defer conn.Close()
    grpcClient := api.NewLiveOpsServiceClient(conn)

    // Context with auth
    ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", grpcAuthKey)

    // Main interaction loop
    for {
        action := selectAction()
        if action == "exit" {
            fmt.Println("Exiting...")
            return
        }

        switch action {
        case "create":
            sendCreateEvent(ctx, grpcClient)
        case "update":
            sendUpdateEvent(ctx, grpcClient)
        case "delete":
            sendDeleteEvent(ctx, grpcClient)
        case "list":
            sendListEvents(ctx, grpcClient)
        case "fetch-active":
            fetchActiveEvents()
        case "fetch-by-id":
            fetchEventByID()
        }
    }
}

// selectAction prompts the user to choose an action
func selectAction() string {
    options := []string{
        "create (gRPC: CreateEvent)",
        "update (gRPC: UpdateEvent)",
        "delete (gRPC: DeleteEvent)",
        "list (gRPC: ListEvents)",
        "fetch-active (HTTP: GET /events)",
        "fetch-by-id (HTTP: GET /events/{id})",
        "exit",
    }
    prompt := &survey.Select{
        Message: "Select an action:",
        Options: options,
    }
    var choice string
    survey.AskOne(prompt, &choice)
    return strings.Split(choice, " ")[0] // Extract action name
}

// sendCreateEvent handles CreateEvent
func sendCreateEvent(ctx context.Context, client api.LiveOpsServiceClient) {
    req := &api.EventRequest{
        Id:          uuid.New().String(),
        Title:       "Event-" + randomString(8),
        Description: randomString(20),
        StartTime:   time.Now().Unix(),
        EndTime:     time.Now().Add(2 * time.Hour).Unix(),
        Rewards:     `{"gold": 500}`, // Default value
    }

    editRequest("CreateEvent", req)
    resp, err := client.CreateEvent(ctx, req)
    if err != nil {
        log.Printf("Failed to create event: %v", err)
        return
    }
    printResponse("CreateEvent", resp)
}

// sendUpdateEvent handles UpdateEvent
func sendUpdateEvent(ctx context.Context, client api.LiveOpsServiceClient) {
    id := promptForID("Enter event ID to update")
    if id == "" {
        return
    }
    req := &api.EventRequest{
        Id:          id,
        Title:       "Updated-" + randomString(8),
        Description: randomString(20),
        StartTime:   time.Now().Unix(),
        EndTime:     time.Now().Add(2 * time.Hour).Unix(),
        Rewards:     `{"gold": 750}`, // Default value
    }

    editRequest("UpdateEvent", req)
    resp, err := client.UpdateEvent(ctx, req)
    if err != nil {
        log.Printf("Failed to update event: %v", err)
        return
    }
    printResponse("UpdateEvent", resp)
}

// sendDeleteEvent handles DeleteEvent
func sendDeleteEvent(ctx context.Context, client api.LiveOpsServiceClient) {
    id := promptForID("Enter event ID to delete")
    if id == "" {
        return
    }
    req := &api.DeleteRequest{Id: id}

    editDeleteRequest(req)
    resp, err := client.DeleteEvent(ctx, req)
    if err != nil {
        log.Printf("Failed to delete event: %v", err)
        return
    }
    printResponse("DeleteEvent", resp)
}

// sendListEvents handles ListEvents
func sendListEvents(ctx context.Context, client api.LiveOpsServiceClient) {
    req := &api.Empty{}
    resp, err := client.ListEvents(ctx, req)
    if err != nil {
        log.Printf("Failed to list events: %v", err)
        return
    }
    printResponse("ListEvents", resp)
}

// fetchActiveEvents handles GET /events
func fetchActiveEvents() {
    req, err := http.NewRequest("GET", serverURL+"/events", nil)
    if err != nil {
        log.Printf("Failed to create request: %v", err)
        return
    }
    req.Header.Set("Authorization", httpAuthKey)

    resp, err := httpClient.Do(req)
    if err != nil {
        log.Printf("Failed to fetch active events: %v", err)
        return
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        log.Printf("Request failed with status: %d", resp.StatusCode)
        return
    }

    var events interface{}
    if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
        log.Printf("Failed to decode response: %v", err)
        return
    }
    printResponse("FetchActiveEvents", events)
}

// fetchEventByID handles GET /events/{id}
func fetchEventByID() {
    id := promptForID("Enter event ID to fetch")
    if id == "" {
        return
    }

    req, err := http.NewRequest("GET", serverURL+"/events/"+id, nil)
    if err != nil {
        log.Printf("Failed to create request: %v", err)
        return
    }
    req.Header.Set("Authorization", httpAuthKey)

    resp, err := httpClient.Do(req)
    if err != nil {
        log.Printf("Failed to fetch event: %v", err)
        return
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        log.Printf("Request failed with status: %d", resp.StatusCode)
        return
    }

    var event interface{}
    if err := json.NewDecoder(resp.Body).Decode(&event); err != nil {
        log.Printf("Failed to decode response: %v", err)
        return
    }
    printResponse("FetchEventByID", event)
}

// editRequest allows editing of EventRequest fields
func editRequest(method string, req *api.EventRequest) {
    questions := []*survey.Question{
        {
            Name:   "id",
            Prompt: &survey.Input{Message: "ID:", Default: req.Id},
        },
        {
            Name:   "title",
            Prompt: &survey.Input{Message: "Title:", Default: req.Title},
        },
        {
            Name:   "description",
            Prompt: &survey.Input{Message: "Description:", Default: req.Description},
        },
        {
            Name:   "startTime",
            Prompt: &survey.Input{Message: "Start Time (Unix):", Default: strconv.FormatInt(req.StartTime, 10)},
        },
        {
            Name:   "endTime",
            Prompt: &survey.Input{Message: "End Time (Unix):", Default: strconv.FormatInt(req.EndTime, 10)},
        },
        {
            Name:   "rewards",
            Prompt: &survey.Input{Message: "Rewards (JSON):", Default: req.Rewards},
        },
    }

    answers := struct {
        ID          string
        Title       string
        Description string
        StartTime   string
        EndTime     string
        Rewards     string
    }{}

    survey.Ask(questions, &answers)

    startTime, _ := strconv.ParseInt(answers.StartTime, 10, 64)
    endTime, _ := strconv.ParseInt(answers.EndTime, 10, 64)

    req.Id = answers.ID
    req.Title = answers.Title
    req.Description = answers.Description
    req.StartTime = startTime
    req.EndTime = endTime
    req.Rewards = answers.Rewards
}

// editDeleteRequest allows editing of DeleteRequest
func editDeleteRequest(req *api.DeleteRequest) {
    prompt := &survey.Input{
        Message: "ID:",
        Default: req.Id,
    }
    survey.AskOne(prompt, &req.Id)
}

// promptForID prompts for an event ID
func promptForID(message string) string {
    var id string
    prompt := &survey.Input{Message: message}
    survey.AskOne(prompt, &id)
    return id
}

// randomString generates a random string
func randomString(length int) string {
    const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    b := make([]byte, length)
    for i := range b {
        b[i] = charset[rand.Intn(len(charset))]
    }
    return string(b)
}

// printResponse formats and prints the response
func printResponse(method string, resp interface{}) {
    data, err := json.MarshalIndent(resp, "", "  ")
    if err != nil {
        log.Printf("Failed to marshal response: %v", err)
        return
    }
    fmt.Printf("%s Response:\n%s\n\n", method, string(data))
}