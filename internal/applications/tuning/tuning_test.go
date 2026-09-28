// Tests for hardware detection, box tiers and per-app plans (tuning.go,
// plans.go).

package tuning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gst-inspect-1.0 output shaped like a Haswell box's Plex runtime, plus
// software decoders that must never count as hardware.
const haswellInspect = `va:  vah264dec: VA-API H.264 Decoder in Intel(R) Haswell Mobile
va:  vajpegdec: VA-API JPEG Decoder in Intel(R) Haswell Mobile
va:  vampeg2dec: VA-API Mpeg2 Decoder in Intel(R) Haswell Mobile
libav:  avdec_h264: libav H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10 decoder
vpx:  vp9dec: On2 VP9 Decoder
dav1d:  dav1ddec: dav1d AV1 decoder
aom:  av1dec: AV1 Decoder
vulkan:  vulkanh264dec: Vulkan H.264 decoder
vulkan:  vulkanh265dec: Vulkan H.265 decoder
`

const modernInspect = `va:  vah264dec: VA-API H.264 Decoder
va:  vah265dec: VA-API H.265 Decoder
va:  vavp9dec: VA-API VP9 Decoder
va:  vaav1dec: VA-API AV1 Decoder
nvcodec:  nvh264dec: NVDEC H.264 Decoder
v4l2codecs:  v4l2slh265dec: V4L2 Stateless H.265 Video Decoder
`

func TestParseDecodersIsHardwareOnlyAndVendorAgnostic(t *testing.T) {
	c := ParseDecoders([]byte(haswellInspect))
	if !c.Has(H264) || c.Has(HEVC) || c.Has(VP9) || c.Has(AV1) {
		t.Fatalf("haswell caps = %+v", c.Hardware)
	}
	if strings.Join(c.Decoders, ",") != "vah264dec" {
		t.Fatalf("decoders = %v (software and Vulkan decoders must not count)", c.Decoders)
	}
	m := ParseDecoders([]byte(modernInspect))
	for _, codec := range Codecs {
		if !m.Has(codec) {
			t.Fatalf("modern box should decode %s: %+v", codec, m.Hardware)
		}
	}
	if none := ParseDecoders([]byte("coreelements:  queue: Queue\n")); len(none.Decoders) != 0 || none.Has(H264) {
		t.Fatalf("no decoders expected: %+v", none)
	}
}

// The reference box (Celeron 2955U) is the floor; an N100 mini PC is a modern
// small box; big is a desktop.
var (
	small    = Host{Cores: 2, ClockGHz: 1.4, TotalGHz: 2.8, MemGiB: 7.6}
	tiny     = Host{Cores: 2, ClockGHz: 1.4, TotalGHz: 2.8, MemGiB: 3.8}
	standard = Host{Cores: 4, ClockGHz: 3.4, TotalGHz: 13.6, MemGiB: 8}
	big      = Host{Cores: 16, ClockGHz: 4.4, TotalGHz: 60, MemGiB: 32}
)

// refDisplay is the reference box's TV output: 1080p at 120 Hz on a 4K TV that
// offers 4K only at 30 Hz. uhd60 is a 4K TV driven at 4K.
var (
	refDisplay = Display{Known: true, Output: "HDMI-1", Width: 1920, Height: 1080, Refresh: 120, UHD: true, UHDMaxHz: 30, BestSmallHz: 120}
	uhd60      = Display{Known: true, Output: "HDMI-1", Width: 3840, Height: 2160, Refresh: 60, UHD: true, UHDMaxHz: 60, BestSmallHz: 120}
)

func TestClassifyScalesFromTheFloorUp(t *testing.T) {
	for _, c := range []struct {
		name  string
		cores int
		ghz   float64
		want  Tier
	}{
		{"Celeron 2955U (reference box)", 2, 2.8, Entry},
		{"any 2-core box", 2, 7.6, Entry},
		{"Atom x5-Z8350", 4, 7.68, Entry},
		{"Raspberry Pi 5", 4, 9.6, Standard},
		{"Celeron J4125", 4, 10.8, Standard},
		{"Intel N100", 4, 13.6, Standard},
		{"RK3588 (4 big + 4 little cores)", 8, 16.8, Standard},
		{"4-core/8-thread laptop", 8, 27.2, High},
		{"clock unknown, 4 cores", 4, 0, Standard},
		{"clock unknown, 8 cores", 8, 0, High},
	} {
		if got := Classify(c.cores, c.ghz); got != c.want {
			t.Errorf("%s: Classify(%d, %.1f) = %s, want %s", c.name, c.cores, c.ghz, got, c.want)
		}
	}
	if (Host{Cores: 16, TotalGHz: 60, Tier: Entry}).Class() != Entry {
		t.Fatal("an explicit tier (BDTV_TIER, --tier) wins over the classification")
	}
	if d := small.Describe(); !strings.Contains(d, "2 CPU cores up to 1.4 GHz") || !strings.Contains(d, "entry level") {
		t.Fatalf("describe = %q", d)
	}
}

