// Package x11 is the X11 desktop adapter: EWMH for foreground observation,
// activation, and close requests; XTEST for the closed set of key taps. Every
// key delivery re-reads _NET_ACTIVE_WINDOW immediately before injecting and
// refuses on mismatch, presses are always paired with a deferred release, and a
// lost X connection turns the adapter unavailable instead of crashing the
// coordinator.
package x11

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"

	"bear-den-tv/internal/platform"
)

// Name is the adapter's backend name.
const Name = "x11-ewmh-xtest"

// ErrUnavailable wraps every operation attempted after the X connection died.
var ErrUnavailable = errors.New("x11: adapter unavailable")

// timestampWait bounds how long Activate/RequestClose wait for a server
// timestamp before falling back to CurrentTime (0).
const timestampWait = 500 * time.Millisecond

const (
	atomNetActiveWindow        = "_NET_ACTIVE_WINDOW"
	atomNetClientList          = "_NET_CLIENT_LIST"
	atomNetWMName              = "_NET_WM_NAME"
	atomUTF8String             = "UTF8_STRING"
	atomNetWMPID               = "_NET_WM_PID"
	atomNetWMState             = "_NET_WM_STATE"
	atomNetWMStateFullscreen   = "_NET_WM_STATE_FULLSCREEN"
	atomNetCloseWindow         = "_NET_CLOSE_WINDOW"
	atomWMProtocols            = "WM_PROTOCOLS"
	atomWMDeleteWindow         = "WM_DELETE_WINDOW"
	atomNetSupported           = "_NET_SUPPORTED"
	atomNetSupportingWMCheck   = "_NET_SUPPORTING_WM_CHECK"
	atomTimestampProp          = "_BDTV_TIMESTAMP"
	sourceIndicationPager      = 2
	activateEventMask          = xproto.EventMaskSubstructureRedirect | xproto.EventMaskSubstructureNotify
	physicalHomeNotImplemented = "not implemented; use the phone remote's Home"
)

var atomNames = []string{atomNetActiveWindow, atomNetClientList, atomNetWMName, atomUTF8String, atomNetWMPID, atomNetWMState, atomNetWMStateFullscreen, atomNetCloseWindow, atomWMProtocols, atomWMDeleteWindow, atomNetSupported, atomNetSupportingWMCheck, atomTimestampProp}

// LockCapability is what the adapter needs from a lock observer to report the
// lock_observation capability; *lock.Observer satisfies it.
type LockCapability interface {
	Capability() platform.Capability
}

// Options configures New.
type Options struct {
	// Display is the X display; empty uses $DISPLAY.
	Display string
	// Lock supplies the lock_observation capability; nil reports it unavailable.
	Lock LockCapability
}

// Info is the static probe view of the X server and window manager.
type Info struct {
	Display         string                      `json:"display"`
	Vendor          string                      `json:"vendor"`
	ProtocolVersion string                      `json:"protocol_version"`
	WMName          string                      `json:"wm_name"`
	EWMH            bool                        `json:"ewmh"`
	Supported       map[string]bool             `json:"supported"`
	XTest           bool                        `json:"xtest"`
	XTestReason     string                      `json:"xtest_reason,omitempty"`
	MinKeycode      uint8                       `json:"min_keycode"`
	MaxKeycode      uint8                       `json:"max_keycode"`
	Keys            map[platform.Key]KeyBinding `json:"keys"`
}

// KeyBinding is how one logical key resolves on the current keyboard mapping.
type KeyBinding struct {
	Keysym   uint32 `json:"keysym"`
	Keycode  uint8  `json:"keycode"`
	Resolved bool   `json:"resolved"`
}

type watcher struct {
	notify chan struct{}
}

// Adapter implements platform.DesktopAdapter on one X connection.
type Adapter struct {
	display string
	conn    *xgb.Conn
	setup   *xproto.SetupInfo
	root    xproto.Window
	helper  xproto.Window
	atoms   map[string]xproto.Atom
	names   map[xproto.Atom]string
	lock    LockCapability

	xtest       bool
	xtestReason string
	wmName      string
	ewmh        bool
	supported   map[xproto.Atom]bool

	inputMu sync.Mutex // serializes DeliverKey

	mu          sync.Mutex
	dead        error
	keymap      *keymap
	keymapStale bool
	watchers    map[*watcher]struct{}
	timeWaiters []chan xproto.Timestamp

	closeOnce sync.Once
	pumpDone  chan struct{}
}

