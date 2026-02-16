package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/docker/compose/v2/pkg/progress"
	"github.com/psviderski/uncloud/internal/machine/api/pb"
	"github.com/psviderski/uncloud/internal/machine/caddyconfig"
	"github.com/psviderski/uncloud/pkg/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetDomain returns the cluster domain name or ErrNotFound if it hasn't been reserved yet.
func (cli *Client) GetDomain(ctx context.Context) (string, error) {
	domain, err := cli.ClusterClient.GetDomain(ctx, nil)
	if err != nil {
		if status.Convert(err).Code() == codes.NotFound {
			return "", api.ErrNotFound
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

		wg.Go(func() {
			if err = verifyCaddyReachable(ctx, m.Machine); err == nil {
				reachableMachines <- m.Machine
			}
		})
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

// verifyCaddyReachable verifies that the Caddy service is reachable on the machine by its public IP.
func verifyCaddyReachable(ctx context.Context, m *pb.MachineInfo) error {
	publicIP, _ := m.PublicIp.ToAddr()

	pw := progress.ContextWriter(ctx)
	eventID := fmt.Sprintf("Machine %s (%s)", m.Name, publicIP)
	pw.Event(progress.NewEvent(eventID, progress.Working, "Querying"))

	verifyURL := getVerifyURL(publicIP)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, verifyURL, nil)
	if err != nil {
		pw.Event(progress.NewEvent(eventID, progress.Error, err.Error()))
		return err
	}

	boff := backoff.WithContext(backoff.NewExponentialBackOff(
		backoff.WithMaxInterval(1*time.Second),
		backoff.WithMaxElapsedTime(5*time.Second),
	), ctx)
	client := &http.Client{Timeout: 3 * time.Second}
	do := func() (*http.Response, error) {
		return client.Do(req)
	}

	resp, err := backoff.RetryWithData(do, boff)
	if err != nil {
		e := unreachable(eventID)
		e.Text = fmt.Sprintf("Failed to send HTTP request: %v", err)
		pw.Event(e)

		return fmt.Errorf("send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		e := unreachable(eventID)
		e.Text = fmt.Sprintf("Unexpected HTTP response status code: %d", resp.StatusCode)
		pw.Event(e)

		return fmt.Errorf("unexpected HTTP response status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		e := unreachable(eventID)
		e.Text = fmt.Sprintf("Failed to read HTTP response body: %v", err)
		pw.Event(e)

		return fmt.Errorf("read HTTP response body: %w", err)
	}

	// Check the response body is the machine ID to ensure the correct Caddy container is responding.
	if string(body) == m.Id {
		pw.Event(progress.NewEvent(eventID, progress.Done, "Reachable"))
		return nil
	} else {
		bodyStr := string(body)
		if len(bodyStr) > 50 {
			bodyStr = bodyStr[:50] + "..."
		}

		e := unreachable(eventID)
		e.Text = fmt.Sprintf("Unexpected HTTP response body: %s", bodyStr)
		pw.Event(e)

		return fmt.Errorf("unexpected HTTP response body: %s", bodyStr)
	}
}

func getVerifyURL(publicIP netip.Addr) string {
	httpFormattedIP := net.JoinHostPort(publicIP.String(), "")
	return fmt.Sprintf("http://%s%s", httpFormattedIP, caddyconfig.VerifyPath)
}

// unreachable creates a new Unreachable error event.
func unreachable(id string) progress.Event {
	return progress.NewEvent(
		id,
		progress.Error,
		"Unreachable (probably behind NAT or firewall)",
	)
}
