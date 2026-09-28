// Tests for the probe report (probe.go).

package probe

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/platform/audio"
)

type fakeLauncher struct{}

func (fakeLauncher) Discover(ctx context.Context, id string) (applications.Installation, error) {
	if id == "tv.plex.PlexHTPC" {
		return applications.Installation{Installed: true, Version: "1.68", Scope: "system"}, nil
	}
	return applications.Installation{Scope: "unknown"}, errors.New("flatpak: not found")
}
func (fakeLauncher) Launch(context.Context, string, []string) (applications.Instance, error) {
	return applications.Instance{}, errors.New("not in probe")
}
func (fakeLauncher) Instances(context.Context) ([]applications.Instance, error) {
	return []applications.Instance{{FlatpakID: "tv.plex.PlexHTPC", InstanceID: "1", PID: 1}}, nil
}
func (fakeLauncher) Kill(context.Context, applications.Instance) error { return nil }

func TestReportWithoutDisplayIsCompleteJSON(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r := ReportWith(ctx, Options{
		Env:            map[string]string{},
		Launcher:       fakeLauncher{},
		FlatpakVersion: func(context.Context) (string, error) { return "1.14.4", nil },
		Audio: audio.New(func(context.Context, []string) ([]byte, error) {
			return []byte("Default Sink: auto_null\n"), nil
		}),
		StepTimeout: 3 * time.Second,
	})
	if r.DisplaySession != "unknown" || r.Adapter.Name != "unavailable" || r.Adapter.Reason == "" || r.X11 != nil {
		t.Fatalf("adapter %+v session %s", r.Adapter, r.DisplaySession)
	}
	if r.Audio.Capability.Available || r.Audio.Info.DefaultSink != "auto_null" {
		t.Fatalf("%+v", r.Audio)
	}
	if r.Flatpak.Version != "1.14.4" || !r.Flatpak.Apps["tv.plex.PlexHTPC"].Installation.Installed || r.Flatpak.Apps["rocks.shy.VacuumTube"].Error == "" {
		t.Fatalf("%+v", r.Flatpak)
	}
	if len(r.Flatpak.Apps["rocks.shy.VacuumTube"].InstallCommand) == 0 || len(r.Flatpak.Instances) != 1 {
		t.Fatalf("%+v", r.Flatpak)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var back ProbeReport
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Adapter.Capabilities["input"].Available || back.MPRIS.Players == nil {
		t.Fatalf("round trip lost fields: %s", data)
	}
}
