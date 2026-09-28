// `bear-den-tv apps`: probe and apply playback tuning for the supported apps
// (docs/APP_PERFORMANCE.md; CLI guide internal/AGENTS.md).

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"bear-den-tv/internal/applications/flatpak"
	"bear-den-tv/internal/applications/tuning"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/doctor"
)

// cmdApps runs the playback detection test (docs/APP_PERFORMANCE.md):
//
//	apps detect [--apply] [--json] [--tier T]
//	                                 hardware decoding per app, the box's tier, the
//	                                 display, what to expect, caveats, and the best
//	                                 settings (dry run; --apply writes them for closed
//	                                 apps, with backups; --tier previews another tier)
//	apps tune ...                    the same as detect
//	apps probe [--json]              only the hardware decoding per app
//
// The coordinator runs the same test after startup and applies the settings
// automatically unless config startup.tune_apps is false.
func cmdApps(args []string) error {
	if len(args) == 0 || (args[0] != "probe" && args[0] != "tune" && args[0] != "detect") {
		return errors.New("usage: bear-den-tv apps detect [--apply] [--json] [--tier entry|standard|high] | apps probe [--json]")
	}
	fs := flag.NewFlagSet("apps "+args[0], flag.ExitOnError)
	apply := fs.Bool("apply", false, "write the best settings for apps that are closed (files are backed up)")
	asJSON := fs.Bool("json", false, "machine-readable output")
	tier := fs.String("tier", "", "treat this box as entry, standard or high (preview what another class of box gets)")
	_ = fs.Parse(args[1:])
	if args[0] == "probe" && *apply {
		return errors.New("probe only reads; use `apps detect --apply`")
	}
	det := tuning.NewDetector(flatpak.New(flatpak.Options{}))
	// Honour the settings the owner chose by hand on the TV (config.json
	// playback.overrides), so --apply never undoes them.
	cfg, err := readConfig(doctor.DefaultPaths().ConfigDir)
	if err != nil {
		return err
	}
	det.Overrides = func(adapter string) tuning.Overrides { return cfg.PlaybackOverrides(adapter) }
	if *tier != "" {
		forced := tuning.Tier(*tier)
		if !forced.Valid() {
			return fmt.Errorf("--tier must be entry, standard or high, not %q", *tier)
		}
		det.HostProbe = func() tuning.Host { h := tuning.DetectHost(); h.Tier = forced; return h }
	}
	report := det.Detect(context.Background())
	if *apply {
		det.ApplyReady(&report)
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	printReport(report, args[0] == "probe", *apply)
	return nil
}

// readConfig reads config.json without writing anything (defaults when absent).
func readConfig(dir string) (config.Config, error) {
	store, err := config.Open(config.Options{Dir: dir})
	if err != nil {
		return config.Config{}, err
	}
	raw, err := os.ReadFile(store.Path())
	if errors.Is(err, os.ErrNotExist) {
		return config.Defaults(), nil
	}
	if err != nil {
		return config.Config{}, err
	}
	cfg, err := config.Parse(raw, config.Rules{})
	if err != nil {
		return config.Config{}, fmt.Errorf("reading %s: %w", store.Path(), err)
	}
	return cfg, nil
}

func printReport(r tuning.Report, probeOnly, applied bool) {
	fmt.Printf("This box: %s\n", r.Host.Describe())
	if r.Display.Known {
		fmt.Printf("Display:  %s %dx%d at %.0f Hz", r.Display.Output, r.Display.Width, r.Display.Height, r.Display.Refresh)
		if r.Display.UHD {
			fmt.Printf(" (the screen also offers 4K, up to %.0f Hz)", r.Display.UHDMaxHz)
		}
		fmt.Println()
	}
	for _, n := range r.Notes {
		fmt.Printf("  %s %s\n", mark(n.Level), n.Text)
	}
	fmt.Println()
	for _, a := range r.Apps {
		fmt.Printf("== %s\n", a.Label)
		if a.Caps.Error != "" {
			fmt.Printf("  hardware decoding: unknown (%s)\n", a.Caps.Error)
		} else {
			var hw []string
			for _, c := range tuning.Codecs {
				if a.Caps.Has(c) {
					hw = append(hw, strings.ToUpper(string(c)))
				}
			}
			if len(hw) == 0 {
				hw = []string{"none (decodes on the CPU)"}
			}
			fmt.Printf("  hardware decoding: %s\n", strings.Join(hw, ", "))
		}
		if probeOnly {
			fmt.Println()
			continue
		}
		for _, e := range a.Expect {
			fmt.Printf("  expect: %s\n", e)
		}
		for _, n := range a.Notes {
			fmt.Printf("  %s %s\n", mark(n.Level), n.Text)
		}
		if a.Plan != nil && len(a.Plan.Changes) > 0 {
			fmt.Printf("  best settings (%s):\n", a.Plan.File)
			for _, c := range a.Plan.Changes {
				from := c.From
				if from == "" {
					from = "(unset)"
				}
				fmt.Printf("    %s: %s -> %s\n      %s\n", c.Key, from, c.To, c.Why)
			}
		}
		if a.Plan != nil {
			for _, n := range a.Plan.Notes {
				fmt.Printf("  note: %s\n", n)
			}
			for _, s := range a.Plan.Settings {
				if s.Overridden {
					fmt.Printf("  chosen by hand on the TV: %s = %s (auto: %s)\n", s.Label, s.OptionLabel(s.Value), s.OptionLabel(s.Auto))
				}
			}
		}
		switch {
		case a.Status == "tuned":
			fmt.Println("  status: already tuned")
		case a.Status == "applied":
			fmt.Printf("  status: applied (backup: %s)\n", a.Backup)
		case a.Status == "pending":
			fmt.Printf("  status: %s is open; close it and run this again (Bear Den's coordinator applies it automatically when it closes)\n", a.Label)
		case a.Status == "error":
			fmt.Printf("  status: NOT APPLIED: %s\n", a.Error)
		case !applied:
			fmt.Println("  status: dry run (add --apply)")
		}
		fmt.Println()
	}
}

func mark(level string) string {
	if level == "warn" {
		return "⚠"
	}
	return "ℹ"
}
