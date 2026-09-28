// Package dbusx is the narrow D-Bus surface the platform adapters use (lock
// observation on the session and system buses, MPRIS on the session bus). It
// exists so adapters can be unit-tested with an injected Bus and so no adapter
// ever autolaunches a bus daemon: connections come only from the addresses the
// session already exports.
package dbusx

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
)

// ErrUnavailable wraps every failure to reach a bus.
var ErrUnavailable = errors.New("dbusx: bus unavailable")

// Signal is one received D-Bus signal.
type Signal struct {
	Sender string
	Path   string
	Name   string // interface.member
	Body   []any
}

// Bus is the subset of a D-Bus connection the adapters depend on. Every method
// is safe for concurrent use.
type Bus interface {
	// Call invokes method ("interface.member") on dest/path and returns the
	// reply body. Errors from the peer are returned as *dbus.Error values.
	Call(ctx context.Context, dest, path, method string, args ...any) ([]any, error)
	// Property reads iface.name through org.freedesktop.DBus.Properties.Get and
	// returns the unwrapped variant value.
	Property(ctx context.Context, dest, path, iface, name string) (any, error)
	// Names lists every name currently owned on the bus (unique and well-known).
	Names(ctx context.Context) ([]string, error)
	// Subscribe delivers signals whose interface and member match until ctx is
	// done; the channel is closed on exit. Empty member matches every member.
	Subscribe(ctx context.Context, iface, member string) (<-chan Signal, error)
	// Close releases the connection.
	Close() error
}

// Conn adapts a godbus connection to Bus.
type Conn struct {
	conn *dbus.Conn

	mu   sync.Mutex
	subs map[chan *dbus.Signal]struct{}
}

// ConnectSession opens a private connection to the session bus named by
// DBUS_SESSION_BUS_ADDRESS (or the per-user runtime bus) without autolaunching
// a daemon. The result wraps ErrUnavailable when no bus can be reached.
func ConnectSession(ctx context.Context) (*Conn, error) {
	return connect(ctx, dbus.SessionBusPrivateNoAutoStartup)
}

// ConnectSystem opens a private connection to the system bus.
func ConnectSystem(ctx context.Context) (*Conn, error) {
	return connect(ctx, dbus.SystemBusPrivate)
}

func connect(ctx context.Context, dial func(opts ...dbus.ConnOption) (*dbus.Conn, error)) (*Conn, error) {
	c, err := dial(dbus.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if err := c.Auth(nil); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("%w: auth: %v", ErrUnavailable, err)
	}
	if err := c.Hello(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("%w: hello: %v", ErrUnavailable, err)
	}
	return &Conn{conn: c, subs: map[chan *dbus.Signal]struct{}{}}, nil
}

// Call implements Bus.
func (c *Conn) Call(ctx context.Context, dest, path, method string, args ...any) ([]any, error) {
	call := c.conn.Object(dest, dbus.ObjectPath(path)).CallWithContext(ctx, method, 0, args...)
	if call.Err != nil {
		return nil, call.Err
	}
	return call.Body, nil
}

// Property implements Bus.
func (c *Conn) Property(ctx context.Context, dest, path, iface, name string) (any, error) {
	body, err := c.Call(ctx, dest, path, "org.freedesktop.DBus.Properties.Get", iface, name)
	if err != nil {
		return nil, err
	}
	if len(body) != 1 {
		return nil, fmt.Errorf("dbusx: Properties.Get %s.%s returned %d values", iface, name, len(body))
	}
	if v, ok := body[0].(dbus.Variant); ok {
		return v.Value(), nil
	}
	return body[0], nil
}

// Names implements Bus.
func (c *Conn) Names(ctx context.Context) ([]string, error) {
	var names []string
	err := c.conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.ListNames", 0).Store(&names)
	return names, err
}

// Subscribe implements Bus.
func (c *Conn) Subscribe(ctx context.Context, iface, member string) (<-chan Signal, error) {
	opts := []dbus.MatchOption{dbus.WithMatchInterface(iface)}
	if member != "" {
		opts = append(opts, dbus.WithMatchMember(member))
	}
	if err := c.conn.AddMatchSignalContext(ctx, opts...); err != nil {
		return nil, err
	}
	raw := make(chan *dbus.Signal, 16)
	c.conn.Signal(raw)
	c.mu.Lock()
	c.subs[raw] = struct{}{}
	c.mu.Unlock()
	out := make(chan Signal, 16)
	go func() {
		defer close(out)
		defer func() {
			c.conn.RemoveSignal(raw)
			c.mu.Lock()
			delete(c.subs, raw)
			c.mu.Unlock()
			_ = c.conn.RemoveMatchSignal(opts...)
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case s, ok := <-raw:
				if !ok {
					return
				}
				if !matches(s, iface, member) {
					continue
				}
				select {
				case out <- Signal{Sender: s.Sender, Path: string(s.Path), Name: s.Name, Body: s.Body}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// matches filters a shared signal channel down to one subscription, since
// godbus fans every matched signal out to every registered channel.
func matches(s *dbus.Signal, iface, member string) bool {
	want := iface + "."
	if member != "" {
		want += member
		return s.Name == want
	}
	return len(s.Name) > len(want) && s.Name[:len(want)] == want
}

// Close implements Bus.
func (c *Conn) Close() error {
	return c.conn.Close()
}

// IsNoOwner reports whether err says the destination name has no owner, which
// is how an absent screensaver or player shows up.
func IsNoOwner(err error) bool {
	var derr dbus.Error
	if errors.As(err, &derr) {
		return derr.Name == "org.freedesktop.DBus.Error.ServiceUnknown" || derr.Name == "org.freedesktop.DBus.Error.NameHasNoOwner"
	}
	var perr *dbus.Error
	if errors.As(err, &perr) {
		return perr.Name == "org.freedesktop.DBus.Error.ServiceUnknown" || perr.Name == "org.freedesktop.DBus.Error.NameHasNoOwner"
	}
	return false
}
