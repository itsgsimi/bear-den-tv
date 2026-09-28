// Package applications owns the registry of approved external applications,
// Flatpak discovery/launch, instance tracking, and the per-application adapters
// (key mapping, window matching, verified capabilities). Launch definitions come
// only from validated configuration; nothing here accepts input from phones.
package applications

import (
	"context"

	"bear-den-tv/internal/platform"
)

// Installation is what discovery learned about an application.
type Installation struct {
	Installed bool
	Version   string // empty when unknown
	Scope     string // user | system | none | unknown
}

// Instance is a running application instance as tracked by the launcher.
type Instance struct {
	FlatpakID  string
	InstanceID string // flatpak instance id; empty for non-flatpak launchers
	PID        int    // host pid of the launcher/bwrap process
}

// Launcher discovers, starts, lists, and kills Flatpak applications with a
// fixed argument vector. Implementations: flatpak (exec of the flatpak CLI),
// fake (tests).
type Launcher interface {
	Discover(ctx context.Context, flatpakID string) (Installation, error)
	Launch(ctx context.Context, flatpakID string, args []string) (Instance, error)
	Instances(ctx context.Context) ([]Instance, error)
	// Kill terminates exactly one tracked instance, never by name.
	Kill(ctx context.Context, inst Instance) error
}

// Adapter is the per-application knowledge the router needs.
type Adapter interface {
	// Name is the config adapter id (plex-htpc, vacuumtube, moonlight).
	Name() string
	// FlatpakID is the only app id this adapter may launch.
	FlatpakID() string
	// ApprovedArgs is the closed list of launch arguments the config may use.
	ApprovedArgs() []string
	// MatchWindow reports whether a window belongs to this application.
	MatchWindow(w platform.WindowInfo) bool
	// KeyFor maps an action name to a logical key, or false when unmapped.
	KeyFor(action string) (platform.Key, bool)
	// MediaMatch is the MPRIS match string (desktop entry / bus-name fragment).
	MediaMatch() string
	// PauseVerified reports whether pause-on-home has been verified for this
	// adapter on this installation; false means Home must not claim "Paused".
	PauseVerified() bool
}
