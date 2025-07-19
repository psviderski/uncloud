package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/libnetwork/networkdb"
)

func main() {
	conf := networkdb.DefaultConfig()
	db, err := networkdb.New(conf)
	if err != nil {
		panic(err)
	}

	conf2 := networkdb.DefaultConfig()
	conf2.BindPort = 12345
	db2, err := networkdb.New(conf2)
	if err != nil {
		panic(err)
	}
	if err = db2.Join([]string{"192.168.1.128:7946"}); err != nil {
		panic(err)
	}

	fmt.Println("Peers 1:", db.ClusterPeers())
	fmt.Println("Peers 2:", db2.ClusterPeers())

	if err = db.JoinNetwork(""); err != nil {
		panic(err)
	}
	if err = db2.JoinNetwork(""); err != nil {
		panic(err)
	}

	if err = db.CreateEntry("cluster-node", "", "key", []byte("value")); err != nil {
		panic(err)
	}
	time.Sleep(500 * time.Millisecond)
	value, err := db2.GetEntry("cluster-node", "", "key")
	if err != nil {
		panic(err)
	}
	fmt.Println("Entry from db2:", string(value))

	_ = db.WalkTable("cluster-node", func(nid, key string, value []byte, deleting bool) bool {
		fmt.Println("Entry from db1:", nid, key, string(value), deleting)
		return false
	})

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	// A channel to signal that shutdown is done.
	done := make(chan bool, 1)
	// Start a goroutine to handle signals.
	go func() {
		sig := <-sigs
		db.Close()
		time.Sleep(1 * time.Second)
		db2.Close()
		slog.Info("Received signal, shutting down", "signal", sig)
		done <- true
	}()
	<-done
}
