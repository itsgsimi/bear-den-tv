// Package wayland is the honest reduced-capability desktop adapter for Wayland
// sessions. Nothing is implemented: activation needs an xdg-activation token
// or a compositor bridge, input needs a consented RemoteDesktop portal session,
// and active-window observation has no portable protocol. Every capability is
// reported unavailable with that reason so the coordinator refuses input
// instead of guessing (spec §6.3).
package wayland

import (
	"context"
	"fmt"

	"bear-den-tv/internal/platform"
)

// Name is the adapter's backend name.
const Name = "wayland-limited"

// Adapter implements platform.DesktopAdapter with no working capability.
type Adapter struct{}

// New returns the limited adapter.
func New() *Adapter { return &Adapter{} }

// Name implements platform.DesktopAdapter.
func (*Adapter) Name() string { return Name }

// DisplaySession implements platform.DesktopAdapter.
func (*Adapter) DisplaySession() string { return "wayland" }

// Capabilities implements platform.DesktopAdapter with precise reasons.
func (*Adapter) Capabilities() map[string]platform.Capability {
	return map[string]platform.Capability{
		platform.CapObserveForeground: {Backend: Name, Reason: "active-window observation on Wayland needs a compositor-specific bridge; not implemented"},
		platform.CapActivate:          {Backend: Name, Reason: "activation on Wayland needs an xdg-activation token or compositor bridge; not implemented"},
		platform.CapInput:             {Backend: Name, Reason: "input on Wayland needs a consented RemoteDesktop portal session; not implemented"},
		platform.CapLockObservation:   {Backend: Name, Reason: "lock observation is not wired for the Wayland profile"},
		platform.CapPhysicalHome:      {Backend: Name, Reason: "physical Home on Wayland needs the GlobalShortcuts portal; not implemented"},
	}
}

func unsupported(op string) error {
	return fmt.Errorf("%w: %s on wayland-limited", platform.ErrUnsupported, op)
}

// ObserveForeground implements platform.DesktopAdapter: always unknown.
func (*Adapter) ObserveForeground(context.Context) (platform.Foreground, error) {
	return platform.Foreground{}, unsupported("observe foreground")
}

// WatchForeground implements platform.DesktopAdapter.
func (*Adapter) WatchForeground(context.Context) (<-chan platform.Foreground, error) {
	return nil, unsupported("watch foreground")
}

// ListWindows implements platform.DesktopAdapter.
func (*Adapter) ListWindows(context.Context) ([]platform.WindowInfo, error) {
	return nil, unsupported("list windows")
}

// Activate implements platform.DesktopAdapter.
func (*Adapter) Activate(context.Context, platform.WindowID) error {
	return unsupported("activate")
}

// RequestClose implements platform.DesktopAdapter.
func (*Adapter) RequestClose(context.Context, platform.WindowID) error {
	return unsupported("request close")
}

// DeliverKey implements platform.DesktopAdapter: input is never injected.
func (*Adapter) DeliverKey(context.Context, platform.WindowID, platform.Key) error {
	return unsupported("deliver key")
}

// Close implements platform.DesktopAdapter.
func (*Adapter) Close() error { return nil }
