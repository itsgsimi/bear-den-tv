// Package probe gathers one JSON-serializable snapshot of everything the
// desktop and application adapters can observe on this machine: display
// session, adapter capabilities, X server facts (XTEST, window manager, key
// resolution, windows, foreground), lock state, MPRIS players, the audio sink,
// Flatpak discovery for the approved applications, and running instances. It
// backs `bear-den-tv doctor` and the bdtv-probe dev command; every step is
// bounded by a timeout and records its own error instead of aborting the run.
package probe

import (
	"context"
	"os"
	"runtime"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/flatpak"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/audio"
	"bear-den-tv/internal/platform/dbusx"
	"bear-den-tv/internal/platform/detect"
	"bear-den-tv/internal/platform/lock"
	"bear-den-tv/internal/platform/mpris"
	"bear-den-tv/internal/platform/x11"
)

// StepTimeout bounds each independent probe step.
const StepTimeout = 5 * time.Second

// ProbeReport is the full snapshot.
type ProbeReport struct {
	RecordedAt     time.Time         `json:"recorded_at"`
	Host           Host              `json:"host"`
	SessionEnv     map[string]string `json:"session_env"`
	DisplaySession string            `json:"display_session"`
	Adapter        AdapterReport     `json:"adapter"`
	X11            *X11Report        `json:"x11,omitempty"`
	Lock           LockReport        `json:"lock"`
	MPRIS          MPRISReport       `json:"mpris"`
	Audio          AudioReport       `json:"audio"`
	Flatpak        FlatpakReport     `json:"flatpak"`
}

// Host identifies the probed machine and process.
type Host struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	UID      int    `json:"uid"`
	PID      int    `json:"pid"`
}

// AdapterReport is the constructed desktop adapter and its capabilities.
type AdapterReport struct {
	Name         string                         `json:"name"`
	Capabilities map[string]platform.Capability `json:"capabilities"`
	Reason       string                         `json:"reason,omitempty"`
}

// X11Report is present only when the X11 adapter connected.
type X11Report struct {
	x11.Info
	Foreground      platform.Foreground   `json:"foreground"`
	ForegroundError string                `json:"foreground_error,omitempty"`
	Windows         []platform.WindowInfo `json:"windows"`
	WindowsError    string                `json:"windows_error,omitempty"`
}

// LockReport is the lock observer's capability and one observation.
type LockReport struct {
	Capability platform.Capability `json:"capability"`
	State      lock.State          `json:"state"`
	Error      string              `json:"error,omitempty"`
}

// MPRISReport lists the players on the session bus.
type MPRISReport struct {
	Available bool               `json:"available"`
	Players   []mpris.PlayerInfo `json:"players"`
	Error     string             `json:"error,omitempty"`
}

// AudioReport is the pactl probe.
type AudioReport struct {
	Capability platform.Capability `json:"capability"`
	Info       audio.Info          `json:"info"`
}

// FlatpakReport covers the CLI, discovery per approved application, and
// running instances.
type FlatpakReport struct {
	Version        string                  `json:"version"`
	VersionError   string                  `json:"version_error,omitempty"`
	Apps           map[string]AppReport    `json:"apps"`
	Instances      []applications.Instance `json:"instances"`
	InstancesError string                  `json:"instances_error,omitempty"`
}

// AppReport is discovery for one approved application.
type AppReport struct {
	Adapter        string                    `json:"adapter"`
	Installation   applications.Installation `json:"installation"`
	Error          string                    `json:"error,omitempty"`
	InstallCommand []string                  `json:"install_command"`
}

// Options configures ReportWith.
type Options struct {
	// Env is the imported session environment; nil uses detect.SessionEnv().
	Env map[string]string
	// Registry supplies the approved applications; nil uses adapters.NewRegistry().
	Registry *adapters.Registry
	// Launcher discovers applications; nil uses flatpak.New with defaults.
	Launcher applications.Launcher
	// FlatpakVersion reports the CLI version; nil uses the default launcher.
	FlatpakVersion func(ctx context.Context) (string, error)
	// Audio probes the sink; nil uses audio.New(nil).
	Audio *audio.Backend
	// StepTimeout overrides StepTimeout; zero keeps the default.
	StepTimeout time.Duration
}

