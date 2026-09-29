// Server: the Unix socket listener with peer UID checks (spec
// contracts/ipc.md).

package shellipc

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
)

// Protocol limits from contracts/ipc.md.
const (
	MaxFrameBytes           = 262144
	DefaultPingInterval     = 5 * time.Second
	DefaultIdleTimeout      = 15 * time.Second
	DefaultReplyTimeout     = 2 * time.Second
	DefaultHandshakeTimeout = 5 * time.Second
)

// SocketDirName and SocketFileName form $XDG_RUNTIME_DIR/bear-den-tv/shell.sock.
const (
	SocketDirName  = "bear-den-tv"
	SocketFileName = "shell.sock"
)

// DefaultSocketPath returns the socket path under runtimeDir
// ($XDG_RUNTIME_DIR when empty; falls back to /run/user/<uid>).
func DefaultSocketPath(runtimeDir string) string {
	if runtimeDir == "" {
		runtimeDir = os.Getenv("XDG_RUNTIME_DIR")
	}
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	return filepath.Join(runtimeDir, SocketDirName, SocketFileName)
}

// Errors reported by Client operations.
var (
	ErrClosed       = errors.New("shellipc: connection closed")
	ErrReplyTimeout = errors.New("shellipc: reply timeout")
	ErrNoShell      = errors.New("shellipc: no shell connected")
)

// Handler receives connection events and messages. Receive runs on one
// dispatcher goroutine per client, in arrival order, so a handler may block
// on Client.Call without stalling the socket reader.
type Handler interface {
	// InitialState returns the snapshot sent right after welcome.
	InitialState(c *Client) contract.State
	// Connected runs after the handshake succeeded.
	Connected(c *Client)
	// Disconnected runs once when the connection ends; err is nil for a
	// clean close and otherwise names the cause (frame too large, idle, read error).
	Disconnected(c *Client, err error)
	// Receive delivers every non-handshake, non-keepalive message that is not
	// a reply awaited by Client.Call.
	Receive(c *Client, m Message)
}

// Options configures a Server.
type Options struct {
	SocketPath         string
	Clock              clock.Clock
	Logger             *slog.Logger
	Handler            Handler
	CoordinatorVersion string
	PingInterval       time.Duration
	IdleTimeout        time.Duration
	ReplyTimeout       time.Duration
	HandshakeTimeout   time.Duration
	MaxFrameBytes      int
	// UID is the only peer uid accepted; defaults to the process uid.
	UID int
	// PeerUID overrides SO_PEERCRED lookup (tests).
	PeerUID func(conn *net.UnixConn) (int, error)
}

// Server listens on the shell socket and owns every connected client.
type Server struct {
	opts     Options
	listener *net.UnixListener
	mu       sync.Mutex
	shell    *Client
	clients  map[*Client]struct{}
	closed   bool
	wg       sync.WaitGroup
}

// Listen creates the socket directory (0700) and socket (0600) and starts
// accepting clients until Close.
func Listen(opts Options) (*Server, error) {
	if opts.SocketPath == "" {
		return nil, errors.New("shellipc: socket path required")
	}
	if opts.Handler == nil {
		return nil, errors.New("shellipc: handler required")
	}
	if opts.Clock == nil {
		opts.Clock = clock.Real{}
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.PingInterval <= 0 {
		opts.PingInterval = DefaultPingInterval
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = DefaultIdleTimeout
	}
	if opts.ReplyTimeout <= 0 {
		opts.ReplyTimeout = DefaultReplyTimeout
	}
	if opts.HandshakeTimeout <= 0 {
		opts.HandshakeTimeout = DefaultHandshakeTimeout
	}
	if opts.MaxFrameBytes <= 0 {
		opts.MaxFrameBytes = MaxFrameBytes
	}
	if opts.UID == 0 && opts.PeerUID == nil {
		opts.UID = os.Getuid()
	}
	if opts.PeerUID == nil {
		opts.PeerUID = peerUID
	}
	dir := filepath.Dir(opts.SocketPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(opts.SocketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("shellipc: %s exists and is not a socket", opts.SocketPath)
		}
		_ = os.Remove(opts.SocketPath)
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: opts.SocketPath, Net: "unix"})
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(opts.SocketPath, 0o600); err != nil {
		_ = l.Close()
		return nil, err
	}
	s := &Server{opts: opts, listener: l, clients: map[*Client]struct{}{}}
	s.wg.Add(1)
	go s.acceptLoop()
	return s, nil
}

