// Tests for the Wayland adapter (wayland.go, client.go, wire.go) against a
// fake compositor that speaks the server side of wl_display, wl_registry,
// wl_callback and zwlr_foreign_toplevel_manager_v1 over an in-memory pipe.
// The live counterpart is scripts/wayland-container-test.sh (headless sway).

package wayland

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bear-den-tv/internal/platform"
)

// request is one message the adapter sent to the fake compositor.
type request struct {
	object uint32
	opcode uint16
	body   []byte
}

// fakeCompositor is the server end of one client connection.
type fakeCompositor struct {
	t       *testing.T
	w       *wire
	globals []global // registry name is index+1

	mu        sync.Mutex
	requests  []request
	registry  uint32
	seat      uint32
	manager   uint32
	nextObj   uint32
	toplevels map[string]uint32 // app_id → handle object
	initial   []fakeToplevel
	changed   chan struct{}
}

type fakeToplevel struct {
	appID, title string
	states       []uint32
}

func newFakeCompositor(t *testing.T, globals []global, initial ...fakeToplevel) (*fakeCompositor, func(context.Context, string) (net.Conn, error)) {
	f := &fakeCompositor{t: t, globals: globals, nextObj: 0xff000000, toplevels: map[string]uint32{}, initial: initial, changed: make(chan struct{}, 64)}
	dial := func(context.Context, string) (net.Conn, error) {
		cli, srv := net.Pipe()
		f.w = newWire(srv)
		go f.serve()
		t.Cleanup(func() { _ = srv.Close() })
		return cli, nil
	}
	return f, dial
}

func wlrGlobals() []global {
	return []global{{"wl_compositor", 6}, {ifaceSeat, 9}, {ifaceWlrToplevel, 3}, {"wl_output", 4}}
}

func (f *fakeCompositor) send(obj uint32, op uint16, body []byte) {
	_ = f.w.send(obj, op, body) // the client may already be gone
}

func words(ws ...uint32) []byte {
	var b []byte
	b = binary.LittleEndian.AppendUint32(b, uint32(4*len(ws)))
	for _, w := range ws {
		b = binary.LittleEndian.AppendUint32(b, w)
	}
	return b
}

func (f *fakeCompositor) serve() {
	for {
		m, err := f.w.read()
		if err != nil {
			return
		}
		d := &decoder{b: m.body}
		f.mu.Lock()
		f.requests = append(f.requests, request{m.object, m.opcode, m.body})
		f.mu.Unlock()
		switch {
		case m.object == displayID && m.opcode == reqDisplayGetRegistry:
			f.registry = d.uint()
			for i, g := range f.globals {
				f.send(f.registry, evRegistryGlobal, (&encoder{}).uint(uint32(i+1)).string(g.iface).uint(g.version).b)
			}
		case m.object == displayID && m.opcode == reqDisplaySync:
			cb := d.uint()
			f.send(cb, evCallbackDone, (&encoder{}).uint(1).b)
			f.send(displayID, evDisplayDeleteID, (&encoder{}).uint(cb).b)
		case m.object == f.registry && m.opcode == reqRegistryBind:
			_, iface, _, id := d.uint(), d.string(), d.uint(), d.uint()
			switch iface {
			case ifaceSeat:
				f.mu.Lock()
				f.seat = id
				f.mu.Unlock()
			case ifaceWlrToplevel:
				f.mu.Lock()
				f.manager = id
				f.mu.Unlock()
				for _, tl := range f.initial {
					f.add(tl)
				}
			}
		}
		select {
		case f.changed <- struct{}{}:
		default:
		}
	}
}

// add announces a toplevel and commits its first state.
func (f *fakeCompositor) add(tl fakeToplevel) uint32 {
	f.mu.Lock()
	id := f.nextObj
	f.nextObj++
	f.toplevels[tl.appID] = id
	f.mu.Unlock()
	f.mu.Lock()
	mgr := f.manager
	f.mu.Unlock()
	f.send(mgr, evManagerToplevel, (&encoder{}).uint(id).b)
	f.send(id, evHandleTitle, (&encoder{}).string(tl.title).b)
	f.send(id, evHandleAppID, (&encoder{}).string(tl.appID).b)
	f.send(id, evHandleState, words(tl.states...))
	f.send(id, evHandleDone, nil)
	return id
}

