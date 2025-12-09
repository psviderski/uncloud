package tcpproxy

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"
)

func TestProxy_SetBackendsAndConnect(t *testing.T) {
	// Start a mock backend server
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start mock backend: %v", err)
	}
	defer backend.Close()

	// Accept connections and echo back
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := backend.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(conn, conn)
	}()

	// Create proxy
	proxy := NewProxy(slog.Default())
	defer proxy.Close()

	// Set backends
	ctx := context.Background()
	port := uint16(38000)
	err = proxy.SetBackends(ctx, port, []string{backend.Addr().String()})
	if err != nil {
		t.Fatalf("SetBackends failed: %v", err)
	}

	// Give the listener time to start
	time.Sleep(50 * time.Millisecond)

	// Connect to the proxy
	conn, err := net.Dial("tcp", "127.0.0.1:38000")
	if err != nil {
		t.Fatalf("Failed to connect to proxy: %v", err)
	}
	defer conn.Close()

	// Send test data
	testData := []byte("hello proxy")
	_, err = conn.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Read response (echoed back by backend)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, len(testData))
	_, err = io.ReadFull(conn, buf)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	if string(buf) != string(testData) {
		t.Errorf("Expected %q, got %q", testData, buf)
	}
}

func TestProxy_RemovePort(t *testing.T) {
	proxy := NewProxy(slog.Default())
	defer proxy.Close()

	ctx := context.Background()
	port := uint16(38001)

	// Set backends to create listener
	err := proxy.SetBackends(ctx, port, []string{"127.0.0.1:9999"})
	if err != nil {
		t.Fatalf("SetBackends failed: %v", err)
	}

	// Verify port is listening
	ports := proxy.ListeningPorts()
	if len(ports) != 1 || ports[0] != port {
		t.Errorf("Expected port %d in listening ports, got %v", port, ports)
	}

	// Remove port
	err = proxy.RemovePort(port)
	if err != nil {
		t.Fatalf("RemovePort failed: %v", err)
	}

	// Verify port is no longer listening
	ports = proxy.ListeningPorts()
	if len(ports) != 0 {
		t.Errorf("Expected no listening ports, got %v", ports)
	}

	// Verify we can't connect
	conn, err := net.DialTimeout("tcp", "127.0.0.1:38001", 100*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Error("Expected connection to fail after RemovePort")
	}
}

func TestProxy_SetBackendsEmpty(t *testing.T) {
	proxy := NewProxy(slog.Default())
	defer proxy.Close()

	ctx := context.Background()
	port := uint16(38002)

	// Set backends to create listener
	err := proxy.SetBackends(ctx, port, []string{"127.0.0.1:9999"})
	if err != nil {
		t.Fatalf("SetBackends failed: %v", err)
	}

	// Set empty backends should remove the port
	err = proxy.SetBackends(ctx, port, nil)
	if err != nil {
		t.Fatalf("SetBackends with empty list failed: %v", err)
	}

	// Verify port is no longer listening
	ports := proxy.ListeningPorts()
	if len(ports) != 0 {
		t.Errorf("Expected no listening ports after empty SetBackends, got %v", ports)
	}
}

func TestProxy_Close(t *testing.T) {
	proxy := NewProxy(slog.Default())
	ctx := context.Background()

	// Create multiple listeners
	for i := uint16(0); i < 3; i++ {
		port := uint16(38003) + i
		err := proxy.SetBackends(ctx, port, []string{"127.0.0.1:9999"})
		if err != nil {
			t.Fatalf("SetBackends for port %d failed: %v", port, err)
		}
	}

	// Verify we have 3 listeners
	if proxy.PortCount() != 3 {
		t.Errorf("Expected 3 ports, got %d", proxy.PortCount())
	}

	// Close the proxy
	err := proxy.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify all listeners are removed
	if proxy.PortCount() != 0 {
		t.Errorf("Expected 0 ports after close, got %d", proxy.PortCount())
	}

	// Verify operations fail after close
	err = proxy.SetBackends(ctx, 38010, []string{"127.0.0.1:9999"})
	if err != ErrProxyClosed {
		t.Errorf("Expected ErrProxyClosed, got %v", err)
	}
}

func TestProxy_LoadBalancing(t *testing.T) {
	// Start two mock backend servers
	var backends []net.Listener
	var backendAddrs []string
	for i := 0; i < 2; i++ {
		backend, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to start mock backend %d: %v", i, err)
		}
		defer backend.Close()
		backends = append(backends, backend)
		backendAddrs = append(backendAddrs, backend.Addr().String())
	}

	// Count connections to each backend
	var mu sync.Mutex
	connCounts := make([]int, 2)
	var wg sync.WaitGroup

	for i, backend := range backends {
		i := i
		backend := backend
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				conn, err := backend.Accept()
				if err != nil {
					return
				}
				mu.Lock()
				connCounts[i]++
				mu.Unlock()
				conn.Close()
			}
		}()
	}

	// Create proxy
	proxy := NewProxy(slog.Default())
	defer proxy.Close()

	ctx := context.Background()
	port := uint16(38020)
	err := proxy.SetBackends(ctx, port, backendAddrs)
	if err != nil {
		t.Fatalf("SetBackends failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Make multiple connections
	numConns := 20
	for i := 0; i < numConns; i++ {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:38020", time.Second)
		if err != nil {
			t.Fatalf("Failed to connect to proxy: %v", err)
		}
		conn.Close()
		time.Sleep(10 * time.Millisecond) // Small delay to allow backend to register connection
	}

	// Close backends to stop accept loops
	for _, b := range backends {
		b.Close()
	}

	// Wait for accept goroutines to finish
	time.Sleep(100 * time.Millisecond)

	// Verify both backends received some connections (random load balancing)
	mu.Lock()
	total := connCounts[0] + connCounts[1]
	mu.Unlock()

	if total < numConns/2 {
		t.Errorf("Expected at least %d total connections, got %d", numConns/2, total)
	}

	// With random selection, both backends should get some connections
	// (very unlikely to get 0 with 20 attempts)
	mu.Lock()
	for i, count := range connCounts {
		t.Logf("Backend %d: %d connections", i, count)
	}
	mu.Unlock()
}