// Path returns the socket path.
func (s *Server) Path() string { return s.opts.SocketPath }

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.AcceptUnix()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}
			s.opts.Logger.Warn("shellipc: accept failed", "error", err.Error())
			continue
		}
		s.wg.Add(1)
		go s.serve(conn)
	}
}

// peerUID reads SO_PEERCRED.
func peerUID(conn *net.UnixConn) (int, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return -1, err
	}
	uid := -1
	var gerr error
	if err := raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			gerr = err
			return
		}
		uid = int(cred.Uid)
	}); err != nil {
		return -1, err
	}
	return uid, gerr
}

func (s *Server) serve(conn *net.UnixConn) {
	defer s.wg.Done()
	uid, err := s.opts.PeerUID(conn)
	if err != nil || uid != s.opts.UID {
		s.opts.Logger.Warn("shellipc: rejected peer", "uid", uid, "expected", s.opts.UID)
		_ = conn.Close()
		return
	}
	c := &Client{server: s, conn: conn, pending: map[string]chan Message{}, inbox: make(chan Message, 64), done: make(chan struct{})}
	c.lastSeen = s.opts.Clock.Now()
	reader := bufio.NewReaderSize(conn, 64*1024)

	handshook := make(chan struct{})
	hsTimer := s.opts.Clock.AfterFunc(s.opts.HandshakeTimeout, func() {
		select {
		case <-handshook:
		default:
			s.opts.Logger.Warn("shellipc: handshake timeout")
			_ = conn.Close()
		}
	})
	frame, err := readFrame(reader, s.opts.MaxFrameBytes)
	if err != nil {
		hsTimer.Stop()
		_ = conn.Close()
		return
	}
	msg, err := Decode(frame)
	hello, ok := msg.(Hello)
	if err != nil || !ok {
		hsTimer.Stop()
		c.sendRaw(Reject{Type: TypeReject, Reason: RejectInvalidHello, Supported: []int{contract.Protocol}})
		_ = conn.Close()
		return
	}
	if hello.Protocol != contract.Protocol {
		hsTimer.Stop()
		c.sendRaw(Reject{Type: TypeReject, Reason: RejectUnsupportedProtocol, Supported: []int{contract.Protocol}})
		_ = conn.Close()
		return
	}
	if hello.Client != ClientShell && hello.Client != ClientCLI {
		hsTimer.Stop()
		c.sendRaw(Reject{Type: TypeReject, Reason: RejectInvalidClient, Supported: []int{contract.Protocol}})
		_ = conn.Close()
		return
	}
	c.kind = hello.Client
	c.pid = hello.PID
	c.version = hello.Version
	c.id = randomSessionID()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		hsTimer.Stop()
		_ = conn.Close()
		return
	}
	if c.kind == ClientShell && s.shell != nil {
		s.mu.Unlock()
		hsTimer.Stop()
		c.sendRaw(Reject{Type: TypeReject, Reason: RejectShellAlreadyConnected, Supported: []int{contract.Protocol}})
		_ = conn.Close()
		return
	}
	if c.kind == ClientShell {
		s.shell = c
	}
	s.clients[c] = struct{}{}
	s.mu.Unlock()
	close(handshook)
	hsTimer.Stop()

	// Welcome first, then the initial state (contracts/ipc.md): until both
	// are written the client is not ready, so Shell(), Clients() and
	// Broadcast do not see it. A broadcast in the meantime only marks it
	// missed, and the snapshot is built again so the client never ends on
	// an older state than the latest publish.
	if err := c.Send(Welcome{Type: TypeWelcome, Protocol: contract.Protocol, SessionID: c.id, CoordinatorVersion: s.opts.CoordinatorVersion}); err != nil {
		s.dropClient(c, err)
		return
	}
	for {
		s.mu.Lock()
		c.missed = false
		s.mu.Unlock()
		if err := c.Send(State{Type: TypeState, State: s.opts.Handler.InitialState(c)}); err != nil {
			s.dropClient(c, err)
			return
		}
		s.mu.Lock()
		missed := c.missed
		c.ready = !missed
		s.mu.Unlock()
		if !missed {
			break
		}
	}
	s.opts.Logger.Info("shellipc: client connected", "client", c.kind, "pid", c.pid, "version", c.version)
	s.wg.Add(1)
	go c.dispatch()
	s.opts.Handler.Connected(c)
	c.armKeepalive()
	readErr := c.readLoop(reader)
	s.dropClient(c, readErr)
}