// New connects to the display, learns the window manager's EWMH support and
// XTEST presence, selects PropertyChange on the root, and starts the event
// pump. It fails only when no connection can be established.
func New(ctx context.Context, opts Options) (*Adapter, error) {
	conn, err := xgb.NewConnDisplay(opts.Display)
	if err != nil {
		return nil, fmt.Errorf("x11: connect to display %q: %w", displayLabel(opts.Display), err)
	}
	a := &Adapter{
		display:  displayLabel(opts.Display),
		conn:     conn,
		setup:    xproto.Setup(conn),
		lock:     opts.Lock,
		atoms:    map[string]xproto.Atom{},
		names:    map[xproto.Atom]string{},
		watchers: map[*watcher]struct{}{},
		pumpDone: make(chan struct{}),
	}
	a.root = a.setup.DefaultScreen(conn).Root
	if err := a.init(); err != nil {
		conn.Close()
		return nil, err
	}
	go a.pump()
	return a, nil
}

func displayLabel(d string) string {
	if d == "" {
		return "$DISPLAY"
	}
	return d
}

func (a *Adapter) init() error {
	cookies := make([]xproto.InternAtomCookie, len(atomNames))
	for i, n := range atomNames {
		cookies[i] = xproto.InternAtom(a.conn, false, uint16(len(n)), n)
	}
	for i, n := range atomNames {
		r, err := cookies[i].Reply()
		if err != nil {
			return fmt.Errorf("x11: intern %s: %w", n, err)
		}
		a.atoms[n] = r.Atom
		a.names[r.Atom] = n
	}
	a.atoms["WM_NAME"] = xproto.AtomWmName
	a.atoms["WM_CLASS"] = xproto.AtomWmClass

	if err := xtest.Init(a.conn); err != nil {
		a.xtestReason = "XTEST extension missing on the X server: " + err.Error()
	} else {
		a.xtest = true
	}

	a.supported = map[xproto.Atom]bool{}
	if r, err := a.getProp(a.root, a.atoms[atomNetSupported], xproto.AtomAtom, 512); err == nil {
		for _, at := range decodeAtoms(r.Value) {
			a.supported[at] = true
		}
	}
	if r, err := a.getProp(a.root, a.atoms[atomNetSupportingWMCheck], xproto.AtomWindow, 1); err == nil {
		if ws := decodeWindows(r.Value); len(ws) == 1 && ws[0] != 0 {
			a.ewmh = true
			a.wmName = a.windowTitle(ws[0])
		}
	}

	if err := xproto.ChangeWindowAttributesChecked(a.conn, a.root, xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange}).Check(); err != nil {
		return fmt.Errorf("x11: select PropertyChange on root: %w", err)
	}

	wid, err := xproto.NewWindowId(a.conn)
	if err != nil {
		return fmt.Errorf("x11: allocate helper window id: %w", err)
	}
	if err := xproto.CreateWindowChecked(a.conn, 0, wid, a.root, -1, -1, 1, 1, 0, xproto.WindowClassInputOnly, 0,
		xproto.CwOverrideRedirect|xproto.CwEventMask, []uint32{1, xproto.EventMaskPropertyChange}).Check(); err != nil {
		return fmt.Errorf("x11: create helper window: %w", err)
	}
	a.helper = wid

	a.keymapStale = true
	return nil
}

// pump dispatches server events until the connection closes.
func (a *Adapter) pump() {
	defer close(a.pumpDone)
	for {
		ev, xerr := a.conn.WaitForEvent()
		if ev == nil && xerr == nil {
			a.markDead(errors.New("X connection closed"))
			return
		}
		if xerr != nil {
			continue
		}
		switch e := ev.(type) {
		case xproto.PropertyNotifyEvent:
			switch {
			case e.Window == a.root && e.Atom == a.atoms[atomNetActiveWindow]:
				a.notifyWatchers()
			case e.Window == a.helper:
				a.deliverTime(e.Time)
			}
		case xproto.MappingNotifyEvent:
			a.mu.Lock()
			a.keymapStale = true
			a.mu.Unlock()
		}
	}
}

