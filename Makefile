.PHONY: server test test-event stress-test cli clean help

# Default target when just running 'make'
.DEFAULT_GOAL := help

server: ## Start the server 
	docker compose up --build go-nacon
serverd: ## Start the server in detached mode
	docker compose up --build -d go-nacon

# Run all tests
test: ## Run all tests
	go test -v ./...

# Run event service tests specifically
test-event: ## Run event service tests
	go test -v ./internal/event

# Run stress tests
stress-test: ## Run stress tests
	docker compose up --build stressclient

# Run stress tests
strategic-test: ## Run stress tests
	docker compose up --build strategicclient


# Run CLI in interactive mode
cli: ## Run CLI in interactive mode
	docker compose run --rm -it --build cli

# Stop and remove containers
clean: ## Stop and remove all containers
	docker compose down

# Show help
help: ## Show this help message
	@echo 'Usage:'
	@echo '  make <target>'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*##"; printf "\033[36m"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-15s %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } END {printf "\033[0m"}' $(MAKEFILE_LIST)

# Build all services
build: ## Build all services
	docker compose build

# Start all services
up: ## Start all services
	docker compose up -d

# Show logs
logs: ## Show logs from all containers
	docker compose logs -f

# Rebuild and restart server
restart-server: ## Rebuild and restart the server
	docker compose up -d --build go-nacon

# Run database migrations
migrate: ## Run database migrations
	docker compose run --rm go-nacon go run cmd/migrate/main.go 