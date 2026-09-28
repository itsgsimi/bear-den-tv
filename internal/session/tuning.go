// Auto-tuning of app playback settings from the coordinator
// (docs/APP_PERFORMANCE.md).

package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"bear-den-tv/internal/applications/tuning"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
)

// Tuner runs the playback detection test (internal/applications/tuning):
// what each app decodes in hardware here, the best settings, and caveats.
// *tuning.Detector implements it; docs/APP_PERFORMANCE.md explains the rules.
type Tuner interface {
	Detect(ctx context.Context) tuning.Report
	ApplyReady(r *tuning.Report)
	ApplyApp(r *tuning.Report, flatpakID string) (*tuning.AppReport, bool)
	// Retune re-plans one app (by adapter) after the owner changed a setting:
	// written now when the app is closed and apply is set, else pending or off.
	Retune(ctx context.Context, r *tuning.Report, adapter string, apply bool) (*tuning.AppReport, bool)
}

// TuneDelay lets the box settle after startup before the test runs (it starts
// each app's sandbox briefly, at low CPU priority).
var TuneDelay = 20 * time.Second

// tuneState is the latest report; tuneMu serialises changes to it (applying
// writes files, so it is never held with c.mu).
type tuneState struct {
	mu     sync.Mutex
	report *tuning.Report
}

// autoTune runs the test once after startup. With config startup.tune_apps
// (default true) it applies the best settings for closed apps now and for
// open ones when they close; otherwise it only reports them.
func (c *Coordinator) autoTune(ctx context.Context) {
	if c.opts.Tuner == nil {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-c.clock.After(TuneDelay):
	}
	report := c.opts.Tuner.Detect(ctx)
	if c.opts.Config.Current().Startup.AutoTune() {
		c.opts.Tuner.ApplyReady(&report)
	} else {
		for i := range report.Apps {
			if report.Apps[i].Status == "" {
				report.Apps[i].Status = "off"
			}
		}
	}
	for _, a := range report.Apps {
		c.log.Info("playback: detection", "app", a.Label, "status", a.Status, "hardware", hardwareList(a.Caps), "backup", a.Backup, "err", a.Error)
	}
	c.tune.mu.Lock()
	c.tune.report = &report
	summary := c.playbackSummary(report)
	c.tune.mu.Unlock()
	c.mu.Lock()
	c.playback = summary
	c.mu.Unlock()
	c.publish()
}

// tuneAfterExit applies settings that waited for these apps to close.
func (c *Coordinator) tuneAfterExit(flatpakIDs []string) {
	if c.opts.Tuner == nil {
		return
	}
	c.tune.mu.Lock()
	if c.tune.report == nil {
		c.tune.mu.Unlock()
		return
	}
	var applied []string
	for _, id := range flatpakIDs {
		if a, ok := c.opts.Tuner.ApplyApp(c.tune.report, id); ok {
			c.log.Info("playback: applied after the app closed", "app", a.Label, "status", a.Status, "backup", a.Backup, "err", a.Error)
			if a.Status == "applied" {
				applied = append(applied, a.Label)
			}
		}
	}
	summary := c.playbackSummary(*c.tune.report)
	c.tune.mu.Unlock()
	c.mu.Lock()
	c.playback = summary
	c.mu.Unlock()
	c.publish()
	for _, label := range applied {
		c.Notify("info", label+" is now tuned for this TV")
	}
}

