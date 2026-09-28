// Package doctor gathers the diagnostics printed by `bear-den-tv doctor` and
// served (redacted) to owners at /api/v1/diagnostics: capability probe, config
// recovery status, shell binary, and — when a coordinator is running — its
// live session summary. Addresses, window titles, tokens and the weather
// place are never included.
package doctor

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform/probe"
	"bear-den-tv/internal/shellipc"
)

// Paths are the XDG locations the coordinator uses.
type Paths struct {
	ConfigDir string
	DataDir   string
	CacheDir  string
	Socket    string
}

// DefaultPaths resolves the XDG directories for the current user.
func DefaultPaths() Paths {
	home, _ := os.UserHomeDir()
	env := func(k, fallback string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return filepath.Join(home, fallback)
	}
	return Paths{
		ConfigDir: filepath.Join(env("XDG_CONFIG_HOME", ".config"), "bear-den-tv"),
		DataDir:   filepath.Join(env("XDG_DATA_HOME", ".local/share"), "bear-den-tv"),
		CacheDir:  filepath.Join(env("XDG_CACHE_HOME", ".cache"), "bear-den-tv"),
		Socket:    shellipc.DefaultSocketPath(""),
	}
}

// Options configures Report.
type Options struct {
	Paths       Paths
	Version     string
	ShellBinary string // explicit --shell-binary, may be empty
	// Probe runs the desktop capability probe (touches the display and D-Bus).
	Probe bool
	// Timeout bounds the whole report.
	Timeout time.Duration
}

// Report collects diagnostics. It never fails; problems are reported inline.
func Report(ctx context.Context, opts Options) map[string]any {
	if opts.Timeout <= 0 {
		opts.Timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	out := map[string]any{
		"version":      opts.Version,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	}
	out["config"] = configReport(opts.Paths.ConfigDir)
	if bin, err := shellipc.LookupBinary(opts.ShellBinary); err != nil {
		out["shell_binary"] = map[string]any{"found": false, "error": err.Error()}
	} else {
		out["shell_binary"] = map[string]any{"found": true, "path": bin}
	}
	out["coordinator"] = Live(ctx, opts.Paths.Socket)
	if opts.Probe {
		out["probe"] = probe.Report(ctx)
	}
	return out
}

func configReport(dir string) map[string]any {
	st, err := config.Open(config.Options{Dir: dir})
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	if _, err := os.Stat(st.Path()); err != nil {
		return map[string]any{"ok": true, "exists": false, "note": "no config yet; defaults apply on first start"}
	}
	raw, err := os.ReadFile(st.Path())
	if err != nil {
		return map[string]any{"ok": false, "exists": true, "error": err.Error()}
	}
	cfg, err := config.Parse(raw, config.Rules{})
	if err != nil {
		lkg, _ := st.LKGStatus()
		return map[string]any{"ok": false, "exists": true, "error": err.Error(), "last_known_good": lkg}
	}
	return map[string]any{
		"ok": true, "exists": true, "revision": cfg.Revision,
		"remote_enabled": cfg.Remote.Enabled, "lan_consent": cfg.Onboarding.LANConsent,
		"transport": cfg.Remote.Transport, "applications": len(cfg.Applications),
		"weather_enabled": cfg.WeatherSettings().Enabled,
	}
}

// Live summarizes a running coordinator reached over the shell socket as a
// trusted cli peer, or reports that none answered.
func Live(ctx context.Context, socket string) map[string]any {
	conn, err := shellipc.Dial(ctx, socket, shellipc.ClientCLI, "doctor")
	if err != nil {
		return map[string]any{"running": false, "error": err.Error()}
	}
	defer conn.Close()
	return Summarize(conn.State)
}

// Summarize is the redacted view of a state snapshot used in diagnostics.
func Summarize(st contract.State) map[string]any {
	caps := map[string]any{}
	for name, c := range st.Capabilities {
		if c.Available {
			caps[name] = "available (" + c.Backend + ")"
		} else {
			caps[name] = "unavailable: " + c.Reason
		}
	}
	apps := []map[string]any{}
	for _, a := range st.Applications {
		apps = append(apps, map[string]any{"id": a.ID, "installed": a.Installed, "installation": a.Installation, "running": a.Running, "launch_state": a.LaunchState})
	}
	// Weather: status only; the place stays off diagnostics (served to owners).
	wx := "not reported"
	if st.Weather != nil {
		wx = st.Weather.Status
		if st.Weather.Message != "" {
			wx += ": " + st.Weather.Message
		}
	}
	return map[string]any{
		"weather":            wx,
		"running":            true,
		"dev_mode":           st.DevMode,
		"context_epoch":      st.ContextEpoch,
		"session":            st.Session,
		"target":             map[string]any{"kind": st.Target.Kind, "label": st.Target.Label, "observed": st.Target.Observed},
		"capabilities":       caps,
		"applications":       apps,
		"remote_enabled":     st.Remote.Enabled,
		"remote_listening":   st.Remote.Listening,
		"remote_transport":   st.Remote.Transport,
		"paired_devices":     st.Remote.PairedDeviceCount,
		"config_revision":    st.ConfigRevision,
		"shell_screen":       st.Shell.Screen,
		"notifications_open": len(st.Notifications),
	}
}
