# Selenium Grid Cleaner

A Go utility for automatically cleaning up long-running Selenium Grid sessions in Kubernetes clusters. This tool helps maintain cluster resources by identifying and terminating Selenium Grid pods that have exceeded a specified lifetime.

## Features

- Automatic detection and cleanup of long-running Selenium Grid sessions
- Kubernetes integration with support for both in-cluster and external access
- Configurable session lifetime threshold
- Parallel processing of cleanup operations
- Port forwarding support for accessing Selenium Grid
- Detailed logging and error reporting

## Prerequisites

- Go 1.19 or later
- Access to a Kubernetes cluster
- `kubectl` installed and configured
- Selenium Grid running in your Kubernetes cluster

## Installation

Clone the repository:

```bash
git clone https://github.com/maxkulish/selenium-grid-cleaner.git
cd selenium-grid-cleaner
```

Build the binary:

```bash
make build
```

## Configuration

The cleaner can be configured using command-line flags:

| Flag          | Description                           | Default Value      |
|---------------|---------------------------------------|-------------------|
| `-context`    | Kubernetes context to use             | Current context   |
| `-port`       | Selenium Grid port                    | 4444              |
| `-namespace`  | Selenium Grid namespace               | selenium          |
| `-service`    | Selenium Grid service name            | selenium-router   |
| `-lifetime`   | Pod lifetime in hours                 | 2.0               |

## Usage Examples

1. Basic usage with default settings:
```bash
./bin/selenium-cleaner
```

2. Specify a different namespace and pod lifetime:
```bash
./bin/selenium-cleaner -namespace testing -lifetime 1.5
```

3. Use a specific Kubernetes context and custom port:
```bash
./bin/selenium-cleaner -context prod-cluster -port 4445
```

4. Full configuration example:
```bash
./bin/selenium-cleaner \
  -context my-cluster \
  -namespace selenium-test \
  -service selenium-hub \
  -port 4446 \
  -lifetime 3.5
```

You can also use environment variables to configure the application:

```bash
export GRID_HOST=selenium-hub.example.com
export GRID_PORT=4444
export NAMESPACE=selenium
export MAX_AGE=7200

make run
```

## Development

### Project Structure

```
├── cmd/
│   └── selenium-cleaner/
│       └── main.go
├── internal/
│   ├── cleaner/
│   ├── downloader/
│   ├── kubernetes/
│   └── portforwarder/
├── Makefile
└── README.md
```

### Available Make Commands

- `make build` - Build the application
- `make run` - Build and run with default configuration
- `make test` - Run tests
- `make coverage` - Generate test coverage report
- `make clean` - Clean build artifacts
- `make deps` - Install dependencies
- `make lint` - Run linter
- `make port-forward` - Set up port forwarding to Selenium Grid
- `make port-forward-bg` - Set up port forwarding in background

## How It Works

1. The tool establishes a connection to your Kubernetes cluster
2. Sets up port forwarding to access the Selenium Grid service
3. Downloads and analyzes the current Grid status
4. Identifies sessions that have exceeded the configured lifetime
5. Terminates the corresponding pods in parallel
6. Waits for confirmation of pod deletion

## Error Handling

The cleaner implements comprehensive error handling:
- Validates all configuration parameters
- Implements timeouts for operations
- Provides detailed error messages
- Ensures clean shutdown on interruption

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.
