// Per-app config file plans (Moonlight, Plex mpv, VacuumTube) and their changes
// (docs/APP_PERFORMANCE.md).

package tuning

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Change is one setting the plan wants to change, with the reason shown to
// the owner.
type Change struct {
	Key  string `json:"key"`
	From string `json:"from"`
	To   string `json:"to"`
	Why  string `json:"why"`
}

// Plan is the opinionated settings for one app: the file, the changes and the
// file's new contents. An empty Changes list means the app is already tuned.
type Plan struct {
	App     string   `json:"app"`     // adapter name (plex-htpc, vacuumtube, moonlight)
	Flatpak string   `json:"flatpak"` // Flatpak application id
	File    string   `json:"file"`
	Changes []Change `json:"changes"`
	Notes   []string `json:"notes,omitempty"` // advice Bear Den cannot apply itself
	// Settings are the adjustable settings with the values this plan uses
	// (Catalog, with the owner's overrides resolved).
	Settings []Setting `json:"settings,omitempty"`
	content  []byte
	exists   bool
}

// Paths of each app's settings inside its Flatpak data directory.
func MoonlightConf(home string) string {
	return filepath.Join(home, ".var/app/com.moonlight_stream.Moonlight/config/Moonlight Game Streaming Project/Moonlight.conf")
}
func PlexMpvConf(home string) string {
	return filepath.Join(home, ".var/app/tv.plex.PlexHTPC/data/plex/mpv.conf")
}
func VacuumTubeConf(home string) string {
	return filepath.Join(home, ".var/app/rocks.shy.VacuumTube/config/VacuumTube/config.json")
}

func readOptional(path string) ([]byte, bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return raw, err == nil, err
}

// --- Moonlight ---------------------------------------------------------------

