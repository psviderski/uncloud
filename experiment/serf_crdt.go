package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/hashicorp/memberlist"
	"github.com/hashicorp/serf/cmd/serf/command/agent"
	"github.com/hashicorp/serf/serf"
	ds "github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger3"
	crdt "github.com/ipfs/go-ds-crdt"
	"github.com/lmittmann/tint"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func createSerfAgentConfig(name, bindAddr, rpcAddr, profile string) *agent.Config {
	config := agent.DefaultConfig()
	config.NodeName = name
	config.BindAddr = bindAddr
	config.RPCAddr = rpcAddr
	config.Profile = profile
	// TODO: configure logging for serf and memberlist to slog.
	//  check: https://github.com/hashicorp/serf/issues/512
	return config
}

func createSerfAgent(config *agent.Config) (*agent.Agent, error) {
	bindIP, bindPort, err := config.AddrParts(config.BindAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid bind address: %w", err)
	}
	serfConfig := serf.DefaultConfig()
	switch config.Profile {
	case "lan":
		serfConfig.MemberlistConfig = memberlist.DefaultLANConfig()
	case "wan":
		serfConfig.MemberlistConfig = memberlist.DefaultWANConfig()
	case "local":
		serfConfig.MemberlistConfig = memberlist.DefaultLocalConfig()
	default:
		return nil, fmt.Errorf("unknown profile: %s", config.Profile)
	}

	serfConfig.MemberlistConfig.BindAddr = bindIP
	serfConfig.MemberlistConfig.BindPort = bindPort
	//serfConfig.MemberlistConfig.AdvertiseAddr = advertiseIP
	//serfConfig.MemberlistConfig.AdvertisePort = advertisePort
	//serfConfig.MemberlistConfig.SecretKey = encryptKey
	serfConfig.NodeName = config.NodeName
	serfConfig.Tags = config.Tags
	serfConfig.SnapshotPath = config.SnapshotPath
	serfConfig.ProtocolVersion = uint8(config.Protocol)
	serfConfig.CoalescePeriod = 3 * time.Second
	serfConfig.QuiescentPeriod = time.Second
	serfConfig.QueryResponseSizeLimit = config.QueryResponseSizeLimit
	serfConfig.QuerySizeLimit = config.QuerySizeLimit
	serfConfig.UserEventSizeLimit = config.UserEventSizeLimit
	serfConfig.UserCoalescePeriod = 3 * time.Second
	serfConfig.UserQuiescentPeriod = time.Second
	if config.ReconnectInterval != 0 {
		serfConfig.ReconnectInterval = config.ReconnectInterval
	}
	if config.ReconnectTimeout != 0 {
		serfConfig.ReconnectTimeout = config.ReconnectTimeout
	}
	if config.TombstoneTimeout != 0 {
		serfConfig.TombstoneTimeout = config.TombstoneTimeout
	}
	serfConfig.EnableNameConflictResolution = !config.DisableNameResolution
	if config.KeyringFile != "" {
		serfConfig.KeyringFile = config.KeyringFile
	}
	serfConfig.RejoinAfterLeave = config.RejoinAfterLeave
	if config.BroadcastTimeout != 0 {
		serfConfig.BroadcastTimeout = config.BroadcastTimeout
	}

	serfAgent, err := agent.Create(config, serfConfig, os.Stdout)
	if err != nil {
		return nil, fmt.Errorf("setup the Serf agent: %w", err)
	}
	return serfAgent, nil
}

func startSerfAgent(config *agent.Config, serfAgent *agent.Agent) (*agent.AgentIPC, error) {
	if err := serfAgent.Start(); err != nil {
		return nil, fmt.Errorf("start Serf agent: %w", err)
	}

	// Setup the RPC listener
	rpcListener, err := net.Listen("tcp", config.RPCAddr)
	if err != nil {
		return nil, fmt.Errorf("start RPC listener: %w", err)
	}

	// Start the IPC layer
	logWriter := agent.NewLogWriter(512)
	ipc := agent.NewAgentIPC(serfAgent, config.RPCAuthKey, rpcListener, os.Stdout, logWriter)

	slog.Info("Started Serf agent",
		"name", config.NodeName,
		"bind_addr", config.BindAddr,
		"rpc_addr", config.RPCAddr,
		"encrypted", serfAgent.Serf().EncryptionEnabled(),
		"snapshot", config.SnapshotPath != "",
		"profile", config.Profile,
		"compression", config.EnableCompression,
	)

	return ipc, nil
}

