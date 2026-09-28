// Package tuning gives each supported app opinionated playback settings that
// fit the box it runs on: video decoded by the GPU/VPU whenever the app's own
// Flatpak runtime can, the cheapest codec that still fills the screen when it
// cannot, heavy work (transcoding, encoding) left to the server or gaming PC,
// and restraint only where the hardware needs it.
//
// It is hardware-agnostic. Hardware decoding comes from asking each app's
// runtime which decoders GStreamer can build on this machine (VA-API, NVDEC,
// V4L2, Quick Sync), not from GPU model tables; each runtime ships its own
// drivers, so the same GPU can decode different codecs in different apps (on a
// Haswell box: H.264 in Plex and Moonlight, nothing in VacuumTube's newer
// runtime). The CPU side is a Tier worked out from core count and clock
// (Classify). The reference box, a 2-core 1.4 GHz Celeron 2955U, is the floor
// ("entry"): it gets the careful settings, and anything faster scales up
// automatically (4K, higher frame rates, full-quality scaling).
//
// Settings are written only while the app is closed (apps rewrite their
// settings on exit), every changed file is backed up first, and the owner can
// turn it off (config startup.tune_apps) or choose single settings by hand
// among the options this box can handle (Catalog; config playback.overrides).
// docs/APP_PERFORMANCE.md explains each rule.
package tuning

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"bear-den-tv/internal/applications/flatpak"
)

// Codec is a video codec the probe knows about.
type Codec string

const (
	H264 Codec = "h264"
	HEVC Codec = "hevc"
	VP9  Codec = "vp9"
	AV1  Codec = "av1"
)

// Codecs lists the codecs in a stable order for reports.
var Codecs = []Codec{H264, HEVC, VP9, AV1}

// Caps is what one app's runtime can decode in hardware on this machine.
type Caps struct {
	App      string         `json:"app"`
	Probed   bool           `json:"probed"`
	Hardware map[Codec]bool `json:"hardware"`
	Decoders []string       `json:"decoders"` // hardware decoder elements found
	Error    string         `json:"error,omitempty"`
}

// Has reports whether codec c decodes in hardware.
func (c Caps) Has(codec Codec) bool { return c.Hardware[codec] }

// hwDecoder matches GStreamer hardware decoder elements by family prefix and
// codec: vah264dec, vaapih265dec, nvvp9dec, v4l2slav1dec, qsvh264dec,
// msdkh265dec, … These families register a decoder only for codecs the
// device's driver really decodes. Vulkan video decoders (vulkanh265dec) are
// left out on purpose: GStreamer registers them whenever it was built with
// Vulkan video, even on GPUs whose driver cannot decode (seen on a Haswell box
// in the KDE runtime). Software decoders (avdec_h264, vp9dec, av1dec,
// dav1ddec, openh264dec) have no family prefix and never match.
var hwDecoder = regexp.MustCompile(`^(va|vaapi|nv|v4l2sl|v4l2|qsv|msdk)(h264|h265|hevc|vp9|av1)(sl)?dec$`)

// ParseDecoders reads `gst-inspect-1.0` output ("plugin:  element: description"
// lines) and returns the hardware decoding capabilities it lists.
func ParseDecoders(out []byte) Caps {
	caps := Caps{Probed: true, Hardware: map[Codec]bool{}}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), ":", 3)
		if len(parts) < 3 {
			continue
		}
		element := strings.TrimSpace(parts[1])
		m := hwDecoder.FindStringSubmatch(element)
		if m == nil {
			continue
		}
		switch m[2] {
		case "h264":
			caps.Hardware[H264] = true
		case "h265", "hevc":
			caps.Hardware[HEVC] = true
		case "vp9":
			caps.Hardware[VP9] = true
		case "av1":
			caps.Hardware[AV1] = true
		}
		caps.Decoders = append(caps.Decoders, element)
	}
	sort.Strings(caps.Decoders)
	return caps
}

// ProbeTimeout bounds one probe (a sandboxed gst-inspect; slower the first time).
const ProbeTimeout = 60 * time.Second

// Probe asks the app's own runtime which hardware decoders it can build here.
// It runs `flatpak run --command=gst-inspect-1.0 <app>` at low CPU priority;
// the app itself does not start.
func Probe(ctx context.Context, runner flatpak.Runner, flatpakID string) Caps {
	if err := flatpak.ValidateAppID(flatpakID); err != nil {
		return Caps{App: flatpakID, Error: err.Error()}
	}
	ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()
	cmd := flatpak.Command{
		Argv: []string{"nice", "-n", "10", "flatpak", "run", "--command=gst-inspect-1.0", flatpakID},
		Env:  flatpak.PassthroughEnv(os.Environ()),
	}
	res, err := runner.Output(ctx, cmd)
	if err != nil {
		return Caps{App: flatpakID, Error: "could not run the probe: " + err.Error()}
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(string(res.Stderr))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return Caps{App: flatpakID, Error: "the runtime has no working GStreamer probe: " + msg}
	}
	caps := ParseDecoders(res.Stdout)
	caps.App = flatpakID
	return caps
}

// Tier is how much work the box's CPU (and, as a proxy, the integrated GPU
// beside it) can take on. Hardware video decoding is judged per app (Caps) and
// the screen by Display; the tier only decides what happens when work lands on
// the CPU/GPU anyway: scaling quality, frame-rate and resolution caps, and
// whether software decoding of 4K is affordable.
type Tier string