// PlanMoonlight: stream only codecs this box decodes in hardware (the gaming
// PC does the encoding on its GPU), never more pixels or frames than the TV
// output shows, at most 1080p on an entry-level box or an H.264-only decoder,
// fullscreen with vsync and frame pacing for smooth motion. The frame rate
// matches the output's refresh (120 fps on a 120 Hz TV; Expectations warns an
// entry box). Codec, frame rate, resolution and frame pacing follow the
// owner's overrides while they are offered here (Settings). Values are
// moonlight-qt's StreamingPreferences (videocfg: 0 auto, 1 H.264, 2 HEVC,
// 4 AV1; videodec: 0 auto, 1 hardware, 2 software; windowmode: 0 fullscreen).
func PlanMoonlight(path string, caps Caps, host Host, disp Display, ov Overrides) (Plan, error) {
	p := Plan{App: "moonlight", Flatpak: "com.moonlight_stream.Moonlight", File: path}
	settings, notes := Settings("moonlight", caps, host, disp, ov)
	p.Settings = settings
	raw, exists, err := readOptional(path)
	if err != nil {
		return p, err
	}
	if !exists {
		p.Notes = append(p.Notes, "Moonlight has not been opened yet; open it once so it writes its settings, then tune.")
		return p, nil
	}
	p.exists = true
	p.Notes = append(p.Notes, notes...)
	text := string(raw)
	set := func(key, value, why string) {
		var from string
		text, from = iniSet(text, "General", key, value)
		if from != value {
			p.Changes = append(p.Changes, Change{Key: key, From: from, To: value, Why: why})
		}
	}
	manual := func(id string) bool { return overridden(settings, id) }
	// What Moonlight streams now, so its default bitrate can follow below.
	oldW, oldH, oldFPS := atoi(iniGet(text, "General", "width")), atoi(iniGet(text, "General", "height")), atoi(iniGet(text, "General", "fps"))
	if oldW == 0 || oldH == 0 {
		oldW, oldH = 1920, 1080 // Moonlight's defaults
	}
	if oldFPS == 0 {
		oldFPS = 60
	}

	modern := caps.Has(HEVC) || caps.Has(AV1)
	switch {
	case manual("codec"):
		set("videocfg", moonlightVideoCfg(valueOf(settings, "codec")), byHand)
	case !caps.Probed:
		set("videocfg", "1", "Could not check this box's hardware decoders, so ask for H.264: every GPU decodes it and it is the cheapest in software.")
	case caps.Has(H264) && !modern:
		set("videocfg", "1", "This box decodes only H.264 in hardware, so the PC encodes H.264 and the GPU here decodes it (HEVC/AV1 would fall to the CPU).")
	case modern:
		set("videocfg", "0", "This box decodes HEVC/AV1 in hardware: let Moonlight pick the best codec both sides accelerate.")
	default:
		set("videocfg", "1", "No hardware decoder found: H.264 is the cheapest stream to decode in software.")
		p.Notes = append(p.Notes, "No hardware video decoder is available to Moonlight here, so streams decode on the CPU; keep resolution and frame rate modest.")
	}
	set("videodec", "0", "Hardware decoding first, software only as a fallback (never forced to software).")

	// Resolution: the owner's choice, else the largest standard size within
	// the caps that apply (never more than the output, 1080p on an entry box
	// or without HEVC/AV1 hardware).
	w, h := moonlightSize(valueOf(settings, "resolution"))
	whyRes := byHand
	if !manual("resolution") {
		if _, _, whyRes = moonlightCap(caps, host, disp); whyRes == "" {
			whyRes = "1080p: the TV output's size could not be read."
		}
	}
	if w != oldW || h != oldH {
		set("width", fmt.Sprint(w), whyRes)
		set("height", fmt.Sprint(h), whyRes)
	}
	// Frame rate: the owner's choice, else match the TV output (120 fps on a
	// 120 Hz TV) on every box; entry boxes get a note to turn it down if games
	// stutter (Expectations).
	fps := atoi(valueOf(settings, "fps"))
	if fps != oldFPS {
		why := byHand
		if !manual("fps") {
			if hz := moonlightRefresh(disp); hz > 0 {
				why = fmt.Sprintf("Match the TV output's %d Hz: the smoothest motion the screen can show.", hz)
			} else {
				why = "Moonlight's default 60 fps: the TV output's refresh rate could not be read."
			}
		}
		set("fps", fmt.Sprint(fps), why)
	}
	// The bitrate follows the size and frame rate while it is still
	// Moonlight's own default; an owner's own bitrate is kept.
	if bitrate := atoi(iniGet(text, "General", "bitrate")); (w != oldW || h != oldH || fps != oldFPS) &&
		(bitrate == 0 || bitrate == MoonlightDefaultBitrate(oldW, oldH, oldFPS)) {
		set("bitrate", fmt.Sprint(MoonlightDefaultBitrate(w, h, fps)), fmt.Sprintf("Moonlight's own default bitrate for %d×%d at %d fps, so each frame keeps its quality.", w, h, fps))
	}
	set("vsync", "true", "No tearing.")
	switch {
	case valueOf(settings, "framepacing") == "off":
		set("framepacing", "false", byHand)
	case manual("framepacing"):
		set("framepacing", "true", byHand)
	default:
		set("framepacing", "true", "Evens out frame timing for smooth motion on a TV (costs up to one frame of latency).")
	}
	set("windowmode", "0", "Fullscreen, so nothing else is composited over the stream.")
	p.Notes = append(p.Notes, "Encoding happens on the gaming PC: in Sunshine/Apollo keep a hardware encoder (NVENC, AMF or Quick Sync), not software x264.")
	p.content = []byte(text)
	return p, nil
}

// MoonlightDefaultBitrate is moonlight-qt's StreamingPreferences::
// getDefaultBitrate (kbps): a per-resolution factor (interpolated by pixel
// count) times a frame-rate factor that grows with the square root above 60.
func MoonlightDefaultBitrate(w, h, fps int) int {
	table := []struct{ pixels, factor float64 }{
		{640 * 360, 1}, {854 * 480, 2}, {1280 * 720, 5}, {1920 * 1080, 10}, {2560 * 1440, 20}, {3840 * 2160, 40},
	}
	px := float64(w * h)
	res := table[len(table)-1].factor
	for i, e := range table {
		if px <= e.pixels {
			if i == 0 || px == e.pixels {
				res = e.factor
			} else {
				prev := table[i-1]
				res = (px-prev.pixels)/(e.pixels-prev.pixels)*(e.factor-prev.factor) + prev.factor
			}
			break
		}
	}
	f := float64(fps)
	if fps > 60 {
		f = math.Sqrt(f/60) * 60
	}
	return int(math.Round(res*f/30)) * 1000
}

// --- Plex HTPC ---------------------------------------------------------------

const (
	mpvBegin = "# >>> bear-den-tv: playback settings (managed by `bear-den-tv apps detect`)"
	mpvEnd   = "# <<< bear-den-tv"
	// mpvMark finds the block whatever its begin line says (older releases
	// wrote a different comment).
	mpvMark = "# >>> bear-den-tv"
)

