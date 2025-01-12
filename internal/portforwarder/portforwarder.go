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
)

type PortForwarder struct {
	namespace   string
	serviceName string
	port        int
	localPort   int
	cmd         *exec.Cmd
	running     bool
	mu          sync.Mutex
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
	}
	fmt.Printf("PortForwarder created: namespace=%s, service=%s, port=%d, localPort=%d\n",
		namespace, serviceName, port, localPort)
	return pf, nil
}

func (pf *PortForwarder) Start(ctx context.Context) error {
	pf.mu.Lock()
	defer pf.mu.Unlock()

	if pf.running {
		return nil // Already running
	}

	portString := fmt.Sprintf("%d:%d", pf.localPort, pf.port)
	args := []string{
		"kubectl", "port-forward",
		"-n", pf.namespace,
		fmt.Sprintf("service/%s", pf.serviceName),
		portString,
	}

	fmt.Println(strings.Join(args, " "))
	pf.cmd = exec.CommandContext(ctx, args[0], args[1:]...)
	pf.cmd.Stderr = nil

	err := pf.cmd.Start()
	if err != nil {
		pf.Stop() // Stop if starting fails!
		return fmt.Errorf("starting port-forward: %w", err)
	}

	pf.running = true
	return nil
}

func (pf *PortForwarder) Stop() {
	pf.mu.Lock()
	defer pf.mu.Unlock()

	if !pf.running || pf.cmd == nil {
		return
	}

	if err := pf.cmd.Process.Kill(); err != nil {
		// Log the error but don't stop execution, as the process might have already exited
		fmt.Printf("Error killing port-forward process: %v\n", err)
	}

	if _, err := pf.cmd.Process.Wait(); err != nil {
		// Log the error but don't stop execution, as the process might have already exited
		fmt.Printf("Error waiting for port-forward process to exit: %v\n", err)
	}
	pf.cmd = nil
	pf.running = false
}

func (pf *PortForwarder) GetLocalURL(remoteURL string) string {
	u, err := url.Parse(remoteURL)
	if err != nil {
		// Handle the error appropriately, perhaps by logging and returning the original URL
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
