package proxy

import (
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"
)

// waitForUDPReady gives the UDP proxy time to start. Unlike TCP, we can't easily
// check if a UDP listener is ready without potentially affecting state.
func waitForUDPReady() {
	// Brief pause to let the goroutine start the listener
	time.Sleep(10 * time.Millisecond)
}

func TestUDPProxy_BasicEcho(t *testing.T) {
	t.Parallel()

	// Start a mock UDP backend that echoes
	backendAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to resolve backend addr: %v", err)
	}
	backend, err := net.ListenUDP("udp", backendAddr)
	if err != nil {
		t.Fatalf("Failed to start mock backend: %v", err)
	}
	t.Cleanup(func() { backend.Close() })

	go func() {
		buf := make([]byte, UDPBufSize)
		for {
			n, addr, err := backend.ReadFromUDP(buf)
			if err != nil {
				return
			}
			backend.WriteToUDP(buf[:n], addr)
		}
	}()

	// Create proxy with dynamic port
	proxy, err := NewUDPProxy(0, slog.Default())
	if err != nil {
		t.Fatalf("NewUDPProxy failed: %v", err)
	}
	t.Cleanup(func() { proxy.Close() })

	proxyAddr := proxy.FrontendAddr().String()

	proxy.SetBackends([]string{backend.LocalAddr().String()})
	go proxy.Run()

	waitForUDPReady()

	// Connect to proxy
	udpAddr, _ := net.ResolveUDPAddr("udp", proxyAddr)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		t.Fatalf("Failed to connect to proxy: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	// Test echo
	testData := []byte("hello udp proxy")
	_, err = conn.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, len(testData))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	if string(buf[:n]) != string(testData) {
		t.Errorf("Expected %q, got %q", testData, buf[:n])
	}
}

func TestUDPProxy_SessionReuse(t *testing.T) {
	t.Parallel()

	// Start backend
	backendAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	backend, err := net.ListenUDP("udp", backendAddr)
	if err != nil {
		t.Fatalf("Failed to start mock backend: %v", err)
	}
	t.Cleanup(func() { backend.Close() })

	var mu sync.Mutex
	clientAddrs := make(map[string]int)

	go func() {
		buf := make([]byte, UDPBufSize)
		for {
			n, addr, err := backend.ReadFromUDP(buf)
			if err != nil {
				return
			}
			mu.Lock()
			clientAddrs[addr.String()]++
			mu.Unlock()
			backend.WriteToUDP(buf[:n], addr)
		}
	}()

	proxy, err := NewUDPProxy(0, slog.Default())
	if err != nil {
		t.Fatalf("NewUDPProxy failed: %v", err)
	}
	t.Cleanup(func() { proxy.Close() })

	proxyAddr := proxy.FrontendAddr().String()

	proxy.SetBackends([]string{backend.LocalAddr().String()})
	go proxy.Run()

	waitForUDPReady()

	// Send multiple packets from same client
	udpAddr, _ := net.ResolveUDPAddr("udp", proxyAddr)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	for i := 0; i < 5; i++ {
		testData := []byte("packet")
		conn.Write(testData)

		conn.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, len(testData))
		conn.Read(buf)
	}

	// Allow time for backend to process
	time.Sleep(50 * time.Millisecond)

	// Backend should see all packets from same proxy connection (session reuse)
	mu.Lock()
	numClients := len(clientAddrs)
	mu.Unlock()

	if numClients != 1 {
		t.Errorf("Expected 1 client address (session reuse), got %d", numClients)
	}
}

func TestUDPProxy_MultipleClients(t *testing.T) {
	t.Parallel()

	// Start backend
	backendAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	backend, err := net.ListenUDP("udp", backendAddr)
	if err != nil {
		t.Fatalf("Failed to start mock backend: %v", err)
	}
	t.Cleanup(func() { backend.Close() })

	go func() {
		buf := make([]byte, UDPBufSize)
		for {
			n, addr, err := backend.ReadFromUDP(buf)
			if err != nil {
				return
			}
			backend.WriteToUDP(buf[:n], addr)
		}
	}()

	proxy, err := NewUDPProxy(0, slog.Default())
	if err != nil {
		t.Fatalf("NewUDPProxy failed: %v", err)
	}
	t.Cleanup(func() { proxy.Close() })

	proxyAddr := proxy.FrontendAddr().String()

	proxy.SetBackends([]string{backend.LocalAddr().String()})
	go proxy.Run()

	waitForUDPReady()

	// Create multiple clients
	var wg sync.WaitGroup
	errs := make(chan error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			udpAddr, _ := net.ResolveUDPAddr("udp", proxyAddr)
			conn, err := net.DialUDP("udp", nil, udpAddr)
			if err != nil {
				errs <- err
				return
			}
			defer conn.Close()

			testData := []byte("client packet")
			_, err = conn.Write(testData)
			if err != nil {
				errs <- err
				return
			}

			conn.SetReadDeadline(time.Now().Add(time.Second))
			buf := make([]byte, len(testData))
			n, err := conn.Read(buf)
			if err != nil {
				errs <- err
				return
			}

			if string(buf[:n]) != string(testData) {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("Client error: %v", err)
		}
	}
}