// PlanPlex: Plex HTPC plays through mpv and reads mpv.conf before each
// playback (its own mpv.conf.md: hwdec, scalers and dithering set by Plex can
// be overridden there). Decode on the GPU/VPU when possible, any vendor, with
// a safe software fallback; on an entry-level box add mpv's "fast" profile
// (cheap scaling, no dithering extras; mpv 0.37+, Plex HTPC ships 0.38), and
// everywhere else keep mpv's full-quality defaults. The owner may pick the
// scaling profile (fast, mpv's default, or high-quality on a capable box) and
// turn hardware decoding off (Settings).
func PlanPlex(path string, caps Caps, host Host, ov Overrides) (Plan, error) {
	p := Plan{App: "plex-htpc", Flatpak: "tv.plex.PlexHTPC", File: path}
	settings, notes := Settings("plex-htpc", caps, host, Display{}, ov)
	p.Settings = settings
	p.Notes = append(p.Notes, notes...)
	raw, exists, err := readOptional(path)
	if err != nil {
		return p, err
	}
	p.exists = exists
	var lines []string
	var why []string
	if valueOf(settings, "hwdec") == "no" {
		lines = append(lines, "hwdec=no                 # chosen by hand: decode on the CPU")
		why = append(why, "Software decoding, chosen by hand")
	} else {
		lines = append(lines, "hwdec=auto-safe          # decode on the GPU/VPU when it can (VA-API, NVDEC, V4L2, …), else on the CPU")
		why = append(why, "Hardware decoding (any GPU vendor) with a safe software fallback")
	}
	lines = append(lines, "video-sync=audio", "interpolation=no")
	switch scaling := valueOf(settings, "scaling"); {
	case scaling == "fast" && !overridden(settings, "scaling"):
		lines = append(lines, "profile=fast             # entry-level box: bilinear scaling, no dithering or peak detection")
		why = append(why, "mpv's fast profile keeps scaling cheap on an entry-level GPU")
	case scaling == "fast":
		lines = append(lines, "profile=fast             # chosen by hand: bilinear scaling, no dithering or peak detection")
		why = append(why, "mpv's fast profile, chosen by hand")
	case scaling == "high-quality":
		lines = append(lines, "profile=high-quality     # chosen by hand: mpv's sharper, heavier scaling")
		why = append(why, "mpv's high-quality profile, chosen by hand")
	case overridden(settings, "scaling"):
		why = append(why, "mpv's own default scaling, chosen by hand")
	default:
		why = append(why, "mpv's own full-quality scaling, which this box has the headroom for")
	}
	text, from := mpvSetBlock(string(raw), lines)
	if text != string(raw) {
		p.Changes = append(p.Changes, Change{Key: "mpv.conf (Bear Den block)", From: from, To: strings.Join(lines, "; "), Why: strings.Join(why, "; ") + "."})
	}
	var hw []string
	for _, c := range Codecs {
		if caps.Has(c) {
			hw = append(hw, strings.ToUpper(string(c)))
		}
	}
	if caps.Probed && !(caps.Has(HEVC) && caps.Has(VP9) && caps.Has(AV1)) {
		p.Notes = append(p.Notes, fmt.Sprintf("This box decodes %s in hardware. For files in other codecs, let the Plex server do the work: play them at a quality below Original (the server transcodes to H.264), or make H.264 \"Optimized versions\" on the server.", orNone(hw)))
	}
	p.content = []byte(text)
	return p, nil
}

// --- VacuumTube (YouTube) ------------------------------------------------------