func main() {
	name := flag.String("name", "", "Machine name")
	bindAddr := flag.String("bind", "0.0.0.0:7946", "Bind address")
	rpcAddr := flag.String("rpc-addr", "127.0.0.1:7373", "RPC address")
	storeDir := flag.String("store-dir", "./store", "Store directory path")
	profile := flag.String("profile", "lan",
		"Timing profile for Serf. The supported choices are \"wan\", \"lan\", and \"local\".")
	runTick := flag.Bool("tick", false, "Periodically update /tick value")
	flag.Parse()

	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{
		AddSource: true,
		Level:     slog.LevelDebug,
		//Level:      slog.LevelInfo,
		TimeFormat: time.RFC3339Nano,
	}))
	slog.SetDefault(logger)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	// A channel to signal that shutdown is done.
	done := make(chan bool, 1)

	config := createSerfAgentConfig(*name, *bindAddr, *rpcAddr, *profile)
	serfAgent, err := createSerfAgent(config)
	if err != nil {
		panic(err)
	}
	ipc, err := startSerfAgent(config, serfAgent)
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	localStore, err := badger.NewDatastore(*storeDir, nil) // default options
	if err != nil {
		panic(err)
	}
	syncer := newDAGSyncer(localStore, ds.NewKey("/node"), serfAgent.Serf())

	// Ideally, the broadcaster should be registered as an event handler before starting the agent.
	// However, we need the agent.serf to be initialized which is done in agent.Start().
	broadcaster := NewSerfBroadcaster(ctx, serfAgent.Serf())
	serfAgent.RegisterEventHandler(broadcaster)
	serfAgent.RegisterEventHandler(syncer)

	opts := crdt.DefaultOptions()
	opts.Logger = newIPFSLogger(logger)
	//opts.MultiHeadProcessing = true
	// TODO: debug why the heads count may grow on the receiving side if the event backlog is huge and the processing
	//  is slow.
	store, err := crdt.New(localStore, ds.NewKey("/"), syncer, broadcaster, opts)
	if err != nil {
		panic(err)
	}

	//ticker := time.NewTicker(10 * time.Millisecond)
	ticker := time.NewTicker(3 * time.Second)
	go func() {
		for {
			select {
			case t := <-ticker.C:
				if *runTick {
					err = store.Put(ctx, ds.NewKey("/tick"), []byte(t.String()))
					if err != nil {
						slog.Error("Put /tick", "error", err)
					}
					value, err := store.Get(ctx, ds.NewKey("/tick"))
					if err != nil {
						slog.Error("Get /tick", "error", err)
					}
					slog.Info("Get /tick", "value", string(value))
					stats := store.InternalStats()
					slog.Info("CRDT store stats", "stats", stats, "heads_count", len(stats.Heads))
				} else {
					value, err := store.Get(ctx, ds.NewKey("/tick"))
					if err != nil {
						slog.Error("Get /tick", "error", err)
					}
					slog.Info("Get /tick", "value", string(value))
					stats := store.InternalStats()
					slog.Info("CRDT store stats", "stats", stats, "heads_count", len(stats.Heads))
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	//err = store.Put(ctx, ds.NewKey("/test3"), []byte("hello3"))
	//if err != nil {
	//	panic(err)
	//}
	//v, err := store.Get(ctx, ds.NewKey("/test"))
	//if err != nil {
	//	slog.Error("Get /test", "error", err)
	//}
	//slog.Info("Get /test", "value", string(v))

	//_ = store.PrintDAG()

	// Start a goroutine to handle signals.
	go func() {
		sig := <-sigs
		slog.Info("Received signal, shutting down Serf agent", "signal", sig)

		cancel()
		ipc.Shutdown()
		if err = serfAgent.Leave(); err != nil {
			slog.Error("Gracefully leave Serf cluster", "error", err)
		}
		if err = serfAgent.Shutdown(); err != nil {
			slog.Error("Gracefully shutdown Serf agent", "error", err)
		}
		fmt.Println("closing crdt store")
		if err = store.Close(); err != nil {
			slog.Error("Close CRDT store", "error", err)
		}
		fmt.Println("closing local store")
		if err = localStore.Close(); err != nil {
			slog.Error("Close local store", "error", err)
		}

		done <- true
	}()

	<-done
}