// Report gathers a snapshot with default options.
func Report(ctx context.Context) ProbeReport {
	return ReportWith(ctx, Options{})
}

// ReportWith gathers a snapshot with opts.
func ReportWith(ctx context.Context, opts Options) ProbeReport {
	step := opts.StepTimeout
	if step == 0 {
		step = StepTimeout
	}
	bounded := func(fn func(ctx context.Context)) {
		sctx, cancel := context.WithTimeout(ctx, step)
		defer cancel()
		fn(sctx)
	}
	env := opts.Env
	if env == nil {
		env = detect.SessionEnv()
	}
	hostname, _ := os.Hostname()
	r := ProbeReport{
		RecordedAt:     time.Now().UTC(),
		Host:           Host{Hostname: hostname, OS: runtime.GOOS, Arch: runtime.GOARCH, UID: os.Getuid(), PID: os.Getpid()},
		SessionEnv:     env,
		DisplaySession: detect.Classify(env),
	}

	var lockObs *lock.Observer
	bounded(func(ctx context.Context) {
		lockObs = lock.New(ctx, lock.Options{Display: env["DISPLAY"], Getenv: func(k string) string { return env[k] }})
		r.Lock.Capability = lockObs.Capability()
		r.Lock.State = lockObs.State(ctx)
		if _, err := lockObs.Locked(ctx); err != nil {
			r.Lock.Error = err.Error()
		}
	})
	defer lockObs.Close()

	bounded(func(ctx context.Context) {
		adapter := detect.NewDesktopAdapter(ctx, detect.Options{Env: env, Lock: lockObs})
		defer adapter.Close()
		r.Adapter = AdapterReport{Name: adapter.Name(), Capabilities: adapter.Capabilities()}
		switch a := adapter.(type) {
		case *detect.Unavailable:
			r.Adapter.Reason = a.Reason()
		case *x11.Adapter:
			x := &X11Report{Info: a.Info(ctx), Windows: []platform.WindowInfo{}}
			var err error
			if x.Foreground, err = a.ObserveForeground(ctx); err != nil {
				x.ForegroundError = err.Error()
			}
			if ws, err := a.ListWindows(ctx); err != nil {
				x.WindowsError = err.Error()
			} else if ws != nil {
				x.Windows = ws
			}
			r.X11 = x
		}
	})

	r.MPRIS.Players = []mpris.PlayerInfo{}
	bounded(func(ctx context.Context) {
		bus, err := dbusx.ConnectSession(ctx)
		if err != nil {
			r.MPRIS.Error = err.Error()
			return
		}
		defer bus.Close()
		players, err := mpris.NewLocator(bus).List(ctx)
		if err != nil {
			r.MPRIS.Error = err.Error()
			return
		}
		r.MPRIS.Available = true
		if players != nil {
			r.MPRIS.Players = players
		}
	})

	bounded(func(ctx context.Context) {
		a := opts.Audio
		if a == nil {
			a = audio.New(nil)
		}
		r.Audio.Info, r.Audio.Capability = a.Probe(ctx)
	})

	registry := opts.Registry
	if registry == nil {
		registry = adapters.NewRegistry()
	}
	launcher := opts.Launcher
	version := opts.FlatpakVersion
	if launcher == nil {
		fl := flatpak.New(flatpak.Options{})
		launcher = fl
		if version == nil {
			version = fl.Version
		}
	}
	r.Flatpak.Apps = map[string]AppReport{}
	r.Flatpak.Instances = []applications.Instance{}
	bounded(func(ctx context.Context) {
		if version != nil {
			if v, err := version(ctx); err != nil {
				r.Flatpak.VersionError = err.Error()
			} else {
				r.Flatpak.Version = v
			}
		}
		for _, ad := range registry.All() {
			app := AppReport{Adapter: ad.Name(), InstallCommand: flatpak.InstallCommand(ad.FlatpakID())}
			var err error
			if app.Installation, err = launcher.Discover(ctx, ad.FlatpakID()); err != nil {
				app.Error = err.Error()
			}
			r.Flatpak.Apps[ad.FlatpakID()] = app
		}
		insts, err := launcher.Instances(ctx)
		if err != nil {
			r.Flatpak.InstancesError = err.Error()
		} else if insts != nil {
			r.Flatpak.Instances = insts
		}
	})
	return r
}
