// cmd/selenium-cleaner/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maxkulish/selenium-grid-cleaner/internal/downloader"
	"github.com/maxkulish/selenium-grid-cleaner/internal/kubernetes"
	"github.com/maxkulish/selenium-grid-cleaner/internal/portforwarder"

	"github.com/maxkulish/selenium-grid-cleaner/internal/cleaner"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Configuration (consider using flags or config file)
	seleniumGridURL := "http://localhost:4444/wd/hub/status"
	seleniumGridPort := 4444
	seleniumGridNamespace := "selenium"
	seleniumGridServiceName := "selenium-router"
	podLifetime := 2 * time.Hour

	// Port-forwarding
	pf, err := portforwarder.NewPortForwarder(seleniumGridNamespace, seleniumGridServiceName, seleniumGridPort)
	if err != nil {
		log.Fatalf("Failed to create port-forwarder: %v (namespace: %s, service: %s, port: %d)", err, seleniumGridNamespace, seleniumGridServiceName, seleniumGridPort)
	}

	if err := pf.Start(ctx); err != nil {
		log.Fatalf("Failed to start port-forwarding: %v", err)
	}
	defer pf.Stop()

	go func() { // Goroutine to handle signals
		<-ctx.Done()
		log.Println("Stopping port forwarding due to signal...")
		pf.Stop() // Explicitly stop port forwarding
		// Add other cleanup tasks here if needed.
		os.Exit(0) // or other appropriate exit code
	}()

	localSeleniumGridURL := pf.GetLocalURL(seleniumGridURL)

	// Download status.json
	status, err := downloader.DownloadStatus(localSeleniumGridURL)
	if err != nil {
		log.Fatalf("Failed to download status: %v", err)
	}

	// Kubernetes client
	k8sClient, err := kubernetes.NewClient()
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// Clean pods
	err = cleaner.CleanPods(ctx, status, k8sClient, podLifetime)
	if err != nil {
		log.Fatalf("Failed to clean pods: %v", err)
	}

	log.Println("Selenium cleaner finished.")
}