func (a *Adapter) markDead(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.dead != nil {
		return
	}
	a.dead = err
	for w := range a.watchers {
		close(w.notify)
	}
	a.watchers = map[*watcher]struct{}{}
	for _, ch := range a.timeWaiters {
		close(ch)
	}
	a.timeWaiters = nil
}

func (a *Adapter) notifyWatchers() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for w := range a.watchers {
		select {
		case w.notify <- struct{}{}:
		default:
		}
	}
}

func (a *Adapter) deliverTime(t xproto.Timestamp) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, ch := range a.timeWaiters {
		ch <- t
	}
	a.timeWaiters = nil
}

// alive returns the death error, wrapping ErrUnavailable, or nil.
func (a *Adapter) alive() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.dead != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, a.dead)
	}
	return nil
}

// await runs fn on its own goroutine so a hung X round trip cannot outlive
// ctx from the caller's point of view; fn still runs to completion, which is
// what keeps a started key tap balanced.
func await[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := fn()
		ch <- result{v, err}
	}()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

func (a *Adapter) getProp(w xproto.Window, prop, typ xproto.Atom, longs uint32) (*xproto.GetPropertyReply, error) {
	r, err := xproto.GetProperty(a.conn, false, w, prop, typ, 0, longs).Reply()
	if err != nil {
		if errors.Is(err, io.EOF) {
			a.markDead(errors.New("X connection closed"))
		}
		return nil, err
	}
	if r.Format == 0 {
		return nil, fmt.Errorf("x11: window %#x has no %s", w, a.atomName(prop))
	}
	return r, nil
}

func (a *Adapter) atomName(at xproto.Atom) string {
	if n, ok := a.names[at]; ok {
		return n
	}
	return fmt.Sprintf("atom %d", at)
}

func (a *Adapter) activeWindow() (xproto.Window, error) {
	r, err := a.getProp(a.root, a.atoms[atomNetActiveWindow], xproto.AtomWindow, 1)
	if err != nil {
		return 0, err
	}
	ws := decodeWindows(r.Value)
	if len(ws) == 0 {
		return 0, nil
	}
	return ws[0], nil
}

func (a *Adapter) clientList() ([]xproto.Window, error) {
	r, err := a.getProp(a.root, a.atoms[atomNetClientList], xproto.AtomWindow, 4096)
	if err != nil {
		return nil, err
	}
	return decodeWindows(r.Value), nil
}

func (a *Adapter) windowTitle(w xproto.Window) string {
	if r, err := a.getProp(w, a.atoms[atomNetWMName], a.atoms[atomUTF8String], 256); err == nil {
		return decodeString(r.Value)
	}
	if r, err := a.getProp(w, xproto.AtomWmName, xproto.AtomAny, 256); err == nil {
		return decodeString(r.Value)
	}
	return ""
}

// windowInfo reads everything the seam exposes about w; it fails only when the
// window no longer exists.
func (a *Adapter) windowInfo(w xproto.Window) (platform.WindowInfo, error) {
	attrs, err := xproto.GetWindowAttributes(a.conn, w).Reply()
	if err != nil {
		if errors.Is(err, io.EOF) {
			a.markDead(errors.New("X connection closed"))
		}
		return platform.WindowInfo{}, fmt.Errorf("x11: window %#x: %w", w, err)
	}
	info := platform.WindowInfo{ID: platform.WindowID(w), Mapped: attrs.MapState == xproto.MapStateViewable}
	info.Title = a.windowTitle(w)
	if r, err := a.getProp(w, xproto.AtomWmClass, xproto.AtomString, 256); err == nil {
		info.Class = decodeWMClass(r.Value)
	}
	if r, err := a.getProp(w, a.atoms[atomNetWMPID], xproto.AtomCardinal, 1); err == nil {
		if pid, ok := decodeCardinal(r.Value); ok {
			info.PID = int(pid)
		}
	}
	if r, err := a.getProp(w, a.atoms[atomNetWMState], xproto.AtomAtom, 64); err == nil {
		info.Fullscreen = containsAtom(decodeAtoms(r.Value), a.atoms[atomNetWMStateFullscreen])
	}
	return info, nil
}

