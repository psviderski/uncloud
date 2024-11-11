package corrosion

import (
	"fmt"
	"gvisor.dev/gvisor/pkg/binary"
	"io"
	"net"
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

func (c *AdminClient) SendCommand(cmd []byte) (<-chan []byte, error) {
	if _, err := c.conn.Write(encodeFrame(cmd)); err != nil {
		return nil, fmt.Errorf("send command: %w", err)
	}

	// Read the first frame immediately to check for errors.
	data, err := c.readFrame()
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	ch := make(chan []byte)
	go func() {
		defer close(ch)
		ch <- data

		for {
			data, err = c.readFrame()
			if err != nil {
				return
			}
			ch <- data
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

func (c *AdminClient) ClusterMembershipStates() (<-chan []byte, error) {
	return c.SendCommand([]byte("{\"Cluster\":\"MembershipStates\"}"))
}