const (
	// Entry is the floor Bear Den is designed and tested on: two cores, or an
	// old low-clocked quad (the reference Celeron 2955U: 2 × 1.4 GHz).
	Entry Tier = "entry"
	// Standard is a modern small box (Intel N100/N5105/J4125, Raspberry Pi 5):
	// it gets each app's full-quality defaults.
	Standard Tier = "standard"
	// High is desktop class (8+ threads, 24+ GHz in total): it can also decode
	// 4K VP9/AV1 in software when the GPU cannot.
	High Tier = "high"
)

// Valid reports whether t is a known tier.
func (t Tier) Valid() bool { return t == Entry || t == Standard || t == High }

// Classify places a CPU from its logical core count and the sum of every
// core's top clock (GHz; 0 when unknown). Core count × clock is crude but
// stable (the same box always lands in the same tier, so settings never flip
// between runs) and vendor-agnostic. Examples, total GHz: Celeron 2955U 2.8
// (entry), Atom x5-Z8350 7.7 (entry), Raspberry Pi 5 9.6, J4125 10.8, N100
// 13.6 (standard), 4-core/8-thread laptop at 3.4 GHz 27 (high).
func Classify(cores int, totalGHz float64) Tier {
	switch {
	case cores <= 2:
		return Entry
	case totalGHz <= 0: // clock unknown: by cores alone
		if cores >= 8 {
			return High
		}
		return Standard
	case totalGHz < 8:
		return Entry
	case cores >= 8 && totalGHz >= 24:
		return High
	}
	return Standard
}

// Host is the machine the apps play on.
type Host struct {
	Cores    int     `json:"cores"`     // logical CPUs
	ClockGHz float64 `json:"clock_ghz"` // top clock of the fastest core (0: unknown)
	TotalGHz float64 `json:"total_ghz"` // sum of every logical CPU's top clock (0: unknown)
	MemGiB   float64 `json:"mem_gib"`
	// Tier is Classify's answer, or the owner's override (BDTV_TIER, or
	// `apps detect --tier`). Empty means "classify from the fields above".
	Tier Tier `json:"tier"`
}

// Class returns the host's tier.
func (h Host) Class() Tier {
	if h.Tier.Valid() {
		return h.Tier
	}
	return Classify(h.Cores, h.TotalGHz)
}

// Entry reports whether this is a floor-class box that needs the careful settings.
func (h Host) Entry() bool { return h.Class() == Entry }

// High reports whether this box is desktop class.
func (h Host) High() bool { return h.Class() == High }

// LowMemory reports 4 GiB of RAM or less.
func (h Host) LowMemory() bool { return h.MemGiB > 0 && h.MemGiB <= 4.5 }

// Describe is the one-line summary shown by the CLI and the Playback screen.
func (h Host) Describe() string {
	cpu := fmt.Sprintf("%d CPU cores", h.Cores)
	if h.ClockGHz > 0 {
		cpu += fmt.Sprintf(" up to %.1f GHz", h.ClockGHz)
	}
	var class string
	switch h.Class() {
	case Entry:
		class = "entry level (Bear Den's floor): settings favour smooth playback"
	case High:
		class = "desktop class: full quality, 4K where the screen has it"
	default:
		class = "a capable small computer: full-quality settings"
	}
	return fmt.Sprintf("%s, %.1f GiB RAM — %s", cpu, h.MemGiB, class)
}

// DetectHost reads the CPU count, each CPU's top clock (cpufreq, else
// /proc/cpuinfo) and /proc/meminfo. BDTV_TIER=entry|standard|high overrides
// the classification.
func DetectHost() Host {
	h := Host{Cores: runtime.NumCPU()}
	h.ClockGHz, h.TotalGHz = cpuClocks("/sys/devices/system/cpu", "/proc/cpuinfo")
	if raw, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			if f := strings.Fields(line); len(f) >= 2 && f[0] == "MemTotal:" {
				if kb, err := strconv.ParseFloat(f[1], 64); err == nil {
					h.MemGiB = kb / (1024 * 1024)
				}
			}
		}
	}
	h.Tier = Classify(h.Cores, h.TotalGHz)
	if forced := Tier(os.Getenv("BDTV_TIER")); forced.Valid() {
		h.Tier = forced
	}
	return h
}

// cpuClocks returns the fastest core's top clock and the sum over all logical
// CPUs, in GHz: cpufreq's cpuinfo_max_freq (kHz) when the kernel exposes it,
// else the "cpu MHz" lines of cpuinfo (virtual machines). 0, 0 when neither.
func cpuClocks(sysCPU, cpuinfo string) (top, total float64) {
	paths, _ := filepath.Glob(filepath.Join(sysCPU, "cpu[0-9]*", "cpufreq", "cpuinfo_max_freq"))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if khz, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64); err == nil && khz > 0 {
			ghz := khz / 1e6
			total += ghz
			top = max(top, ghz)
		}
	}
	if total > 0 {
		return top, total
	}
	raw, err := os.ReadFile(cpuinfo)
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "cpu MHz" {
			if mhz, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && mhz > 0 {
				total += mhz / 1000
				top = max(top, mhz/1000)
			}
		}
	}
	return top, total
}
