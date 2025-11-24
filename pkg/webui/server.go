package webui

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/psviderski/uncloud/pkg/client"
	datastar "github.com/starfederation/datastar-go/datastar"
	"golang.org/x/net/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// UnixConnector establishes a connection to the machine API through a unix socket.
type UnixConnector struct {
	sockPath string
}

func NewUnixConnector(sockPath string) *UnixConnector {
	return &UnixConnector{sockPath: sockPath}
}

func (c *UnixConnector) Connect(ctx context.Context) (*grpc.ClientConn, error) {
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return net.Dial("unix", addr)
	}

	conn, err := grpc.NewClient("passthrough:///"+c.sockPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dialer),
	)
	if err != nil {
		return nil, fmt.Errorf("dial unix socket: %w", err)
	}
	return conn, nil
}

func (c *UnixConnector) Dialer() (proxy.ContextDialer, error) {
	return nil, fmt.Errorf("proxy connections not supported on unix connector")
}

func (c *UnixConnector) Close() error {
	return nil
}

type Server struct {
	mux    *http.ServeMux
	client *client.Client
}

func NewServer(sockPath string) (*Server, error) {
	ctx := context.Background()

	conn := NewUnixConnector(sockPath)
	cli, err := client.New(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	s := &Server{
		mux:    http.NewServeMux(),
		client: cli,
	}

	s.setupRoutes()
	return s, nil
}

func (s *Server) setupRoutes() {
	s.mux.HandleFunc("GET /", s.handleIndex)
	s.mux.HandleFunc("GET /machines", s.handleMachines)
	s.mux.HandleFunc("GET /services", s.handleServices)
	s.mux.HandleFunc("DELETE /services/{id}", s.handleDeleteService)
	s.mux.HandleFunc("GET /volumes", s.handleVolumes)
	s.mux.HandleFunc("DELETE /machines/{machine}/volumes/{volume}", s.handleDeleteVolume)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	// Basic layout with Datastar CDN for now
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>Uncloud WebUI</title>
    <script type="module" src="https://cdn.jsdelivr.net/gh/starfederation/datastar@main/bundles/datastar.js"></script>
    <style>
        body { font-family: system-ui, sans-serif; padding: 20px; }
        .card { border: 1px solid #ccc; padding: 10px; margin-bottom: 10px; border-radius: 5px; position: relative; }
        .grid { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 20px; }
        .delete-btn {
            position: absolute; top: 10px; right: 10px;
            background: #ff4444; color: white; border: none; padding: 5px 10px;
            cursor: pointer; border-radius: 3px;
        }
        .delete-btn:hover { background: #cc0000; }
    </style>
</head>
<body>
    <h1>Uncloud Dashboard</h1>
    <div class="grid">
        <div>
            <h2>Machines</h2>
            <div data-on-load="@get('/machines')">
                <div id="machine-list">Loading machines...</div>
            </div>
        </div>
        <div>
            <h2>Services</h2>
            <div data-on-load="@get('/services')">
                <div id="service-list">Loading services...</div>
            </div>
        </div>
        <div>
            <h2>Volumes</h2>
            <div data-on-load="@get('/volumes')">
                <div id="volume-list">Loading volumes...</div>
            </div>
        </div>
    </div>
</body>
</html>`
	w.Write([]byte(html))
}

func (s *Server) handleMachines(w http.ResponseWriter, r *http.Request) {
	sse := datastar.NewSSE(w, r)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	machines, err := s.client.ListMachines(ctx, nil)
	if err != nil {
		sse.PatchElements(fmt.Sprintf(`<div id="machine-list" style="color: red;">Error: %s</div>`, err.Error()))
		return
	}

	html := `<div id="machine-list">`
	if len(machines) == 0 {
		html += "<div>No machines found.</div>"
	} else {
		for _, m := range machines {
			statusColor := "green"
			if m.State.String() != "UP" {
				statusColor = "red"
			}

			ip := "-"
			if m.Machine.Network.ManagementIp != nil {
				if addr, err := m.Machine.Network.ManagementIp.ToAddr(); err == nil {
					ip = addr.String()
				}
			}

			html += fmt.Sprintf(`<div class="card">
                <h3>%s (%s)</h3>
                <p>Status: <span style="color: %s;">%s</span></p>
                <p>IP: %s</p>
            </div>`, m.Machine.Name, m.Machine.Id, statusColor, m.State.String(), ip)
		}
	}
	html += `</div>`

	sse.PatchElements(html)
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	sse := datastar.NewSSE(w, r)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	services, err := s.client.ListServices(ctx)
	if err != nil {
		sse.PatchElements(fmt.Sprintf(`<div id="service-list" style="color: red;">Error: %s</div>`, err.Error()))
		return
	}

	html := `<div id="service-list">`
	if len(services) == 0 {
		html += "<div>No services found.</div>"
	} else {
		for _, svc := range services {
			html += fmt.Sprintf(`<div class="card" id="service-%s">
                <h3>%s</h3>
                <p>ID: %s</p>
                <p>Mode: %s</p>
                <p>Containers: %d</p>
                <button class="delete-btn" data-on-click="@delete('/services/%s')">Delete</button>
            </div>`, svc.ID, svc.Name, svc.ID, svc.Mode, len(svc.Containers), svc.ID)
		}
	}
	html += `</div>`

	sse.PatchElements(html)
}

func (s *Server) handleDeleteService(w http.ResponseWriter, r *http.Request) {
	sse := datastar.NewSSE(w, r)
	id := r.PathValue("id")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second) // Longer timeout for deletion
	defer cancel()

	err := s.client.RemoveService(ctx, id)
	if err != nil {
		sse.ConsoleError(err)
		return
	}

	// Remove the service card from the UI
	sse.RemoveElement(fmt.Sprintf("#service-%s", id))
}

func (s *Server) handleVolumes(w http.ResponseWriter, r *http.Request) {
	sse := datastar.NewSSE(w, r)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	volumes, err := s.client.ListVolumes(ctx, nil)
	if err != nil {
		sse.PatchElements(fmt.Sprintf(`<div id="volume-list" style="color: red;">Error: %s</div>`, err.Error()))
		return
	}

	html := `<div id="volume-list">`
	if len(volumes) == 0 {
		html += "<div>No volumes found.</div>"
	} else {
		for _, v := range volumes {
			// Construct a unique ID for the volume card
			cardID := fmt.Sprintf("volume-%s-%s", v.MachineID, v.Volume.Name)

			html += fmt.Sprintf(`<div class="card" id="%s">
                <h3>%s</h3>
                <p>Machine: %s</p>
                <p>Driver: %s</p>
                <button class="delete-btn" data-on-click="@delete('/machines/%s/volumes/%s')">Delete</button>
            </div>`, cardID, v.Volume.Name, v.MachineName, v.Volume.Driver, v.MachineID, v.Volume.Name)
		}
	}
	html += `</div>`

	sse.PatchElements(html)
}

func (s *Server) handleDeleteVolume(w http.ResponseWriter, r *http.Request) {
	sse := datastar.NewSSE(w, r)
	machineID := r.PathValue("machine")
	volumeName := r.PathValue("volume")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Assuming force=false for safety. Could add a query param if needed.
	err := s.client.RemoveVolume(ctx, machineID, volumeName, false)
	if err != nil {
		sse.ConsoleError(err)
		return
	}

	// Remove the volume card from the UI
	cardID := fmt.Sprintf("volume-%s-%s", machineID, volumeName)
	sse.RemoveElement("#" + cardID)
}
