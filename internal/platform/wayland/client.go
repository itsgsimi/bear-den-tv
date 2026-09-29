// The protocol client behind the Wayland adapter (wayland.go): wl_display,
// wl_registry and wl_callback to learn the compositor's globals, wl_seat as
// the seat an activation names, and zwlr_foreign_toplevel_manager_v1 /
// zwlr_foreign_toplevel_handle_v1 (wlr-protocols, version 3) for the list of
// toplevel windows, which one is activated, and activate / close /
// fullscreen requests. One goroutine reads events; state changes are applied
// only at each handle's `done`, as the protocol asks. Plan and limits:
// docs/decisions/0007-wayland-profile.md.

package wayland

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"bear-den-tv/internal/platform"
)

// Interface names we look for in the registry.
const (
	ifaceSeat         = "wl_seat"
	ifaceWlrToplevel  = "zwlr_foreign_toplevel_manager_v1"
	ifaceExtToplevel  = "ext_foreign_toplevel_list_v1"
	ifaceKDEWindowMgt = "org_kde_plasma_window_management"
	ifaceGnomeShell   = "gtk_shell1"
)

// Opcodes. Requests are what we send, events what we receive.
const (
	displayID = 1

	reqDisplaySync        = 0
	reqDisplayGetRegistry = 1
	evDisplayError        = 0
	evDisplayDeleteID     = 1

	reqRegistryBind     = 0
	evRegistryGlobal    = 0
	evRegistryGlobalRem = 1

	evCallbackDone = 0

	evManagerToplevel = 0
	evManagerFinished = 1

	reqHandleActivate        = 4
	reqHandleClose           = 5
	reqHandleDestroy         = 7
	reqHandleSetFullscreen   = 8
	reqHandleUnsetFullscreen = 9

	evHandleTitle  = 0
	evHandleAppID  = 1
	evHandleState  = 4
	evHandleDone   = 5
	evHandleClosed = 6

	stateMaximized  = 0
	stateMinimized  = 1
	stateActivated  = 2
	stateFullscreen = 3

	// managerMaxVersion is the newest zwlr_foreign_toplevel_manager_v1 we
	// speak; version 2 added set_fullscreen, version 3 the parent event.
	managerMaxVersion = 3
)

// global is one registry announcement.
type global struct {
	iface   string
	version uint32
}

// toplevelState is what a handle's `done` commits.
type toplevelState struct {
	title, appID string
	activated    bool
	minimized    bool
	fullscreen   bool
}

// toplevel is one zwlr_foreign_toplevel_handle_v1.
type toplevel struct {
	object  uint32
	win     platform.WindowID
	pending toplevelState
	current toplevelState
	ready   bool // at least one done received
}

// client speaks the protocol on one connection.
type client struct {
	w *wire

	mu             sync.Mutex
	nextID         uint32
	registry       uint32
	globals        map[uint32]global // registry name → global
	callbacks      map[uint32]chan struct{}
	seat           uint32
	manager        uint32
	managerVersion uint32
	handles        map[uint32]*toplevel
	nextWin        platform.WindowID
	dead           error
	onChange       func()
	readDone       chan struct{}
}

// newClient starts reading events from w. onChange runs (without the lock
// held) after every committed toplevel change and when the connection dies.
func newClient(w *wire, onChange func()) *client {
	c := &client{
		w:         w,
		nextID:    2,
		globals:   map[uint32]global{},
		callbacks: map[uint32]chan struct{}{},
		handles:   map[uint32]*toplevel{},
		onChange:  onChange,
		readDone:  make(chan struct{}),
	}
	go c.readLoop()
	return c
}

func (c *client) newIDLocked() uint32 {
	id := c.nextID
	c.nextID++
	return id
}

func (c *client) err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dead
}

func (c *client) fail(err error) {
	c.mu.Lock()
	if c.dead != nil {
		c.mu.Unlock()
		return
	}
	c.dead = err
	for id, ch := range c.callbacks {
		close(ch)
		delete(c.callbacks, id)
	}
	c.mu.Unlock()
	if c.onChange != nil {
		c.onChange()
	}
}