func (s *Server) dropClient(c *Client, err error) {
	c.closeOnce.Do(func() {
		c.stopTimers()
		_ = c.conn.Close()
		close(c.done)
		s.mu.Lock()
		delete(s.clients, c)
		if s.shell == c {
			s.shell = nil
		}
		s.mu.Unlock()
		c.mu.Lock()
		for id, ch := range c.pending {
			close(ch)
			delete(c.pending, id)
		}
		c.mu.Unlock()
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
			s.opts.Logger.Warn("shellipc: client disconnected", "client", c.kind, "error", err.Error())
		} else {
			s.opts.Logger.Info("shellipc: client disconnected", "client", c.kind)
		}
		s.opts.Handler.Disconnected(c, err)
	})
}

// Shell returns the connected shell client once its handshake (welcome and
// initial state) is written, or nil.
func (s *Server) Shell() *Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shell == nil || !s.shell.ready {
		return nil
	}
	return s.shell
}

// Clients returns every client whose handshake is written.
func (s *Server) Clients() []*Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readyLocked()
}

func (s *Server) readyLocked() []*Client {
	out := make([]*Client, 0, len(s.clients))
	for c := range s.clients {
		if c.ready {
			out = append(out, c)
		}
	}
	return out
}

// Broadcast sends a snapshot to every client; view builds it per client.
// A client still in its handshake is not sent anything: it is marked
// missed and gets a fresh snapshot right after its initial one.
func (s *Server) Broadcast(view func(c *Client) contract.State) {
	s.mu.Lock()
	for c := range s.clients {
		if !c.ready {
			c.missed = true
		}
	}
	ready := s.readyLocked()
	s.mu.Unlock()
	for _, c := range ready {
		if err := c.Send(State{Type: TypeState, State: view(c)}); err != nil && !errors.Is(err, ErrClosed) {
			s.opts.Logger.Warn("shellipc: broadcast failed", "client", c.kind, "error", err.Error())
		}
	}
}

// Close stops accepting, closes every client, and removes the socket file.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	err := s.listener.Close()
	for _, c := range s.Clients() {
		s.dropClient(c, nil)
	}
	s.wg.Wait()
	_ = os.Remove(s.opts.SocketPath)
	return err
}

// Client is one connected shell or cli peer.
type Client struct {
	server  *Server
	conn    *net.UnixConn
	kind    string
	id      string
	pid     int
	version string

	// ready (the handshake is written) and missed (a broadcast came during
	// the handshake) are guarded by server.mu.
	ready, missed bool

	writeMu   sync.Mutex
	mu        sync.Mutex
	pending   map[string]chan Message
	lastSeen  time.Time
	pingTimer clock.Timer
	idleTimer clock.Timer
	inbox     chan Message
	done      chan struct{}
	closeOnce sync.Once
}

// Kind is "shell" or "cli".
func (c *Client) Kind() string { return c.kind }

// PID is the pid the client declared in hello (informational).
func (c *Client) PID() int { return c.pid }

// SessionID is the id assigned in welcome.
func (c *Client) SessionID() string { return c.id }

// Done is closed when the connection ends.
func (c *Client) Done() <-chan struct{} { return c.done }

// Close ends the connection.
func (c *Client) Close() { c.server.dropClient(c, nil) }

// Send writes one frame.
func (c *Client) Send(m Message) error {
	select {
	case <-c.done:
		return ErrClosed
	default:
	}
	return c.sendRaw(m)
}

func (c *Client) sendRaw(m Message) error {
	raw, err := Encode(m)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = c.conn.Write(raw)
	if err != nil {
		return err
	}
	return nil
}

