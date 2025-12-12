package main

import (
	"log"
	"net/http"
	"os"

	"github.com/psviderski/uncloud/internal/machine"
	"github.com/psviderski/uncloud/pkg/webui"
)

func main() {
	sockPath := machine.DefaultUncloudSockPath
	if envPath := os.Getenv("UNCLOUD_SOCK"); envPath != "" {
		sockPath = envPath
	}

	srv, err := webui.NewServer(sockPath)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	addr := ":8080"
	log.Printf("Starting WebUI on http://localhost%s connected to %s", addr, sockPath)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
