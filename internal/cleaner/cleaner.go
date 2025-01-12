// internal/cleaner/cleaner.go
package cleaner

import (
	"context"
	"log"
	"time"

	"github.com/maxkulish/selenium-grid-cleaner/internal/downloader"
	"github.com/maxkulish/selenium-grid-cleaner/internal/kubernetes"
)

func CleanPods(ctx context.Context, status *downloader.Status, k8sClient *kubernetes.Client, lifetime time.Duration) error {
	now := time.Now()
	for _, node := range status.Value.Nodes {
		startTime := time.Unix(node.StartTime/1000, 0) // Convert milliseconds to seconds
		if now.Sub(startTime) > lifetime && node.SessionId != "" {
			podName, err := k8sClient.GetPodNameBySessionID(ctx, node.SessionId)
			if err != nil {
				log.Printf("Failed to get pod name for session %s: %v", node.SessionId, err)
				continue
			}
			if podName != "" {
				if err := k8sClient.DeletePod(ctx, podName); err != nil {
					log.Printf("Failed to delete pod %s: %v", podName, err)
				} else {
					log.Printf("Deleted pod %s (session %s)", podName, node.SessionId)
				}
			} else {
				log.Printf("Pod not found for session %s", node.SessionId)
			}
		}
	}
	return nil
}
