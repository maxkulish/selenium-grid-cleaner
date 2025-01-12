package portforwarder

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type PortForwarder struct {
	namespace   string
	serviceName string
	port        int
	localPort   int
	cmd         *exec.Cmd
	running     bool
	mu          sync.Mutex
	done        chan struct{}
}

func NewPortForwarder(namespace, serviceName string, port int) (*PortForwarder, error) {
	localPort, err := getAvailablePort()
	if err != nil {
		return nil, fmt.Errorf("failed to get available port: %w", err)
	}

	pf := &PortForwarder{
		namespace:   namespace,
		serviceName: serviceName,
		port:        port,
		localPort:   localPort,
		done:        make(chan struct{}),
	}
	fmt.Printf("PortForwarder created: namespace=%s, service=%s, port=%d, localPort=%d\n",
		namespace, serviceName, port, localPort)
	return pf, nil
}

func (pf *PortForwarder) Start(ctx context.Context) error {
	pf.mu.Lock()
	defer pf.mu.Unlock()

	if pf.running {
		return nil
	}

	// Create a child context that we can cancel when stopping
	childCtx, cancel := context.WithCancel(ctx)

	portString := fmt.Sprintf("%d:%d", pf.localPort, pf.port)
	args := []string{
		"port-forward",
		"-n", pf.namespace,
		fmt.Sprintf("service/%s", pf.serviceName),
		portString,
	}

	fmt.Printf("kubectl %s\n", strings.Join(args, " "))
	pf.cmd = exec.CommandContext(childCtx, "kubectl", args...)

	stderr, err := pf.cmd.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := pf.cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("starting port-forward: %w", err)
	}

	// Handle process cleanup in a goroutine
	go func() {
		defer cancel() // Ensure context is cancelled when we're done
		defer close(pf.done)

		// Wait for the command to complete
		if err := pf.cmd.Wait(); err != nil {
			if childCtx.Err() == nil { // Only log if we haven't cancelled deliberately
				fmt.Printf("port-forward process ended unexpectedly: %v\n", err)
			}
		}

		pf.mu.Lock()
		pf.running = false
		pf.mu.Unlock()
	}()

	// Handle stderr output
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				fmt.Printf("kubectl stderr: %s", buf[:n])
			}
			if err != nil {
				break
			}
		}
	}()

	// Wait for the port to become available
	if err := pf.waitForConnection(childCtx); err != nil {
		pf.Stop() // Clean up if connection fails
		return fmt.Errorf("port-forward connection failed: %w", err)
	}

	pf.running = true
	return nil
}

func (pf *PortForwarder) waitForConnection(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(30 * time.Second)

	addr := fmt.Sprintf("localhost:%d", pf.localPort)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for port-forward to be ready")
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", addr, time.Second)
			if err == nil {
				conn.Close()
				fmt.Printf("Port-forward is ready on %s\n", addr)
				return nil
			}
		}
	}
}

func (pf *PortForwarder) Stop() {
	pf.mu.Lock()
	if !pf.running || pf.cmd == nil {
		pf.mu.Unlock()
		return
	}
	pf.running = false
	cmd := pf.cmd
	pf.cmd = nil
	pf.mu.Unlock()

	// Kill the process
	if cmd.Process != nil {
		if err := cmd.Process.Kill(); err != nil {
			fmt.Printf("Error killing port-forward process: %v\n", err)
		}
	}

	// Wait for the process to be fully cleaned up
	select {
	case <-pf.done:
		// Process has exited
	case <-time.After(5 * time.Second):
		fmt.Println("Warning: Timeout waiting for port-forward process to exit")
	}
}

func (pf *PortForwarder) GetLocalURL(remoteURL string) string {
	u, err := url.Parse(remoteURL)
	if err != nil {
		fmt.Printf("Error parsing URL: %v\n", err)
		return remoteURL
	}

	parts := strings.Split(u.Host, ":")
	hostname := "localhost"

	if len(parts) > 1 {
		hostname = parts[0]
	}
	u.Host = hostname + ":" + strconv.Itoa(pf.localPort)

	return u.String()
}

func getAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("failed to listen on a free port: %w", err)
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}
