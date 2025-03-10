package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"liveops/api"
	"liveops/internal/db"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

type Credentials struct {
    Username string    `json:"username"`
    UserType db.UserType `json:"user_type"`
    Token    string    `json:"token"` // base64 encoded username:password
}

var (
    serverURL     = getEnv("SERVER_URL", defaultServerURL)
    grpcServerURL = getEnv("GRPC_SERVER_URL", defaultGRPCServerURL)
    httpClient    = &http.Client{Timeout: 5 * time.Second}
    credentials   *Credentials
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

// userCmd represents the user management command
var userCmd = &cobra.Command{
    Use:   "user",
    Short: "User management commands",
    Long:  `Commands for managing users in the system.`,
}

// createUserCmd creates a new user
var createUserCmd = &cobra.Command{
    Use:   "create",
    Short: "Create a new user",
    Run: func(cmd *cobra.Command, args []string) {
        username, _ := cmd.Flags().GetString("username")
        password, _ := cmd.Flags().GetString("password")
        userType, _ := cmd.Flags().GetString("type")

        if username == "" || password == "" {
            log.Fatal("Username and password are required")
        }

        // Validate user type
        uType := db.UserType(userType)
        if uType != db.UserTypeHTTP && uType != db.UserTypeAdmin {
            log.Fatal("Invalid user type. Must be 'http' or 'admin'")
        }

        // Initialize database
        database, err := db.NewDB("./liveops.db")
        if err != nil {
            log.Fatalf("Failed to initialize database: %v", err)
        }

        userRepo := db.NewUserRepository(database.DB)
        err = userRepo.CreateUser(username, password, uType)
        if err != nil {
            log.Fatalf("Failed to create user: %v", err)
        }

        fmt.Printf("User %s created successfully with type %s\n", username, userType)
    },
}

func init() {
    rootCmd.AddCommand(interactCmd)
    rootCmd.AddCommand(userCmd)

    // User management command flags
    createUserCmd.Flags().String("username", "", "Username for the new user")
    createUserCmd.Flags().String("password", "", "Password for the new user")
    createUserCmd.Flags().String("type", "http", "User type (http or admin)")
    userCmd.AddCommand(createUserCmd)
}

func main() {
    if err := rootCmd.Execute(); err != nil {
        log.Fatalf("Failed to execute CLI: %v", err)
    }
}

// getCredentialsPath returns the path to the credentials file
func getCredentialsPath() string {
    homeDir, err := os.UserHomeDir()
    if err != nil {
        log.Fatal("Could not determine home directory")
    }
    return filepath.Join(homeDir, ".liveops-credentials")
}

// loadCredentials loads stored credentials if they exist
func loadCredentials() *Credentials {
    path := getCredentialsPath()
    data, err := os.ReadFile(path)
    if err != nil {
        return nil
    }

    var creds Credentials
    if err := json.Unmarshal(data, &creds); err != nil {
        return nil
    }
    return &creds
}

// saveCredentials saves credentials to disk
func saveCredentials(creds *Credentials) error {
    data, err := json.Marshal(creds)
    if err != nil {
        return err
    }
    return os.WriteFile(getCredentialsPath(), data, 0600)
}

// authenticate handles user authentication
func authenticate() error {
    // Check if we already have credentials
    if creds := loadCredentials(); creds != nil {
        credentials = creds
        return nil
    }

    // Ask whether to sign up or sign in
    var action string
    prompt := &survey.Select{
        Message: "Choose action:",
        Options: []string{"sign-in", "sign-up", "exit"},
    }
    survey.AskOne(prompt, &action)

    switch action {
    case "sign-up":
        return signUp()
    case "sign-in":
        return signIn()
    default:
        os.Exit(0)
        return nil
    }
}

func signUp() error {
    var username, password string
    survey.AskOne(&survey.Input{
        Message: "Enter username:",
    }, &username)

    survey.AskOne(&survey.Password{
        Message: "Enter password:",
    }, &password)

    typePrompt := &survey.Select{
        Message: "Select user type:",
        Options: []string{"http", "admin"},
    }
    var userType string
    survey.AskOne(typePrompt, &userType)

    // Create the user
    database, err := db.NewDB("./liveops.db")
    if err != nil {
        return fmt.Errorf("failed to initialize database: %v", err)
    }

    userRepo := db.NewUserRepository(database.DB)
    err = userRepo.CreateUser(username, password, db.UserType(userType))
    if err != nil {
        return fmt.Errorf("failed to create user: %v", err)
    }

    // Store credentials
    token := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
    credentials = &Credentials{
        Username: username,
        UserType: db.UserType(userType),
        Token:    token,
    }
    return saveCredentials(credentials)
}

func signIn() error {
    var username, password string
    survey.AskOne(&survey.Input{
        Message: "Enter username:",
    }, &username)

    survey.AskOne(&survey.Password{
        Message: "Enter password:",
    }, &password)

    // Validate credentials
    database, err := db.NewDB("./liveops.db")
    if err != nil {
        return fmt.Errorf("failed to initialize database: %v", err)
    }

    userRepo := db.NewUserRepository(database.DB)
    user, err := userRepo.ValidateUser(username, password)
    if err != nil {
        return fmt.Errorf("invalid credentials: %v", err)
    }

    // Store credentials
    token := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
    credentials = &Credentials{
        Username: username,
        UserType: user.Type,
        Token:    token,
    }
    return saveCredentials(credentials)
}

// runInteractive handles the interactive CLI flow
func runInteractive() {
    // First authenticate
    if err := authenticate(); err != nil {
        log.Fatalf("Authentication failed: %v", err)
    }

    fmt.Printf("Authenticated as %s (type: %s)\n", credentials.Username, credentials.UserType)

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

    // Context with auth - add "Basic " prefix
    authToken := "Basic " + credentials.Token
    fmt.Printf("Using authorization token: %s\n", authToken)
    ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", authToken)

    // Main interaction loop
    for {
        action := selectAction()
        if action == "exit" {
            fmt.Println("Exiting...")
            return
        }

        // Check user permissions
        if credentials.UserType == db.UserTypeHTTP && 
           (action == "create" || action == "update" || action == "delete" || action == "list") {
            fmt.Println("Error: HTTP users cannot use gRPC endpoints")
            continue
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
        case "create-user":
            createUserInteractive()
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
        "create-user (Create new user)",
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
    req.Header.Set("Authorization", "Basic "+credentials.Token)

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
    req.Header.Set("Authorization", "Basic "+credentials.Token)

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

func createUserInteractive() {
    var username, password string
    prompt := &survey.Input{
        Message: "Enter username:",
    }
    survey.AskOne(prompt, &username)

    survey.AskOne(&survey.Password{
        Message: "Enter password:",
    }, &password)

    typePrompt := &survey.Select{
        Message: "Select user type:",
        Options: []string{"http", "admin"},
    }
    var userType string
    survey.AskOne(typePrompt, &userType)

    // Initialize database
    database, err := db.NewDB("./liveops.db")
    if err != nil {
        log.Printf("Failed to initialize database: %v", err)
        return
    }

    userRepo := db.NewUserRepository(database.DB)
    err = userRepo.CreateUser(username, password, db.UserType(userType))
    if err != nil {
        log.Printf("Failed to create user: %v", err)
        return
    }

    fmt.Printf("User %s created successfully with type %s\n", username, userType)
}