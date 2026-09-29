// Installer: a pretend Flathub for `bear-den-tv dev --dev-installs`. Some
// apps start missing on the fake launcher; an install walks through
// preparing, downloading (runtime, then app) and installing on a timer,
// with DEMO sizes, then marks the app installed. Nothing touches the
// network or flatpak. The real one is internal/applications/install.

package fake

import (
	"context"
	"sync"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/install"
	"bear-den-tv/internal/contract"
)

// DemoMissing are the Flatpak ids --dev-installs reports as not installed.
var DemoMissing = []string{"com.moonlight_stream.Moonlight", "org.libretro.RetroArch", "org.jellyfin.JellyfinDesktop", "org.chromium.Chromium", "com.brave.Browser"}

// SetMissing makes Discover report ids as not installed until SetInstalled.
func (l *Launcher) SetMissing(ids ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.missing == nil {
		l.missing = map[string]bool{}
	}
	for _, id := range ids {
		l.missing[id] = true
	}
}

// SetInstalled makes Discover report id as installed for the user.
func (l *Launcher) SetInstalled(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.missing, id)
}

func (l *Launcher) isMissing(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.missing[id]
}

// Installer is the dev session's pretend installer (session.AppInstaller).
type Installer struct {
	Launcher *Launcher
	Allow    []string
	OnChange func()
	// Step is one tick of the pretend download (default 1 s; 20 ticks).
	Step time.Duration

	mu     sync.Mutex
	status map[string]install.Status
	cancel context.CancelFunc
}

// NewInstaller returns a pretend installer over launcher.
func NewInstaller(launcher *Launcher, allow []string) *Installer {
	return &Installer{Launcher: launcher, Allow: allow, Step: time.Second, status: map[string]install.Status{}}
}

func (f *Installer) Available() (bool, string) { return true, "" }
func (f *Installer) Allowed(id string) bool {
	for _, a := range f.Allow {
		if a == id {
			return true
		}
	}
	return false
}
func (f *Installer) Status(id string) install.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status[id]
}
func (f *Installer) set(id string, fn func(*install.Status)) {
	f.mu.Lock()
	st := f.status[id]
	fn(&st)
	f.status[id] = st
	cb := f.OnChange
	f.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// Info reports DEMO sizes.
func (f *Installer) Info(_ context.Context, id string) (install.Sizes, error) {
	sz := install.Sizes{Download: 417_600_000, Disk: 1_118_600_000, RuntimeMissing: true, RuntimeDisk: 1_100_000_000}
	f.set(id, func(s *install.Status) { s.SizeBytes, s.DiskBytes = sz.Download, sz.Expected() })
	return sz, nil
}

// Start walks the pretend install.
func (f *Installer) Start(id string) error {
	f.mu.Lock()
	if f.cancel != nil {
		f.mu.Unlock()
		return install.ErrBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	f.mu.Unlock()
	f.set(id, func(s *install.Status) {
		s.State, s.Phase, s.Progress, s.Message = contract.InstallPreparing, contract.PhaseChecking, 0, ""
	})
	go f.run(ctx, id)
	return nil
}

func (f *Installer) run(ctx context.Context, id string) {
	defer func() {
		f.mu.Lock()
		f.cancel = nil
		f.mu.Unlock()
	}()
	step := f.Step
	for tick := 0; tick <= 21; tick++ {
		select {
		case <-ctx.Done():
			f.set(id, func(s *install.Status) {
				s.State, s.Phase, s.Progress, s.Message = contract.InstallAvailable, "", 0, install.ReasonCancelled
			})
			return
		case <-time.After(step):
		}
		switch {
		case tick < 20:
			phase := contract.PhaseRuntime
			if tick >= 14 {
				phase = contract.PhaseApp
			}
			f.set(id, func(s *install.Status) { s.State, s.Phase, s.Progress = contract.InstallDownloading, phase, tick*5 })
		case tick == 20:
			f.set(id, func(s *install.Status) { s.State, s.Phase = contract.InstallInstalling, contract.PhaseFinishing })
		default:
			f.Launcher.SetInstalled(id)
			f.set(id, func(s *install.Status) { s.State, s.Phase, s.Progress = contract.InstallDone, "", 100 })
		}
	}
}

// Cancel stops the pretend install.
func (f *Installer) Cancel(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.status[id].State
	if f.cancel == nil || (st != contract.InstallPreparing && st != contract.InstallDownloading) {
		return install.ErrNotRunning
	}
	f.cancel()
	return nil
}
func (f *Installer) Busy() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cancel != nil
}
func (f *Installer) Updating() bool { return false }

// Update pretends there is nothing to update.
func (f *Installer) Update(context.Context, []string) error { return nil }
func (f *Installer) CancelUpdate() bool                     { return false }

var _ applications.Launcher = (*Launcher)(nil)
