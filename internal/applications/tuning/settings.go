// The playback settings catalog each app offers and owner overrides
// (docs/APP_PERFORMANCE.md).

package tuning

import (
	"fmt"
	"math"
)

// Option is one choice for an adjustable setting. Note says what it costs on
// this box ("decodes on the CPU"), when that matters.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note,omitempty"`
}

// Setting is one playback setting the owner may adjust by hand (TV Settings →
// Advanced playback). Auto is what detection chose; Value is what the plan
// uses: the owner's override when one is stored and still offered, else Auto.
// Options are only the choices this box's hardware can handle.
type Setting struct {
	ID         string   `json:"id"`
	Label      string   `json:"label"`
	Auto       string   `json:"auto"`
	Value      string   `json:"value"`
	Overridden bool     `json:"overridden"`
	Options    []Option `json:"options"`
}

// Offers reports whether value is one of the setting's options.
func (s Setting) Offers(value string) bool {
	for _, o := range s.Options {
		if o.Value == value {
			return true
		}
	}
	return false
}

// OptionLabel is the label of value ("" when not offered).
func (s Setting) OptionLabel(value string) string {
	for _, o := range s.Options {
		if o.Value == value {
			return o.Label
		}
	}
	return ""
}

// Overrides are one app's manual choices: setting id → option value
// (config.json playback.overrides.<adapter>).
type Overrides map[string]string

// Catalog lists the adjustable settings of one app (by adapter) with the
// options this box can handle and detection's choice as Auto (and Value). An
// unknown adapter has none.
func Catalog(adapter string, caps Caps, host Host, disp Display) []Setting {
	var out []Setting
	switch adapter {
	case "moonlight":
		out = moonlightCatalog(caps, host, disp)
	case "vacuumtube":
		out = vacuumTubeCatalog(caps, host, disp)
	case "plex-htpc":
		out = plexCatalog(host)
	}
	for i := range out {
		out[i].Value = out[i].Auto
	}
	return out
}

// Settings is Catalog with the owner's overrides resolved: an override is used
// only while it is one of the offered options; otherwise the setting falls
// back to Auto and a note says why (hardware or tier changed, hand-edited
// config).
func Settings(adapter string, caps Caps, host Host, disp Display, ov Overrides) ([]Setting, []string) {
	settings := Catalog(adapter, caps, host, disp)
	var notes []string
	for i := range settings {
		s := &settings[i]
		v, ok := ov[s.ID]
		if !ok || v == "" {
			continue
		}
		if !s.Offers(v) {
			notes = append(notes, fmt.Sprintf("Your choice %q for %s is not available on this box, so Bear Den uses Auto (%s).", v, s.Label, s.OptionLabel(s.Auto)))
			continue
		}
		s.Value, s.Overridden = v, true
	}
	return settings, notes
}

func valueOf(settings []Setting, id string) string {
	for _, s := range settings {
		if s.ID == id {
			return s.Value
		}
	}
	return ""
}

func overridden(settings []Setting, id string) bool {
	for _, s := range settings {
		if s.ID == id {
			return s.Overridden
		}
	}
	return false
}

// byHand is the reason given for a change that follows an override.
const byHand = "Chosen by hand in Settings → Advanced playback."

func onOff(onNote, offNote string) []Option {
	return []Option{{Value: "on", Label: "On", Note: onNote}, {Value: "off", Label: "Off", Note: offNote}}
}

