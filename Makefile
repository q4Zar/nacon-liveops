.PHONY: server serverd tests stress cli logs help

# Default target when just running 'make'
.DEFAULT_GOAL := help

server: ## Start the server 
	sh compile-proto.sh
	docker compose up --build go-nacon redis

serverd: ## Start the server in detached mode
	sh compile-proto.sh
	docker compose up --build -d go-nacon redis

# Run all tests
tests: ## Run all tests
	go test -v ./...

# Run stress tests
stress: ## Run stress tests
	docker compose up --build stressclient

# Run CLI in interactive mode
cli: ## Run CLI in interactive mode
	docker compose run --rm -it --build cli

# Show logs
logs: ## Show logs from all containers
	docker compose logs -f

# Show help
help: ## Show this help message
	@echo 'Usage:'
	@echo '  make <target>'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*##"; printf "\033[36m"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-15s %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } END {printf "\033[0m"}' $(MAKEFILE_LIST)