func TestDetectHostHonoursTheOwnersTier(t *testing.T) {
	t.Setenv("BDTV_TIER", "entry")
	if h := DetectHost(); h.Tier != Entry || h.Cores < 1 {
		t.Fatalf("BDTV_TIER=entry: %+v", h)
	}
	t.Setenv("BDTV_TIER", "turbo")
	if h := DetectHost(); h.Tier != Classify(h.Cores, h.TotalGHz) {
		t.Fatalf("an unknown BDTV_TIER is ignored: %+v", h)
	}
}

func TestCPUClocksReadsCpufreqThenCpuinfo(t *testing.T) {
	sys := t.TempDir()
	for i, khz := range []string{"3400000", "3400000", "2400000", "2400000"} {
		dir := filepath.Join(sys, "cpu"+string(rune('0'+i)), "cpufreq")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "cpuinfo_max_freq"), []byte(khz+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if top, total := cpuClocks(sys, "/nonexistent"); top != 3.4 || total < 11.59 || total > 11.61 {
		t.Fatalf("cpufreq: top %.2f total %.2f", top, total)
	}
	info := write(t, "cpuinfo", "processor\t: 0\ncpu MHz\t\t: 2400.000\nprocessor\t: 1\ncpu MHz\t\t: 2400.000\n")
	if top, total := cpuClocks(t.TempDir(), info); top != 2.4 || total != 4.8 {
		t.Fatalf("cpuinfo fallback: top %.2f total %.2f", top, total)
	}
	if top, total := cpuClocks(t.TempDir(), "/nonexistent"); top != 0 || total != 0 {
		t.Fatalf("unknown clocks must be 0: %.2f %.2f", top, total)
	}
}

func write(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func changed(p Plan) map[string]string {
	out := map[string]string{}
	for _, c := range p.Changes {
		out[c.Key] = c.To
	}
	return out
}

const moonlightConf = "[General]\nbitrate=20000\nfps=120\nheight=2160\nvideocfg=0\nvideodec=2\nvsync=false\nwidth=3840\nwindowmode=2\n\n[hosts]\n1\\localport=47989\n"

func TestMoonlightStreamsOnlyWhatDecodesInHardware(t *testing.T) {
	path := write(t, "Moonlight.conf", moonlightConf)
	p, err := PlanMoonlight(path, ParseDecoders([]byte(haswellInspect)), small, refDisplay, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"videocfg": "1", "videodec": "0", "width": "1920", "height": "1080", "vsync": "true", "framepacing": "true", "windowmode": "0"}
	got := changed(p)
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	if _, capped := got["fps"]; capped {
		t.Fatalf("120 fps on a 120 Hz output is the owner's call, even on the floor box: %v", got)
	}
	// Moonlight's 60 fps default on a 120 Hz TV: 120 fps, bitrate raised with it.
	at60 := write(t, "at60.conf", "[General]\nbitrate=20000\nfps=60\nheight=1080\nwidth=1920\n")
	if got := changed(must(PlanMoonlight(at60, ParseDecoders([]byte(haswellInspect)), small, refDisplay, nil))); got["fps"] != "120" || got["bitrate"] != "28000" {
		t.Fatalf("120 Hz TV streams at 120 fps: %v", got)
	}
	custom := write(t, "custom.conf", "[General]\nbitrate=35000\nfps=60\nheight=1080\nwidth=1920\n")
	if got := changed(must(PlanMoonlight(custom, ParseDecoders([]byte(haswellInspect)), small, refDisplay, nil))); got["fps"] != "120" || got["bitrate"] != "" {
		t.Fatalf("an owner's own bitrate is kept: %v", got)
	}
	sixty := refDisplay
	sixty.Refresh = 60
	if got := changed(must(PlanMoonlight(path, ParseDecoders([]byte(haswellInspect)), small, sixty, nil))); got["fps"] != "60" {
		t.Fatalf("never more frames than a 60 Hz output shows: %v", got)
	}
	text := string(p.content)
	if !strings.Contains(text, "[hosts]\n1\\localport=47989") || !strings.Contains(text, "bitrate=20000") {
		t.Fatalf("other settings must be kept:\n%s", text)
	}
	if strings.Index(text, "framepacing=true") > strings.Index(text, "[hosts]") {
		t.Fatalf("new keys belong in [General]:\n%s", text)
	}
	// A box that decodes HEVC/AV1 in hardware lets Moonlight choose, and a
	// capable one streams 4K120 to a 4K 120 Hz TV untouched.
	uhd120 := uhd60
	uhd120.Refresh = 120
	for _, host := range []Host{standard, big} {
		if got := changed(must(PlanMoonlight(path, ParseDecoders([]byte(modernInspect)), host, uhd120, nil))); got["videocfg"] != "" || got["fps"] != "" || got["height"] != "" {
			t.Fatalf("%s box: no caps on a 4K 120 Hz TV: %v", host.Class(), got)
		}
	}
	// … but never more pixels than the output shows, on any box.
	if got := changed(must(PlanMoonlight(path, ParseDecoders([]byte(modernInspect)), big, refDisplay, nil))); got["height"] != "1080" || got["width"] != "1920" {
		t.Fatalf("4K stream to a 1080p output: %v", got)
	}
	// The floor box keeps 1080p even when its TV runs 4K.
	if got := changed(must(PlanMoonlight(path, ParseDecoders([]byte(modernInspect)), small, uhd60, nil))); got["height"] != "1080" {
		t.Fatalf("entry box on a 4K output: %v", got)
	}
}

func must(p Plan, err error) Plan {
	if err != nil {
		panic(err)
	}
	return p
}

func TestPlexGetsAManagedMpvBlockOnce(t *testing.T) {
	path := write(t, "mpv.conf", "# user line\nvolume-max=150\n")
	p, err := PlanPlex(path, ParseDecoders([]byte(haswellInspect)), small, nil)
	if err != nil || len(p.Changes) != 1 {
		t.Fatalf("plan = %+v, %v", p, err)
	}
	text := string(p.content)
	for _, want := range []string{"# user line", "volume-max=150", mpvBegin, "hwdec=auto-safe", "profile=fast", mpvEnd} {
		if !strings.Contains(text, want) {
			t.Fatalf("mpv.conf missing %q:\n%s", want, text)
		}
	}
	if len(p.Notes) == 0 || !strings.Contains(p.Notes[0], "H264") {
		t.Fatalf("expected a note about letting the server transcode: %v", p.Notes)
	}
	// Applying and planning again changes nothing; a big box drops profile=fast in place.
	if _, err := Apply(p, time.Now()); err != nil {
		t.Fatal(err)
	}
	again, _ := PlanPlex(path, ParseDecoders([]byte(haswellInspect)), small, nil)
	if len(again.Changes) != 0 {
		t.Fatalf("second plan should be empty: %+v", again.Changes)
	}
	for _, host := range []Host{standard, big} {
		plan, _ := PlanPlex(path, ParseDecoders([]byte(haswellInspect)), host, nil)
		if strings.Contains(string(plan.content), "profile=fast") || strings.Count(string(plan.content), mpvBegin) != 1 {
			t.Fatalf("%s box: full-quality scaling, block replaced not duplicated:\n%s", host.Class(), plan.content)
		}
	}
	// A block written by an older release (another begin comment) is replaced.
	legacy := write(t, "legacy.conf", "volume-max=150\n\n# >>> bear-den-tv: small-device playback (managed by `bear-den-tv apps tune`)\nhwdec=auto-safe\nprofile=fast\n# <<< bear-den-tv\nsub-scale=1.2\n")
	lp, _ := PlanPlex(legacy, Caps{Probed: true}, standard, nil)
	if text := string(lp.content); strings.Count(text, "# >>> bear-den-tv") != 1 || strings.Contains(text, "profile=fast") || !strings.Contains(text, "sub-scale=1.2") {
		t.Fatalf("legacy block must be replaced in place:\n%s", text)
	}
}

const vacuumConf = `{
    "volume": 100,
    "h264ify": false,
    "hardware_decoding": false,
    "sponsorblock_uuid": "abc",
    "disabled_userstyles": [],
    "pause_on_blur": false
}
`

func TestVacuumTubeFiltersCodecsByHardwareAndKeepsOrder(t *testing.T) {
	path := write(t, "config.json", vacuumConf)
	p, err := PlanVacuumTube(path, Caps{Probed: true, Hardware: map[Codec]bool{}}, tiny, refDisplay, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := changed(p)
	for k, v := range map[string]string{"hardware_decoding": "true", "h264ify": "true", "h264ify_disable_vp9": "true", "h264ify_disable_av1": "true", "pause_on_blur": "true", "remove_super_resolution": "true", "low_memory_mode": "true", "fullscreen": "true"} {
		if got[k] != v {
			t.Fatalf("%s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	text := string(p.content)
	if !strings.HasPrefix(text, "{\n    \"volume\": 100,\n    \"h264ify\": true,") || !strings.Contains(text, `"sponsorblock_uuid": "abc"`) {
		t.Fatalf("order, indentation and unknown keys must be kept:\n%s", text)
	}
	// AV1 in hardware: let YouTube send its efficient codecs.
	av1, _ := PlanVacuumTube(path, ParseDecoders([]byte(modernInspect)), standard, uhd60, nil)
	if got := changed(av1); got["h264ify"] != "" || got["low_memory_mode"] != "" || got["remove_super_resolution"] != "" {
		t.Fatalf("modern small box: h264ify stays off, no entry-level settings: %v", got)
	}
	// VP9 only: block just AV1.
	vp9 := Caps{Probed: true, Hardware: map[Codec]bool{H264: true, VP9: true}, Decoders: []string{"vah264dec", "vavp9dec"}}
	v, _ := PlanVacuumTube(path, vp9, small, refDisplay, nil)
	if got := changed(v); got["h264ify_disable_vp9"] != "false" || got["h264ify_disable_av1"] != "true" || got["low_memory_mode"] != "" {
		t.Fatalf("vp9 box: %v", got)
	}
	// No VP9/AV1 hardware on a 4K screen: a desktop decodes 4K itself, a
	// small box asks for H.264 (1080p) instead.
	none := Caps{Probed: true, Hardware: map[Codec]bool{}}
	if got := changed(must(PlanVacuumTube(path, none, big, uhd60, nil))); got["h264ify"] != "" {
		t.Fatalf("desktop on a 4K screen keeps VP9/AV1: %v", got)
	}
	if got := changed(must(PlanVacuumTube(path, none, standard, uhd60, nil))); got["h264ify"] != "true" || got["remove_super_resolution"] != "" {
		t.Fatalf("small box without VP9 hardware: H.264, but no entry-level extras: %v", got)
	}
	if got := changed(must(PlanVacuumTube(path, none, big, refDisplay, nil))); got["h264ify"] != "true" {
		t.Fatalf("on a 1080p screen H.264 fills it for less work, even on a desktop: %v", got)
	}
}

func TestApplyBacksUpAndIsAtomic(t *testing.T) {
	path := write(t, "config.json", vacuumConf)
	p, _ := PlanVacuumTube(path, Caps{Probed: true, Hardware: map[Codec]bool{}}, small, refDisplay, nil)
	now := time.Date(2026, 9, 22, 21, 0, 0, 0, time.UTC)
	backup, err := Apply(p, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(backup, ".bak-bear-den-20260922-210000") {
		t.Fatalf("backup = %s", backup)
	}
	if old, _ := os.ReadFile(backup); string(old) != vacuumConf {
		t.Fatalf("backup must hold the original:\n%s", old)
	}
	if cur, _ := os.ReadFile(path); !strings.Contains(string(cur), `"pause_on_blur": true`) {
		t.Fatalf("file not updated:\n%s", cur)
	}
	if _, err := os.Stat(path + ".bear-den.tmp"); !os.IsNotExist(err) {
		t.Fatal("temporary file left behind")
	}
	if b, err := Apply(Plan{File: path}, now); err != nil || b != "" {
		t.Fatalf("empty plan must not touch anything: %q %v", b, err)
	}
}

func TestMissingSettingsFilesAreReportedNotCreated(t *testing.T) {
	dir := t.TempDir()
	m, err := PlanMoonlight(filepath.Join(dir, "none.conf"), Caps{}, small, Display{}, nil)
	if err != nil || len(m.Changes) != 0 || len(m.Notes) == 0 {
		t.Fatalf("moonlight: %+v %v", m, err)
	}
	v, err := PlanVacuumTube(filepath.Join(dir, "none.json"), Caps{}, small, Display{}, nil)
	if err != nil || len(v.Changes) != 0 || len(v.Notes) == 0 {
		t.Fatalf("vacuumtube: %+v %v", v, err)
	}
}

func TestMoonlightDefaultBitrateMatchesMoonlight(t *testing.T) {
	for _, c := range []struct{ w, h, fps, want int }{
		{1920, 1080, 60, 20000}, {1280, 720, 60, 10000}, {3840, 2160, 60, 80000}, {1920, 1080, 120, 28000}, {1920, 1080, 30, 10000},
	} {
		if got := MoonlightDefaultBitrate(c.w, c.h, c.fps); got != c.want {
			t.Errorf("%dx%d@%d = %d, want %d", c.w, c.h, c.fps, got, c.want)
		}
	}
}
