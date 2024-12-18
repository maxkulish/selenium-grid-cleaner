package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	// Create command
	cmd := exec.Command("kubectl", "run", "curl-status",
		"--image=curlimages/curl",
		"--rm",
		"-i",
		"--restart=Never",
		"--",
		"curl",
		"-s", // Silent mode for curl
		"http://selenium-router:4444/status")

	// Create buffer for output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Stderr: %s\n", stderr.String())
		os.Exit(1)
	}

	// Get output and clean it
	output := stdout.String()

	// Find the last occurrence of '}'
	lastBrace := strings.LastIndex(output, "}")
	if lastBrace == -1 {
		fmt.Fprintf(os.Stderr, "No valid JSON found in output\n")
		os.Exit(1)
	}

	// Extract just the JSON part
	jsonStr := output[:lastBrace+1]

	// Validate JSON
	var jsonMap map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &jsonMap); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid JSON: %v\n", err)
		os.Exit(1)
	}

	// Pretty print the JSON
	prettyJSON, err := json.MarshalIndent(jsonMap, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to format JSON: %v\n", err)
		os.Exit(1)
	}

	// Write the cleaned and formatted JSON to file
	err = os.WriteFile("status.json", prettyJSON, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Successfully retrieved and validated Selenium status to status.json")
}
