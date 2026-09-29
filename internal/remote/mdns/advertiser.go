// Advertiser: the LAN remote's mDNS advertisement for the coordinator's
// remote host (cmd/bear-den-tv/session.go). Start publishes one Avahi
// service per selected interface, restricted to that interface's index, and
// only when the caller's LAN listener is up; Stop frees every entry group
// and the bus. The caller never starts it in dev (loopback) mode or with
// config remote.mdns off. Best effort: a missing Avahi or D-Bus is an error
// to log, and the remote keeps working without it. Spec: docs/security.md
// (LAN listener), contracts/config.md (remote.mdns).

package mdns

import (
	"errors"
	"fmt"
	"net"
	"sync"
)

// Advertiser publishes and withdraws the remote's advertisement.
type Advertiser struct {
	// NewBus connects to the system D-Bus (NewSystemBus when nil).
	NewBus func() (Bus, error)
	// Index maps an interface name to its index (net.InterfaceByName when nil).
	Index func(name string) (int, error)

	mu   sync.Mutex
	bus  Bus
	pubs []*Publication
}

// Start withdraws any earlier advertisement, then publishes svc once per
// interface in ifaces (svc.InterfaceIndex is set per interface; an empty
// list publishes nothing). Interfaces that fail are skipped and reported in
// the error; the others stay published.
func (a *Advertiser) Start(svc Service, ifaces []string) error {
	a.Stop()
	if len(ifaces) == 0 {
		return nil
	}
	newBus, index := a.NewBus, a.Index
	if newBus == nil {
		newBus = NewSystemBus
	}
	if index == nil {
		index = func(name string) (int, error) {
			i, err := net.InterfaceByName(name)
			if err != nil {
				return 0, err
			}
			return i.Index, nil
		}
	}
	bus, err := newBus()
	if err != nil {
		return err
	}
	var errs error
	var pubs []*Publication
	for _, name := range ifaces {
		idx, err := index(name)
		if err != nil || idx <= 0 {
			// Never fall back to "every interface" (index 0).
			errs = errors.Join(errs, fmt.Errorf("mdns: interface %s: %v", name, err))
			continue
		}
		s := svc
		s.InterfaceIndex = idx
		pub, err := Publish(bus, s)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("mdns: interface %s: %w", name, err))
			continue
		}
		pubs = append(pubs, pub)
	}
	if len(pubs) == 0 {
		_ = bus.Close()
		return errs
	}
	a.mu.Lock()
	a.bus, a.pubs = bus, pubs
	a.mu.Unlock()
	return errs
}

// Stop withdraws the advertisement and closes the bus; safe to call again.
func (a *Advertiser) Stop() {
	a.mu.Lock()
	bus, pubs := a.bus, a.pubs
	a.bus, a.pubs = nil, nil
	a.mu.Unlock()
	for _, p := range pubs {
		_ = p.Close()
	}
	if bus != nil {
		_ = bus.Close()
	}
}

// Active reports how many entry groups are published.
func (a *Advertiser) Active() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pubs)
}
