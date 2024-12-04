package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// Config holds the application configuration
type Config struct {
	GridHost    string `yaml:"gridHost"`
	GridPort    string `yaml:"gridPort"`
	Namespace   string `yaml:"namespace"`
	MaxAgeSecs  int64  `yaml:"maxAgeSecs"`
	HealthURL   string `yaml:"healthUrl"`
	MaxRetries  int    `yaml:"maxRetries"`
	MaxParallel int    `yaml:"maxParallel"`
	LogLevel    string `yaml:"logLevel"`
	LogFormat   string `yaml:"logFormat"`
	ConfigPath  string `yaml:"configPath"`
}

// GridStatus represents the response from the grid status endpoint
type GridStatus struct {
	Value struct {
		Ready bool `json:"ready"`
		Nodes []struct {
			URI   string `json:"uri"`
			Slots []struct {
				Session *struct {
					Start     string `json:"start"`
					SessionID string `json:"sessionId"`
				} `json:"session"`
				State string `json:"state"`
			} `json:"slots"`
		} `json:"nodes"`
	} `json:"value"`
}

// SessionInfo holds information about a grid session
type SessionInfo struct {
	NodeIP    string
	StartTime time.Time
	SessionID string
	PodName   string
	SlotState string
}

// GridCleaner handles the cleaning of old grid sessions
type GridCleaner struct {
	config Config
	client *http.Client
	logger *logrus.Logger
	errors []error
	mutex  sync.Mutex
}

// NewGridCleaner creates a new instance of GridCleaner
func NewGridCleaner(config Config) (*GridCleaner, error) {
	logger := logrus.New()

	// Configure logging
	level, err := logrus.ParseLevel(config.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}
	logger.SetLevel(level)

	if config.LogFormat == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
	}

	return &GridCleaner{
		config: config,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
		errors: make([]error, 0),
	}, nil
}

// addError thread-safely adds an error to the error list
func (gc *GridCleaner) addError(err error) {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()
	gc.errors = append(gc.errors, err)
}

// checkGridHealth verifies the Selenium Grid health status
func (gc *GridCleaner) checkGridHealth(ctx context.Context) error {
	healthURL := gc.config.HealthURL
	if healthURL == "" {
		healthURL = fmt.Sprintf("http://%s:%s/status", gc.config.GridHost, gc.config.GridPort)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := gc.client.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unhealthy grid status: %d", resp.StatusCode)
	}

	var status GridStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return fmt.Errorf("failed to decode health status: %w", err)
	}

	if !status.Value.Ready {
		return fmt.Errorf("grid is not ready")
	}

	return nil
}

// getGridStatus fetches the current status from the grid with retries
func (gc *GridCleaner) getGridStatus(ctx context.Context) (*GridStatus, error) {
	var status *GridStatus
	var lastErr error

	for i := 0; i < gc.config.MaxRetries; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			if i > 0 {
				time.Sleep(time.Duration(i) * time.Second)
			}

			url := fmt.Sprintf("http://%s:%s/status", gc.config.GridHost, gc.config.GridPort)
			gc.logger.WithField("url", url).Debug("Fetching grid status")

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				lastErr = fmt.Errorf("failed to create request: %w", err)
				continue
			}

			resp, err := gc.client.Do(req)
			if err != nil {
				lastErr = fmt.Errorf("failed to get grid status: %w", err)
				continue
			}

			status = &GridStatus{}
			if err := json.NewDecoder(resp.Body).Decode(status); err != nil {
				resp.Body.Close()
				lastErr = fmt.Errorf("failed to decode response: %w", err)
				continue
			}
			resp.Body.Close()

			return status, nil
		}
	}

	return nil, fmt.Errorf("failed to get grid status after %d attempts: %w", gc.config.MaxRetries, lastErr)
}

// parseSessionInfo extracts session information from grid status
func (gc *GridCleaner) parseSessionInfo(status *GridStatus) ([]SessionInfo, error) {
	var sessions []SessionInfo

	for _, node := range status.Value.Nodes {
		nodeURL, err := url.Parse(node.URI)
		if err != nil {
			return nil, fmt.Errorf("failed to parse node URI %s: %w", node.URI, err)
		}

		nodeIP := strings.Split(nodeURL.Host, ":")[0]

		for _, slot := range node.Slots {
			if slot.Session == nil {
				continue
			}

			startTime, err := time.Parse(time.RFC3339, slot.Session.Start)
			if err != nil {
				gc.logger.WithError(err).WithField("startTime", slot.Session.Start).Error("Failed to parse start time")
				continue
			}

			sessions = append(sessions, SessionInfo{
				NodeIP:    nodeIP,
				StartTime: startTime,
				SessionID: slot.Session.SessionID,
				SlotState: slot.State,
			})
		}
	}

	return sessions, nil
}

// getPodName retrieves the pod name for a given node IP
func (gc *GridCleaner) getPodName(nodeIP string) (string, error) {
	cmd := exec.Command("kubectl", "get", "pods", "-n", gc.config.Namespace, "-o", "wide", "--field-selector", fmt.Sprintf("status.podIP=%s", nodeIP))
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get pod list: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return "", fmt.Errorf("no pod found for IP %s", nodeIP)
	}

	fields := strings.Fields(lines[1])
	if len(fields) == 0 {
		return "", fmt.Errorf("invalid pod data for IP %s", nodeIP)
	}

	return fields[0], nil
}

