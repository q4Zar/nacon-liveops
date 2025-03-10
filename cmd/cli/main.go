package main

import (
	"context"
	"encoding/json"
	"fmt"
	"liveops/api"
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
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type User struct {
    ID       uint   `gorm:"primaryKey"`
    Username string `gorm:"unique;not null"`
    Key      string `gorm:"unique;not null"`
    Type     string `gorm:"not null;check:type IN ('admin', 'public')"`
}

const (
    defaultServerURL     = "http://go-nacon:8080"
    defaultGRPCServerURL = "go-nacon:8080"
)

var (
    serverURL     = getEnv("SERVER_URL", defaultServerURL)
    grpcServerURL = getEnv("GRPC_SERVER_URL", defaultGRPCServerURL)
    httpClient    = &http.Client{Timeout: 5 * time.Second}
    db            *gorm.DB
)

func getEnv(key, defaultValue string) string {
    if value, exists := os.LookupEnv(key); exists {
        return value
    }
    return defaultValue
}

var rootCmd = &cobra.Command{Use: "liveops-cli", Short: "CLI for liveops server"}
var interactCmd = &cobra.Command{Use: "interact", Short: "Interact with server", Run: func(cmd *cobra.Command, args []string) { runInteractive() }}
var createUserCmd = &cobra.Command{Use: "create-user", Short: "Create a user", Run: func(cmd *cobra.Command, args []string) { createUser() }}
var deleteUserCmd = &cobra.Command{Use: "delete-user", Short: "Delete a user", Run: func(cmd *cobra.Command, args []string) { deleteUser() }}

func init() {
    rootCmd.AddCommand(interactCmd, createUserCmd, deleteUserCmd)
    var err error
    db, err = gorm.Open(sqlite.Open("./db/users.db"), &gorm.Config{})
    if err != nil {
        log.Fatalf("Failed to open database: %v", err)
    }
    if err := db.AutoMigrate(&User{}); err != nil {
        log.Fatalf("Failed to migrate users table: %v", err)
    }
}

func main() {
    if err := rootCmd.Execute(); err != nil {
        log.Fatalf("Failed to execute CLI: %v", err)
    }
}

func runInteractive() {
    conn, err := grpc.Dial(grpcServerURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        log.Fatalf("Failed to connect to gRPC server: %v", err)
    }
    defer conn.Close()
    grpcClient := api.NewLiveOpsServiceClient(conn)

    for {
        action := selectAction()
        if action == "exit" {
            fmt.Println("Exiting...")
            return
        }
        key := promptForKey()
        ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", key)

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
            fetchActiveEvents(key)
        case "fetch-by-id":
            fetchEventByID(key)
        }
    }
}

func promptForKey() string {
    var username string
    prompt := &survey.Input{Message: "Enter username for auth:"}
    survey.AskOne(prompt, &username)
    var user User
    if err := db.Where("username = ?", username).First(&user).Error; err != nil {
        log.Printf("Failed to get key for %s: %v", username, err)
        return ""
    }
    return user.Key
}

func createUser() {
    var username, key, userType string
    prompt := &survey.Input{Message: "Username:"}
    survey.AskOne(prompt, &username)
    prompt = &survey.Input{Message: "Key:"}
    survey.AskOne(prompt, &key)
    prompt = &survey.Select{Message: "Type:", Options: []string{"admin", "public"}}
    survey.AskOne(prompt, &userType)

    user := User{Username: username, Key: key, Type: userType}
    if err := db.Create(&user).Error; err != nil {
        log.Printf("Failed to create user: %v", err)
        return
    }
    fmt.Printf("Created user: %s (%s)\n", username, userType)
}

func deleteUser() {
    var username string
    prompt := &survey.Input{Message: "Username to delete:"}
    survey.AskOne(prompt, &username)

    result := db.Where("username = ?", username).Delete(&User{})
    if err := result.Error; err != nil {
        log.Printf("Failed to delete user: %v", err)
        return
    }
    if result.RowsAffected == 0 {
        fmt.Println("No user found")
    } else {
        fmt.Printf("Deleted user: %s\n", username)
    }
}

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
    prompt := &survey.Select{Message: "Select an action:", Options: options}
    var choice string
    survey.AskOne(prompt, &choice)
    return strings.Split(choice, " ")[0]
}

