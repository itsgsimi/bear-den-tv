// Tests for Avahi publishing with a fake bus (mdns.go).

package mdns

import (
	"errors"
	"testing"

	"github.com/godbus/dbus/v5"
)

type call struct {
	dest   string
	path   dbus.ObjectPath
	method string
	args   []any
}

type fakeBus struct {
	calls   []call
	failOn  string
	closed  bool
	newBody []any
}

func (b *fakeBus) Call(dest string, path dbus.ObjectPath, method string, args ...any) ([]any, error) {
	b.calls = append(b.calls, call{dest, path, method, args})
	if b.failOn != "" && method == b.failOn {
		return nil, errors.New("avahi says no")
	}
	if method == avahiServerIface+".EntryGroupNew" {
		if b.newBody != nil {
			return b.newBody, nil
		}
		return []any{dbus.ObjectPath("/Client1/EntryGroup1")}, nil
	}
	return nil, nil
}

func (b *fakeBus) Close() error { b.closed = true; return nil }

func TestPublishCommitsAndFrees(t *testing.T) {
	bus := &fakeBus{}
	pub, err := Publish(bus, Service{Type: ServiceTypeHTTP, Port: 8090, InterfaceIndex: 3, TXT: []string{"protocol=1"}})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(bus.calls) != 3 {
		t.Fatalf("expected 3 calls, got %d: %+v", len(bus.calls), bus.calls)
	}
	if bus.calls[0].method != avahiServerIface+".EntryGroupNew" || bus.calls[0].path != avahiServerPath {
		t.Fatalf("first call: %+v", bus.calls[0])
	}
	add := bus.calls[1]
	if add.method != avahiGroupIface+".AddService" || add.path != "/Client1/EntryGroup1" {
		t.Fatalf("add call: %+v", add)
	}
	if add.args[0] != int32(3) || add.args[1] != avahiProtoUnspec || add.args[3] != "Bear Den TV" || add.args[4] != ServiceTypeHTTP || add.args[7] != uint16(8090) {
		t.Fatalf("AddService args: %+v", add.args)
	}
	if txt := add.args[8].([][]byte); len(txt) != 1 || string(txt[0]) != "protocol=1" {
		t.Fatalf("txt: %v", txt)
	}
	if bus.calls[2].method != avahiGroupIface+".Commit" {
		t.Fatalf("commit call: %+v", bus.calls[2])
	}
	if err := pub.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if last := bus.calls[len(bus.calls)-1]; last.method != avahiGroupIface+".Free" || last.path != "/Client1/EntryGroup1" {
		t.Fatalf("free call: %+v", last)
	}
	if err := pub.Close(); err != nil || len(bus.calls) != 4 {
		t.Fatalf("second Close must be a no-op: err=%v calls=%d", err, len(bus.calls))
	}
}

func TestPublishNilBusIsNoop(t *testing.T) {
	pub, err := Publish(nil, Service{Type: ServiceTypeHTTPS, Port: 8090})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if pub == nil || pub.Close() != nil {
		t.Fatal("no-op publication must be closable")
	}
}

func TestPublishAddServiceFailureFreesGroup(t *testing.T) {
	bus := &fakeBus{failOn: avahiGroupIface + ".AddService"}
	pub, err := Publish(bus, Service{Type: ServiceTypeHTTP, Port: 8090})
	if err == nil {
		t.Fatal("expected error")
	}
	if last := bus.calls[len(bus.calls)-1]; last.method != avahiGroupIface+".Free" {
		t.Fatalf("group not freed after failure: %+v", bus.calls)
	}
	if pub.Close() != nil {
		t.Fatal("publication after failure must close cleanly")
	}
}

func TestPublishRejectsBadInput(t *testing.T) {
	bus := &fakeBus{}
	if _, err := Publish(bus, Service{Type: "_ssh._tcp", Port: 22}); err == nil {
		t.Fatal("unknown type accepted")
	}
	if _, err := Publish(bus, Service{Type: ServiceTypeHTTP}); err == nil {
		t.Fatal("zero port accepted")
	}
	if len(bus.calls) != 0 {
		t.Fatalf("bus must not be touched on invalid input: %+v", bus.calls)
	}
	bus.newBody = []any{"not-a-path"}
	if _, err := Publish(bus, Service{Type: ServiceTypeHTTP, Port: 1}); err == nil {
		t.Fatal("bad EntryGroupNew reply accepted")
	}
}
