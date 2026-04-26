package prometheus

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const DefaultPort = ":51004"

type Server struct {
	*http.Server
}

func New() *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{Handler: mux, ReadTimeout: 5 * time.Second}
	return &Server{server}
}