// Name implements platform.DesktopAdapter.
func (*Adapter) Name() string { return Name }

// DisplaySession implements platform.DesktopAdapter.
func (*Adapter) DisplaySession() string { return "x11" }

func (a *Adapter) observeReason() string {
	switch {
	case !a.ewmh:
		return "no EWMH window manager on the display (_NET_SUPPORTING_WM_CHECK missing)"
	case !a.supported[a.atoms[atomNetActiveWindow]]:
		return "window manager does not advertise _NET_ACTIVE_WINDOW"
	}
	return ""
}

// Capabilities implements platform.DesktopAdapter.
func (a *Adapter) Capabilities() map[string]platform.Capability {
	caps := map[string]platform.Capability{}
	if err := a.alive(); err != nil {
		for _, k := range []string{platform.CapObserveForeground, platform.CapActivate, platform.CapInput} {
			caps[k] = platform.Capability{Backend: Name, Reason: err.Error()}
		}
	} else {
		observe := platform.Capability{Backend: Name, Available: true}
		if reason := a.observeReason(); reason != "" {
			observe = platform.Capability{Backend: Name, Reason: reason}
		}
		caps[platform.CapObserveForeground] = observe
		caps[platform.CapActivate] = observe
		switch {
		case !a.xtest:
			caps[platform.CapInput] = platform.Capability{Backend: Name, Reason: a.xtestReason}
		case !observe.Available:
			caps[platform.CapInput] = platform.Capability{Backend: Name, Reason: "foreground cannot be verified: " + observe.Reason}
		default:
			caps[platform.CapInput] = platform.Capability{Backend: Name, Available: true}
		}
	}
	if a.lock != nil {
		caps[platform.CapLockObservation] = a.lock.Capability()
	} else {
		caps[platform.CapLockObservation] = platform.Capability{Backend: Name, Reason: "no lock observer configured"}
	}
	caps[platform.CapPhysicalHome] = platform.Capability{Backend: Name, Reason: physicalHomeNotImplemented}
	return caps
}

// Info reports the static server facts and how every logical key resolves.
func (a *Adapter) Info(ctx context.Context) Info {
	info := Info{
		Display:         a.display,
		Vendor:          a.setup.Vendor,
		ProtocolVersion: fmt.Sprintf("%d.%d", a.setup.ProtocolMajorVersion, a.setup.ProtocolMinorVersion),
		WMName:          a.wmName,
		EWMH:            a.ewmh,
		Supported:       map[string]bool{},
		XTest:           a.xtest,
		XTestReason:     a.xtestReason,
		MinKeycode:      uint8(a.setup.MinKeycode),
		MaxKeycode:      uint8(a.setup.MaxKeycode),
		Keys:            map[platform.Key]KeyBinding{},
	}
	for _, n := range []string{atomNetActiveWindow, atomNetClientList, atomNetCloseWindow, atomNetWMName, atomNetWMPID, atomNetWMState, atomNetWMStateFullscreen} {
		info.Supported[n] = a.supported[a.atoms[n]]
	}
	km, _ := await(ctx, func() (*keymap, error) { return a.currentKeymap() })
	for k, sym := range keysyms {
		b := KeyBinding{Keysym: uint32(sym)}
		if code, ok := km.keycodeFor(sym); ok {
			b.Keycode, b.Resolved = uint8(code), true
		}
		info.Keys[k] = b
	}
	return info
}

