// internal/cleaner/cleaner.go
package cleaner

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/maxkulish/selenium-grid-cleaner/internal/downloader"
	"github.com/maxkulish/selenium-grid-cleaner/internal/kubernetes"
	"k8s.io/apimachinery/pkg/watch"
)

// SessionInfo holds information about a grid session
type SessionInfo struct {
    NodeIP    string    // IP address of the node
    StartTime time.Time // Session start time
    SessionID string    // Selenium session ID
    PodName   string    // Kubernetes pod name
    URI       string    // Node URI
}

// Cleaner handles the cleaning of old grid sessions
type Cleaner struct {
    k8sClient   *kubernetes.Client
    maxParallel int
    errors      []error
    mutex       sync.Mutex
}

// NewCleaner creates a new instance of Cleaner
func NewCleaner(k8sClient *kubernetes.Client, maxParallel int) *Cleaner {
    if maxParallel <= 0 {
        maxParallel = 10 // default value
    }

    return &Cleaner{
        k8sClient:   k8sClient,
        maxParallel: maxParallel,
        errors:      make([]error, 0),
    }
}

// addError thread-safely adds an error to the error list
func (c *Cleaner) addError(err error) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    c.errors = append(c.errors, err)
}

// parseSessionInfo extracts session information from grid status
func (c *Cleaner) parseSessionInfo(status *downloader.Status) ([]SessionInfo, error) {
    var sessions []SessionInfo

    for _, node := range status.Value.Nodes {
        nodeURL, err := url.Parse(node.URI)
        if err != nil {
            return nil, fmt.Errorf("failed to parse node URI %s: %w", node.URI, err)
        }

        nodeIP := strings.Split(nodeURL.Host, ":")[0]
        if nodeIP == "" || nodeIP == "localhost" {
            log.Printf("Warning: Invalid node IP from URI %s", node.URI)
            continue
        }

        for _, slot := range node.Slots {
            if slot.Session.SessionID == "" {
                continue
            }

            startTime, err := time.Parse(time.RFC3339Nano, slot.LastStarted)
            if err != nil {
                log.Printf("Warning: Could not parse start time for session %s: %v",
                    slot.Session.SessionID, err)
                continue
            }

            sessions = append(sessions, SessionInfo{
                NodeIP:    nodeIP,
                StartTime: startTime,
                SessionID: slot.Session.SessionID,
                URI:       node.URI,
            })
        }
    }

    return sessions, nil
}

// getPodName retrieves the pod name for a given node IP
func (c *Cleaner) getPodName(ctx context.Context, nodeIP string) (string, error) {
    pods, err := c.k8sClient.GetPodsByIP(ctx, nodeIP)
    if err != nil {
        return "", fmt.Errorf("failed to get pods by IP %s: %w", nodeIP, err)
    }

    if len(pods) == 0 {
        return "", fmt.Errorf("no pod found for IP %s", nodeIP)
    }

    return pods[0], nil
}

// waitForPodDeletion waits for the pod to be deleted
func (c *Cleaner) waitForPodDeletion(ctx context.Context, podName string) error {
    watcher, err := c.k8sClient.WatchPod(ctx, podName)
    if err != nil {
        return fmt.Errorf("failed to create pod watcher: %w", err)
    }
    defer watcher.Stop()

    timeout := time.After(2 * time.Minute)
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-timeout:
            return fmt.Errorf("timeout waiting for pod %s deletion", podName)
        case event, ok := <-watcher.ResultChan():
            if !ok {
                return fmt.Errorf("watch channel closed unexpectedly")
            }
            switch event.Type {
            case watch.Deleted:
                return nil
            case watch.Error:
                return fmt.Errorf("error watching pod %s: %v", podName, event.Object)
            }
        }
    }
}

// cleanupSession handles the cleanup of a single session
func (c *Cleaner) cleanupSession(ctx context.Context, session SessionInfo) error {
    logger := log.Default()
    logger.Printf("Processing session %s on node %s", session.SessionID, session.NodeIP)

    podName, err := c.getPodName(ctx, session.NodeIP)
    if err != nil {
        return fmt.Errorf("failed to get pod name for IP %s: %w", session.NodeIP, err)
    }

    // Delete the pod
    if err := c.k8sClient.DeletePod(ctx, podName); err != nil {
        return fmt.Errorf("failed to delete pod %s: %w", podName, err)
    }

    // Wait for pod deletion confirmation
    if err := c.waitForPodDeletion(ctx, podName); err != nil {
        return fmt.Errorf("failed to confirm pod %s deletion: %w", podName, err)
    }

    logger.Printf("Successfully deleted pod %s for session %s", podName, session.SessionID)
    return nil
}

// CleanPods identifies and terminates Selenium Grid pods that have been running longer than the specified duration
func (c *Cleaner) CleanPods(ctx context.Context, status *downloader.Status, maxAge time.Duration) error {
    log.Printf("Starting pod cleanup with max age of %v", maxAge)

    sessions, err := c.parseSessionInfo(status)
    if err != nil {
        return fmt.Errorf("failed to parse session info: %w", err)
    }

    sessionCount := len(sessions)
    log.Printf("Found %d active sessions", sessionCount)

    if sessionCount == 0 {
        log.Println("No sessions to clean up")
        return nil
    }

    var wg sync.WaitGroup
    sem := make(chan struct{}, c.maxParallel)

    for _, session := range sessions {
        age := time.Since(session.StartTime)
        if age <= maxAge {
            log.Printf("Session %s age %v is within limit, skipping",
                session.SessionID, age.Round(time.Second))
            continue
        }

        log.Printf("Session %s has been running for %v, exceeding max age of %v",
            session.SessionID, age.Round(time.Second), maxAge)

        wg.Add(1)
        sem <- struct{}{}

        go func(session SessionInfo) {
            defer wg.Done()
            defer func() { <-sem }()

            if err := c.cleanupSession(ctx, session); err != nil {
                log.Printf("Failed to cleanup session %s: %v", session.SessionID, err)
                c.addError(fmt.Errorf("failed to cleanup session %s: %w", session.SessionID, err))
            }
        }(session)
    }

    wg.Wait()

    if len(c.errors) > 0 {
        return fmt.Errorf("encountered %d errors during cleanup: %v", len(c.errors), c.errors)
    }

    log.Println("Pod cleanup completed successfully")
    return nil
}
