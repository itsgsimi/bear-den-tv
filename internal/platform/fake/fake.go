// Package fake is an in-memory DesktopAdapter for tests and `bear-den-tv dev`.
// It models a window list, one active window, and a record of delivered keys;
// it never touches a real display.
package fake

import (
	"context"
	"sync"

	"bear-den-tv/internal/platform"
)

// Name is the backend name reported in state.session.desktop_adapter.
const Name = "fake"

// Delivery is one recorded DeliverKey call.
type Delivery struct {
	Window platform.WindowID
	Key    platform.Key
}

// Desktop is a scriptable DesktopAdapter.
type Desktop struct {
	mu       sync.Mutex
	windows  map[platform.WindowID]platform.WindowInfo
	order    []platform.WindowID
	active   platform.WindowID
	next     platform.WindowID
	keys     []Delivery
	watchers []chan platform.Foreground
	// OnActivate, when set, runs after Activate changes the active window.
	OnActivate func(platform.WindowID)
	onClose    func(platform.WindowID)
}

// New returns an empty desktop with no active window.
func New() *Desktop {
	return &Desktop{windows: map[platform.WindowID]platform.WindowInfo{}, next: 0x100}
}

// AddWindow maps a window and returns its id. It does not activate it.
func (d *Desktop) AddWindow(w platform.WindowInfo) platform.WindowID {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.next++
	w.ID = d.next
	w.Mapped = true
	d.windows[w.ID] = w
	d.order = append(d.order, w.ID)
	return w.ID
}

// RemoveWindow unmaps a window; if it was active the foreground becomes unknown.
func (d *Desktop) RemoveWindow(id platform.WindowID) {
	d.mu.Lock()
	delete(d.windows, id)
	for i, o := range d.order {
		if o == id {
			d.order = append(d.order[:i], d.order[i+1:]...)
			break
		}
	}
	changed := d.active == id
	if changed {
		d.active = 0
	}
	d.mu.Unlock()
	if changed {
		d.broadcast()
	}
}

// SetActive makes id the foreground window (0 = none).
func (d *Desktop) SetActive(id platform.WindowID) {
	d.mu.Lock()
	d.active = id
	d.mu.Unlock()
	d.broadcast()
}

// Keys returns every delivered key in order.
func (d *Desktop) Keys() []Delivery {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Delivery(nil), d.keys...)
}

func (d *Desktop) foregroundLocked() platform.Foreground {
	w, ok := d.windows[d.active]
	if !ok {
		return platform.Foreground{}
	}
	return platform.Foreground{Known: true, Window: w}
}

func (d *Desktop) broadcast() {
	// Send under the lock (sends never block) so a watcher cannot be closed
	// between the snapshot and the send.
	d.mu.Lock()
	defer d.mu.Unlock()
	fg := d.foregroundLocked()
	for _, ch := range d.watchers {
		select {
		case ch <- fg:
		default:
		}
	}
}

// Name implements platform.DesktopAdapter.
func (*Desktop) Name() string { return Name }

// DisplaySession implements platform.DesktopAdapter.
func (*Desktop) DisplaySession() string { return "unknown" }

// Capabilities implements platform.DesktopAdapter; everything is available.
func (*Desktop) Capabilities() map[string]platform.Capability {
	ok := platform.Capability{Available: true, Backend: Name}
	return map[string]platform.Capability{
		platform.CapObserveForeground: ok,
		platform.CapActivate:          ok,
		platform.CapInput:             ok,
		platform.CapLockObservation:   ok,
		platform.CapPhysicalHome:      {Available: false, Backend: Name, Reason: "fake desktop has no keyboard"},
	}
}

// ObserveForeground implements platform.DesktopAdapter.
func (d *Desktop) ObserveForeground(context.Context) (platform.Foreground, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.foregroundLocked(), nil
}

// WatchForeground implements platform.DesktopAdapter.
func (d *Desktop) WatchForeground(ctx context.Context) (<-chan platform.Foreground, error) {
	ch := make(chan platform.Foreground, 16)
	d.mu.Lock()
	d.watchers = append(d.watchers, ch)
	d.mu.Unlock()
	go func() {
		<-ctx.Done()
		d.mu.Lock()
		for i, w := range d.watchers {
			if w == ch {
				d.watchers = append(d.watchers[:i], d.watchers[i+1:]...)
				break
			}
		}
		close(ch)
		d.mu.Unlock()
	}()
	return ch, nil
}

// ListWindows implements platform.DesktopAdapter.
func (d *Desktop) ListWindows(context.Context) ([]platform.WindowInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]platform.WindowInfo, 0, len(d.order))
	for _, id := range d.order {
		out = append(out, d.windows[id])
	}
	return out, nil
}

// Activate implements platform.DesktopAdapter.
func (d *Desktop) Activate(_ context.Context, w platform.WindowID) error {
	d.mu.Lock()
	if _, ok := d.windows[w]; !ok {
		d.mu.Unlock()
		return platform.ErrNotForeground
	}
	d.active = w
	cb := d.OnActivate
	d.mu.Unlock()
	d.broadcast()
	if cb != nil {
		cb(w)
	}
	return nil
}

// SetFullscreen implements platform.Fullscreener.
func (d *Desktop) SetFullscreen(_ context.Context, w platform.WindowID, on bool) error {
	d.mu.Lock()
	info, ok := d.windows[w]
	if ok {
		info.Fullscreen = on
		d.windows[w] = info
	}
	d.mu.Unlock()
	if !ok {
		return platform.ErrNotForeground
	}
	d.broadcast()
	return nil
}

// HasWindow reports whether w is still mapped.
func (d *Desktop) HasWindow(w platform.WindowID) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.windows[w]
	return ok
}

// RequestClose implements platform.DesktopAdapter; the window closes at once.
func (d *Desktop) RequestClose(_ context.Context, w platform.WindowID) error {
	d.RemoveWindow(w)
	d.mu.Lock()
	fn := d.onClose
	d.mu.Unlock()
	if fn != nil {
		fn(w)
	}
	return nil
}

// SetOnClose sets fn to run after RequestClose removes a window, e.g. to model
// an app that opens another window when asked to close.
func (d *Desktop) SetOnClose(fn func(platform.WindowID)) {
	d.mu.Lock()
	d.onClose = fn
	d.mu.Unlock()
}

// DeliverKey implements platform.DesktopAdapter.
func (d *Desktop) DeliverKey(_ context.Context, w platform.WindowID, k platform.Key) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.active != w {
		return platform.ErrNotForeground
	}
	d.keys = append(d.keys, Delivery{Window: w, Key: k})
	return nil
}

// Close implements platform.DesktopAdapter.
func (*Desktop) Close() error { return nil }

var (
	_ platform.DesktopAdapter = (*Desktop)(nil)
	_ platform.Fullscreener   = (*Desktop)(nil)
)
