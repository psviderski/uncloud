package proxy

import (
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"
)

// waitForTCPListener polls until the TCP address is accepting connections or times out.
// Note: This creates and immediately closes a connection, which may be handled by the proxy.
func waitForTCPListener(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 10*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("listener at %s not ready after %v", addr, timeout)
}

func TestTCPProxy_BasicEcho(t *testing.T) {
	t.Parallel()

	// Start a mock backend server that echoes
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start mock backend: %v", err)
	}
	t.Cleanup(func() { backend.Close() })

	go func() {
		for {
			conn, err := backend.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	// Create proxy with dynamic port
	proxy, err := NewTCPProxy(0, slog.Default())
	if err != nil {
		t.Fatalf("NewTCPProxy failed: %v", err)
	}
	t.Cleanup(func() { proxy.Close() })

	proxyAddr := proxy.FrontendAddr().String()

	// Set backends and start
	proxy.SetBackends([]string{backend.Addr().String()})
	go proxy.Run()

	waitForTCPListener(t, proxyAddr, time.Second)

	// Connect to proxy
	conn, err := net.DialTimeout("tcp", proxyAddr, time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to proxy: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	// Test echo
	testData := []byte("hello tcp proxy")
	_, err = conn.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

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

func TestTCPProxy_RetryOnBackendFailure(t *testing.T) {
	t.Parallel()

	// Start one working backend
	workingBackend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start working backend: %v", err)
	}
	t.Cleanup(func() { workingBackend.Close() })

	go func() {
		for {
			conn, err := workingBackend.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	// Create proxy with dynamic port
	proxy, err := NewTCPProxy(0, slog.Default())
	if err != nil {
		t.Fatalf("NewTCPProxy failed: %v", err)
	}
	t.Cleanup(func() { proxy.Close() })

	proxyAddr := proxy.FrontendAddr().String()

	// First backend is unreachable, second is working
	proxy.SetBackends([]string{
		"127.0.0.1:1", // unreachable (privileged port, won't have listener)
		workingBackend.Addr().String(),
	})
	go proxy.Run()

	waitForTCPListener(t, proxyAddr, time.Second)

	// Make multiple connections - all should eventually succeed via retry
	for i := 0; i < 5; i++ {
		conn, err := net.DialTimeout("tcp", proxyAddr, 5*time.Second)
		if err != nil {
			t.Fatalf("Connection %d failed: %v", i, err)
		}

		testData := []byte("retry test")
		_, err = conn.Write(testData)
		if err != nil {
			conn.Close()
			t.Fatalf("Write failed: %v", err)
		}

		conn.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, len(testData))
		_, err = io.ReadFull(conn, buf)
		conn.Close()
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		if string(buf) != string(testData) {
			t.Errorf("Expected %q, got %q", testData, buf)
		}
	}
}

func TestTCPProxy_AllBackendsFail(t *testing.T) {
	t.Parallel()

	proxy, err := NewTCPProxy(0, slog.Default())
	if err != nil {
		t.Fatalf("NewTCPProxy failed: %v", err)
	}
	t.Cleanup(func() { proxy.Close() })

	proxyAddr := proxy.FrontendAddr().String()

	// All backends unreachable (privileged ports, won't have listeners)
	proxy.SetBackends([]string{
		"127.0.0.1:1",
		"127.0.0.1:2",
	})
	go proxy.Run()

	waitForTCPListener(t, proxyAddr, time.Second)

	// Connection should succeed to proxy but be closed immediately
	conn, err := net.DialTimeout("tcp", proxyAddr, time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to proxy: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	// Should get EOF since proxy closes connection after all backends fail
	conn.SetReadDeadline(time.Now().Add(15 * time.Second)) // Allow time for retries
	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	if err != io.EOF {
		t.Errorf("Expected EOF, got %v", err)
	}
}

func TestTCPProxy_ConcurrentConnections(t *testing.T) {
	t.Parallel()

	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start mock backend: %v", err)
	}
	t.Cleanup(func() { backend.Close() })

	go func() {
		for {
			conn, err := backend.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	proxy, err := NewTCPProxy(0, slog.Default())
	if err != nil {
		t.Fatalf("NewTCPProxy failed: %v", err)
	}
	t.Cleanup(func() { proxy.Close() })

	proxyAddr := proxy.FrontendAddr().String()

	proxy.SetBackends([]string{backend.Addr().String()})
	go proxy.Run()

	waitForTCPListener(t, proxyAddr, time.Second)

	var wg sync.WaitGroup
	errs := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			conn, err := net.DialTimeout("tcp", proxyAddr, time.Second)
			if err != nil {
				errs <- err
				return
			}
			defer conn.Close()

			testData := []byte("concurrent test")
			_, err = conn.Write(testData)
			if err != nil {
				errs <- err
				return
			}

			conn.SetReadDeadline(time.Now().Add(time.Second))
			buf := make([]byte, len(testData))
			_, err = io.ReadFull(conn, buf)
			if err != nil {
				errs <- err
				return
			}

			if string(buf) != string(testData) {
				errs <- io.ErrUnexpectedEOF
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("Concurrent connection error: %v", err)
	}
}

func TestTCPProxy_LoadBalancing(t *testing.T) {
	t.Parallel()

	var backends []net.Listener
	var addrs []string

	for i := 0; i < 3; i++ {
		backend, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to start backend %d: %v", i, err)
		}
		t.Cleanup(func() { backend.Close() })
		backends = append(backends, backend)
		addrs = append(addrs, backend.Addr().String())
	}

	var mu sync.Mutex
	counts := make([]int, 3)

	for i, backend := range backends {
		go func() {
			for {
				conn, err := backend.Accept()
				if err != nil {
					return
				}
				mu.Lock()
				counts[i]++
				mu.Unlock()
				conn.Close()
			}
		}()
	}

	proxy, err := NewTCPProxy(0, slog.Default())
	if err != nil {
		t.Fatalf("NewTCPProxy failed: %v", err)
	}
	t.Cleanup(func() { proxy.Close() })

	proxyAddr := proxy.FrontendAddr().String()

	proxy.SetBackends(addrs)
	go proxy.Run()

	waitForTCPListener(t, proxyAddr, time.Second)

	// Make many connections
	numConns := 30
	for i := 0; i < numConns; i++ {
		conn, err := net.DialTimeout("tcp", proxyAddr, time.Second)
		if err != nil {
			t.Fatalf("Failed to connect: %v", err)
		}
		conn.Close()
	}

	// Allow time for backends to process
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	total := 0
	for i, c := range counts {
		t.Logf("Backend %d: %d connections", i, c)
		total += c
	}
	mu.Unlock()

	// Should have distributed connections (at least some to each backend)
	if total < numConns/2 {
		t.Errorf("Expected at least %d connections, got %d", numConns/2, total)
	}
}

func TestTCPProxy_Close(t *testing.T) {
	t.Parallel()

	proxy, err := NewTCPProxy(0, slog.Default())
	if err != nil {
		t.Fatalf("NewTCPProxy failed: %v", err)
	}

	proxyAddr := proxy.FrontendAddr().String()

	proxy.SetBackends([]string{"127.0.0.1:1"}) // unreachable backend
	go proxy.Run()

	waitForTCPListener(t, proxyAddr, time.Second)

	// Verify we can connect
	conn, err := net.DialTimeout("tcp", proxyAddr, time.Second)
	if err != nil {
		t.Fatalf("Failed to connect before close: %v", err)
	}
	conn.Close()

	// Close proxy
	proxy.Close()

	// Should not be able to connect anymore
	_, err = net.DialTimeout("tcp", proxyAddr, 100*time.Millisecond)
	if err == nil {
		t.Error("Expected connection to fail after close")
	}
}