func sendCreateEvent(ctx context.Context, client api.LiveOpsServiceClient) {
    req := &api.EventRequest{
        Id:          uuid.New().String(),
        Title:       "Event-" + randomString(8),
        Description: randomString(20),
        StartTime:   timestamppb.Now(),
        EndTime:     timestamppb.New(time.Now().Add(2 * time.Hour)),
        Rewards:     `{"gold": 500}`,
    }
    editRequest("CreateEvent", req)
    resp, err := client.CreateEvent(ctx, req)
    if err != nil {
        log.Printf("Failed to create event: %v", err)
        return
    }
    printResponse("CreateEvent", resp)
}

func sendUpdateEvent(ctx context.Context, client api.LiveOpsServiceClient) {
    id := promptForID("Enter event ID to update")
    if id == "" {
        return
    }
    req := &api.EventRequest{
        Id:          id,
        Title:       "Updated-" + randomString(8),
        Description: randomString(20),
        StartTime:   timestamppb.Now(),
        EndTime:     timestamppb.New(time.Now().Add(2 * time.Hour)),
        Rewards:     `{"gold": 750}`,
    }
    editRequest("UpdateEvent", req)
    resp, err := client.UpdateEvent(ctx, req)
    if err != nil {
        log.Printf("Failed to update event: %v", err)
        return
    }
    printResponse("UpdateEvent", resp)
}

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

func sendListEvents(ctx context.Context, client api.LiveOpsServiceClient) {
    resp, err := client.ListEvents(ctx, &api.Empty{})
    if err != nil {
        log.Printf("Failed to list events: %v", err)
        return
    }
    printResponse("ListEvents", resp)
}

func fetchActiveEvents(key string) {
    req, _ := http.NewRequest("GET", serverURL+"/events", nil)
    req.Header.Set("Authorization", key)
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
    json.NewDecoder(resp.Body).Decode(&events)
    printResponse("FetchActiveEvents", events)
}

func fetchEventByID(key string) {
    id := promptForID("Enter event ID to fetch")
    if id == "" {
        return
    }
    req, _ := http.NewRequest("GET", serverURL+"/events/"+id, nil)
    req.Header.Set("Authorization", key)
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
    json.NewDecoder(resp.Body).Decode(&event)
    printResponse("FetchEventByID", event)
}

func editRequest(method string, req *api.EventRequest) {
    questions := []*survey.Question{
        {Name: "id", Prompt: &survey.Input{Message: "ID:", Default: req.Id}},
        {Name: "title", Prompt: &survey.Input{Message: "Title:", Default: req.Title}},
        {Name: "description", Prompt: &survey.Input{Message: "Description:", Default: req.Description}},
        {Name: "startTime", Prompt: &survey.Input{Message: "Start Time (Unix):", Default: strconv.FormatInt(req.StartTime.AsTime().Unix(), 10)}},
        {Name: "endTime", Prompt: &survey.Input{Message: "End Time (Unix):", Default: strconv.FormatInt(req.EndTime.AsTime().Unix(), 10)}},
        {Name: "rewards", Prompt: &survey.Input{Message: "Rewards (JSON):", Default: req.Rewards}},
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
    req.StartTime = timestamppb.New(time.Unix(startTime, 0))
    req.EndTime = timestamppb.New(time.Unix(endTime, 0))
    req.Rewards = answers.Rewards
}

func editDeleteRequest(req *api.DeleteRequest) {
    prompt := &survey.Input{Message: "ID:", Default: req.Id}
    survey.AskOne(prompt, &req.Id)
}

func promptForID(message string) string {
    var id string
    prompt := &survey.Input{Message: message}
    survey.AskOne(prompt, &id)
    return id
}

func randomString(length int) string {
    const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    b := make([]byte, length)
    for i := range b {
        b[i] = charset[rand.Intn(len(charset))]
    }
    return string(b)
}

func printResponse(method string, resp interface{}) {
    data, err := json.MarshalIndent(resp, "", "  ")
    if err != nil {
        log.Printf("Failed to marshal response: %v", err)
        return
    }
    fmt.Printf("%s Response:\n%s\n\n", method, string(data))
}