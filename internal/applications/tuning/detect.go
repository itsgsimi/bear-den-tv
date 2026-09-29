// Detection for tuning: display mode from xrandr, per-app plans and reports
// (docs/APP_PERFORMANCE.md).

package tuning

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"bear-den-tv/internal/applications/flatpak"
)

// App is a supported app Bear Den knows how to tune.
type App struct {
	FlatpakID string
	Adapter   string
	Label     string
	plan      func(home string, caps Caps, host Host, disp Display, ov Overrides) (Plan, error)
}

// Apps are the tunable apps, in display order. Adding one: a Plan<App>
// function in plans.go, its Catalog in settings.go, its Expectations case
// below, a row here, and tests (docs/APP_PERFORMANCE.md, "Adding an app").
var Apps = []App{
	{"tv.plex.PlexHTPC", "plex-htpc", "Plex", func(h string, c Caps, x Host, _ Display, ov Overrides) (Plan, error) {
		return PlanPlex(PlexMpvConf(h), c, x, ov)
	}},
	{"rocks.shy.VacuumTube", "vacuumtube", "YouTube", func(h string, c Caps, x Host, d Display, ov Overrides) (Plan, error) {
		return PlanVacuumTube(VacuumTubeConf(h), c, x, d, ov)
	}},
	{"com.moonlight_stream.Moonlight", "moonlight", "Moonlight", func(h string, c Caps, x Host, d Display, ov Overrides) (Plan, error) {
		return PlanMoonlight(MoonlightConf(h), c, x, d, ov)
	}},
	// Optional apps with nothing Bear Den tunes yet (NoTuning says why).
	{"com.spotify.Client", "spotify", "Spotify", func(string, Caps, Host, Display, Overrides) (Plan, error) {
		return NoTuning("spotify", "com.spotify.Client", "Spotify streams compressed audio, which any box plays without help; Bear Den leaves its settings alone."), nil
	}},
	{"org.jellyfin.JellyfinDesktop", "jellyfin", "Jellyfin", func(string, Caps, Host, Display, Overrides) (Plan, error) {
		return NoTuning("jellyfin", "org.jellyfin.JellyfinDesktop", "Jellyfin Desktop keeps its playback settings on your Jellyfin server and in its own profile; Bear Den does not edit them yet. If a file stutters, lower its quality in the player so the server converts it."), nil
	}},
	{"org.libretro.RetroArch", "retroarch", "RetroArch", func(string, Caps, Host, Display, Overrides) (Plan, error) {
		return NoTuning("retroarch", "org.libretro.RetroArch", "RetroArch's video and audio settings belong to each core and game; Bear Den leaves retroarch.cfg alone."), nil
	}},
}

// NoTuning is the plan of an app Bear Den knows but does not tune: no
// settings file, no changes, and one note that says why.
func NoTuning(adapter, flatpakID, why string) Plan {
	return Plan{App: adapter, Flatpak: flatpakID, Notes: []string{why}}
}

// Plan builds this app's settings plan with the owner's overrides.
func (a App) Plan(home string, caps Caps, host Host, disp Display, ov Overrides) (Plan, error) {
	return a.plan(home, caps, host, disp, ov)
}

// Display is the TV output: the current mode and the best the screen offers.
type Display struct {
	Known       bool    `json:"known"`
	Output      string  `json:"output,omitempty"`
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	Refresh     float64 `json:"refresh,omitempty"`
	UHD         bool    `json:"uhd"`                    // the screen offers 3840×2160 or more
	UHDMaxHz    float64 `json:"uhd_max_hz,omitempty"`   // best refresh at that size
	BestSmallHz float64 `json:"best_1080_hz,omitempty"` // best refresh at 1920×1080
}

var (
	xrandrOutput = regexp.MustCompile(`^(\S+) connected (?:primary )?(\d+)x(\d+)\+`)
	xrandrMode   = regexp.MustCompile(`^\s+(\d+)x(\d+)i?\s+(.*)$`)
)

// ParseXrandr reads `xrandr --current`: the first connected output with a
// mode, its current refresh (the rate marked *), and the modes it offers.
func ParseXrandr(out []byte) Display {
	var d Display
	in := false
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if m := xrandrOutput.FindStringSubmatch(line); m != nil {
			if d.Known {
				break
			}
			d.Output, d.Width, d.Height = m[1], atoi(m[2]), atoi(m[3])
			d.Known, in = true, true
			continue
		}
		if !in {
			continue
		}
		m := xrandrMode.FindStringSubmatch(line)
		if m == nil {
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				in = false
			}
			continue
		}
		w, h := atoi(m[1]), atoi(m[2])
		for _, tok := range strings.Fields(m[3]) {
			current := strings.Contains(tok, "*")
			hz, err := strconv.ParseFloat(strings.TrimRight(tok, "*+"), 64)
			if err != nil {
				continue
			}
			if current && w == d.Width && h == d.Height {
				d.Refresh = hz
			}
			if w >= 3840 && h >= 2160 {
				d.UHD = true
				if hz > d.UHDMaxHz {
					d.UHDMaxHz = hz
				}
			}
			if w == 1920 && h == 1080 && hz > d.BestSmallHz {
				d.BestSmallHz = hz
			}
		}
	}
	return d
}

