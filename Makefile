# Set the Go binary
GO := go

# Set the project name
PROJECT := selenium-cleaner
BINARY_FOLDER := ./bin
BINARY := $(BINARY_FOLDER)/$(PROJECT)

# Build the application
build: clean dir
	$(GO) build -o $(BINARY) ./cmd/$(PROJECT)/main.go

dir:
	mkdir -p $(BINARY_FOLDER)

# Run the application
run: build
	./$(PROJECT) -host localhost -port 4444 -namespace selenium -max-age 7200

# Run with custom flags.  Example:
run-custom: build
	./$(PROJECT) -host my-grid-host -port 4445 -namespace my-namespace -max-age 3600

# Test the application
test:
	$(GO) test ./... -coverprofile=coverage.out

# Generate a test coverage report
coverage: test
	$(GO) tool cover -html=coverage.out -o coverage.html

# Clean up build artifacts
clean:
	rm -rf $(PROJECT) $(BINARY) coverage.out coverage.html

# Install dependencies
deps:
	$(GO) mod tidy

# Lint the code
lint:
	golangci-lint run

.PHONY: build run test coverage clean deps lint run-custom