// deletePod removes a pod with proper error handling
func (gc *GridCleaner) deletePod(ctx context.Context, podName string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "delete", "pod", podName, "-n", gc.config.Namespace)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete pod %s: %w (output: %s)", podName, err, string(output))
	}
	return nil
}

// CheckAndCleanup performs the main cleanup operation
func (gc *GridCleaner) CheckAndCleanup(ctx context.Context) error {
	gc.logger.Info("Starting cleanup process")

	if err := gc.checkGridHealth(ctx); err != nil {
		return fmt.Errorf("grid health check failed: %w", err)
	}

	status, err := gc.getGridStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to get grid status: %w", err)
	}

	sessions, err := gc.parseSessionInfo(status)
	if err != nil {
		return fmt.Errorf("failed to parse session info: %w", err)
	}

	gc.logger.WithField("sessionCount", len(sessions)).Info("Found active sessions")

	var wg sync.WaitGroup
	sem := make(chan struct{}, gc.config.MaxParallel)

	for _, session := range sessions {
		age := time.Since(session.StartTime)
		logger := gc.logger.WithFields(logrus.Fields{
			"nodeIP":    session.NodeIP,
			"sessionID": session.SessionID,
			"age":       age.String(),
			"slotState": session.SlotState,
			"startTime": session.StartTime.Format(time.RFC3339),
		})

		if age.Seconds() <= float64(gc.config.MaxAgeSecs) {
			logger.Debug("Session within age limit, skipping cleanup")
			continue
		}

		logger.Info("Session exceeds age limit, initiating cleanup")

		podName, err := gc.getPodName(session.NodeIP)
		if err != nil {
			logger.WithError(err).Error("Failed to get pod name")
			gc.addError(fmt.Errorf("pod name retrieval failed for %s: %w", session.NodeIP, err))
			continue
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(session SessionInfo, podName string) {
			defer wg.Done()
			defer func() { <-sem }()

			logger := gc.logger.WithFields(logrus.Fields{
				"podName":   podName,
				"sessionID": session.SessionID,
				"nodeIP":    session.NodeIP,
				"slotState": session.SlotState,
				"age":       time.Since(session.StartTime).String(),
			})

			logger.Info("Attempting pod deletion")

			if err := gc.deletePod(ctx, podName); err != nil {
				logger.WithError(err).Error("Pod deletion failed")
				gc.addError(fmt.Errorf("failed to delete pod %s: %w", podName, err))
			} else {
				logger.Info("Successfully deleted pod")
			}
		}(session, podName)
	}

	wg.Wait()

	if len(gc.errors) > 0 {
		gc.logger.WithField("errorCount", len(gc.errors)).Error("Cleanup completed with errors")
		return fmt.Errorf("encountered %d errors during cleanup: %v", len(gc.errors), gc.errors)
	}

	gc.logger.Info("Cleanup completed successfully")
	return nil
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		GridHost:    "localhost",
		GridPort:    "4444",
		Namespace:   "selenium",
		MaxAgeSecs:  7200,
		MaxRetries:  3,
		MaxParallel: 10,
		LogLevel:    "info",
		LogFormat:   "text",
	}
}

// LoadConfig loads configuration from a YAML file or returns default config if path is empty
func LoadConfig(path string) (Config, error) {
	// Start with default configuration
	config := DefaultConfig()

	// If no config file specified, return defaults
	if path == "" {
		return config, nil
	}

	file, err := os.ReadFile(path)
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(file, &config); err != nil {
		return config, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Override empty values with defaults
	if config.GridHost == "" {
		config.GridHost = DefaultConfig().GridHost
	}
	if config.GridPort == "" {
		config.GridPort = DefaultConfig().GridPort
	}
	if config.Namespace == "" {
		config.Namespace = DefaultConfig().Namespace
	}
	if config.MaxAgeSecs == 0 {
		config.MaxAgeSecs = DefaultConfig().MaxAgeSecs
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = DefaultConfig().MaxRetries
	}
	if config.MaxParallel == 0 {
		config.MaxParallel = DefaultConfig().MaxParallel
	}
	if config.LogLevel == "" {
		config.LogLevel = DefaultConfig().LogLevel
	}
	if config.LogFormat == "" {
		config.LogFormat = DefaultConfig().LogFormat
	}

	return config, nil
}

func main() {
	var config Config
	var err error

	if len(os.Args) > 1 {
		config, err = LoadConfig(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config file: %v\nUsing default configuration\n", err)
			config = DefaultConfig()
		}
	} else {
		config = DefaultConfig()
		fmt.Println("No config file provided, using default configuration")
	}

	cleaner, err := NewGridCleaner(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create cleaner: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := cleaner.CheckAndCleanup(ctx); err != nil {
		cleaner.logger.WithError(err).Fatal("Cleanup failed")
	}
}
