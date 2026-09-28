// Tests for lock-state verdicts and fail-closed behaviour (lock.go).

package lock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/dbusx"
)

type fixture struct {
	screensaver    *bool
	screensaverErr error
	lockedHint     bool
	active         bool
	sessionDisplay string
	sessions       [][]any
	displays       map[string]string
}

func (f *fixture) buses(t *testing.T) (*dbusx.Fake, *dbusx.Fake) {
	t.Helper()
	session := &dbusx.Fake{
		CallFn: func(ctx context.Context, dest, path, method string, args ...any) ([]any, error) {
			if dest != screensaverDest || method != screensaverIface+".GetActive" {
				t.Fatalf("unexpected session call %s %s", dest, method)
			}
			if f.screensaverErr != nil {
				return nil, f.screensaverErr
			}
			return []any{*f.screensaver}, nil
		},
	}
	system := &dbusx.Fake{
		CallFn: func(ctx context.Context, dest, path, method string, args ...any) ([]any, error) {
			switch method {
			case logindManagerIface + ".GetSession":
				return []any{dbus.ObjectPath("/org/freedesktop/login1/session/" + args[0].(string))}, nil
			case logindManagerIface + ".GetSessionByPID":
				return []any{dbus.ObjectPath("/org/freedesktop/login1/session/bypid")}, nil
			case logindManagerIface + ".ListSessions":
				return []any{f.sessions}, nil
			}
			return nil, errors.New("unexpected system call " + method)
		},
		PropertyFn: func(ctx context.Context, dest, path, iface, name string) (any, error) {
			switch name {
			case "LockedHint":
				return f.lockedHint, nil
			case "Active":
				return f.active, nil
			case "Id":
				return "bypid", nil
			case "Display":
				if f.displays != nil {
					return f.displays[path], nil
				}
				return f.sessionDisplay, nil
			}
			return nil, errors.New("unexpected property " + name)
		},
	}
	return session, system
}

func newObserver(t *testing.T, f *fixture, mod func(*Options)) (*Observer, *dbusx.Fake, *dbusx.Fake) {
	t.Helper()
	session, system := f.buses(t)
	opts := Options{
		ConnectSession: func(context.Context) (dbusx.Bus, error) { return session, nil },
		ConnectSystem:  func(context.Context) (dbusx.Bus, error) { return system, nil },
		SessionID:      "c1",
		Getenv:         func(string) string { return "" },
	}
	if mod != nil {
		mod(&opts)
	}
	return New(context.Background(), opts), session, system
}

func boolp(b bool) *bool { return &b }

func TestLockedVerdicts(t *testing.T) {
	cases := []struct {
		name string
		f    fixture
		want bool
	}{
		{"all unlocked", fixture{screensaver: boolp(false), active: true}, false},
		{"screensaver active", fixture{screensaver: boolp(true), active: true}, true},
		{"locked hint", fixture{screensaver: boolp(false), lockedHint: true, active: true}, true},
		{"session inactive (seat on greeter)", fixture{screensaver: boolp(false), active: false}, true},
		{"no screensaver, logind unlocked", fixture{screensaverErr: dbus.Error{Name: "org.freedesktop.DBus.Error.ServiceUnknown"}, active: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o, _, _ := newObserver(t, &tc.f, nil)
			locked, err := o.Locked(context.Background())
			if err != nil {
				t.Fatalf("Locked: %v", err)
			}
			if locked != tc.want {
				t.Fatalf("locked=%v want %v (state %+v)", locked, tc.want, o.State(context.Background()))
			}
			if c := o.Capability(); !c.Available || c.Backend != Backend {
				t.Fatalf("capability %+v", c)
			}
		})
	}
}

func TestNoSourceFailsClosed(t *testing.T) {
	dialErr := errors.New("dial tcp: no bus")
	o := New(context.Background(), Options{
		ConnectSession: func(context.Context) (dbusx.Bus, error) { return nil, dialErr },
		ConnectSystem:  func(context.Context) (dbusx.Bus, error) { return nil, dialErr },
	})
	locked, err := o.Locked(context.Background())
	if locked || !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("Locked=%v err=%v; want false, ErrUnsupported", locked, err)
	}
	if c := o.Capability(); c.Available || c.Reason == "" {
		t.Fatalf("capability %+v", c)
	}
	if _, err := o.Watch(context.Background()); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("Watch err=%v", err)
	}
}

func TestScreensaverErrorAndLogindUnresolvedIsUnknown(t *testing.T) {
	f := &fixture{screensaverErr: errors.New("timeout")}
	o, _, _ := newObserver(t, f, func(o *Options) {
		o.ConnectSystem = func(context.Context) (dbusx.Bus, error) { return nil, errors.New("no system bus") }
	})
	if _, err := o.Locked(context.Background()); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("err=%v", err)
	}
	st := o.State(context.Background())
	if st.Known || st.ScreensaverError != "timeout" || st.LogindError != "no system bus" {
		t.Fatalf("state %+v", st)
	}
}

func TestSessionResolvedByPIDThenDisplay(t *testing.T) {
	f := &fixture{
		screensaver: boolp(false), active: true,
		sessions: [][]any{
			{"5", uint32(1000), "alice", "", dbus.ObjectPath("/org/freedesktop/login1/session/_35")},
			{"c1", uint32(1000), "alice", "seat0", dbus.ObjectPath("/org/freedesktop/login1/session/c1")},
		},
		displays: map[string]string{
			"/org/freedesktop/login1/session/bypid": "",
			"/org/freedesktop/login1/session/_35":   "",
			"/org/freedesktop/login1/session/c1":    ":0",
		},
	}
	o, _, _ := newObserver(t, f, func(o *Options) {
		o.SessionID = ""
		o.PID = 4242
		o.Display = ":0"
	})
	if o.sessionID != "c1" || o.sessionPath != "/org/freedesktop/login1/session/c1" {
		t.Fatalf("resolved %q %q err=%v", o.sessionID, o.sessionPath, o.resolveErr)
	}
	// Without a display hint the pid session is used as-is.
	o2, _, _ := newObserver(t, f, func(o *Options) { o.SessionID = ""; o.PID = 4242 })
	if o2.sessionID != "bypid" {
		t.Fatalf("resolved %q", o2.sessionID)
	}
}

func TestWatchEmitsInitialAndChanges(t *testing.T) {
	f := &fixture{screensaver: boolp(false), active: true}
	o, session, system := newObserver(t, f, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := o.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recv := func(want bool) {
		t.Helper()
		select {
		case got, ok := <-ch:
			if !ok || got != want {
				t.Fatalf("got %v ok=%v want %v", got, ok, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("no emission (want %v)", want)
		}
	}
	recv(false)
	*f.screensaver = true
	session.Emit(dbusx.Signal{Name: screensaverIface + ".ActiveChanged", Body: []any{true}})
	recv(true)
	// Unrelated PropertiesChanged on another path does not re-emit; the same
	// path does once the verdict changes.
	*f.screensaver = false
	system.Emit(dbusx.Signal{Name: propertiesIface + ".PropertiesChanged", Path: "/elsewhere"})
	select {
	case v := <-ch:
		t.Fatalf("unexpected emission %v", v)
	case <-time.After(50 * time.Millisecond):
	}
	system.Emit(dbusx.Signal{Name: propertiesIface + ".PropertiesChanged", Path: o.sessionPath})
	recv(false)
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel not closed after cancel")
	}
}
