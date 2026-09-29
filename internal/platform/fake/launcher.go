// Launcher: an in-memory applications.Launcher paired with the fake Desktop.

package fake

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/platform"
)

// Launcher is a development applications.Launcher: "launching" maps a window
// on the fake desktop whose WM_CLASS is derived from the Flatpak id, then
// activates it after Delay, like a real client that takes a moment to start.
type Launcher struct {
	Desk  *Desktop
	Delay time.Duration

	mu        sync.Mutex
	instances map[string]applications.Instance
	windows   map[string]platform.WindowID
	seq       int
	missing   map[string]bool // --dev-installs (installer.go)
}

// NewLauncher returns a launcher that maps windows on desk.
func NewLauncher(desk *Desktop) *Launcher {
	return &Launcher{Desk: desk, Delay: 400 * time.Millisecond, instances: map[string]applications.Instance{}, windows: map[string]platform.WindowID{}}
}

// ClassFor is the fake WM_CLASS for a Flatpak id ("tv.plex.PlexHTPC" → "plexhtpc").
func ClassFor(flatpakID string) string {
	parts := strings.Split(flatpakID, ".")
	return strings.ToLower(parts[len(parts)-1])
}

// Discover implements applications.Launcher; every app is "installed"
// unless SetMissing named it (installer.go).
func (l *Launcher) Discover(_ context.Context, id string) (applications.Installation, error) {
	if l.isMissing(id) {
		return applications.Installation{Scope: "none"}, nil
	}
	return applications.Installation{Installed: true, Version: "dev", Scope: "user"}, nil
}

// Launch implements applications.Launcher.
func (l *Launcher) Launch(_ context.Context, flatpakID string, _ []string) (applications.Instance, error) {
	l.mu.Lock()
	l.seq++
	inst := applications.Instance{FlatpakID: flatpakID, InstanceID: strconv.Itoa(l.seq), PID: 20000 + l.seq}
	l.instances[inst.InstanceID] = inst
	l.mu.Unlock()
	class := ClassFor(flatpakID)
	w := l.Desk.AddWindow(platform.WindowInfo{PID: inst.PID, Class: []string{class, class}, Title: flatpakID})
	l.mu.Lock()
	l.windows[inst.InstanceID] = w
	l.mu.Unlock()
	time.AfterFunc(l.Delay, func() { l.Desk.SetActive(w) })
	return inst, nil
}

// Instances implements applications.Launcher; an instance exits when its
// window is closed, like a real single-window client.
func (l *Launcher) Instances(context.Context) ([]applications.Instance, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]applications.Instance, 0, len(l.instances))
	for id, i := range l.instances {
		if !l.Desk.HasWindow(l.windows[id]) {
			delete(l.instances, id)
			delete(l.windows, id)
			continue
		}
		out = append(out, i)
	}
	return out, nil
}

// Kill implements applications.Launcher.
func (l *Launcher) Kill(_ context.Context, inst applications.Instance) error {
	l.mu.Lock()
	w, ok := l.windows[inst.InstanceID]
	delete(l.instances, inst.InstanceID)
	delete(l.windows, inst.InstanceID)
	l.mu.Unlock()
	if ok {
		l.Desk.RemoveWindow(w)
	}
	return nil
}

var _ applications.Launcher = (*Launcher)(nil)
