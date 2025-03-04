#!/bin/bash

# Configuration
HOST="http://localhost:8080"
VALID_KEY="public-key-123"
INVALID_KEY="invalid-key"
EVENT_IDS=("evt1" "evt2" "nonexistent" "evt3") # Mix of assumed existing and non-existing IDs
CONCURRENT_REQUESTS=50 # Number of parallel requests per test
TOTAL_REQUESTS=200     # Total requests per endpoint
LOG_FILE="api_test_results.log"
OUTPUT_DIR="test_output"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Ensure output directory exists
mkdir -p "$OUTPUT_DIR"

# Function to log results
test_endpoint() {
    local endpoint="$1"
    local auth_key="$2"
    local output_file="$OUTPUT_DIR/${endpoint//\//_}_${TIMESTAMP}_$(basename "$auth_key").txt"
    local url="$HOST$endpoint"

    # Run curl with authentication and capture status
    status=$(curl -s -o "$output_file" -w "%{http_code}" -H "Authorization: $auth_key" "$url")
    if [ "$status" -eq 200 ]; then
        log_result "$endpoint" "$status" "Request succeeded"
    elif [ "$status" -eq 401 ]; then
        log_result "$endpoint" "$status" "Unauthorized - invalid key"
    elif [ "$status" -eq 404 ]; then
        log_result "$endpoint" "$status" "Not Found"
    else
        log_result "$endpoint" "$status" "Unexpected response"
    fi
}

# Function to log results (moved outside to be callable)
log_result() {
    local endpoint="$1"
    local status="$2"
    local message="$3"
    echo "$(date '+%Y-%m-%d %H:%M:%S') - Endpoint: $endpoint, Status: $status, Message: $message" >> "$LOG_FILE"
    if [ "$status" -ge 400 ]; then
        echo -e "${RED}[$endpoint] Failed - Status: $status - $message${NC}"
    else
        echo -e "${GREEN}[$endpoint] Success - Status: $status - $message${NC}"
    fi
}

# Export the function so subprocesses can use it
export -f test_endpoint
export -f log_result

# Function to run concurrent tests
run_concurrent_test() {
    local endpoint="$1"
    local auth_key="$2"
    echo "Starting hardcore test for $endpoint with $TOTAL_REQUESTS requests ($CONCURRENT_REQUESTS concurrent)..."

    # Generate a sequence of numbers and run test_endpoint in parallel
    seq 1 "$TOTAL_REQUESTS" | xargs -n1 -P"$CONCURRENT_REQUESTS" -I{} bash -c "test_endpoint '$endpoint' '$auth_key'"
}

# Clear previous log file
> "$LOG_FILE"

# Test 1: GET /events with valid key
echo "Testing GET /events with valid key..."
run_concurrent_test "/events" "$VALID_KEY"

# Test 2: GET /events with invalid key
echo "Testing GET /events with invalid key..."
run_concurrent_test "/events" "$INVALID_KEY"

# Test 3: GET /events/{id} with valid key for multiple IDs
for id in "${EVENT_IDS[@]}"; do
    echo "Testing GET /events/$id with valid key..."
    run_concurrent_test "/events/$id" "$VALID_KEY"
done

# Test 4: GET /events/{id} with invalid key for multiple IDs
for id in "${EVENT_IDS[@]}"; do
    echo "Testing GET /events/$id with invalid key..."
    run_concurrent_test "/events/$id" "$INVALID_KEY"
done

# Summary
echo -e "\nTest Summary:"
total_requests=$((TOTAL_REQUESTS * (2 + 2 * ${#EVENT_IDS[@]}))) # 2 for /events, 2 per ID
echo "Total requests sent: $total_requests"
echo "Results logged to: $LOG_FILE"
echo "Raw responses saved in: $OUTPUT_DIR"
echo "Check $LOG_FILE for detailed results."