// Call sends a request_id-bearing message and waits for its terminal reply
// (input_result for input/home, confirm_result for confirm_request) up to the
// reply timeout. A timeout returns ErrReplyTimeout; a closed connection
// returns ErrClosed.
func (c *Client) Call(ctx context.Context, requestID string, m Message) (Message, error) {
	ch := make(chan Message, 1)
	c.mu.Lock()
	c.pending[requestID] = ch
	c.mu.Unlock()
	cleanup := func() {
		c.mu.Lock()
		delete(c.pending, requestID)
		c.mu.Unlock()
	}
	if err := c.Send(m); err != nil {
		cleanup()
		return nil, err
	}
	timer := c.server.opts.Clock.NewTimer(c.server.opts.ReplyTimeout)
	defer timer.Stop()
	select {
	case reply, ok := <-ch:
		if !ok {
			return nil, ErrClosed
		}
		return reply, nil
	case <-timer.C():
		cleanup()
		return nil, ErrReplyTimeout
	case <-ctx.Done():
		cleanup()
		return nil, ctx.Err()
	case <-c.done:
		return nil, ErrClosed
	}
}

func (c *Client) armKeepalive() {
	s := c.server
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pingTimer = s.opts.Clock.AfterFunc(s.opts.PingInterval, c.onPing)
	c.idleTimer = s.opts.Clock.AfterFunc(s.opts.IdleTimeout, c.onIdle)
}

func (c *Client) onPing() {
	select {
	case <-c.done:
		return
	default:
	}
	_ = c.Send(Ping{Type: TypePing})
	c.mu.Lock()
	if c.pingTimer != nil {
		c.pingTimer.Reset(c.server.opts.PingInterval)
	}
	c.mu.Unlock()
}

func (c *Client) onIdle() {
	c.mu.Lock()
	idle := c.server.opts.Clock.Since(c.lastSeen)
	timeout := c.server.opts.IdleTimeout
	if idle < timeout {
		if c.idleTimer != nil {
			c.idleTimer.Reset(timeout - idle)
		}
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	c.server.dropClient(c, errors.New("idle timeout"))
}

func (c *Client) stopTimers() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pingTimer != nil {
		c.pingTimer.Stop()
	}
	if c.idleTimer != nil {
		c.idleTimer.Stop()
	}
}

func (c *Client) readLoop(reader *bufio.Reader) error {
	for {
		frame, err := readFrame(reader, c.server.opts.MaxFrameBytes)
		if err != nil {
			return err
		}
		c.mu.Lock()
		c.lastSeen = c.server.opts.Clock.Now()
		c.mu.Unlock()
		msg, err := Decode(frame)
		if err != nil {
			c.server.opts.Logger.Warn("shellipc: dropped frame", "client", c.kind, "error", err.Error())
			continue
		}
		switch m := msg.(type) {
		case Ping:
			_ = c.Send(Pong{Type: TypePong})
			continue
		case Pong:
			continue
		case Hello:
			continue
		case InputResult:
			if c.deliverReply(m.RequestID, m) {
				continue
			}
		case ConfirmResult:
			if c.deliverReply(m.ConfirmID, m) {
				continue
			}
		}
		select {
		case c.inbox <- msg:
		case <-c.done:
			return ErrClosed
		}
	}
}

func (c *Client) deliverReply(id string, m Message) bool {
	c.mu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if ok {
		ch <- m
	}
	return ok
}

func (c *Client) dispatch() {
	defer c.server.wg.Done()
	for {
		select {
		case m := <-c.inbox:
			c.server.opts.Handler.Receive(c, m)
		case <-c.done:
			return
		}
	}
}

// ErrFrameTooLarge closes a connection whose frame exceeds the limit.
var ErrFrameTooLarge = errors.New("shellipc: frame exceeds size limit")

// readFrame reads one newline-terminated frame of at most limit bytes.
func readFrame(r *bufio.Reader, limit int) ([]byte, error) {
	var frame []byte
	for {
		chunk, err := r.ReadSlice('\n')
		frame = append(frame, chunk...)
		if len(frame) > limit+1 {
			return nil, ErrFrameTooLarge
		}
		if err == nil {
			return frame[:len(frame)-1], nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) && len(frame) > 0 {
			return frame, nil
		}
		return nil, err
	}
}

func randomSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "session"
	}
	return hex.EncodeToString(b[:])
}
