// A minimal Chrome DevTools Protocol client over Chromium's
// --remote-debugging-pipe: JSON messages separated by NUL bytes, requests
// with ids, flat sessions (sessionId) and events. The pipe is two anonymous
// fds only the coordinator and its Chromium child hold, so nothing listens
// anywhere and no other process can speak on it
// (docs/decisions/0010-web-apps-over-cdp-pipe.md).

package web

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// MaxMessage bounds one message from Chromium; a larger one closes the pipe.
const MaxMessage = 8 << 20

// ErrClosed is returned once the pipe is gone (Chromium exited).
var ErrClosed = errors.New("web: devtools pipe closed")

// CDPError is a protocol error reply.
type CDPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *CDPError) Error() string { return fmt.Sprintf("devtools: %s (%d)", e.Message, e.Code) }

// Event is one protocol event.
type Event struct {
	Method    string
	SessionID string
	Params    json.RawMessage
}

type message struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *CDPError       `json:"error,omitempty"`
}

type reply struct {
	result json.RawMessage
	err    error
}

// Conn is one DevTools connection. Events are handed to the handler on the
// reader goroutine, in order; the handler must not block on Call.
type Conn struct {
	wmu     sync.Mutex
	w       io.Writer
	r       *bufio.Reader
	next    atomic.Int64
	mu      sync.Mutex
	pending map[int64]chan reply
	onEvent func(Event)
	closed  chan struct{}
	once    sync.Once
	err     error
}

// NewConn wraps the pipe ends (r: Chromium's fd 4, w: its fd 3) and starts
// reading. onEvent may be nil.
func NewConn(r io.Reader, w io.Writer, onEvent func(Event)) *Conn {
	c := &Conn{w: w, r: bufio.NewReaderSize(r, 64<<10), pending: map[int64]chan reply{}, onEvent: onEvent, closed: make(chan struct{})}
	go c.read()
	return c
}

// Done is closed when the pipe ends.
func (c *Conn) Done() <-chan struct{} { return c.closed }

func (c *Conn) close(err error) {
	c.once.Do(func() {
		c.mu.Lock()
		c.err = err
		for id, ch := range c.pending {
			ch <- reply{err: ErrClosed}
			delete(c.pending, id)
		}
		c.mu.Unlock()
		close(c.closed)
	})
}

func (c *Conn) read() {
	for {
		raw, err := c.readMessage()
		if err != nil {
			c.close(err)
			return
		}
		var m message
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		if m.ID != 0 {
			c.mu.Lock()
			ch := c.pending[m.ID]
			delete(c.pending, m.ID)
			c.mu.Unlock()
			if ch != nil {
				if m.Error != nil {
					ch <- reply{err: m.Error}
				} else {
					ch <- reply{result: m.Result}
				}
			}
			continue
		}
		if m.Method != "" && c.onEvent != nil {
			c.onEvent(Event{Method: m.Method, SessionID: m.SessionID, Params: m.Params})
		}
	}
}

func (c *Conn) readMessage() ([]byte, error) {
	var buf []byte
	for {
		chunk, err := c.r.ReadSlice(0)
		buf = append(buf, chunk...)
		if len(buf) > MaxMessage {
			return nil, fmt.Errorf("web: devtools message over %d bytes", MaxMessage)
		}
		switch {
		case err == nil:
			return buf[:len(buf)-1], nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		default:
			return nil, err
		}
	}
}

// Call sends method with params on a session ("" = the browser) and decodes
// the result into out (may be nil).
func (c *Conn) Call(ctx context.Context, sessionID, method string, params any, out any) error {
	id := c.next.Add(1)
	msg := message{ID: id, Method: method, SessionID: sessionID}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return err
		}
		msg.Params = raw
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	ch := make(chan reply, 1)
	c.mu.Lock()
	select {
	case <-c.closed:
		c.mu.Unlock()
		return ErrClosed
	default:
	}
	c.pending[id] = ch
	c.mu.Unlock()
	c.wmu.Lock()
	_, err = c.w.Write(append(raw, 0))
	c.wmu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		c.close(err)
		return ErrClosed
	}
	select {
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if out != nil && len(r.result) > 0 {
			return json.Unmarshal(r.result, out)
		}
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	}
}