// currentKeymap returns the cached keyboard mapping, refreshing it after a
// MappingNotify.
func (a *Adapter) currentKeymap() (*keymap, error) {
	a.mu.Lock()
	stale, km := a.keymapStale, a.keymap
	a.mu.Unlock()
	if !stale && km != nil {
		return km, nil
	}
	min, max := a.setup.MinKeycode, a.setup.MaxKeycode
	r, err := xproto.GetKeyboardMapping(a.conn, min, byte(max-min+1)).Reply()
	if err != nil {
		if errors.Is(err, io.EOF) {
			a.markDead(errors.New("X connection closed"))
		}
		return nil, fmt.Errorf("x11: keyboard mapping: %w", err)
	}
	km = newKeymap(min, r)
	a.mu.Lock()
	a.keymap, a.keymapStale = km, false
	a.mu.Unlock()
	return km, nil
}

// ObserveForeground implements platform.DesktopAdapter. Known is false when
// _NET_ACTIVE_WINDOW is None or names a window that no longer exists.
func (a *Adapter) ObserveForeground(ctx context.Context) (platform.Foreground, error) {
	if err := a.alive(); err != nil {
		return platform.Foreground{}, err
	}
	if reason := a.observeReason(); reason != "" {
		return platform.Foreground{}, fmt.Errorf("%w: %s", platform.ErrUnsupported, reason)
	}
	return await(ctx, func() (platform.Foreground, error) {
		w, err := a.activeWindow()
		if err != nil {
			return platform.Foreground{}, err
		}
		if w == 0 {
			return platform.Foreground{}, nil
		}
		info, err := a.windowInfo(w)
		if err != nil {
			return platform.Foreground{}, nil
		}
		return platform.Foreground{Known: true, Window: info}, nil
	})
}

