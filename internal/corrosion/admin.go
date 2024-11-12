package corrosion

import (
	"encoding/json"
	"errors"
	"fmt"
	"gvisor.dev/gvisor/pkg/binary"
	"io"
	"net"
	"net/netip"
	"time"
)

// AdminClient is a client for the Corrosion admin API.
type AdminClient struct {
	conn net.Conn
}

func NewAdminClient(sockPath string) (*AdminClient, error) {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("connect to admin socket: %w", err)
	}
	return &AdminClient{conn: conn}, nil
}

func (c *AdminClient) Close() error {
	return c.conn.Close()
}

type Response struct {
	JSON map[string]any
	// Err is set if the response is an error or if an error occurred while processing the response.
	Err error
}

// SendCommand sends a command to the Corrosion admin API and returns a channel that will receive responses.
// The channel will be closed after sending the last or error response. The caller must read from the channel until
// it is closed.
func (c *AdminClient) SendCommand(cmd []byte) (<-chan Response, error) {
	if _, err := c.conn.Write(encodeFrame(cmd)); err != nil {
		return nil, fmt.Errorf("send command: %w", err)
	}

	ch := make(chan Response)
	go func() {
		defer close(ch)

		for {
			r := Response{}

			data, err := c.readFrame()
			if err != nil {
				r.Err = err
				ch <- r
				return
			}

			var decoded any
			if err = json.Unmarshal(data, &decoded); err != nil {
				r.Err = fmt.Errorf("unmarshal response: %w", err)
				ch <- r
				// TODO: should we drain the connection here?
				return
			}

			switch v := decoded.(type) {
			case string:
				if v == "Success" {
					return
				}
				// Ignore other strings.
			case map[string]any:
				if errData, ok := v["Error"].(map[string]any); ok {
					if errMsg, ok := errData["msg"].(string); ok {
						r.Err = errors.New(errMsg)
					} else {
						r.Err = fmt.Errorf("invalid error response: %v", errData)
					}
					ch <- r
					return
				} else if jsonData, ok := v["Json"].(map[string]any); ok {
					r.JSON = jsonData
					ch <- r
				}
				// Ignore other maps.
			default:
				// Ignore other types.
			}
		}
	}()

	return ch, nil
}

// encodeFrame encodes a length_delimited Tokio frame by prefacing frame data with a frame head that specifies
// the length of the frame.
func encodeFrame(data []byte) []byte {
	encoded := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(encoded, uint32(len(data)))
	copy(encoded[4:], data)
	return encoded
}

// readFrame reads a length_delimited Tokio frame by extracting the frame data that follows the frame head.
func (c *AdminClient) readFrame() ([]byte, error) {
	// Read the frame head (4 bytes).
	head := make([]byte, 4)
	if _, err := io.ReadFull(c.conn, head); err != nil {
		return nil, fmt.Errorf("read frame head: %w", err)
	}
	// Read the frame data (length specified in the frame head).
	length := binary.BigEndian.Uint32(head)
	data := make([]byte, length)
	if _, err := io.ReadFull(c.conn, data); err != nil {
		return nil, fmt.Errorf("read frame data: %w", err)
	}

	return data, nil
}

type ClusterMembershipState struct {
	ID        string
	Addr      netip.AddrPort
	State     string
	Timestamp time.Time
}

var (
	// MembershipStateAlive indicates that the member is active.
	MembershipStateAlive string = "Alive"
	// MembershipStateSuspect indicates that the member is active, but at least one cluster member suspects its down.
	// For all purposes, a Suspect member is treated as if it were Alive until either it refutes the suspicion
	// (becoming Alive) or fails to do so (being declared Down).
	MembershipStateSuspect string = "Suspect"
	// MembershipStateDown indicates that the member is confirmed Down. A member that reaches this state can't join
	// the cluster with the same identity until the cluster forgets this knowledge.
	MembershipStateDown string = "Down"
)

