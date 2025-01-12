// internal/downloader/downloader.go
package downloader

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	dataDirName  = "data"
	statusFile   = "status.json"
	permissions  = 0644
)

type Status struct {
	Value struct {
		Message string `json:"message"`
		Nodes   []struct {
			ID        string `json:"id"`
			URI       string `json:"uri"`
			Slots     []struct {
				ID      struct {
					HostID string `json:"hostId"`
					ID     string `json:"id"`
				} `json:"id"`
				LastStarted string `json:"lastStarted"`
				Session     struct {
					SessionID  string `json:"sessionId"`
					Start     string `json:"start"`
					URI       string `json:"uri"`
				} `json:"session"`
			} `json:"slots"`
		} `json:"nodes"`
	} `json:"value"`
}

// getDataDir returns the path to the data directory
func getDataDir() (string, error) {
	// Get the executable's directory
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Use the directory containing the executable
	baseDir := filepath.Dir(execPath)
	dataDir := filepath.Join(baseDir, dataDirName)

	return dataDir, nil
}

// ensureDataDir creates the data directory if it doesn't exist
func ensureDataDir() (string, error) {
	dataDir, err := getDataDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create data directory: %w", err)
	}
	return dataDir, nil
}

// downloadFile downloads the status from the given URL and saves it to the data directory
func downloadFile(url string) (string, error) {
	dataDir, err := ensureDataDir()
	if err != nil {
		return "", err
	}

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("http get error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Create a timestamped filename
	timestamp := time.Now().UTC().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s", timestamp, statusFile)
	filePath := filepath.Join(dataDir, filename)

	// Create the file
	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy the response body to the file
	if _, err := io.Copy(file, resp.Body); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	// Create/Update symlink to latest status file
	latestLink := filepath.Join(dataDir, statusFile)
	_ = os.Remove(latestLink) // Remove existing symlink if it exists
	if err := os.Symlink(filePath, latestLink); err != nil {
		return "", fmt.Errorf("failed to create symlink: %w", err)
	}

	return filePath, nil
}

// parseStatusFile reads and parses the status file
func parseStatusFile(filePath string) (*Status, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read status file: %w", err)
	}

	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status file: %w", err)
	}

	return &status, nil
}

// DownloadStatus downloads the status from the URL, saves it to a file, and returns the parsed status
func DownloadStatus(url string) (*Status, error) {
	// Download and save the file
	filePath, err := downloadFile(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download status: %w", err)
	}

	// Parse the saved file
	status, err := parseStatusFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse status: %w", err)
	}

	return status, nil
}