func boolWord(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// --- Moonlight ---------------------------------------------------------------

// Resolution choices, smallest first: value, label, width, height.
var moonlightSizes = []struct {
	value, label string
	w, h         int
}{
	{"720p", "720p", 1280, 720},
	{"1080p", "1080p", 1920, 1080},
	{"1440p", "1440p", 2560, 1440},
	{"4k", "4K", 3840, 2160},
}

// moonlightRefresh is the TV output's refresh rounded to whole Hz (0: unknown
// or implausible).
func moonlightRefresh(disp Display) int {
	if !disp.Known || disp.Refresh < 23 {
		return 0
	}
	return int(math.Round(disp.Refresh))
}

// moonlightCap is the largest stream size that suits this box and screen
// (PlanMoonlight's rule): never more than the output shows, at most 1080p on
// an entry-level box or without HEVC/AV1 hardware decoding. 0, 0 = no cap.
func moonlightCap(caps Caps, host Host, disp Display) (int, int, string) {
	capW, capH, why := 0, 0, ""
	limit := func(w, h int, reason string) {
		if w > 0 && h > 0 && (capH == 0 || h < capH) {
			capW, capH, why = w, h, reason
		}
	}
	if disp.Known {
		limit(disp.Width, disp.Height, fmt.Sprintf("Never more than the TV output shows (%d×%d): extra pixels would be decoded only to be scaled away.", disp.Width, disp.Height))
	}
	if !(caps.Has(HEVC) || caps.Has(AV1)) {
		limit(1920, 1080, "At most 1080p: 4K in H.264 exceeds what this box's decoder handles at game frame rates.")
	}
	if host.Entry() {
		limit(1920, 1080, "At most 1080p on an entry-level box: 4K frames are four times the decoding and drawing.")
	}
	return capW, capH, why
}

func moonlightCatalog(caps Caps, host Host, disp Display) []Setting {
	modern := caps.Has(HEVC) || caps.Has(AV1)
	codec := Setting{ID: "codec", Label: "Video codec", Auto: "h264", Options: []Option{
		{Value: "auto", Label: "Automatic", Note: "Moonlight picks the best codec both the PC and this box accelerate."},
		{Value: "h264", Label: "H.264", Note: "Every GPU decodes it; the most bandwidth for the same quality."},
	}}
	if modern {
		codec.Auto = "auto"
	}
	if caps.Has(HEVC) {
		codec.Options = append(codec.Options, Option{Value: "hevc", Label: "HEVC", Note: "Decoded in hardware here; sharper than H.264 at the same bitrate."})
	}
	if caps.Has(AV1) {
		codec.Options = append(codec.Options, Option{Value: "av1", Label: "AV1", Note: "Decoded in hardware here; needs an AV1 encoder on the gaming PC."})
	}

	hz := moonlightRefresh(disp)
	fps := Setting{ID: "fps", Label: "Frame rate", Auto: "60", Options: []Option{
		{Value: "30", Label: "30 fps", Note: "Half the work for this box; slower games only."},
		{Value: "60", Label: "60 fps", Note: "Moonlight's default."},
	}}
	if hz > 0 {
		fps.Auto = fmt.Sprint(hz)
		if hz != 30 && hz != 60 {
			note := fmt.Sprintf("Matches the TV output's %d Hz.", hz)
			if hz > 60 && (host.Entry() || len(caps.Decoders) == 0) {
				note += " A lot for this box: lower it if games stutter."
			}
			fps.Options = append(fps.Options, Option{Value: fmt.Sprint(hz), Label: fmt.Sprintf("%d fps", hz), Note: note})
		}
	}

	res := Setting{ID: "resolution", Label: "Resolution"}
	for _, s := range moonlightSizes {
		big := s.h > 1080
		if big && !(disp.Known && disp.Height >= s.h && !host.Entry() && modern) {
			continue
		}
		o := Option{Value: s.value, Label: s.label}
		if disp.Known && s.h > disp.Height {
			o.Note = fmt.Sprintf("More than the TV output shows (%d×%d); the extra pixels are scaled away.", disp.Width, disp.Height)
		}
		res.Options = append(res.Options, o)
	}
	_, capH, _ := moonlightCap(caps, host, disp)
	res.Auto = moonlightAutoSize(capH)

	pacing := Setting{ID: "framepacing", Label: "Frame pacing", Auto: "on",
		Options: onOff("Evens out frame timing for smooth motion (up to one frame of latency).", "Lowest latency; motion may judder.")}
	return []Setting{codec, fps, res, pacing}
}

// moonlightAutoSize is the largest size option that fits the cap.
func moonlightAutoSize(capH int) string {
	if capH == 0 {
		return "1080p"
	}
	auto := "720p"
	for _, s := range moonlightSizes {
		if s.h <= capH {
			auto = s.value
		}
	}
	return auto
}

// moonlightSize returns a size option's pixels.
func moonlightSize(value string) (int, int) {
	for _, s := range moonlightSizes {
		if s.value == value {
			return s.w, s.h
		}
	}
	return 1920, 1080
}

// moonlightVideoCfg is moonlight-qt's videocfg value for a codec choice.
func moonlightVideoCfg(codec string) string {
	switch codec {
	case "auto":
		return "0"
	case "hevc":
		return "2"
	case "av1":
		return "4"
	}
	return "1"
}

// --- VacuumTube ----------------------------------------------------------------

// vacuumTubeAutoCodecs is PlanVacuumTube's codec rule as a choice.
func vacuumTubeAutoCodecs(caps Caps, host Host, disp Display) string {
	switch {
	case caps.Probed && caps.Has(AV1):
		return "av1"
	case caps.Probed && caps.Has(VP9):
		return "vp9"
	case caps.Probed && host.High() && disp.Known && disp.Height >= 2160:
		return "av1"
	}
	return "h264"
}

func vacuumTubeCatalog(caps Caps, host Host, disp Display) []Setting {
	codecs := Setting{ID: "codecs", Label: "Video codecs", Auto: vacuumTubeAutoCodecs(caps, host, disp), Options: []Option{
		{Value: "h264", Label: "H.264 only", Note: "Cheapest to decode; YouTube stops at 1080p."},
	}}
	cpu := " VP9 decodes on the CPU here."
	if caps.Has(VP9) {
		cpu = ""
	}
	if caps.Has(VP9) || host.High() {
		codecs.Options = append(codecs.Options, Option{Value: "vp9", Label: "VP9, no AV1", Note: "Up to 4K in VP9; AV1 is blocked." + cpu})
	}
	if caps.Has(AV1) || host.High() {
		note := "YouTube sends its most efficient streams, up to 4K."
		if !caps.Has(AV1) {
			note += " AV1 decodes on the CPU here."
		}
		codecs.Options = append(codecs.Options, Option{Value: "av1", Label: "All (AV1, VP9)", Note: note})
	}
	superRes := Setting{ID: "super_resolution", Label: "AI-upscaled streams", Auto: boolWord(!host.Entry()),
		Options: onOff("YouTube's \"Super resolution\" streams, heavier to decode.", "Skips the AI-upscaled streams; lighter on the decoder.")}
	pause := Setting{ID: "pause_on_blur", Label: "Pause behind Home", Auto: "on",
		Options: onOff("Pauses when Bear Den's Home is in front.", "Keeps playing behind Home (still decoding video nobody sees).")}
	lowMem := Setting{ID: "low_memory", Label: "Low-memory mode", Auto: boolWord(host.LowMemory()),
		Options: onOff("YouTube's own low-memory mode, for 4 GiB boxes.", "YouTube's normal mode.")}
	return []Setting{codecs, superRes, pause, lowMem}
}

// --- Plex HTPC -----------------------------------------------------------------

func plexCatalog(host Host) []Setting {
	scaling := Setting{ID: "scaling", Label: "Video scaling", Auto: "default", Options: []Option{
		{Value: "fast", Label: "Fast", Note: "mpv's fast profile: bilinear scaling, no dithering; lightest on the GPU."},
		{Value: "default", Label: "Standard", Note: "mpv's own defaults."},
	}}
	if host.Entry() {
		scaling.Auto = "fast"
	} else {
		scaling.Options = append(scaling.Options, Option{Value: "high-quality", Label: "High quality", Note: "mpv's high-quality profile: sharper scaling, more GPU work."})
	}
	hwdec := Setting{ID: "hwdec", Label: "Hardware decoding", Auto: "auto-safe", Options: []Option{
		{Value: "auto-safe", Label: "GPU when possible", Note: "Decodes on the GPU/VPU when it can, else on the CPU."},
		{Value: "no", Label: "CPU only", Note: "Every file decodes on the CPU; only to work around a GPU driver problem."},
	}}
	return []Setting{scaling, hwdec}
}
