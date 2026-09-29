// Tests for the IPC server's handshake (server.go, spec contracts/ipc.md →
// Handshake): welcome, then the initial state, and nothing else before them.

package shellipc

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"bear-den-tv/internal/contract"
)

// handshakeHandler publishes from inside its first InitialState: a
// coordinator publish that lands while a client's handshake is still being
// written. Each snapshot carries a sequence number in GeneratedAtMs.
type handshakeHandler struct {
	t   *testing.T
	srv func() *Server

	mu      sync.Mutex
	seq     int64
	visible []string // what Shell()/Clients() exposed during the handshake
}

func (h *handshakeHandler) next() contract.State {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	return contract.State{GeneratedAtMs: h.seq}
}

func (h *handshakeHandler) InitialState(c *Client) contract.State {
	st := h.next()
	if st.GeneratedAtMs == 1 {
		s := h.srv()
		h.mu.Lock()
		if s.Shell() != nil {
			h.visible = append(h.visible, "Shell()")
		}
		if len(s.Clients()) != 0 {
			h.visible = append(h.visible, "Clients()")
		}
		h.mu.Unlock()
		s.Broadcast(func(*Client) contract.State { return h.next() })
	}
	return st
}

func (*handshakeHandler) Connected(*Client)           {}
func (*handshakeHandler) Disconnected(*Client, error) {}
func (*handshakeHandler) Receive(*Client, Message)    {}

// TestNothingReachesAClientBeforeItsHandshake: a publish during the
// handshake used to be written before welcome (the client saw "unexpected
// state during handshake") or before the older initial state. Now a client
// is invisible until welcome and its initial state are written, and it ends
// on the newest snapshot.
func TestNothingReachesAClientBeforeItsHandshake(t *testing.T) {
	var srv *Server
	h := &handshakeHandler{t: t, srv: func() *Server { return srv }}
	var err error
	srv, err = Listen(Options{SocketPath: filepath.Join(t.TempDir(), "s.sock"), Handler: h})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := Dial(ctx, srv.Path(), ClientShell, "test")
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	defer conn.Close()
	h.mu.Lock()
	visible := h.visible
	h.mu.Unlock()
	if len(visible) != 0 {
		t.Fatalf("the client was visible through %v before its handshake was written", visible)
	}
	if conn.State.GeneratedAtMs != 1 {
		t.Fatalf("initial state #%d, want #1", conn.State.GeneratedAtMs)
	}
	// The publish during the handshake is followed by a snapshot built after
	// it (#2 or later), never by an older one.
	m, err := conn.RecvUntil(time.Now().Add(5*time.Second), func(m Message) bool { _, ok := m.(State); return ok })
	if err != nil {
		t.Fatalf("no state after the handshake: %v", err)
	}
	if got := m.(State).State.GeneratedAtMs; got < 2 {
		t.Fatalf("state after the handshake #%d, want one built after the publish (#2 or later)", got)
	}
	// Ready is set right after that write, so wait for it.
	for deadline := time.Now().Add(5 * time.Second); srv.Shell() == nil || len(srv.Clients()) != 1; {
		if time.Now().After(deadline) {
			t.Fatal("the client is not visible after its handshake")
		}
		time.Sleep(time.Millisecond)
	}
}