// PlanVacuumTube: let YouTube send only codecs this box decodes in hardware
// (VacuumTube's codec filter, like h264ify), unless a desktop-class CPU can
// decode the 4K streams a 4K screen deserves; pause when Bear Den's Home is in
// front; skip AI-upscaled streams on an entry-level box; YouTube's low-memory
// mode on 4 GiB machines. The owner may choose each of these by hand
// (Settings), among the options this box can handle.
func PlanVacuumTube(path string, caps Caps, host Host, disp Display, ov Overrides) (Plan, error) {
	p := Plan{App: "vacuumtube", Flatpak: "rocks.shy.VacuumTube", File: path}
	settings, notes := Settings("vacuumtube", caps, host, disp, ov)
	p.Settings = settings
	raw, exists, err := readOptional(path)
	if err != nil {
		return p, err
	}
	if !exists {
		p.Notes = append(p.Notes, "VacuumTube has not been opened yet; open it once so it writes its settings, then tune.")
		return p, nil
	}
	p.exists = true
	p.Notes = append(p.Notes, notes...)
	obj, err := parseOrdered(raw)
	if err != nil {
		return p, fmt.Errorf("vacuumtube settings: %w", err)
	}
	set := func(key string, value bool, why string) {
		from := obj.get(key)
		to := fmt.Sprint(value)
		if from != to {
			obj.set(key, value)
			p.Changes = append(p.Changes, Change{Key: key, From: from, To: to, Why: why})
		}
	}
	set("hardware_decoding", true, "Decode on the GPU whenever the runtime can.")
	manualCodecs := overridden(settings, "codecs")
	switch codecs := valueOf(settings, "codecs"); {
	case codecs == "av1" && manualCodecs:
		set("h264ify", false, byHand)
	case codecs == "av1" && caps.Has(AV1):
		set("h264ify", false, "This box decodes AV1 in hardware: let YouTube send its most efficient streams.")
	case codecs == "av1":
		set("h264ify", false, "The screen is 4K and this desktop-class CPU can decode YouTube's 4K VP9/AV1 itself; H.264 would stop at 1080p.")
	case codecs == "vp9":
		why := func(auto string) string {
			if manualCodecs {
				return byHand
			}
			return auto
		}
		set("h264ify", true, why("Block only the codecs this box cannot decode in hardware."))
		set("h264ify_disable_vp9", false, why("VP9 decodes in hardware here."))
		set("h264ify_disable_webm", false, why("VP9 arrives in WebM."))
		set("h264ify_disable_av1", true, why("AV1 would decode on the CPU here."))
		set("h264ify_disable_vp8", true, why("VP8 is never hardware-decoded."))
	default:
		why := "No VP9/AV1 hardware decoding here, so ask YouTube for H.264: the cheapest codec on the CPU (and the one GPUs decode most widely)."
		if manualCodecs {
			why = byHand
		}
		set("h264ify", true, why)
		set("h264ify_disable_webm", true, why)
		set("h264ify_disable_vp8", true, why)
		set("h264ify_disable_vp9", true, why)
		set("h264ify_disable_av1", true, why)
	}
	if caps.Probed && len(caps.Decoders) == 0 {
		p.Notes = append(p.Notes, "VacuumTube's runtime has no hardware video decoder for this GPU, so YouTube decodes on the CPU; H.264 keeps that affordable up to 1080p.")
	}
	// Switches VacuumTube leaves out of its file are off; they are written
	// when they must change (on, or back to off after an override).
	setSwitch := func(key string, value bool, why string) {
		if !value && obj.get(key) == "" {
			return
		}
		set(key, value, why)
	}
	reason := func(id, auto string) string {
		if overridden(settings, id) {
			return byHand
		}
		return auto
	}
	if valueOf(settings, "super_resolution") == "on" {
		setSwitch("remove_super_resolution", false, reason("super_resolution", "This box has the headroom for YouTube's AI-upscaled streams."))
	} else {
		setSwitch("remove_super_resolution", true, reason("super_resolution", "Skips YouTube's AI-upscaled \"Super resolution\" streams, which are heavier to decode."))
	}
	if valueOf(settings, "low_memory") == "on" {
		setSwitch("low_memory_mode", true, reason("low_memory", "4 GiB of RAM or less: YouTube's own low-memory mode."))
	} else {
		setSwitch("low_memory_mode", false, reason("low_memory", "More than 4 GiB of RAM: YouTube's normal mode."))
	}
	set("pause_on_blur", valueOf(settings, "pause_on_blur") == "on", reason("pause_on_blur", "Pauses when Bear Den's Home is in front, instead of decoding video nobody sees."))
	set("fullscreen", true, "Starts fullscreen (Bear Den also enforces it).")
	out, err := obj.marshal()
	if err != nil {
		return p, err
	}
	p.content = out
	return p, nil
}