func TestUDPProxy_LoadBalancing(t *testing.T) {
	t.Parallel()

	// Start multiple backends
	var backends []*net.UDPConn
	var addrs []string

	for i := 0; i < 2; i++ {
		addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
		backend, err := net.ListenUDP("udp", addr)
		if err != nil {
			t.Fatalf("Failed to start backend %d: %v", i, err)
		}
		t.Cleanup(func() { backend.Close() })
		backends = append(backends, backend)
		addrs = append(addrs, backend.LocalAddr().String())
	}

	var mu sync.Mutex
	counts := make([]int, 2)

	for i, backend := range backends {
		go func() {
			buf := make([]byte, UDPBufSize)
			for {
				n, addr, err := backend.ReadFromUDP(buf)
				if err != nil {
					return
				}
				mu.Lock()
				counts[i]++
				mu.Unlock()
				backend.WriteToUDP(buf[:n], addr)
			}
		}()
	}

	proxy, err := NewUDPProxy(0, slog.Default())
	if err != nil {
		t.Fatalf("NewUDPProxy failed: %v", err)
	}
	t.Cleanup(func() { proxy.Close() })

	proxyAddr := proxy.FrontendAddr().String()

	proxy.SetBackends(addrs)
	go proxy.Run()

	waitForUDPReady()

	// Create multiple clients (each gets assigned to a random backend)
	numClients := 10
	for i := 0; i < numClients; i++ {
		udpAddr, _ := net.ResolveUDPAddr("udp", proxyAddr)
		conn, err := net.DialUDP("udp", nil, udpAddr)
		if err != nil {
			t.Fatalf("Failed to connect: %v", err)
		}

		conn.Write([]byte("test"))
		conn.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, 10)
		conn.Read(buf)
		conn.Close()
	}

	// Allow time for backends to process
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	total := counts[0] + counts[1]
	t.Logf("Backend 0: %d, Backend 1: %d", counts[0], counts[1])
	mu.Unlock()

	if total < numClients/2 {
		t.Errorf("Expected at least %d packets, got %d", numClients/2, total)
	}
}

func TestUDPProxy_Close(t *testing.T) {
	t.Parallel()

	proxy, err := NewUDPProxy(0, slog.Default())
	if err != nil {
		t.Fatalf("NewUDPProxy failed: %v", err)
	}

	proxyAddr := proxy.FrontendAddr().String()

	proxy.SetBackends([]string{"127.0.0.1:1"}) // unreachable backend
	go proxy.Run()

	waitForUDPReady()

	// Close the proxy
	proxy.Close()

	// Sending should fail or not get response (port closed)
	udpAddr, _ := net.ResolveUDPAddr("udp", proxyAddr)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		// Connection itself might fail
		return
	}
	t.Cleanup(func() { conn.Close() })

	conn.Write([]byte("test"))
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	buf := make([]byte, 10)
	_, err = conn.Read(buf)
	// Should timeout since proxy is closed
	if err == nil {
		t.Error("Expected timeout/error after proxy close")
	}
}

func TestUDPProxy_NoBackends(t *testing.T) {
	t.Parallel()

	proxy, err := NewUDPProxy(0, slog.Default())
	if err != nil {
		t.Fatalf("NewUDPProxy failed: %v", err)
	}
	t.Cleanup(func() { proxy.Close() })

	proxyAddr := proxy.FrontendAddr().String()

	// Don't set any backends
	go proxy.Run()

	waitForUDPReady()

	udpAddr, _ := net.ResolveUDPAddr("udp", proxyAddr)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	conn.Write([]byte("test"))
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	buf := make([]byte, 10)
	_, err = conn.Read(buf)

	// Should timeout since no backends to forward to
	if err == nil {
		t.Error("Expected timeout with no backends")
	}
}
