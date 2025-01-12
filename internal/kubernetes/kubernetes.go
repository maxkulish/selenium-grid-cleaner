// internal/kubernetes/kubernetes.go
package kubernetes

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Client struct {
        clientset *kubernetes.Clientset
}

func NewClient() (*Client, error) {
        config, err := rest.InClusterConfig()
        if err != nil {
                return nil, fmt.Errorf("failed to create in cluster config: %w", err)
        }

        clientset, err := kubernetes.NewForConfig(config)
        if err != nil {
                return nil, fmt.Errorf("failed to create clientset: %w", err)
        }

        return &Client{clientset: clientset}, nil
}

func (c *Client) DeletePod(ctx context.Context, podName string) error {
        parts := strings.Split(podName, "-")
        if len(parts) < 2 {
                return fmt.Errorf("invalid pod name format: %s", podName)
        }
        namespace := parts[len(parts)-2]

        deletePolicy := metav1.DeletePropagationForeground
        deleteOptions := metav1.DeleteOptions{PropagationPolicy: &deletePolicy}

        err := c.clientset.CoreV1().Pods(namespace).Delete(ctx, podName, deleteOptions)
        if err != nil {
                return fmt.Errorf("failed to delete pod: %w", err)
        }
        return nil
}

func (c *Client) GetPodNameBySessionID(ctx context.Context, sessionID string) (string, error) {
    pods, err := c.clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
    if err != nil {
        return "", fmt.Errorf("failed to list pods: %w", err)
    }

    for _, pod := range pods.Items {
        for _, container := range pod.Spec.Containers {
            for _, envVar := range container.Env {
                if envVar.Name == "SE_SESSION_ID" && envVar.Value == sessionID {
                    return pod.Name, nil
                }
            }
        }
    }
    return "", nil
}
