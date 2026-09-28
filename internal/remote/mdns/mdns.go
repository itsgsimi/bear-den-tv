// Package mdns advertises the LAN remote through Avahi over the system D-Bus
// (org.freedesktop.Avahi.Server → EntryGroup AddService/Commit). Publication
// is best effort: every failure is an error the caller logs, never a fatal
// condition, and a nil Bus yields a no-op Publication.
package mdns

import (
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"
)

// Avahi D-Bus names and the "unspecified" interface/protocol sentinels.
const (
	avahiDest         = "org.freedesktop.Avahi"
	avahiServerPath   = dbus.ObjectPath("/")
	avahiServerIface  = "org.freedesktop.Avahi.Server"
	avahiGroupIface   = "org.freedesktop.Avahi.EntryGroup"
	avahiIfaceUnspec  = int32(-1)
	avahiProtoUnspec  = int32(-1)
	avahiDefaultName  = "Bear Den TV"
	ServiceTypeHTTP   = "_http._tcp"
	ServiceTypeHTTPS  = "_https._tcp"
	avahiPublishFlags = uint32(0)
)

// ErrUnavailable reports that no D-Bus/Avahi connection exists; the
// Publication returned alongside it is a safe no-op.
var ErrUnavailable = errors.New("mdns: avahi is unavailable")

// Bus is the D-Bus method-call seam; the real implementation wraps
// *dbus.Conn and tests inject a recorder.
type Bus interface {
	// Call invokes method (interface-qualified, e.g. "org.freedesktop.Avahi.Server.EntryGroupNew")
	// on dest/path and returns the reply body.
	Call(dest string, path dbus.ObjectPath, method string, args ...any) ([]any, error)
	// Close releases the connection.
	Close() error
}

// Service describes one advertisement.
type Service struct {
	// Name is the instance name shown to browsers; empty selects "Bear Den TV".
	Name string
	// Type is ServiceTypeHTTP or ServiceTypeHTTPS.
	Type string
	// Port is the TCP port.
	Port uint16
	// InterfaceIndex restricts the advertisement to one interface (net.Interface.Index);
	// zero advertises on every interface Avahi manages.
	InterfaceIndex int
	// TXT records, "key=value" each.
	TXT []string
}

// Publication is a committed Avahi entry group; Close frees it.
type Publication struct {
	bus  Bus
	path dbus.ObjectPath
}

// NewSystemBus connects to the system D-Bus; the caller logs the error and
// continues without mDNS when it fails.
func NewSystemBus() (Bus, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return &dbusBus{conn: conn}, nil
}

type dbusBus struct{ conn *dbus.Conn }

func (b *dbusBus) Call(dest string, path dbus.ObjectPath, method string, args ...any) ([]any, error) {
	call := b.conn.Object(dest, path).Call(method, 0, args...)
	if call.Err != nil {
		return nil, call.Err
	}
	return call.Body, nil
}

func (b *dbusBus) Close() error { return b.conn.Close() }

// Publish registers svc through bus and commits it. With a nil bus it
// returns a no-op Publication and ErrUnavailable. On any Avahi error the
// half-built entry group is freed and the error returned; the Publication is
// still safe to Close.
func Publish(bus Bus, svc Service) (*Publication, error) {
	if bus == nil {
		return &Publication{}, ErrUnavailable
	}
	if svc.Type != ServiceTypeHTTP && svc.Type != ServiceTypeHTTPS {
		return &Publication{}, fmt.Errorf("mdns: service type must be %s or %s, got %q", ServiceTypeHTTP, ServiceTypeHTTPS, svc.Type)
	}
	if svc.Port == 0 {
		return &Publication{}, errors.New("mdns: port must be non-zero")
	}
	name := svc.Name
	if name == "" {
		name = avahiDefaultName
	}
	iface := avahiIfaceUnspec
	if svc.InterfaceIndex > 0 {
		iface = int32(svc.InterfaceIndex)
	}
	body, err := bus.Call(avahiDest, avahiServerPath, avahiServerIface+".EntryGroupNew")
	if err != nil {
		return &Publication{}, fmt.Errorf("mdns: EntryGroupNew: %w", err)
	}
	if len(body) != 1 {
		return &Publication{}, fmt.Errorf("mdns: EntryGroupNew returned %d values", len(body))
	}
	path, ok := body[0].(dbus.ObjectPath)
	if !ok {
		return &Publication{}, fmt.Errorf("mdns: EntryGroupNew returned %T, want object path", body[0])
	}
	pub := &Publication{bus: bus, path: path}
	txt := make([][]byte, 0, len(svc.TXT))
	for _, t := range svc.TXT {
		txt = append(txt, []byte(t))
	}
	if _, err := bus.Call(avahiDest, path, avahiGroupIface+".AddService", iface, avahiProtoUnspec, avahiPublishFlags, name, svc.Type, "", "", svc.Port, txt); err != nil {
		_ = pub.Close()
		return &Publication{}, fmt.Errorf("mdns: AddService: %w", err)
	}
	if _, err := bus.Call(avahiDest, path, avahiGroupIface+".Commit"); err != nil {
		_ = pub.Close()
		return &Publication{}, fmt.Errorf("mdns: Commit: %w", err)
	}
	return pub, nil
}

// Close frees the entry group; safe on a no-op Publication and idempotent.
func (p *Publication) Close() error {
	if p == nil || p.bus == nil || p.path == "" {
		return nil
	}
	_, err := p.bus.Call(avahiDest, p.path, avahiGroupIface+".Free")
	p.path = ""
	if err != nil {
		return fmt.Errorf("mdns: Free: %w", err)
	}
	return nil
}