func parseClusterMembershipState(json map[string]any) (ClusterMembershipState, error) {
	// Example JSON:
	//  {
	//    "id": {
	//      "addr": "[fdcc:1d51:6bae:6bb2:53c0:8796:1be0:b783]:51001",
	//      "cluster_id": 0,
	//      "id": "10d69d6f-6578-4dcf-a285-e860e40c5f06",
	//      "ts": 7435936225798880256
	//    },
	//    "incarnation": 0,
	//    "state": "Down"
	//  }

	var state ClusterMembershipState
	var err error

	idObj, ok := json["id"].(map[string]any)
	if !ok {
		return state, fmt.Errorf("missing or invalid 'id' field")
	}

	// ID
	if id, ok := idObj["id"].(string); ok {
		state.ID = id
	} else {
		return state, fmt.Errorf("missing or invalid 'id' field")
	}

	// Addr
	if addr, ok := idObj["addr"].(string); ok {
		state.Addr, err = netip.ParseAddrPort(addr)
		if err != nil {
			return state, fmt.Errorf("parse 'addr' field: %w", err)
		}
	} else {
		return state, fmt.Errorf("missing or invalid 'addr' field")
	}

	// State
	if stateStr, ok := json["state"].(string); ok {
		switch stateStr {
		case MembershipStateAlive, MembershipStateSuspect, MembershipStateDown:
			state.State = stateStr
		default:
			return state, fmt.Errorf("invalid 'state' field: %s", stateStr)
		}
	} else {
		return state, fmt.Errorf("missing or invalid 'state' field")
	}

	// Timestamp
	if ts, ok := idObj["ts"].(float64); ok {
		state.Timestamp = ntp64ToTime(uint64(ts))
	} else {
		return state, fmt.Errorf("missing or invalid 'ts' field")
	}

	return state, nil
}

// nt64ToTime converts a 64-bit NTP timestamp relative to the Unix epoch (1st Jan 1970) to time.Time.
// See for more details: https://datatracker.ietf.org/doc/html/rfc5905#section-6
func ntp64ToTime(ntp uint64) time.Time {
	// The NTP timestamp returned from Corrosion is relative to the Unix epoch (1st Jan 1970)
	// so no need to subtract the 70 years offset.
	secs := uint32(ntp >> 32)
	frac := uint32(ntp)
	// Convert the fraction to nanoseconds: frac * 1e9 / 2^32
	nsecs := (uint64(frac) * 1000_000_000) >> 32

	return time.Unix(int64(secs), int64(nsecs))
}

// ClusterMembershipStates returns the current membership SWIM states of all cluster members.
// If latest is true, only the latest state of each member is returned.
func (c *AdminClient) ClusterMembershipStates(latest bool) ([]ClusterMembershipState, error) {
	respCh, err := c.SendCommand([]byte("{\"Cluster\":\"MembershipStates\"}"))
	if err != nil {
		return nil, err
	}

	var (
		states       []ClusterMembershipState
		latestStates map[string]ClusterMembershipState
		parseErr     error
	)
	if latest {
		latestStates = make(map[string]ClusterMembershipState)
	}

	for r := range respCh {
		if r.Err != nil {
			// It's safe to return here because the channel is closed after the first error response.
			return nil, r.Err
		}

		s, err := parseClusterMembershipState(r.JSON)
		if err != nil {
			// Do not return early to drain the channel.
			parseErr = errors.Join(parseErr, err)
		} else {
			if latest {
				if existing, ok := latestStates[s.ID]; !ok || existing.Timestamp.Before(s.Timestamp) {
					latestStates[s.ID] = s
				}
			} else {
				states = append(states, s)
			}
		}
	}

	if latest {
		states = make([]ClusterMembershipState, 0, len(latestStates))
		for _, s := range latestStates {
			states = append(states, s)
		}
	}
	return states, parseErr
}