// setPlayback is the shell's playback.set: one app's setting chosen by hand
// (value "" returns it to automatic). The value must be one of the options
// this box offers now; it is stored in config.json (playback.overrides), the
// app is re-planned, and the plan is written at once when the app is closed
// or when it next closes. With startup.tune_apps false the choice is stored
// and shown but nothing is written: tuning stays the owner's to run
// (`bear-den-tv apps detect --apply`, which honours the stored choices).
func (c *Coordinator) setPlayback(ctx context.Context, adapter, setting, value string) error {
	if c.opts.Tuner == nil {
		return errors.New("playback tuning is not available in this session")
	}
	c.tune.mu.Lock()
	if c.tune.report == nil {
		c.tune.mu.Unlock()
		return errors.New("the playback test has not run yet; try again in a moment")
	}
	r := c.tune.report
	var app *tuning.AppReport
	for i := range r.Apps {
		if r.Apps[i].Adapter == adapter {
			app = &r.Apps[i]
		}
	}
	if app == nil {
		c.tune.mu.Unlock()
		return fmt.Errorf("%q is not an installed app Bear Den tunes", adapter)
	}
	var found *tuning.Setting
	catalog := tuning.Catalog(adapter, app.Caps, r.Host, r.Display)
	for i := range catalog {
		if catalog[i].ID == setting {
			found = &catalog[i]
		}
	}
	switch {
	case found == nil:
		c.tune.mu.Unlock()
		return fmt.Errorf("%s has no adjustable setting %q", app.Label, setting)
	case value != "" && !found.Offers(value):
		c.tune.mu.Unlock()
		return fmt.Errorf("%q is not offered for %s on this box", value, found.Label)
	}
	if _, err := c.opts.Config.Update(func(cfg *config.Config) error {
		cfg.SetPlaybackOverride(adapter, setting, value)
		return nil
	}); err != nil {
		c.tune.mu.Unlock()
		return fmt.Errorf("could not save the setting: %w", err)
	}
	autoTune := c.opts.Config.Current().Startup.AutoTune()
	a, _ := c.opts.Tuner.Retune(ctx, r, adapter, autoTune)
	status, label := "", app.Label
	if a != nil {
		status = a.Status
		c.log.Info("playback: setting chosen by hand", "app", a.Label, "setting", setting, "value", value, "status", a.Status, "backup", a.Backup, "err", a.Error)
	}
	summary := c.playbackSummary(*r)
	c.tune.mu.Unlock()
	c.mu.Lock()
	c.playback = summary
	c.mu.Unlock()
	c.publish()
	switch status {
	case "pending":
		c.Notify("info", label+" is open: the new setting applies when it closes")
	case "off":
		c.Notify("info", "Saved. Automatic tuning is off, so "+label+"'s own settings are unchanged")
	case "error":
		c.Notify("error", "Could not change "+label+"'s settings")
	}
	return nil
}

func hardwareList(caps tuning.Caps) []string {
	out := []string{}
	for _, codec := range tuning.Codecs {
		if caps.Has(codec) {
			out = append(out, strings.ToUpper(string(codec)))
		}
	}
	return out
}

// playbackSummary turns a report into the shell's Playback screens' data,
// with each app's adjustable settings resolved against the stored overrides.
func (c *Coordinator) playbackSummary(r tuning.Report) *contract.Playback {
	cfg := c.opts.Config.Current()
	return playbackSummary(r, func(adapter string) tuning.Overrides { return cfg.PlaybackOverrides(adapter) })
}

func playbackSummary(r tuning.Report, overrides func(adapter string) tuning.Overrides) *contract.Playback {
	p := &contract.Playback{AtMs: r.At.UnixMilli(), Notes: []contract.PlaybackNote{}, Apps: []contract.PlaybackApp{}}
	p.Summary = r.Host.Describe() + "."
	if r.Display.Known {
		p.Display = fmt.Sprintf("%d×%d at %.0f Hz on %s", r.Display.Width, r.Display.Height, r.Display.Refresh, r.Display.Output)
	}
	for _, n := range r.Notes {
		p.Notes = append(p.Notes, contract.PlaybackNote{Level: n.Level, Text: n.Text})
	}
	for _, a := range r.Apps {
		pa := contract.PlaybackApp{Label: a.Label, Adapter: a.Adapter, Hardware: hardwareList(a.Caps), Expect: a.Expect,
			Notes: []contract.PlaybackNote{}, Status: a.Status, Changes: []string{}}
		if pa.Expect == nil {
			pa.Expect = []string{}
		}
		if pa.Status == "" {
			pa.Status = "suggested"
		}
		for _, n := range a.Notes {
			pa.Notes = append(pa.Notes, contract.PlaybackNote{Level: n.Level, Text: n.Text})
		}
		if a.Plan != nil {
			for _, ch := range a.Plan.Changes {
				pa.Changes = append(pa.Changes, ch.Key+" → "+ch.To+": "+ch.Why)
			}
		}
		settings, refused := tuning.Settings(a.Adapter, a.Caps, r.Host, r.Display, overrides(a.Adapter))
		for _, n := range refused {
			pa.Notes = append(pa.Notes, contract.PlaybackNote{Level: "info", Text: n})
		}
		for _, s := range settings {
			ps := contract.PlaybackSetting{ID: s.ID, Label: s.Label, Auto: s.Auto, Value: s.Value, Overridden: s.Overridden, Options: []contract.PlaybackOption{}}
			for _, o := range s.Options {
				ps.Options = append(ps.Options, contract.PlaybackOption{Value: o.Value, Label: o.Label, Note: o.Note})
			}
			pa.Settings = append(pa.Settings, ps)
		}
		p.Apps = append(p.Apps, pa)
	}
	return p
}
