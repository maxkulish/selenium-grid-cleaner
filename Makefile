# Set the Go binary
GO := go

# Set the project name and paths
PROJECT := selenium-cleaner
BINARY_DIR := bin
BINARY := $(BINARY_DIR)/$(PROJECT)
CMD_DIR := cmd/$(PROJECT)

# Define default configuration (can be overridden by environment variables)
GRID_HOST ?= localhost
GRID_PORT ?= 4444
NAMESPACE ?= selenium
MAX_AGE ?= 7200

# Build the application
build: clean mkdir
	$(GO) build -o $(BINARY) $(CMD_DIR)/main.go

# Create necessary directories
mkdir:
	mkdir -p $(BINARY_DIR)

# Run the application with configurable parameters from environment variables or command line
run: build
	$(BINARY) \
		-host $(GRID_HOST) \
		-port $(GRID_PORT) \
		-namespace $(NAMESPACE) \
		-max-age $(MAX_AGE)

# Run tests
test:
	$(GO) test $(CMD_DIR) -coverprofile=coverage.out

# Generate a test coverage report
coverage: test
	$(GO) tool cover -html=coverage.out -o coverage.html

# Clean up build artifacts
clean:
	rm -rf $(BINARY_DIR) coverage.out coverage.html

# Install dependencies
deps:
	$(GO) mod tidy

# Lint the code
lint:
	golangci-lint run

# Port-forward to the Kubernetes cluster (requires kubectl to be installed and configured)
port-forward:
	kubectl port-forward svc/selenium-grid 4444:4444 -n $(NAMESPACE)

# Helper target to run the port-forward in the background.
# Use Ctrl+C to stop it.
port-forward-bg:
	kubectl port-forward svc/selenium-grid 4444:4444 -n $(NAMESPACE) &

.PHONY: build run test coverage clean deps lint port-forward port-forward-bg
