// cmd/selenium-cleaner/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/maxkulish/selenium-grid-cleaner/internal/cleaner"
	"github.com/maxkulish/selenium-grid-cleaner/internal/downloader"
	"github.com/maxkulish/selenium-grid-cleaner/internal/kubernetes"
	"github.com/maxkulish/selenium-grid-cleaner/internal/portforwarder"
)

func printConfig(params map[string]interface{}) {
	var maxKeyLength int
	for k := range params {
		if len(k) > maxKeyLength {
			maxKeyLength = len(k)
		}
	}

	var output strings.Builder
	output.WriteString("\nSelenium Grid Cleaner Configuration:\n")
	output.WriteString(strings.Repeat("=", 50) + "\n")

	for k, v := range params {
		padding := strings.Repeat(" ", maxKeyLength-len(k))
		output.WriteString(fmt.Sprintf("%s%s : %v\n", k, padding, v))
	}
	output.WriteString(strings.Repeat("=", 50) + "\n")

	log.Print(output.String())
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[Selenium Cleaner] ")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create a WaitGroup to ensure all cleanup is done before exiting
	var wg sync.WaitGroup

	// Command line flags
	kubeContext := flag.String("context", "", "Kubernetes context to use")
	seleniumGridPort := flag.Int("port", 4444, "Selenium Grid port")
	seleniumGridNamespace := flag.String("namespace", "selenium", "Selenium Grid namespace")
	seleniumGridServiceName := flag.String("service", "selenium-router", "Selenium Grid service name")
	podLifetimeHours := flag.Float64("lifetime", 2.0, "Pod lifetime in hours")
	flag.Parse()

	// Log configuration parameters
	config := map[string]interface{}{
		"Kubernetes Context": func() string {
			if *kubeContext == "" {
				return "default from kubeconfig"
			}
			return *kubeContext
		}(),
		"Grid Port":      *seleniumGridPort,
		"Grid Namespace": *seleniumGridNamespace,
		"Grid Service":   *seleniumGridServiceName,
		"Pod Lifetime":   fmt.Sprintf("%.1f hours", *podLifetimeHours),
		"Kubeconfig": func() string {
			if kc := os.Getenv("KUBECONFIG"); kc != "" {
				return kc
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return "unknown"
			}
			return filepath.Join(home, ".kube", "config")
		}(),
	}
	printConfig(config)

	podLifetime := time.Duration(*podLifetimeHours * float64(time.Hour))

	log.Println("Starting port forwarder...")
	// Port-forwarding
	pf, err := portforwarder.NewPortForwarder(*seleniumGridNamespace, *seleniumGridServiceName, *seleniumGridPort)
	if err != nil {
		log.Fatalf("Failed to create port-forwarder: %v", err)
	}

	// Add to WaitGroup before starting
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		log.Println("Shutting down port forwarder...")
		pf.Stop()
	}()

	if err := pf.Start(ctx); err != nil {
		log.Fatalf("Failed to start port-forwarding: %v", err)
	}

	seleniumGridURL := fmt.Sprintf("http://localhost:%d/wd/hub/status", *seleniumGridPort)
	localSeleniumGridURL := pf.GetLocalURL(seleniumGridURL)

	log.Println("Downloading Selenium Grid status...")
	// Download status.json
	status, err := downloader.DownloadStatus(localSeleniumGridURL)
	if err != nil {
		log.Fatalf("Failed to download status: %v", err)
	}

	log.Println("Creating Kubernetes client...")
	// Kubernetes client
	k8sClient, err := kubernetes.NewClient(*kubeContext)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	log.Println("Starting pod cleanup...")
	// Clean pods
	err = cleaner.CleanPods(ctx, status, k8sClient, podLifetime)
	if err != nil {
		log.Fatalf("Failed to clean pods: %v", err)
	}

	log.Println("Selenium cleaner finished successfully.")
	// Cancel context to initiate cleanup
	cancel()

	// Wait for cleanup to complete
	wg.Wait()
	log.Println("Cleanup completed, exiting...")
}
