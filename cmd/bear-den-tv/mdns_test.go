// Tests for the remote host's mDNS wiring (session.go mdnsPlan, stopLocked):
// the listener is advertised only as a consented LAN listener with
// remote.mdns on, on its selected interfaces, never the dev loopback
// listener, and the advertisement stops with the listener.

package main

import (
	"reflect"
	"testing"

	"bear-den-tv/internal/remote"
	"bear-den-tv/internal/remote/mdns"
)

func TestMDNSPlanOnlyForTheConsentedLANListener(t *testing.T) {
	lan := listenSpec{Interfaces: []string{"enp1s0"}, Port: 8090, Transport: remote.TransportTrustedLANHTTP, MDNS: true, Name: "Living room"}
	svc, ifaces, ok := mdnsPlan(lan)
	if !ok || !reflect.DeepEqual(ifaces, []string{"enp1s0"}) || !reflect.DeepEqual(svc, mdns.Service{Name: "Living room", Type: mdns.ServiceTypeHTTP, Port: 8090, TXT: []string{"protocol=1"}}) {
		t.Fatalf("lan: %+v %v %v", svc, ifaces, ok)
	}
	https := lan
	https.Transport = remote.TransportHTTPS
	if svc, _, _ := mdnsPlan(https); svc.Type != mdns.ServiceTypeHTTPS {
		t.Fatalf("https advertised as %s", svc.Type)
	}
	for name, s := range map[string]listenSpec{
		"dev loopback": {Dev: "127.0.0.1:8787", Interfaces: []string{"lo"}, Port: 8787, Transport: remote.TransportTrustedLANHTTP, MDNS: true},
		"mdns off":     {Interfaces: []string{"enp1s0"}, Port: 8090, Transport: remote.TransportTrustedLANHTTP},
		"not enabled":  {}, // spec() returns the zero spec until consent and enable
	} {
		if _, _, ok := mdnsPlan(s); ok {
			t.Errorf("%s: advertised", name)
		}
	}
}

type fakeAdvertiser struct{ starts, stops int }

func (f *fakeAdvertiser) Start(mdns.Service, []string) error { f.starts++; return nil }
func (f *fakeAdvertiser) Stop()                              { f.stops++ }

func TestMDNSStopsWithTheListener(t *testing.T) {
	fa := &fakeAdvertiser{}
	h := &remoteHost{mdns: fa}
	h.stop()
	if fa.stops != 1 {
		t.Fatalf("stops %d", fa.stops)
	}
}
