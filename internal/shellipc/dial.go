// Conn: the client side of the shell socket, used by the CLI and tests (spec
// contracts/ipc.md).

package shellipc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"bear-den-tv/internal/contract"
)

// Conn is a client-side connection used by cli clients (the bear-den-tv
// command) and tests. It is not safe for concurrent Recv calls.
type Conn struct {
	conn   net.Conn
	reader *bufio.Reader
	// Welcome is the accepted handshake reply.
	Welcome Welcome
	// State is the first snapshot received after welcome.
	State contract.State
}

// Dial connects to the socket, completes the handshake as the given client
// kind, and returns after the welcome and initial state arrived. A reject
// is returned as *RejectedError.
func Dial(ctx context.Context, socketPath, clientKind, version string) (*Conn, error) {
	d := net.Dialer{}
	nc, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, err
	}
	c := &Conn{conn: nc, reader: bufio.NewReaderSize(nc, 64*1024)}
	if deadline, ok := ctx.Deadline(); ok {
		_ = nc.SetDeadline(deadline)
	} else {
		_ = nc.SetDeadline(time.Now().Add(10 * time.Second))
	}
	if err := c.Send(Hello{Type: TypeHello, Protocol: contract.Protocol, Client: clientKind, Version: version, PID: os.Getpid()}); err != nil {
		_ = nc.Close()
		return nil, err
	}
	first, err := c.Recv()
	if err != nil {
		_ = nc.Close()
		return nil, err
	}
	switch m := first.(type) {
	case Welcome:
		c.Welcome = m
	case Reject:
		_ = nc.Close()
		return nil, &RejectedError{Reason: m.Reason, Supported: m.Supported}
	default:
		_ = nc.Close()
		return nil, fmt.Errorf("shellipc: unexpected %s during handshake", first.Kind())
	}
	second, err := c.Recv()
	if err != nil {
		_ = nc.Close()
		return nil, err
	}
	st, ok := second.(State)
	if !ok {
		_ = nc.Close()
		return nil, fmt.Errorf("shellipc: expected state after welcome, got %s", second.Kind())
	}
	c.State = st.State
	_ = nc.SetDeadline(time.Time{})
	return c, nil
}

// RejectedError is a handshake reject.
type RejectedError struct {
	Reason    string
	Supported []int
}

// Error implements error.
func (e *RejectedError) Error() string {
	return fmt.Sprintf("shellipc: rejected: %s (supported protocols %v)", e.Reason, e.Supported)
}

// Send writes one message.
func (c *Conn) Send(m Message) error {
	raw, err := Encode(m)
	if err != nil {
		return err
	}
	_, err = c.conn.Write(raw)
	return err
}

// Recv reads the next message, answering pings transparently.
func (c *Conn) Recv() (Message, error) {
	for {
		frame, err := readFrame(c.reader, MaxFrameBytes)
		if err != nil {
			return nil, err
		}
		m, err := Decode(frame)
		if err != nil {
			return nil, err
		}
		if _, ok := m.(Ping); ok {
			if err := c.Send(Pong{Type: TypePong}); err != nil {
				return nil, err
			}
			continue
		}
		return m, nil
	}
}

// RecvUntil reads messages until pred returns true or the deadline passes.
func (c *Conn) RecvUntil(deadline time.Time, pred func(Message) bool) (Message, error) {
	_ = c.conn.SetReadDeadline(deadline)
	defer c.conn.SetReadDeadline(time.Time{})
	for {
		m, err := c.Recv()
		if err != nil {
			return nil, err
		}
		if pred(m) {
			return m, nil
		}
	}
}

// Result waits for the Result reply with the given request id.
func (c *Conn) Result(requestID string, timeout time.Duration) (Result, error) {
	m, err := c.RecvUntil(time.Now().Add(timeout), func(m Message) bool {
		r, ok := m.(Result)
		return ok && r.RequestID == requestID
	})
	if err != nil {
		return Result{}, err
	}
	r := m.(Result)
	if !r.OK {
		return r, errors.New(r.Error)
	}
	return r, nil
}

// WeatherPlaces waits for the weather_places reply with the given request id.
func (c *Conn) WeatherPlaces(requestID string, timeout time.Duration) (WeatherPlaces, error) {
	m, err := c.RecvUntil(time.Now().Add(timeout), func(m Message) bool {
		r, ok := m.(WeatherPlaces)
		return ok && r.RequestID == requestID
	})
	if err != nil {
		return WeatherPlaces{}, err
	}
	return m.(WeatherPlaces), nil
}

// Close closes the connection.
func (c *Conn) Close() error { return c.conn.Close() }
