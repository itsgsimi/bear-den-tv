// The WebSocket channel: state, results and holds pushed to phones (spec
// contracts/http.md).

package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/time/rate"

	"bear-den-tv/internal/contract"
)

// WebSocket close codes used by the server.
const (
	// websocketStatusRevoked (4001) follows a "revoked" message or a session end.
	websocketStatusRevoked   websocket.StatusCode = 4001
	websocketStatusGoingAway                      = websocket.StatusGoingAway
	wsWriteTimeout                                = 5 * time.Second
)

// Server → client message envelopes (contracts/http.md).
type wsStateMsg struct {
	Type  string         `json:"type"`
	State contract.State `json:"state"`
}

type wsResultMsg struct {
	Type   string                `json:"type"`
	Result contract.ActionResult `json:"result"`
}

type wsHoldMsg struct {
	Type   string `json:"type"`
	HoldID string `json:"hold_id"`
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

type wsTypeOnly struct {
	Type string `json:"type"`
}

// connRegistry tracks live WebSockets so revocation, logout, and shutdown can
// close them.
type connRegistry struct {
	mu    sync.Mutex
	conns map[*wsConn]struct{}
}

func newConnRegistry() *connRegistry {
	return &connRegistry{conns: map[*wsConn]struct{}{}}
}

func (r *connRegistry) add(c *wsConn) {
	r.mu.Lock()
	r.conns[c] = struct{}{}
	r.mu.Unlock()
}

func (r *connRegistry) remove(c *wsConn) {
	r.mu.Lock()
	delete(r.conns, c)
	r.mu.Unlock()
}

func (r *connRegistry) snapshot() []*wsConn {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*wsConn, 0, len(r.conns))
	for c := range r.conns {
		out = append(out, c)
	}
	return out
}

// revoke sends "revoked" and closes with 4001 every socket of deviceID
// ("*" means every device). Each close runs on its own goroutine so a slow
// peer never delays the others.
func (r *connRegistry) revoke(deviceID string) {
	for _, c := range r.snapshot() {
		if deviceID == "*" || c.viewer.DeviceID == deviceID {
			go c.revoke()
		}
	}
}

// closeSession closes every socket authenticated with sessionToken.
func (r *connRegistry) closeSession(sessionToken string, code websocket.StatusCode, reason string) {
	for _, c := range r.snapshot() {
		if c.sessionToken == sessionToken {
			go c.close(code, reason)
		}
	}
}

// closeAll closes every socket and waits for the close handshakes.
func (r *connRegistry) closeAll(code websocket.StatusCode, reason string) {
	var wg sync.WaitGroup
	for _, c := range r.snapshot() {
		wg.Add(1)
		go func(c *wsConn) {
			defer wg.Done()
			c.close(code, reason)
		}(c)
	}
	wg.Wait()
}

// wsConn is one authenticated /api/v1/events connection.
type wsConn struct {
	s            *Server
	c            *websocket.Conn
	viewer       Viewer
	sessionToken string
	ctx          context.Context
	cancel       context.CancelFunc
	limiter      *rate.Limiter

	writeMu sync.Mutex

	mu    sync.Mutex
	holds map[string]struct{}

	closeOnce sync.Once
}

// handleEvents upgrades the request and runs the connection until the socket
// closes. The request context is not used after Accept because the
// connection is hijacked; the server context bounds the socket instead.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.originPatterns()})
	if err != nil {
		s.log.Warn("remote: websocket accept failed", "request_id", scope.id, "device_id", scope.viewer.DeviceID, "err", err)
		return
	}
	c.SetReadLimit(wsMaxMessageBytes)
	ctx, cancel := context.WithCancel(s.ctx)
	conn := &wsConn{
		s:            s,
		c:            c,
		viewer:       *scope.viewer,
		sessionToken: scope.sessionToken,
		ctx:          ctx,
		cancel:       cancel,
		limiter:      rate.NewLimiter(rate.Limit(wsMessagesPerSec), wsMessagesPerSec),
		holds:        map[string]struct{}{},
	}
	s.conns.add(conn)
	defer conn.teardown()
	s.opts.Devices.Touch(ctx, conn.viewer.DeviceID, true)

	states, err := s.opts.Backend.Subscribe(ctx, &conn.viewer)
	if err != nil {
		s.log.Warn("remote: state subscribe failed", "request_id", scope.id, "device_id", conn.viewer.DeviceID, "err", err)
		conn.close(websocket.StatusInternalError, "subscribe failed")
		return
	}
	results, err := s.opts.Backend.Results(ctx, conn.viewer)
	if err != nil {
		s.log.Warn("remote: results subscribe failed", "request_id", scope.id, "device_id", conn.viewer.DeviceID, "err", err)
		conn.close(websocket.StatusInternalError, "subscribe failed")
		return
	}
	if err := conn.send(wsStateMsg{Type: "state", State: s.opts.Backend.Snapshot(ctx, &conn.viewer)}); err != nil {
		return
	}
	go conn.pushLoop(states, results)
	conn.readLoop()
}