func (f *fakeCompositor) boundSeat() uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seat
}

func (f *fakeCompositor) handle(appID string) uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.toplevels[appID]
}

// waitRequest waits until the adapter sent opcode to object.
func (f *fakeCompositor) waitRequest(object uint32, opcode uint16) request {
	f.t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		f.mu.Lock()
		for _, r := range f.requests {
			if r.object == object && r.opcode == opcode {
				f.mu.Unlock()
				return r
			}
		}
		f.mu.Unlock()
		select {
		case <-f.changed:
		case <-deadline:
			f.t.Fatalf("no request %d on object %d", opcode, object)
		}
	}
}

type fakeLock struct{ c platform.Capability }

func (l fakeLock) Capability() platform.Capability { return l.c }

var testEnv = map[string]string{"WAYLAND_DISPLAY": "wayland-9", "XDG_RUNTIME_DIR": "/nonexistent"}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWlrListObserveActivateClose(t *testing.T) {
	f, dial := newFakeCompositor(t, wlrGlobals(),
		fakeToplevel{appID: "test.one", title: "One", states: []uint32{stateActivated, stateMaximized}},
		fakeToplevel{appID: "test.two", title: "Two"},
		fakeToplevel{appID: "test.hidden", title: "Hidden", states: []uint32{stateMinimized}},
	)
	ctx := context.Background()
	a := New(ctx, Options{Env: testEnv, Dial: dial, Lock: fakeLock{platform.Capability{Available: true, Backend: "lk"}}})
	defer a.Close()

	if a.Name() != NameWlr || a.Family() != FamilyWlroots || a.DisplaySession() != "wayland" {
		t.Fatalf("name %s family %s", a.Name(), a.Family())
	}
	caps := a.Capabilities()
	for _, k := range []string{platform.CapObserveForeground, platform.CapActivate} {
		if c := caps[k]; !c.Available || c.Backend != BackendWlr {
			t.Fatalf("%s: %+v", k, c)
		}
	}
	if c := caps[platform.CapInput]; c.Available || c.Reason != reasonInput {
		t.Fatalf("input: %+v", c)
	}
	if c := caps[platform.CapPhysicalHome]; c.Available || c.Reason != reasonPhysicalHome {
		t.Fatalf("physical_home: %+v", c)
	}
	if c := caps[platform.CapLockObservation]; !c.Available || c.Backend != "lk" {
		t.Fatalf("lock: %+v", c)
	}

	wins, err := a.ListWindows(ctx)
	if err != nil || len(wins) != 3 {
		t.Fatalf("list: %v %+v", err, wins)
	}
	byApp := map[string]platform.WindowInfo{}
	for _, w := range wins {
		byApp[w.AppID] = w
	}
	if w := byApp["test.one"]; w.Title != "One" || !w.Mapped || w.Class != nil || w.PID != 0 {
		t.Fatalf("one: %+v", w)
	}
	if w := byApp["test.hidden"]; w.Mapped {
		t.Fatalf("a minimized toplevel must be listed unmapped: %+v", w)
	}

	fg, err := a.ObserveForeground(ctx)
	if err != nil || !fg.Known || fg.Window.AppID != "test.one" {
		t.Fatalf("foreground: %v %+v", err, fg)
	}

	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, err := a.WatchForeground(wctx)
	if err != nil {
		t.Fatal(err)
	}
	if first := <-ch; first.Window.AppID != "test.one" {
		t.Fatalf("first watch value: %+v", first)
	}

	two := byApp["test.two"].ID
	if err := a.Activate(ctx, two); err != nil {
		t.Fatal(err)
	}
	r := f.waitRequest(f.handle("test.two"), reqHandleActivate)
	if seat := (&decoder{b: r.body}).uint(); seat == 0 || seat != f.boundSeat() {
		t.Fatalf("activate named seat %d, bound seat %d", seat, f.boundSeat())
	}
	// The compositor answers with the new activated state, both handles in
	// one flush: watchers see one change, never a moment with no foreground.
	one, twoObj := f.handle("test.one"), f.handle("test.two")
	if err := f.w.sendBatch(
		message{one, evHandleState, words(stateMaximized)}, message{one, evHandleDone, nil},
		message{twoObj, evHandleState, words(stateActivated)}, message{twoObj, evHandleDone, nil},
	); err != nil {
		t.Fatal(err)
	}
	select {
	case fg := <-ch:
		if !fg.Known || fg.Window.AppID != "test.two" || fg.Window.ID != two {
			t.Fatalf("watch after activate: %+v", fg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no foreground change after activate")
	}

	if err := a.SetFullscreen(ctx, two, true); err != nil {
		t.Fatal(err)
	}
	if r := f.waitRequest(f.handle("test.two"), reqHandleSetFullscreen); (&decoder{b: r.body}).uint() != 0 {
		t.Fatal("set_fullscreen must pass a null output")
	}
	if err := a.RequestClose(ctx, two); err != nil {
		t.Fatal(err)
	}
	f.waitRequest(f.handle("test.two"), reqHandleClose)

	// The compositor closes it: gone from the list, handle destroyed, and
	// with nothing activated the foreground is unknown.
	f.send(f.handle("test.two"), evHandleClosed, nil)
	f.waitRequest(f.handle("test.two"), reqHandleDestroy)
	waitFor(t, "closed toplevel to leave the list", func() bool {
		ws, _ := a.ListWindows(ctx)
		return len(ws) == 2
	})
	if fg, err := a.ObserveForeground(ctx); err != nil || fg.Known {
		t.Fatalf("foreground after close: %v %+v", err, fg)
	}
	if err := a.Activate(ctx, two); err == nil {
		t.Fatal("activating a closed toplevel succeeded")
	}
	if err := a.DeliverKey(ctx, byApp["test.one"].ID, platform.KeyUp); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("deliver key: %v", err)
	}
}

func TestPendingStateIsInvisibleUntilDone(t *testing.T) {
	f, dial := newFakeCompositor(t, wlrGlobals(), fakeToplevel{appID: "test.one", states: []uint32{stateActivated}})
	ctx := context.Background()
	a := New(ctx, Options{Env: testEnv, Dial: dial})
	defer a.Close()
	id := f.handle("test.one")
	f.send(id, evHandleAppID, (&encoder{}).string("test.renamed").b)
	f.send(id, evHandleState, words())
	// A roundtrip proves both events were read; neither is committed yet.
	if err := a.client.roundtrip(ctx); err != nil {
		t.Fatal(err)
	}
	if fg, _ := a.ObserveForeground(ctx); !fg.Known || fg.Window.AppID != "test.one" {
		t.Fatalf("uncommitted state leaked: %+v", fg)
	}
	f.send(id, evHandleDone, nil)
	if err := a.client.roundtrip(ctx); err != nil {
		t.Fatal(err)
	}
	if fg, _ := a.ObserveForeground(ctx); fg.Known {
		t.Fatalf("committed deactivation not seen: %+v", fg)
	}
	if ws, _ := a.ListWindows(ctx); len(ws) != 1 || ws[0].AppID != "test.renamed" {
		t.Fatalf("committed app_id not seen: %+v", ws)
	}
}

// A focus switch arrives as one flush with two `done`s; the client must
// report it as one change, or the coordinator would see a moment with no
// foreground (and bump the epoch, cancelling holds) on every switch.
func TestOneFlushIsOneChange(t *testing.T) {
	f, dial := newFakeCompositor(t, wlrGlobals(),
		fakeToplevel{appID: "test.one", states: []uint32{stateActivated}},
		fakeToplevel{appID: "test.two"},
	)
	ctx := context.Background()
	conn, _ := dial(ctx, "")
	var changes atomic.Int32
	c := newClient(newWire(conn), func() { changes.Add(1) })
	defer c.shutdown()
	if err := c.start(ctx); err != nil {
		t.Fatal(err)
	}
	before := changes.Load()
	one, two := f.handle("test.one"), f.handle("test.two")
	if err := f.w.sendBatch(
		message{one, evHandleState, words()}, message{one, evHandleDone, nil},
		message{two, evHandleState, words(stateActivated)}, message{two, evHandleDone, nil},
	); err != nil {
		t.Fatal(err)
	}
	if err := c.roundtrip(ctx); err != nil {
		t.Fatal(err)
	}
	if n := changes.Load() - before; n != 1 {
		t.Fatalf("one flush reported as %d changes", n)
	}
	if w, ok := c.active(); !ok || w.AppID != "test.two" {
		t.Fatalf("active after switch: %+v %v", w, ok)
	}
}

func TestTwoActivatedIsUnknown(t *testing.T) {
	_, dial := newFakeCompositor(t, wlrGlobals(),
		fakeToplevel{appID: "test.one", states: []uint32{stateActivated}},
		fakeToplevel{appID: "test.two", states: []uint32{stateActivated}},
	)
	a := New(context.Background(), Options{Env: testEnv, Dial: dial})
	defer a.Close()
	if fg, err := a.ObserveForeground(context.Background()); err != nil || fg.Known {
		t.Fatalf("an ambiguous foreground must be unknown: %v %+v", err, fg)
	}
}

func TestCompositorErrorMakesAdapterUnavailable(t *testing.T) {
	f, dial := newFakeCompositor(t, wlrGlobals(), fakeToplevel{appID: "test.one", states: []uint32{stateActivated}})
	ctx := context.Background()
	a := New(ctx, Options{Env: testEnv, Dial: dial})
	defer a.Close()
	ch, err := a.WatchForeground(ctx)
	if err != nil {
		t.Fatal(err)
	}
	<-ch
	f.send(displayID, evDisplayError, (&encoder{}).uint(7).uint(1).string("boom").b)
	select {
	case _, ok := <-ch:
		if ok {
			// a last value may race the close; the close must follow
			if _, ok := <-ch; ok {
				t.Fatal("watch channel stayed open after a protocol error")
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watch channel stayed open after a protocol error")
	}
	c := a.Capabilities()[platform.CapObserveForeground]
	if c.Available || !strings.Contains(c.Reason, "boom") {
		t.Fatalf("observe after error: %+v", c)
	}
	if _, err := a.ObserveForeground(ctx); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("observe after error: %v", err)
	}
}

func TestFamiliesWithoutWlrAreHonest(t *testing.T) {
	cases := []struct {
		name    string
		globals []global
		desktop string
		family  string
		reason  string
	}{
		{"kde", []global{{ifaceSeat, 9}, {ifaceKDEWindowMgt, 16}}, "KDE", FamilyKDE, reasonKDE},
		{"kde by desktop", []global{{ifaceSeat, 9}}, "KDE", FamilyKDE, reasonKDE},
		{"gnome", []global{{ifaceSeat, 9}, {ifaceGnomeShell, 5}}, "ubuntu:GNOME", FamilyGNOME, reasonGNOME},
		{"ext list only", []global{{ifaceSeat, 9}, {ifaceExtToplevel, 1}}, "", FamilyExtList, reasonExtList},
		{"nothing", []global{{ifaceSeat, 9}}, "", FamilyUnknown, reasonUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, dial := newFakeCompositor(t, tc.globals)
			env := map[string]string{"WAYLAND_DISPLAY": "wayland-9", "XDG_RUNTIME_DIR": "/nonexistent", "XDG_CURRENT_DESKTOP": tc.desktop}
			a := New(context.Background(), Options{Env: env, Dial: dial})
			defer a.Close()
			if a.Name() != Name || a.Family() != tc.family {
				t.Fatalf("name %s family %s", a.Name(), a.Family())
			}
			assertLimited(t, a, tc.reason)
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.manager != 0 {
				t.Fatal("bound a toplevel manager the compositor did not offer")
			}
		})
	}
}

func TestUnreachableDisplay(t *testing.T) {
	a := New(context.Background(), Options{Env: map[string]string{"WAYLAND_DISPLAY": "wayland-0"}})
	if a.Family() != FamilyNoSocket {
		t.Fatal(a.Family())
	}
	assertLimited(t, a, "XDG_RUNTIME_DIR is not set")
	a = New(context.Background(), Options{Env: map[string]string{"WAYLAND_DISPLAY": "/nonexistent/wayland-bdtv-test"}})
	assertLimited(t, a, "cannot connect")
}

func assertLimited(t *testing.T, a *Adapter, reason string) {
	t.Helper()
	caps := a.Capabilities()
	for _, k := range []string{platform.CapObserveForeground, platform.CapActivate, platform.CapInput, platform.CapLockObservation, platform.CapPhysicalHome} {
		c, ok := caps[k]
		if !ok || c.Available || c.Reason == "" {
			t.Fatalf("%s: %+v", k, c)
		}
	}
	if r := caps[platform.CapObserveForeground].Reason; !strings.Contains(r, reason) {
		t.Fatalf("observe reason %q does not say %q", r, reason)
	}
	ctx := context.Background()
	if fg, err := a.ObserveForeground(ctx); fg.Known || !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(fg, err)
	}
	if _, err := a.WatchForeground(ctx); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if _, err := a.ListWindows(ctx); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := a.Activate(ctx, 1); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := a.RequestClose(ctx, 1); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := a.SetFullscreen(ctx, 1, true); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := a.DeliverKey(ctx, 1, platform.KeyUp); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSocketPath(t *testing.T) {
	for _, tc := range []struct {
		env  map[string]string
		want string
		err  bool
	}{
		{map[string]string{"WAYLAND_DISPLAY": "wayland-1", "XDG_RUNTIME_DIR": "/run/user/1"}, "/run/user/1/wayland-1", false},
		{map[string]string{"XDG_RUNTIME_DIR": "/run/user/1"}, "/run/user/1/wayland-0", false},
		{map[string]string{"WAYLAND_DISPLAY": "/tmp/w"}, "/tmp/w", false},
		{map[string]string{"WAYLAND_DISPLAY": "wayland-1"}, "", true},
	} {
		got, err := SocketPath(tc.env)
		if got != tc.want || (err != nil) != tc.err {
			t.Errorf("%v: %q %v", tc.env, got, err)
		}
	}
}

func TestWireEncoding(t *testing.T) {
	b, err := frame(3, 1, (&encoder{}).string("abc").uint(7).string("").b)
	if err != nil {
		t.Fatal(err)
	}
	// header 8 + "abc\0" (4+4) + 7 (4) + "" (4+4: length 1, NUL, padding)
	if len(b) != 8+8+4+8 {
		t.Fatalf("frame length %d", len(b))
	}
	if size := binary.LittleEndian.Uint32(b[4:]) >> 16; size != uint32(len(b)) {
		t.Fatalf("size field %d", size)
	}
	d := &decoder{b: b[8:]}
	if s, v, e := d.string(), d.uint(), d.string(); s != "abc" || v != 7 || e != "" || d.err != nil {
		t.Fatalf("%q %d %q %v", s, v, e, d.err)
	}
	// A string without its NUL, a truncated body, and a lying length fail.
	for name, body := range map[string][]byte{
		"no nul":    append(binary.LittleEndian.AppendUint32(nil, 4), 'a', 'b', 'c', 'd'),
		"truncated": binary.LittleEndian.AppendUint32(nil, 9),
		"short":     {1, 2},
	} {
		d := &decoder{b: body}
		_ = d.string()
		if d.err == nil {
			t.Errorf("%s: decoded without error", name)
		}
	}
	if _, err := frame(1, 0, make([]byte, maxMessage)); err == nil {
		t.Fatal("an oversized message was framed")
	}
}
