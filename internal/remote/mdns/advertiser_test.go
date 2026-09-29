// Tests for the Advertiser (advertiser.go) with the fake bus of
// mdns_test.go: one entry group per selected interface, restricted to that
// interface's index; an interface that cannot be resolved is skipped, never
// widened to every interface; Stop frees every group and the bus.

package mdns

import (
	"errors"
	"testing"
)

func indexOf(m map[string]int) func(string) (int, error) {
	return func(name string) (int, error) {
		if i, ok := m[name]; ok {
			return i, nil
		}
		return 0, errors.New("no such interface")
	}
}

func TestAdvertiserPublishesPerInterfaceAndStops(t *testing.T) {
	bus := &fakeBus{}
	a := &Advertiser{NewBus: func() (Bus, error) { return bus, nil }, Index: indexOf(map[string]int{"enp1s0": 2, "wlp2s0": 5})}
	if err := a.Start(Service{Type: ServiceTypeHTTP, Port: 8090}, []string{"enp1s0", "wlp2s0"}); err != nil {
		t.Fatal(err)
	}
	if a.Active() != 2 {
		t.Fatalf("active %d", a.Active())
	}
	var ifaces []any
	for _, c := range bus.calls {
		if c.method == avahiGroupIface+".AddService" {
			ifaces = append(ifaces, c.args[0])
		}
	}
	if len(ifaces) != 2 || ifaces[0] != int32(2) || ifaces[1] != int32(5) {
		t.Fatalf("published on %v", ifaces)
	}
	bus.calls = nil
	a.Stop()
	frees := 0
	for _, c := range bus.calls {
		if c.method == avahiGroupIface+".Free" {
			frees++
		}
	}
	if frees != 2 || !bus.closed || a.Active() != 0 {
		t.Fatalf("stop: %d frees, closed %v, active %d", frees, bus.closed, a.Active())
	}
	a.Stop() // idempotent
}

func TestAdvertiserNeverWidensAndFailsSoftly(t *testing.T) {
	// An interface that cannot be resolved is skipped; the rest publish.
	bus := &fakeBus{}
	a := &Advertiser{NewBus: func() (Bus, error) { return bus, nil }, Index: indexOf(map[string]int{"enp1s0": 2})}
	if err := a.Start(Service{Type: ServiceTypeHTTP, Port: 8090}, []string{"gone0", "enp1s0"}); err == nil {
		t.Fatal("the missing interface was not reported")
	}
	for _, c := range bus.calls {
		if c.method == avahiGroupIface+".AddService" && c.args[0] != int32(2) {
			t.Fatalf("published on interface %v", c.args[0])
		}
	}
	if a.Active() != 1 {
		t.Fatalf("active %d", a.Active())
	}
	a.Stop()

	// Nothing resolvable: nothing published, the bus closed.
	bus2 := &fakeBus{}
	a2 := &Advertiser{NewBus: func() (Bus, error) { return bus2, nil }, Index: indexOf(nil)}
	if err := a2.Start(Service{Type: ServiceTypeHTTP, Port: 8090}, []string{"gone0"}); err == nil || a2.Active() != 0 || !bus2.closed {
		t.Fatalf("err %v active %d closed %v", err, a2.Active(), bus2.closed)
	}
	for _, c := range bus2.calls {
		if c.method == avahiServerIface+".EntryGroupNew" {
			t.Fatal("an entry group was made for no interface")
		}
	}

	// No Avahi: an error to log, nothing active.
	a3 := &Advertiser{NewBus: func() (Bus, error) { return nil, ErrUnavailable }, Index: indexOf(map[string]int{"enp1s0": 2})}
	if err := a3.Start(Service{Type: ServiceTypeHTTP, Port: 8090}, []string{"enp1s0"}); !errors.Is(err, ErrUnavailable) || a3.Active() != 0 {
		t.Fatalf("err %v active %d", err, a3.Active())
	}
	// No interfaces: no bus at all.
	a4 := &Advertiser{NewBus: func() (Bus, error) { t.Fatal("bus opened for no interfaces"); return nil, nil }}
	if err := a4.Start(Service{Type: ServiceTypeHTTP, Port: 8090}, nil); err != nil {
		t.Fatal(err)
	}
}
