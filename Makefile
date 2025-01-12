# Set the Go binary path
GO := go

# Project settings
BINARY := bin/selenium-cleaner
CMD_DIR := cmd/selenium-cleaner

# Go test settings
TEST_COVERAGE := coverage.out
TEST_REPORT := coverage.html

# Build settings
LDFLAGS := -s -w

.PHONY: build clean test coverage deps lint port-forward port-forward-bg run

# Build the application with optimizations
build: clean
	mkdir -p bin
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BINARY) $(CMD_DIR)/main.go

# Run the application (builds first)
run: build
	$(BINARY)

# Run all tests
test:
	$(GO) test ./... -coverprofile=$(TEST_COVERAGE)

# Generate test coverage report
coverage: test
	$(GO) tool cover -html=$(TEST_COVERAGE) -o $(TEST_REPORT)
	@echo "Coverage report generated: $(TEST_REPORT)"

# Clean up build artifacts
clean:
	rm -rf bin $(TEST_COVERAGE) $(TEST_REPORT)

# Install and verify dependencies
deps:
	$(GO) mod tidy
	$(GO) mod verify

# Run golangci-lint
lint:
	golangci-lint run ./...

# Helper targets for port forwarding
port-forward:
	kubectl port-forward svc/selenium-grid 4444:4444

port-forward-bg:
	kubectl port-forward svc/selenium-grid 4444:4444 &
