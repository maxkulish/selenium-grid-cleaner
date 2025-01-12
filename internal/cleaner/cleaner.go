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
	log.Println(now)
	return nil
}