// Apply writes a plan: backs up the current file (<file>.bak-bear-den-<time>)
// and replaces it atomically. It never runs with no changes.
func Apply(p Plan, now time.Time) (backup string, err error) {
	if len(p.Changes) == 0 {
		return "", nil
	}
	if err := os.MkdirAll(filepath.Dir(p.File), 0o755); err != nil {
		return "", err
	}
	mode := os.FileMode(0o644)
	if p.exists {
		st, err := os.Stat(p.File)
		if err != nil {
			return "", err
		}
		mode = st.Mode().Perm()
		old, err := os.ReadFile(p.File)
		if err != nil {
			return "", err
		}
		backup = p.File + ".bak-bear-den-" + now.Format("20060102-150405")
		if err := os.WriteFile(backup, old, mode); err != nil {
			return "", err
		}
	}
	tmp := p.File + ".bear-den.tmp"
	if err := os.WriteFile(tmp, p.content, mode); err != nil {
		return backup, err
	}
	return backup, os.Rename(tmp, p.File)
}

// --- Small editors -------------------------------------------------------------

func atoi(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func orNone(list []string) string {
	if len(list) == 0 {
		return "no codec"
	}
	return strings.Join(list, ", ")
}

// iniGet returns key's value in [section] of a QSettings-style INI text.
func iniGet(text, section, key string) string {
	in := false
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			in = t == "["+section+"]"
			continue
		}
		if in {
			if k, v, ok := strings.Cut(t, "="); ok && strings.TrimSpace(k) == key {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// iniSet sets key=value in [section], keeping every other line as it was; the
// key is appended to the section (created at the top if missing). It returns
// the new text and the previous value ("" when absent).
func iniSet(text, section, key, value string) (string, string) {
	lines := strings.Split(text, "\n")
	start, end := -1, len(lines)
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if start >= 0 {
				end = i
				break
			}
			if t == "["+section+"]" {
				start = i
			}
		}
	}
	if start < 0 {
		head := []string{"[" + section + "]", key + "=" + value, ""}
		return strings.Join(append(head, lines...), "\n"), ""
	}
	last := start
	for i := start + 1; i < end; i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			continue
		}
		last = i
		if k, v, ok := strings.Cut(t, "="); ok && strings.TrimSpace(k) == key {
			lines[i] = key + "=" + value
			return strings.Join(lines, "\n"), strings.TrimSpace(v)
		}
	}
	out := append([]string{}, lines[:last+1]...)
	out = append(out, key+"="+value)
	out = append(out, lines[last+1:]...)
	return strings.Join(out, "\n"), ""
}

// mpvSetBlock replaces (or appends) Bear Den's marked block in an mpv.conf and
// returns the new text and the old block's settings (joined), if any.
func mpvSetBlock(text string, lines []string) (string, string) {
	block := mpvBegin + "\n" + strings.Join(lines, "\n") + "\n" + mpvEnd + "\n"
	if i := strings.Index(text, mpvMark); i >= 0 {
		head := strings.IndexByte(text[i:], '\n')
		if j := strings.Index(text[i:], mpvEnd); j >= 0 && head >= 0 && head < j {
			old := strings.TrimSpace(text[i+head : i+j])
			rest := strings.TrimPrefix(text[i+j+len(mpvEnd):], "\n")
			return text[:i] + block + rest, strings.Join(strings.Split(old, "\n"), "; ")
		}
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" {
		text += "\n"
	}
	return text + block, ""
}

// ordered is a JSON object that keeps its keys' order and formatting style.
type ordered struct {
	keys   []string
	values map[string]json.RawMessage
	indent string
}

func parseOrdered(raw []byte) (*ordered, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("not a JSON object")
	}
	o := &ordered{values: map[string]json.RawMessage{}, indent: detectIndent(raw)}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := kt.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		if _, dup := o.values[key]; !dup {
			o.keys = append(o.keys, key)
		}
		o.values[key] = v
	}
	return o, nil
}

func detectIndent(raw []byte) string {
	for _, line := range strings.Split(string(raw), "\n")[1:] {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed != line && trimmed != "" {
			return line[:len(line)-len(trimmed)]
		}
	}
	return "  "
}

func (o *ordered) get(key string) string {
	v, ok := o.values[key]
	if !ok {
		return ""
	}
	return string(v)
}

func (o *ordered) set(key string, value any) {
	b, _ := json.Marshal(value)
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = b
}

func (o *ordered) marshal() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("{\n")
	for i, k := range o.keys {
		kb, _ := json.Marshal(k)
		var v bytes.Buffer
		if err := json.Indent(&v, o.values[k], o.indent, o.indent); err != nil {
			return nil, err
		}
		buf.WriteString(o.indent)
		buf.Write(kb)
		buf.WriteString(": ")
		buf.Write(v.Bytes())
		if i < len(o.keys)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}