// WatchForeground implements platform.DesktopAdapter. The current foreground
// is emitted first, then one Foreground per _NET_ACTIVE_WINDOW change
// (coalesced; identical consecutive window ids are suppressed). The channel
// closes when ctx ends or the connection dies.
func (a *Adapter) WatchForeground(ctx context.Context) (<-chan platform.Foreground, error) {
	if err := a.alive(); err != nil {
		return nil, err
	}
	if reason := a.observeReason(); reason != "" {
		return nil, fmt.Errorf("%w: %s", platform.ErrUnsupported, reason)
	}
	w := &watcher{notify: make(chan struct{}, 1)}
	a.mu.Lock()
	a.watchers[w] = struct{}{}
	a.mu.Unlock()
	out := make(chan platform.Foreground, 1)
	go func() {
		defer close(out)
		defer func() {
			a.mu.Lock()
			delete(a.watchers, w)
			a.mu.Unlock()
		}()
		first := true
		var lastKnown bool
		var lastID platform.WindowID
		emit := func() bool {
			fg, err := a.ObserveForeground(ctx)
			if err != nil {
				fg = platform.Foreground{}
			}
			if !first && fg.Known == lastKnown && fg.Window.ID == lastID {
				return true
			}
			first, lastKnown, lastID = false, fg.Known, fg.Window.ID
			select {
			case out <- fg:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if !emit() {
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-w.notify:
				if !ok {
					return
				}
				if !emit() {
					return
				}
			}
		}
	}()
	return out, nil
}

// ListWindows implements platform.DesktopAdapter with the window manager's
// _NET_CLIENT_LIST. Iconified windows are included with Mapped=false so an
// already-running application can still be activated.
func (a *Adapter) ListWindows(ctx context.Context) ([]platform.WindowInfo, error) {
	if err := a.alive(); err != nil {
		return nil, err
	}
	if !a.ewmh {
		return nil, fmt.Errorf("%w: %s", platform.ErrUnsupported, a.observeReason())
	}
	return await(ctx, func() ([]platform.WindowInfo, error) {
		ws, err := a.clientList()
		if err != nil {
			return nil, err
		}
		out := make([]platform.WindowInfo, 0, len(ws))
		for _, w := range ws {
			info, err := a.windowInfo(w)
			if err != nil {
				continue
			}
			out = append(out, info)
		}
		return out, nil
	})
}

func (a *Adapter) managed(w xproto.Window) error {
	ws, err := a.clientList()
	if err != nil {
		return err
	}
	for _, c := range ws {
		if c == w {
			return nil
		}
	}
	return fmt.Errorf("x11: window %#x is not a managed top-level window", w)
}

// serverTime obtains a fresh server timestamp through a property change on
// the helper window; 0 (CurrentTime) when the server does not answer in time.
func (a *Adapter) serverTime(ctx context.Context) xproto.Timestamp {
	ch := make(chan xproto.Timestamp, 1)
	a.mu.Lock()
	if a.dead != nil {
		a.mu.Unlock()
		return 0
	}
	a.timeWaiters = append(a.timeWaiters, ch)
	a.mu.Unlock()
	if err := xproto.ChangePropertyChecked(a.conn, xproto.PropModeReplace, a.helper, a.atoms[atomTimestampProp], xproto.AtomString, 8, 1, []byte{0}).Check(); err != nil {
		return 0
	}
	select {
	case t := <-ch:
		return t
	case <-ctx.Done():
		return 0
	case <-time.After(timestampWait):
		return 0
	}
}

func (a *Adapter) sendClientMessage(dest, window xproto.Window, typ xproto.Atom, mask uint32, data [5]uint32) error {
	ev := xproto.ClientMessageEvent{
		Format: 32,
		Window: window,
		Type:   typ,
		Data:   xproto.ClientMessageDataUnionData32New(data[:]),
	}
	err := xproto.SendEventChecked(a.conn, false, dest, mask, string(ev.Bytes())).Check()
	if err != nil && errors.Is(err, io.EOF) {
		a.markDead(errors.New("X connection closed"))
	}
	return err
}

// Activate implements platform.DesktopAdapter with a _NET_ACTIVE_WINDOW client
// message (source indication 2, fresh timestamp, current active window).
func (a *Adapter) Activate(ctx context.Context, w platform.WindowID) error {
	if err := a.alive(); err != nil {
		return err
	}
	if reason := a.observeReason(); reason != "" {
		return fmt.Errorf("%w: %s", platform.ErrUnsupported, reason)
	}
	_, err := await(ctx, func() (struct{}, error) {
		win := xproto.Window(w)
		if err := a.managed(win); err != nil {
			return struct{}{}, err
		}
		ts := a.serverTime(ctx)
		active, _ := a.activeWindow()
		return struct{}{}, a.sendClientMessage(a.root, win, a.atoms[atomNetActiveWindow], activateEventMask,
			[5]uint32{sourceIndicationPager, uint32(ts), uint32(active), 0, 0})
	})
	return err
}

// SetFullscreen implements platform.Fullscreener with an EWMH _NET_WM_STATE
// client message (add/remove _NET_WM_STATE_FULLSCREEN) to the root window.
func (a *Adapter) SetFullscreen(ctx context.Context, w platform.WindowID, on bool) error {
	if err := a.alive(); err != nil {
		return err
	}
	if !a.ewmh || !a.supported[a.atoms[atomNetWMStateFullscreen]] {
		return fmt.Errorf("%w: window manager does not advertise _NET_WM_STATE_FULLSCREEN", platform.ErrUnsupported)
	}
	action := uint32(0) // _NET_WM_STATE_REMOVE
	if on {
		action = 1 // _NET_WM_STATE_ADD
	}
	_, err := await(ctx, func() (struct{}, error) {
		win := xproto.Window(w)
		if err := a.managed(win); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, a.sendClientMessage(a.root, win, a.atoms[atomNetWMState], activateEventMask,
			[5]uint32{action, uint32(a.atoms[atomNetWMStateFullscreen]), 0, sourceIndicationPager, 0})
	})
	return err
}

// RequestClose implements platform.DesktopAdapter: _NET_CLOSE_WINDOW when the
// window manager supports it, else WM_DELETE_WINDOW when the window offers it.
func (a *Adapter) RequestClose(ctx context.Context, w platform.WindowID) error {
	if err := a.alive(); err != nil {
		return err
	}
	if !a.ewmh {
		return fmt.Errorf("%w: %s", platform.ErrUnsupported, a.observeReason())
	}
	_, err := await(ctx, func() (struct{}, error) {
		win := xproto.Window(w)
		if err := a.managed(win); err != nil {
			return struct{}{}, err
		}
		ts := a.serverTime(ctx)
		if a.supported[a.atoms[atomNetCloseWindow]] {
			return struct{}{}, a.sendClientMessage(a.root, win, a.atoms[atomNetCloseWindow], activateEventMask,
				[5]uint32{uint32(ts), sourceIndicationPager, 0, 0, 0})
		}
		r, err := a.getProp(win, a.atoms[atomWMProtocols], xproto.AtomAtom, 32)
		if err != nil || !containsAtom(decodeAtoms(r.Value), a.atoms[atomWMDeleteWindow]) {
			return struct{}{}, fmt.Errorf("%w: window %#x offers neither _NET_CLOSE_WINDOW nor WM_DELETE_WINDOW", platform.ErrUnsupported, win)
		}
		return struct{}{}, a.sendClientMessage(win, win, a.atoms[atomWMProtocols], 0,
			[5]uint32{uint32(a.atoms[atomWMDeleteWindow]), uint32(ts), 0, 0, 0})
	})
	return err
}

// KeycodeFor resolves a logical key on the current keyboard mapping.
func (a *Adapter) KeycodeFor(ctx context.Context, k platform.Key) (xproto.Keycode, error) {
	sym, ok := KeysymFor(k)
	if !ok {
		return 0, fmt.Errorf("%w: key %q is not in the injection set", platform.ErrUnsupported, k)
	}
	km, err := await(ctx, func() (*keymap, error) { return a.currentKeymap() })
	if err != nil {
		return 0, err
	}
	code, ok := km.keycodeFor(sym)
	if !ok {
		return 0, fmt.Errorf("%w: keysym %#x (%s) has no keycode in the current keyboard mapping", platform.ErrUnsupported, sym, k)
	}
	return code, nil
}

// DeliverKey implements platform.DesktopAdapter: one XTEST press+release of
// the keycode bound to k, only after _NET_ACTIVE_WINDOW re-reads as w. The
// release is deferred so no failure path leaves a key down.
func (a *Adapter) DeliverKey(ctx context.Context, w platform.WindowID, k platform.Key) error {
	if err := a.alive(); err != nil {
		return err
	}
	if !a.xtest {
		return fmt.Errorf("%w: %s", platform.ErrUnsupported, a.xtestReason)
	}
	if reason := a.observeReason(); reason != "" {
		return fmt.Errorf("%w: foreground cannot be verified: %s", platform.ErrUnsupported, reason)
	}
	code, err := a.KeycodeFor(ctx, k)
	if err != nil {
		return err
	}
	_, err = await(ctx, func() (struct{}, error) {
		return struct{}{}, a.tap(xproto.Window(w), code)
	})
	return err
}

func (a *Adapter) tap(w xproto.Window, code xproto.Keycode) (err error) {
	a.inputMu.Lock()
	defer a.inputMu.Unlock()
	active, aerr := a.activeWindow()
	if aerr != nil {
		return fmt.Errorf("x11: re-read active window: %w", aerr)
	}
	if active != w {
		return fmt.Errorf("%w: active window is %#x, target %#x", platform.ErrNotForeground, active, w)
	}
	defer func() {
		rerr := xtest.FakeInputChecked(a.conn, xproto.KeyRelease, byte(code), 0, xproto.WindowNone, 0, 0, 0).Check()
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				a.markDead(errors.New("X connection closed"))
			}
			if err == nil {
				err = fmt.Errorf("x11: key release: %w", rerr)
			}
		}
	}()
	if perr := xtest.FakeInputChecked(a.conn, xproto.KeyPress, byte(code), 0, xproto.WindowNone, 0, 0, 0).Check(); perr != nil {
		if errors.Is(perr, io.EOF) {
			a.markDead(errors.New("X connection closed"))
		}
		return fmt.Errorf("x11: key press: %w", perr)
	}
	return nil
}

// Close implements platform.DesktopAdapter; safe to call more than once.
func (a *Adapter) Close() error {
	a.closeOnce.Do(func() {
		a.conn.Close()
		<-a.pumpDone
		a.markDead(errors.New("adapter closed"))
	})
	return nil
}