// teardown runs once the read loop has returned: holds are stopped with
// reason "disconnect", presence is cleared, and the push loop is cancelled.
func (conn *wsConn) teardown() {
	conn.s.conns.remove(conn)
	conn.stopHolds("disconnect")
	conn.s.opts.Devices.Touch(conn.s.ctx, conn.viewer.DeviceID, false)
	conn.cancel()
	_ = conn.c.CloseNow()
}

func (conn *wsConn) readLoop() {
	for {
		typ, data, err := conn.c.Read(conn.ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			conn.close(websocket.StatusUnsupportedData, "text messages only")
			return
		}
		if !conn.limiter.AllowN(conn.s.clock.Now(), 1) {
			conn.rateLimited(data)
			continue
		}
		if !conn.dispatch(data) {
			return
		}
	}
}

// rateLimited answers a message that exceeded 60/s without processing it.
func (conn *wsConn) rateLimited(data []byte) {
	var head struct {
		Type    string `json:"type"`
		HoldID  string `json:"hold_id"`
		Request struct {
			RequestID string `json:"request_id"`
		} `json:"request"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return
	}
	switch head.Type {
	case "action":
		res := conn.s.submitRaw(conn.ctx, conn.viewer, mustJSON(map[string]any{"request_id": head.Request.RequestID}), func() bool { return false })
		_ = conn.send(wsResultMsg{Type: "action_result", Result: res})
	case "hold.start", "hold.renew", "hold.stop":
		_ = conn.send(wsHoldMsg{Type: "hold", HoldID: head.HoldID, State: "rejected", Reason: "rate_limited"})
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// dispatch handles one client message; false means the socket was closed.
func (conn *wsConn) dispatch(data []byte) bool {
	var head wsTypeOnly
	if err := json.Unmarshal(data, &head); err != nil {
		conn.close(websocket.StatusUnsupportedData, "invalid message")
		return false
	}
	switch head.Type {
	case "action":
		var m struct {
			Type    string          `json:"type"`
			Request json.RawMessage `json:"request"`
		}
		if !strictUnmarshal(data, &m) {
			conn.close(websocket.StatusUnsupportedData, "invalid action message")
			return false
		}
		res := conn.s.submitRaw(conn.ctx, conn.viewer, m.Request, func() bool { return true })
		return conn.send(wsResultMsg{Type: "action_result", Result: res}) == nil
	case "hold.start", "hold.renew", "hold.stop":
		var m contract.HoldMessage
		if !strictUnmarshal(data, &m) {
			conn.close(websocket.StatusUnsupportedData, "invalid hold message")
			return false
		}
		return conn.handleHold(m) == nil
	case "visibility":
		var m struct {
			Type   string `json:"type"`
			Hidden bool   `json:"hidden"`
		}
		if !strictUnmarshal(data, &m) {
			conn.close(websocket.StatusUnsupportedData, "invalid visibility message")
			return false
		}
		if m.Hidden {
			conn.stopHolds("hidden")
			return true
		}
		return conn.send(wsStateMsg{Type: "state", State: conn.s.opts.Backend.Snapshot(conn.ctx, &conn.viewer)}) == nil
	case "ping":
		return conn.send(wsTypeOnly{Type: "pong"}) == nil
	default:
		conn.close(websocket.StatusUnsupportedData, "unknown message type")
		return false
	}
}

func strictUnmarshal(data []byte, v any) bool {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return false
	}
	return !dec.More()
}

func (conn *wsConn) handleHold(m contract.HoldMessage) error {
	if !isUUID(m.HoldID) {
		return conn.send(wsHoldMsg{Type: "hold", HoldID: m.HoldID, State: "rejected", Reason: "invalid_hold_id"})
	}
	var st HoldStatus
	switch m.Type {
	case "hold.start":
		if !contract.IsNav(m.Action) {
			return conn.send(wsHoldMsg{Type: "hold", HoldID: m.HoldID, State: "rejected", Reason: "not_holdable"})
		}
		st = conn.s.opts.Backend.HoldStart(conn.ctx, conn.viewer, m)
	case "hold.renew":
		st = conn.s.opts.Backend.HoldRenew(conn.ctx, conn.viewer, m.HoldID)
	case "hold.stop":
		conn.s.opts.Backend.HoldStop(conn.ctx, conn.viewer, m.HoldID, "stop")
		st = HoldStatus{HoldID: m.HoldID, State: "cancelled", Reason: "stop"}
	}
	if st.HoldID == "" {
		st.HoldID = m.HoldID
	}
	conn.mu.Lock()
	if st.State == "active" {
		conn.holds[st.HoldID] = struct{}{}
	} else {
		delete(conn.holds, st.HoldID)
	}
	conn.mu.Unlock()
	return conn.send(wsHoldMsg{Type: "hold", HoldID: st.HoldID, State: st.State, Reason: st.Reason})
}

// stopHolds cancels every lease this connection started and, unless the
// socket is gone, reports each as cancelled with the reason.
func (conn *wsConn) stopHolds(reason string) {
	conn.mu.Lock()
	ids := make([]string, 0, len(conn.holds))
	for id := range conn.holds {
		ids = append(ids, id)
	}
	conn.holds = map[string]struct{}{}
	conn.mu.Unlock()
	for _, id := range ids {
		conn.s.opts.Backend.HoldStop(conn.s.ctx, conn.viewer, id, reason)
		if reason != "disconnect" {
			_ = conn.send(wsHoldMsg{Type: "hold", HoldID: id, State: "cancelled", Reason: reason})
		}
	}
}

// pushLoop forwards state and late results and keeps the peer alive with
// pings; an unanswered ping within the idle timeout closes the socket.
func (conn *wsConn) pushLoop(states <-chan contract.State, results <-chan contract.ActionResult) {
	interval := conn.s.opts.IdleTimeout / 2
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-conn.ctx.Done():
			return
		case st, ok := <-states:
			if !ok {
				states = nil
				continue
			}
			if conn.send(wsStateMsg{Type: "state", State: st}) != nil {
				return
			}
		case res, ok := <-results:
			if !ok {
				results = nil
				continue
			}
			if conn.send(wsResultMsg{Type: "action_result", Result: res}) != nil {
				return
			}
		case <-ticker.C:
			pctx, cancel := context.WithTimeout(conn.ctx, interval)
			err := conn.c.Ping(pctx)
			cancel()
			if err != nil {
				conn.s.log.Info("remote: websocket idle timeout", "device_id", conn.viewer.DeviceID)
				_ = conn.c.CloseNow()
				return
			}
		}
	}
}

// send serialises writes so pushes, results, and control messages never
// interleave.
func (conn *wsConn) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	conn.writeMu.Lock()
	defer conn.writeMu.Unlock()
	ctx, cancel := context.WithTimeout(conn.ctx, wsWriteTimeout)
	defer cancel()
	return conn.c.Write(ctx, websocket.MessageText, b)
}

// revoke delivers "revoked" then closes with 4001.
func (conn *wsConn) revoke() {
	_ = conn.send(wsTypeOnly{Type: "revoked"})
	conn.close(websocketStatusRevoked, "revoked")
}

// close performs the close handshake once; later calls are no-ops.
func (conn *wsConn) close(code websocket.StatusCode, reason string) {
	conn.closeOnce.Do(func() {
		_ = conn.c.Close(code, reason)
	})
}