// roundtrip sends wl_display.sync and waits for its done: every event the
// compositor queued before it has then been handled.
func (c *client) roundtrip(ctx context.Context) error {
	c.mu.Lock()
	if c.dead != nil {
		c.mu.Unlock()
		return c.dead
	}
	id := c.newIDLocked()
	ch := make(chan struct{})
	c.callbacks[id] = ch
	c.mu.Unlock()
	if err := c.w.send(displayID, reqDisplaySync, (&encoder{}).uint(id).b); err != nil {
		c.fail(fmt.Errorf("wayland: write: %w", err))
		return c.err()
	}
	select {
	case <-ch:
		return c.err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// start asks for the registry, collects every global, and binds the seat
// and the wlr toplevel manager when both are offered.
func (c *client) start(ctx context.Context) error {
	c.mu.Lock()
	c.registry = c.newIDLocked()
	reg := c.registry
	c.mu.Unlock()
	if err := c.w.send(displayID, reqDisplayGetRegistry, (&encoder{}).uint(reg).b); err != nil {
		return fmt.Errorf("wayland: write: %w", err)
	}
	if err := c.roundtrip(ctx); err != nil {
		return err
	}
	seatName, _, okSeat := c.find(ifaceSeat)
	mgrName, mgrVer, okMgr := c.find(ifaceWlrToplevel)
	if !okMgr {
		return nil
	}
	c.mu.Lock()
	if okSeat {
		c.seat = c.newIDLocked()
	}
	c.manager = c.newIDLocked()
	c.managerVersion = min(mgrVer, managerMaxVersion)
	seat, manager, mv := c.seat, c.manager, c.managerVersion
	c.mu.Unlock()
	if okSeat {
		if err := c.bind(seatName, ifaceSeat, 1, seat); err != nil {
			return err
		}
	}
	if err := c.bind(mgrName, ifaceWlrToplevel, mv, manager); err != nil {
		return err
	}
	return c.roundtrip(ctx)
}

func (c *client) bind(name uint32, iface string, version, id uint32) error {
	body := (&encoder{}).uint(name).string(iface).uint(version).uint(id).b
	if err := c.w.send(c.registry, reqRegistryBind, body); err != nil {
		return fmt.Errorf("wayland: bind %s: %w", iface, err)
	}
	return nil
}

// find returns the lowest-named global implementing iface.
func (c *client) find(iface string) (name, version uint32, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for n, g := range c.globals {
		if g.iface == iface && (!ok || n < name) {
			name, version, ok = n, g.version, true
		}
	}
	return name, version, ok
}

// has reports whether the compositor advertises iface.
func (c *client) has(iface string) bool {
	_, _, ok := c.find(iface)
	return ok
}

// readLoop decodes events until the connection ends. Watchers hear about
// committed changes only once everything the compositor flushed together
// has been applied, so a focus switch (old handle deactivated, new one
// activated: two separate `done`s) is one change, never a moment with
// nothing in front.
func (c *client) readLoop() {
	defer close(c.readDone)
	pending := false
	for {
		m, err := c.w.read()
		if err != nil {
			c.fail(fmt.Errorf("wayland: connection lost: %w", err))
			return
		}
		changed, err := c.dispatch(m)
		if err != nil {
			c.fail(err)
			_ = c.w.close()
			return
		}
		pending = pending || changed
		if pending && c.w.idle() {
			pending = false
			if c.onChange != nil {
				c.onChange()
			}
		}
	}
}

var errClosedByUs = errors.New("wayland: connection closed")

// dispatch applies one event and reports whether it committed a change to
// the toplevel list. An error ends the connection.
func (c *client) dispatch(m message) (bool, error) {
	d := &decoder{b: m.body}
	changed := false
	c.mu.Lock()
	switch {
	case m.object == displayID:
		switch m.opcode {
		case evDisplayError:
			obj, code, msg := d.uint(), d.uint(), d.string()
			c.mu.Unlock()
			return false, fmt.Errorf("wayland: compositor error on object %d (code %d): %s", obj, code, msg)
		case evDisplayDeleteID:
			_ = d.uint()
		}
	case m.object == c.registry && c.registry != 0:
		switch m.opcode {
		case evRegistryGlobal:
			name, iface, ver := d.uint(), d.string(), d.uint()
			if d.err == nil {
				c.globals[name] = global{iface: iface, version: ver}
			}
		case evRegistryGlobalRem:
			delete(c.globals, d.uint())
		}
	case c.callbacks[m.object] != nil:
		if m.opcode == evCallbackDone {
			close(c.callbacks[m.object])
			delete(c.callbacks, m.object)
		}
	case m.object == c.manager && c.manager != 0:
		switch m.opcode {
		case evManagerToplevel:
			id := d.uint()
			if d.err == nil {
				c.nextWin++
				c.handles[id] = &toplevel{object: id, win: c.nextWin}
			}
		case evManagerFinished:
			c.mu.Unlock()
			return false, errors.New("wayland: the compositor stopped the toplevel list (finished)")
		}
	case c.handles[m.object] != nil:
		h := c.handles[m.object]
		switch m.opcode {
		case evHandleTitle:
			h.pending.title = d.string()
		case evHandleAppID:
			h.pending.appID = d.string()
		case evHandleState:
			st := d.array()
			h.pending.activated, h.pending.minimized, h.pending.fullscreen = false, false, false
			for _, s := range st {
				switch s {
				case stateActivated:
					h.pending.activated = true
				case stateMinimized:
					h.pending.minimized = true
				case stateFullscreen:
					h.pending.fullscreen = true
				}
			}
		case evHandleDone:
			h.current, h.ready = h.pending, true
			changed = true
		case evHandleClosed:
			delete(c.handles, m.object)
			changed = true
			// The handle is inert now; destroy it so the compositor frees it.
			// Not from the reader itself: a write that blocks while the
			// compositor is blocked writing to us would deadlock.
			go func(obj uint32) { _ = c.w.send(obj, reqHandleDestroy, nil) }(m.object)
		}
	}
	err := d.err
	c.mu.Unlock()
	if err != nil {
		return false, fmt.Errorf("wayland: malformed event %d on object %d: %w", m.opcode, m.object, err)
	}
	return changed, nil
}

// windows returns every toplevel that has committed its first state, in
// announcement order.
func (c *client) windows() []platform.WindowInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]platform.WindowInfo, 0, len(c.handles))
	for _, h := range c.handles {
		if h.ready {
			out = append(out, h.info())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// active returns the activated toplevel, if exactly one is known.
func (c *client) active() (platform.WindowInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var found *toplevel
	for _, h := range c.handles {
		if h.ready && h.current.activated {
			if found != nil {
				return platform.WindowInfo{}, false // ambiguous: never guess
			}
			found = h
		}
	}
	if found == nil {
		return platform.WindowInfo{}, false
	}
	return found.info(), true
}

func (h *toplevel) info() platform.WindowInfo {
	return platform.WindowInfo{
		ID:         h.win,
		AppID:      h.current.appID,
		Title:      h.current.title,
		Mapped:     !h.current.minimized,
		Fullscreen: h.current.fullscreen,
	}
}

// handleFor maps a WindowID back to its live handle object.
func (c *client) handleFor(w platform.WindowID) (uint32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dead != nil {
		return 0, c.dead
	}
	for _, h := range c.handles {
		if h.win == w {
			return h.object, nil
		}
	}
	return 0, fmt.Errorf("wayland: window %d is not a known toplevel", w)
}

func (c *client) activate(w platform.WindowID) error {
	obj, err := c.handleFor(w)
	if err != nil {
		return err
	}
	c.mu.Lock()
	seat := c.seat
	c.mu.Unlock()
	if seat == 0 {
		return fmt.Errorf("%w: the compositor offers no wl_seat to activate with", platform.ErrUnsupported)
	}
	return c.w.send(obj, reqHandleActivate, (&encoder{}).uint(seat).b)
}

func (c *client) close(w platform.WindowID) error {
	obj, err := c.handleFor(w)
	if err != nil {
		return err
	}
	return c.w.send(obj, reqHandleClose, nil)
}

func (c *client) setFullscreen(w platform.WindowID, on bool) error {
	obj, err := c.handleFor(w)
	if err != nil {
		return err
	}
	c.mu.Lock()
	v := c.managerVersion
	c.mu.Unlock()
	if v < 2 {
		return fmt.Errorf("%w: the compositor's zwlr_foreign_toplevel_manager_v1 is version %d; fullscreen needs 2", platform.ErrUnsupported, v)
	}
	if on {
		return c.w.send(obj, reqHandleSetFullscreen, (&encoder{}).uint(0).b) // null output: the compositor picks
	}
	return c.w.send(obj, reqHandleUnsetFullscreen, nil)
}

// shutdown closes the connection and waits for the reader to stop.
func (c *client) shutdown() {
	c.mu.Lock()
	if c.dead == nil {
		c.dead = errClosedByUs
	}
	c.mu.Unlock()
	_ = c.w.close()
	<-c.readDone
}
