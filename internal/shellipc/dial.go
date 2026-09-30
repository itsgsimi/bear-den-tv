// Conn: the client side of the shell socket, used by the CLI and tests, and
// Ping, the watchdog's liveness check (spec contracts/ipc.md).

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
	return dial(ctx, socketPath, Hello{Type: TypeHello, Protocol: contract.Protocol, Client: clientKind, Version: version, PID: os.Getpid()})
}

// CheckAlive checks that the coordinator behind socketPath is responsive within
// ctx's deadline (5 s when ctx has none): a quiet cli handshake (welcome,
// then the state snapshot the coordinator builds under its own lock), then
// ping → pong (the connection's reader). A stopped or deadlocked
// coordinator still has its connection accepted by the kernel but never
// answers, so CheckAlive fails when the deadline passes. It changes nothing on
// the coordinator. The watchdog in scripts/start-session.sh runs it through
// `bear-den-tv doctor --ping`.
func CheckAlive(ctx context.Context, socketPath, version string) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}
	deadline, _ := ctx.Deadline()
	c, err := dial(ctx, socketPath, Hello{Type: TypeHello, Protocol: contract.Protocol, Client: ClientCLI, Version: version, PID: os.Getpid(), Quiet: true})
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Send(Ping{Type: TypePing}); err != nil {
		return err
	}
	_, err = c.RecvUntil(deadline, func(m Message) bool { _, ok := m.(Pong); return ok })
	return err
}

func dial(ctx context.Context, socketPath string, hello Hello) (*Conn, error) {
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
	if err := c.Send(hello); err != nil {
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
