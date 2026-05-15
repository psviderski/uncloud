// netbird-load launches N userspace NetBird nodes and has each one periodically
// TCP-dial a peer to measure connect latency through the wireguard overlay.
//
// Usage:
//
//	NB_SETUP_KEY=nb-... go run . -count 10 -interval 5s -target 100.x.y.z:22
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/netbirdio/netbird/client/embed"
)

func main() {
	setupKey := flag.String("setupkey", os.Getenv("NB_SETUP_KEY"),
		"NetBird reusable setup key (or set NB_SETUP_KEY).")
	mgmt := flag.String("mgmt", os.Getenv("NB_MGMT_URL"),
		"NetBird management URL (empty = embed default, i.e. NetBird Cloud).")
	count := flag.Int("count", 10, "Number of embedded netbird nodes to launch.")
	interval := flag.Duration("interval", 5*time.Second, "Interval between actions per node.")
	hostnamePrefix := flag.String("hostname-prefix", "uncloud", "Hostname prefix for netbird nodes.")
	stateRoot := flag.String("state-dir", "./state", "Directory to keep netbird state (empty = a fresh temp dir).")
	target := flag.String("target", "", "TCP host:port to dial each tick (required).")
	flag.Parse()

	if *setupKey == "" {
		log.Fatal("missing NetBird setup key: pass -setupkey or set NB_SETUP_KEY.")
	}
	if *target == "" {
		log.Fatal("missing -target: provide a TCP host:port reachable through the netbird network.")
	}

	root := *stateRoot
	if root == "" {
		var err error
		root, err = os.MkdirTemp("", "netbird-load-*")
		if err != nil {
			log.Fatalf("mktemp: %v", err)
		}
		defer os.RemoveAll(root)
	} else if err := os.MkdirAll(root, 0o700); err != nil {
		log.Fatalf("mkdir state dir: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	for i := 0; i < *count; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runNode(ctx, id, *setupKey, *mgmt, *hostnamePrefix, root, *target, *interval)
		}(i)
	}
	wg.Wait()
}

func runNode(ctx context.Context, id int, setupKey, mgmt, prefix, stateRoot, target string, interval time.Duration) {
	logPrefix := fmt.Sprintf("[node-%02d] ", id)
	logger := log.New(os.Stderr, logPrefix, log.LstdFlags|log.Lmsgprefix)

	nodeDir := filepath.Join(stateRoot, fmt.Sprintf("node-%02d", id))
	deviceName := fmt.Sprintf("%s-%02d", prefix, id)

	client, err := embed.New(embed.Options{
		DeviceName:    deviceName,
		SetupKey:      setupKey,
		ManagementURL: mgmt,
		StatePath:     nodeDir,
		ConfigPath:    filepath.Join(nodeDir, "config"),
		// Silence per-node netbird chatter. Swap to os.Stderr to debug.
		LogOutput: io.Discard,
		//NoUserspace: true,
	})
	if err != nil {
		logger.Printf("netbird new: %v", err)
		return
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.Stop(stopCtx)
	}()

	upCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := client.Start(upCtx); err != nil {
		logger.Printf("netbird start: %v", err)
		return
	}
	logger.Printf("up as %s", deviceName)

	// Stagger ticks so all nodes don't fire in lockstep.
	time.Sleep(time.Duration(rand.Int64N(int64(interval))))

	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			doDial(ctx, client, target, logger)
		}
	}
}

func doDial(ctx context.Context, c *embed.Client, target string, logger *log.Logger) {
	dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	start := time.Now()
	conn, err := c.Dial(dctx, "tcp", target)
	if err != nil {
		logger.Printf("dial %s: %v", target, err)
		return
	}
	latency := time.Since(start)
	_ = conn.Close()
	logger.Printf("dial %s ok in %.1fms", target, float64(latency.Microseconds())/1000)
}
