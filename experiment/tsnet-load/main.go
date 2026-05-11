// tsnet-load launches N userspace Tailscale nodes and has each one periodically
// ping a tailnet peer (or HTTP-GET a subnet-routed host).
//
// Usage:
//
//	TS_AUTHKEY=tskey-... go run . -count 10 -interval 5s
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"tailscale.com/client/tailscale"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
	"tailscale.com/tsnet"
)

const (
	httpTarget = "http://10.210.1.2"
	pingTarget = "100.92.123.72"
)

func main() {
	authKey := flag.String("authkey", os.Getenv("TS_AUTHKEY"),
		"Tailscale reusable ephemeral auth key (or set TS_AUTHKEY).")
	count := flag.Int("count", 10, "Number of tsnet nodes to launch.")
	interval := flag.Duration("interval", 5*time.Second, "Interval between actions per node.")
	hostnamePrefix := flag.String("hostname-prefix", "uncloud", "Hostname prefix for tsnet nodes.")
	stateRoot := flag.String("state-dir", "./state", "Directory to keep tsnet state (default: a fresh temp dir).")
	flag.Parse()

	if *authKey == "" {
		log.Fatal("missing Tailscale auth key: pass -authkey or set TS_AUTHKEY.")
	}

	root := *stateRoot
	if root == "" {
		var err error
		root, err = os.MkdirTemp("", "tsnet-load-*")
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
			runNode(ctx, id, *authKey, *hostnamePrefix, root, *interval)
		}(i)
	}
	wg.Wait()
}

func runNode(ctx context.Context, id int, authKey, prefix, stateRoot string, interval time.Duration) {
	logPrefix := fmt.Sprintf("[node-%02d] ", id)
	logger := log.New(os.Stderr, logPrefix, log.LstdFlags|log.Lmsgprefix)

	s := &tsnet.Server{
		Hostname:  fmt.Sprintf("%s-%02d", prefix, id),
		AuthKey:   authKey,
		Ephemeral: true,
		Dir:       filepath.Join(stateRoot, fmt.Sprintf("node-%02d", id)),
		// Silence per-node tsnet chatter. Comment out to debug.
		Logf: func(string, ...any) {},
	}
	defer s.Close()

	upCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	status, err := s.Up(upCtx)
	if err != nil {
		logger.Printf("tsnet up: %v", err)
		return
	}
	logger.Printf("up as %s (%v)", s.Hostname, status.TailscaleIPs)

	// Accept subnet routes advertised by other tailnet peers. tsnet defaults
	// to RouteAll=false, so without this 10.210.0.0/24 etc. are unreachable.
	lc, err := s.LocalClient()
	if err != nil {
		logger.Printf("local client: %v", err)
		return
	}
	if _, err := lc.EditPrefs(ctx, &ipn.MaskedPrefs{
		RouteAllSet: true,
		Prefs:       ipn.Prefs{RouteAll: true},
	}); err != nil {
		logger.Printf("edit prefs (RouteAll): %v", err)
		return
	}

	httpClient := &http.Client{
		Transport: &http.Transport{DialContext: s.Dial},
		Timeout:   10 * time.Second,
	}
	_ = httpClient

	// Stagger ticks so all nodes don't fire in lockstep.
	time.Sleep(time.Duration(rand.Int64N(int64(interval))))

	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			doPing(ctx, lc, logger)
			//doHTTP(ctx, httpClient, logger)
		}
	}
}

func doPing(ctx context.Context, lc *tailscale.LocalClient, logger *log.Logger) {
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := lc.Ping(pctx, netip.MustParseAddr(pingTarget), tailcfg.PingICMP)
	if err != nil {
		logger.Printf("ping %s: %v", pingTarget, err)
		return
	}
	if res.Err != "" {
		logger.Printf("ping %s: %s", pingTarget, res.Err)
		return
	}
	logger.Printf("ping %s ok in %.1fms via %s", pingTarget, res.LatencySeconds*1000, res.Endpoint)
}

func doHTTP(ctx context.Context, c *http.Client, logger *log.Logger) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpTarget, nil)
	if err != nil {
		logger.Printf("http build req: %v", err)
		return
	}
	start := time.Now()
	resp, err := c.Do(req)
	if err != nil {
		logger.Printf("http %s: %v", httpTarget, err)
		return
	}
	defer resp.Body.Close()
	n, _ := io.Copy(io.Discard, resp.Body)
	logger.Printf("http %s: %s, %d bytes in %v", httpTarget, resp.Status, n, time.Since(start).Round(time.Millisecond))
}