// DetectDisplay runs `xrandr --current` (X11); unknown when it cannot.
func DetectDisplay(ctx context.Context) Display {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xrandr", "--current").Output()
	if err != nil {
		return Display{}
	}
	return ParseXrandr(out)
}

// Note is a caveat shown with the report; Level is "info" or "warn".
type Note struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

// AppReport is one app's detection result, expectations and tuning status.
type AppReport struct {
	Label     string   `json:"label"`
	Adapter   string   `json:"adapter"`
	FlatpakID string   `json:"flatpak"`
	Caps      Caps     `json:"caps"`
	Expect    []string `json:"expect"`
	Notes     []Note   `json:"notes"`
	Plan      *Plan    `json:"plan,omitempty"`
	Running   bool     `json:"running"`
	// Status: "tuned" (nothing to change), "applied", "pending" (applies when
	// the app closes), "off" (automatic tuning disabled), "error".
	Status string `json:"status"`
	Backup string `json:"backup,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Report is a whole detection run.
type Report struct {
	At      time.Time   `json:"at"`
	Host    Host        `json:"host"`
	Display Display     `json:"display"`
	Notes   []Note      `json:"notes"`
	Apps    []AppReport `json:"apps"`
}

// Detector runs the detection test; the coordinator and the CLI share it.
type Detector struct {
	Runner    flatpak.Runner
	Launcher  *flatpak.Launcher
	Home      string
	Now       func() time.Time
	Display   func(context.Context) Display
	HostProbe func() Host
	// Overrides returns the owner's manual choices for one app (config.json
	// playback.overrides); nil means none.
	Overrides func(adapter string) Overrides
}

func (d *Detector) overrides(adapter string) Overrides {
	if d.Overrides == nil {
		return nil
	}
	return d.Overrides(adapter)
}

// NewDetector builds a Detector for this machine.
func NewDetector(launcher *flatpak.Launcher) *Detector {
	home, _ := os.UserHomeDir()
	return &Detector{Runner: flatpak.ExecRunner{}, Launcher: launcher, Home: home, Now: time.Now, Display: DetectDisplay, HostProbe: DetectHost}
}

// Detect probes the host, the display and every installed tunable app, and
// works out the best settings, what to expect, and the caveats. It changes
// nothing (see ApplyReady).
func (d *Detector) Detect(ctx context.Context) Report {
	r := Report{At: d.Now(), Host: d.HostProbe(), Display: d.Display(ctx)}
	running := map[string]bool{}
	if inst, err := d.Launcher.Instances(ctx); err == nil {
		for _, i := range inst {
			running[i.FlatpakID] = true
		}
	}
	for _, app := range Apps {
		inst, err := d.Launcher.Discover(ctx, app.FlatpakID)
		if err != nil || !inst.Installed {
			continue
		}
		ar := AppReport{Label: app.Label, Adapter: app.Adapter, FlatpakID: app.FlatpakID, Running: running[app.FlatpakID]}
		ar.Caps = Probe(ctx, d.Runner, app.FlatpakID)
		plan, err := app.Plan(d.Home, ar.Caps, r.Host, r.Display, d.overrides(app.Adapter))
		if err != nil {
			ar.Status, ar.Error = "error", err.Error()
		} else {
			ar.Plan = &plan
			if len(plan.Changes) == 0 {
				ar.Status = "tuned"
			}
		}
		ar.Expect, ar.Notes = Expectations(app.Adapter, ar.Caps, r.Host, r.Display)
		r.Apps = append(r.Apps, ar)
	}
	r.Notes = HostNotes(r.Host, r.Display)
	return r
}

// ApplyReady writes the plans of apps that are closed and marks running ones
// "pending" (call ApplyApp when they close). Apps already tuned are untouched.
func (d *Detector) ApplyReady(r *Report) {
	for i := range r.Apps {
		a := &r.Apps[i]
		if a.Plan == nil || len(a.Plan.Changes) == 0 || a.Status == "error" {
			continue
		}
		if a.Running {
			a.Status = "pending"
			continue
		}
		d.apply(a)
	}
}

// ApplyApp applies a pending app's plan now (it has just closed). The plan is
// rebuilt from the file on disk first, since the app may have rewritten it.
func (d *Detector) ApplyApp(r *Report, flatpakID string) (*AppReport, bool) {
	for i := range r.Apps {
		a := &r.Apps[i]
		if a.FlatpakID != flatpakID || a.Status != "pending" {
			continue
		}
		for _, app := range Apps {
			if app.FlatpakID == flatpakID {
				if plan, err := app.Plan(d.Home, a.Caps, r.Host, r.Display, d.overrides(app.Adapter)); err == nil {
					a.Plan = &plan
				}
			}
		}
		a.Running = false
		if a.Plan == nil {
			return a, false
		}
		if len(a.Plan.Changes) == 0 {
			a.Status = "tuned"
			return a, true
		}
		d.apply(a)
		return a, true
	}
	return nil, false
}

// Retune rebuilds one app's plan (by adapter) from the file on disk with the
// current overrides, after the owner changed one. With apply, the plan is
// written now when the app is closed, or the app is marked "pending" (ApplyApp
// writes it when the app closes); without apply (automatic tuning off) nothing
// is written and the app is "off" unless it needs no change.
func (d *Detector) Retune(ctx context.Context, r *Report, adapter string, apply bool) (*AppReport, bool) {
	for i := range r.Apps {
		a := &r.Apps[i]
		if a.Adapter != adapter {
			continue
		}
		for _, app := range Apps {
			if app.Adapter != adapter {
				continue
			}
			if d.Launcher != nil {
				if inst, err := d.Launcher.Instances(ctx); err == nil {
					a.Running = false
					for _, in := range inst {
						a.Running = a.Running || in.FlatpakID == a.FlatpakID
					}
				}
			}
			a.Error, a.Backup = "", ""
			plan, err := app.Plan(d.Home, a.Caps, r.Host, r.Display, d.overrides(adapter))
			if err != nil {
				a.Plan, a.Status, a.Error = nil, "error", err.Error()
				return a, true
			}
			a.Plan = &plan
			switch {
			case len(plan.Changes) == 0:
				a.Status = "tuned"
			case !apply:
				a.Status = "off"
			case a.Running:
				a.Status = "pending"
			default:
				d.apply(a)
			}
			return a, true
		}
	}
	return nil, false
}

func (d *Detector) apply(a *AppReport) {
	backup, err := Apply(*a.Plan, d.Now())
	if err != nil {
		a.Status, a.Error = "error", err.Error()
		return
	}
	a.Status, a.Backup = "applied", backup
}

// Expectations says, in plain words, what each app will play here and the
// caveats that follow from what was detected.
func Expectations(adapter string, caps Caps, host Host, disp Display) ([]string, []Note) {
	var expect []string
	var notes []Note
	uhd := disp.Known && disp.Height >= 2160
	screen := "1080p"
	if uhd {
		screen = "4K"
	}
	switch adapter {
	case "vacuumtube":
		switch {
		case caps.Has(AV1):
			expect = append(expect, fmt.Sprintf("Up to %s: YouTube's AV1 and VP9 decode on the GPU.", screen))
		case caps.Has(VP9):
			expect = append(expect, fmt.Sprintf("Up to %s: VP9 decodes on the GPU (AV1 is blocked, it would fall to the CPU).", screen))
		case caps.Probed && host.High() && uhd:
			expect = append(expect, "Up to 4K in VP9/AV1, decoded on the CPU: this desktop-class CPU has the headroom.")
			notes = append(notes, Note{"info", "YouTube's runtime has no hardware VP9/AV1 decoder for this GPU, so 4K keeps the CPU busy; HDR is unavailable."})
		case caps.Probed:
			where := "the CPU"
			if caps.Has(H264) {
				where = "the GPU"
			}
			expect = append(expect, fmt.Sprintf("Up to 1080p in H.264, decoded on %s.", where))
			if len(caps.Decoders) == 0 {
				notes = append(notes, Note{"warn", "No hardware decoding for YouTube on this box (its runtime has no driver for this GPU): 4K and HDR are unavailable."})
			} else if uhd {
				notes = append(notes, Note{"info", "YouTube's 4K needs VP9 or AV1 hardware decoding, which this box lacks, so YouTube stops at 1080p."})
			}
			if host.Entry() && !caps.Has(H264) {
				notes = append(notes, Note{"warn", "1080p at 60 fps keeps an entry-level CPU busy; if a video stutters, pick 1080p30 or 720p in YouTube's quality menu."})
			}
		default:
			expect = append(expect, "Hardware decoding could not be checked; YouTube is limited to H.264 to stay safe.")
		}
	case "plex-htpc":
		var hw []string
		for _, c := range Codecs {
			if caps.Has(c) {
				hw = append(hw, strings.ToUpper(string(c)))
			}
		}
		switch {
		case len(hw) > 0:
			expect = append(expect, fmt.Sprintf("%s files play directly on the GPU, up to %s.", strings.Join(hw, ", "), screen))
		case host.High():
			expect = append(expect, "Files decode on the CPU, which has the headroom for most of them.")
		default:
			expect = append(expect, "Files decode on the CPU; the Plex server should convert them.")
		}
		if !caps.Has(HEVC) {
			if host.High() {
				notes = append(notes, Note{"info", "HEVC (x265) files decode on the CPU here: fine at 1080p; if 4K HDR stutters, play it below Original quality so the Plex server converts it."})
			} else {
				notes = append(notes, Note{"warn", "HEVC (x265) and 4K HDR files are not hardware-decoded here: play them below Original quality so the Plex server converts them, or make H.264 Optimized Versions."})
			}
		}
		if !caps.Has(AV1) && !caps.Has(VP9) && caps.Has(HEVC) {
			notes = append(notes, Note{"info", "AV1/VP9 files are converted by the Plex server."})
		}
	case "spotify":
		expect = append(expect, "Music plays on any box; pick this TV in Spotify's device list on your phone.")
	case "jellyfin":
		if len(caps.Decoders) > 0 {
			expect = append(expect, "Files your GPU decodes play directly; the Jellyfin server converts the rest.")
		} else {
			expect = append(expect, "Files decode on the CPU; the Jellyfin server should convert heavy ones.")
		}
	case "retroarch":
		expect = append(expect, "Older consoles run well on any box; 3D-era cores need a capable CPU.")
		if host.Entry() {
			notes = append(notes, Note{"info", "An entry-level box: prefer lightweight cores and leave shaders off."})
		}
	case "moonlight":
		modern := caps.Has(AV1) || caps.Has(HEVC)
		codec := "H.264"
		if modern {
			codec = "the best codec both sides accelerate"
		}
		size := "1080p"
		if uhd && modern && !host.Entry() {
			size = "4K"
		}
		where := "decoded on the GPU"
		if len(caps.Decoders) == 0 {
			where = "decoded on the CPU"
		}
		expect = append(expect, fmt.Sprintf("Streams up to %s in %s, %s; the gaming PC encodes it.", size, codec, where))
		notes = append(notes, Note{"info", "For a full-screen picture, set the gaming PC (or its virtual display) to 16:9 — a 16:10 desktop shows bars."})
		if hz := math.Round(disp.Refresh); disp.Known && hz > 61 {
			if host.Entry() || len(caps.Decoders) == 0 {
				notes = append(notes, Note{"warn", fmt.Sprintf("Moonlight streams at %.0f fps to match the TV. This is a low-power box: if games stutter or Moonlight's statistics overlay (Ctrl+Alt+Shift+S) shows dropped frames, lower the frame rate to 60 fps, or the resolution.", hz)})
			} else {
				notes = append(notes, Note{"info", fmt.Sprintf("Moonlight streams at %.0f fps to match the TV; the gaming PC must keep up for games to feel smooth.", hz)})
			}
		}
	}
	return expect, notes
}

// HostNotes are caveats about the box and its display.
func HostNotes(host Host, disp Display) []Note {
	var notes []Note
	if !disp.Known {
		notes = append(notes, Note{"info", "The display mode could not be read (not an X11 session?)."})
		return notes
	}
	if host.Entry() && disp.Refresh > 61 {
		notes = append(notes, Note{"info", fmt.Sprintf("The TV output runs at %.0f Hz, so menus and animations draw up to %.0f frames a second, more than twice the work of 60 Hz for an entry-level box. Video plays at its own 24–60 fps either way; if menus ever feel sluggish, 60 Hz (Display settings) is the lighter choice.", disp.Refresh, disp.Refresh)})
	}
	if disp.UHD && disp.Height < 2160 {
		switch {
		case disp.UHDMaxHz > 0 && disp.UHDMaxHz < 50:
			notes = append(notes, Note{"info", fmt.Sprintf("The TV is 4K and upscales this box's 1080p picture. 4K output here tops out at %.0f Hz, which would make menus choppy — keep 1080p.", disp.UHDMaxHz)})
		case host.Entry():
			notes = append(notes, Note{"info", "The TV is 4K and upscales this box's 1080p picture; 4K output would give an entry-level GPU four times the pixels to draw — keep 1080p."})
		default:
			notes = append(notes, Note{"info", fmt.Sprintf("The TV offers 4K at %.0f Hz and this box can drive it: choose 3840×2160 in Display settings for sharper menus and 4K video.", disp.UHDMaxHz)})
		}
	}
	if host.LowMemory() {
		notes = append(notes, Note{"info", "4 GiB of RAM or less: YouTube runs in its low-memory mode."})
	}
	sort.SliceStable(notes, func(i, j int) bool { return notes[i].Level == "warn" && notes[j].Level != "warn" })
	return notes
}
