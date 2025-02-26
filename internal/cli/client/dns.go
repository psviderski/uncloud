package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/compose/v2/pkg/progress"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"net/http"
	"sync"
	"time"
	"uncloud/internal/machine/api/pb"
	"uncloud/internal/machine/caddyfile"
)

// GetDomain returns the cluster domain name or ErrNotFound if it hasn't been reserved yet.
func (cli *Client) GetDomain(ctx context.Context) (string, error) {
	domain, err := cli.ClusterClient.GetDomain(ctx, nil)
	if err != nil {
		if status.Convert(err).Code() == codes.NotFound {
			return "", ErrNotFound
		}
		return "", err
	}

	return domain.Name, nil
}

var ErrNoReachableMachines = errors.New("no internet-reachable machines running service containers")

// CreateIngressRecords verifies which machines running the specified service (typically Caddy) are reachable from
// the internet, then creates DNS records for the cluster domain pointing to those machines. It tests each machine
// by sending HTTP requests to their public IPs. Only machines that respond correctly with their machine ID are included
// in the resulting DNS configuration. Returns the created DNS records or an error.
func (cli *Client) CreateIngressRecords(ctx context.Context, serviceID string) ([]*pb.DNSRecord, error) {
	svc, err := cli.InspectService(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("inspect service '%s': %w", serviceID, err)
	}

	machineIDs := make(map[string]struct{}, len(svc.Containers))
	for _, mc := range svc.Containers {
		machineIDs[mc.MachineID] = struct{}{}
	}

	var wg sync.WaitGroup
	reachableMachines := make(chan *pb.MachineInfo)

	for id := range machineIDs {
		m, err := cli.InspectMachine(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("inspect machine '%s': %w", id, err)
		}

		if m.Machine.PublicIp == nil {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			// Verify that the Caddy container is reachable on the machine by its public IP.
			publicIP, _ := m.Machine.PublicIp.ToAddr()

			pw := progress.ContextWriter(ctx)
			eventID := fmt.Sprintf("Machine %s (%s)", m.Machine.Name, publicIP)
			pw.Event(progress.NewEvent(eventID, progress.Working, "Querying"))

			verifyURL := fmt.Sprintf("http://%s%s", publicIP, caddyfile.VerifyPath)

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, verifyURL, nil)
			if err != nil {
				pw.Event(progress.NewEvent(eventID, progress.Error, err.Error()))
				return
			}

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				e := unreachable(eventID)
				e.Text = fmt.Sprintf("Failed to send HTTP request: %v", err)
				pw.Event(e)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				e := unreachable(eventID)
				e.Text = fmt.Sprintf("Unexpected HTTP response status code: %d", resp.StatusCode)
				pw.Event(e)
				return
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				e := unreachable(eventID)
				e.Text = fmt.Sprintf("Failed to read HTTP response body: %v", err)
				pw.Event(e)
				return
			}

			// Check the response body is the machine ID to ensure the correct Caddy container is responding.
			if string(body) == m.Machine.Id {
				pw.Event(progress.NewEvent(eventID, progress.Done, "Reachable"))
				reachableMachines <- m.Machine
			} else {
				bodyStr := string(body)
				if len(bodyStr) > 50 {
					bodyStr = bodyStr[:50] + "..."
				}

				e := unreachable(eventID)
				e.Text = fmt.Sprintf("Unexpected HTTP response body: %s", bodyStr)
				pw.Event(e)
			}
		}()
	}

	go func() {
		wg.Wait()
		close(reachableMachines)
	}()

	var ingressIPs []string
	for m := range reachableMachines {
		ip, _ := m.PublicIp.ToAddr()
		ingressIPs = append(ingressIPs, ip.String())
	}
	if len(ingressIPs) == 0 {
		return nil, ErrNoReachableMachines
	}

	req := &pb.CreateDomainRecordsRequest{
		Records: []*pb.DNSRecord{
			{
				Name:   "*",
				Type:   pb.DNSRecord_A,
				Values: ingressIPs,
			},
			// TODO: Add AAAA record with routable IPv6 addresses of machines running Caddy containers.
		},
	}
	resp, err := cli.CreateDomainRecords(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create cluster domain records in Uncloud DNS: %w", err)
	}

	return resp.Records, nil
}

// unreachable creates a new Unreachable error event.
func unreachable(id string) progress.Event {
	return progress.NewEvent(
		id,
		progress.Error,
		"Unreachable (probably behind NAT or firewall)",
	)
}
