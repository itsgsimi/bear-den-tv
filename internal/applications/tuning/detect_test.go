// Tests for display detection and tuning reports (detect.go).

package tuning

import (
	"os"
	"strings"
	"testing"
	"time"
)

// testdata/xrandr-reference-box.txt is `xrandr --current` from the reference box:
// a 4K LG TV offering 4K only up to 30 Hz, driven at 1920×1080 120 Hz.
func TestParseXrandrReadsModeAndOffers(t *testing.T) {
	raw, err := os.ReadFile("testdata/xrandr-reference-box.txt")
	if err != nil {
		t.Fatal(err)
	}
	d := ParseXrandr(raw)
	if !d.Known || d.Output != "HDMI-1" || d.Width != 1920 || d.Height != 1080 || d.Refresh != 120 {
		t.Fatalf("display = %+v", d)
	}
	if !d.UHD || d.UHDMaxHz != 30 || d.BestSmallHz != 120 {
		t.Fatalf("offers = %+v", d)
	}
	if ParseXrandr([]byte("Screen 0: minimum 8 x 8\nHDMI-1 disconnected\n")).Known {
		t.Fatal("no connected output means unknown")
	}
}

func TestCaveatsForTheReferenceBox(t *testing.T) {
	raw, _ := os.ReadFile("testdata/xrandr-reference-box.txt")
	disp := ParseXrandr(raw)
	host := small
	joined := joinNotes(HostNotes(host, disp))
	for _, want := range []string{"info:The TV output runs at 120 Hz", "4K output here tops out at 30 Hz"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("host notes missing %q:\n%s", want, joined)
		}
	}
	// 120 Hz is no burden for a modern small box: no note about it.
	if n := joinNotes(HostNotes(standard, disp)); strings.Contains(n, "120 Hz") {
		t.Fatalf("a standard box at 120 Hz needs no caveat:\n%s", n)
	}
	// Moonlight on a 120 Hz TV: suggest 120 fps; the floor box also gets the check-your-stats advice.
	_, ml := Expectations("moonlight", ParseDecoders([]byte(haswellInspect)), host, disp)
	if n := joinNotes(ml); !strings.Contains(n, "warn:Moonlight streams at 120 fps") || !strings.Contains(n, "lower the frame rate to 60 fps") {
		t.Fatalf("moonlight notes:\n%s", n)
	}
	if _, ml := Expectations("moonlight", ParseDecoders([]byte(modernInspect)), standard, disp); strings.Contains(joinNotes(ml), "statistics overlay") {
		t.Fatal("only entry-level boxes get the dropped-frames advice")
	}
	// YouTube with no hardware decoding: 1080p H.264 on the CPU, no 4K/HDR, stutter advice.
	expect, yt := Expectations("vacuumtube", Caps{Probed: true, Hardware: map[Codec]bool{}}, host, disp)
	if !strings.Contains(expect[0], "1080p in H.264") || len(yt) < 2 || yt[0].Level != "warn" {
		t.Fatalf("youtube: %v %v", expect, yt)
	}
	// Hardware H.264 but no VP9/AV1: 1080p on the GPU, and no CPU warnings.
	h264 := Caps{Probed: true, Hardware: map[Codec]bool{H264: true}, Decoders: []string{"vah264dec"}}
	if expect, yt := Expectations("vacuumtube", h264, host, disp); !strings.Contains(expect[0], "decoded on the GPU") || len(yt) != 0 {
		t.Fatalf("h264-only youtube: %v %v", expect, yt)
	}
	// A modern box: 4K AV1 on a 4K display, no warnings.
	expect, yt = Expectations("vacuumtube", ParseDecoders([]byte(modernInspect)), standard, uhd60)
	if !strings.Contains(expect[0], "Up to 4K") || len(yt) != 0 {
		t.Fatalf("modern youtube: %v %v", expect, yt)
	}
	if n := HostNotes(standard, uhd60); len(n) != 0 {
		t.Fatalf("a 60 Hz 4K modern box needs no caveats: %v", n)
	}
	// A desktop without VP9/AV1 hardware still gets 4K YouTube, on its CPU.
	if expect, _ := Expectations("vacuumtube", Caps{Probed: true, Hardware: map[Codec]bool{}}, big, uhd60); !strings.Contains(expect[0], "Up to 4K") {
		t.Fatalf("desktop youtube: %v", expect)
	}
	// A capable box on a 4K60 TV driven at 1080p is told it can go 4K; the floor box is told to stay.
	tv := Display{Known: true, Width: 1920, Height: 1080, Refresh: 60, UHD: true, UHDMaxHz: 60}
	if n := joinNotes(HostNotes(standard, tv)); !strings.Contains(n, "can drive it") {
		t.Fatalf("standard box on a 4K60 TV: %s", n)
	}
	if n := joinNotes(HostNotes(host, tv)); !strings.Contains(n, "keep 1080p") {
		t.Fatalf("entry box on a 4K60 TV: %s", n)
	}
	// Plex: HEVC not in hardware → let the server convert.
	_, px := Expectations("plex-htpc", ParseDecoders([]byte(haswellInspect)), host, disp)
	if len(px) == 0 || !strings.Contains(px[0].Text, "HEVC") {
		t.Fatalf("plex caveat: %v", px)
	}
}

func TestApplyReadyAndPendingUntilClosed(t *testing.T) {
	home := t.TempDir()
	conf := VacuumTubeConf(home)
	if err := os.MkdirAll(conf[:strings.LastIndex(conf, "/")], 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conf, []byte(vacuumConf), 0o600); err != nil {
		t.Fatal(err)
	}
	caps := Caps{Probed: true, Hardware: map[Codec]bool{}}
	plan, _ := PlanVacuumTube(conf, caps, Host{Cores: 2}, Display{}, nil)
	d := &Detector{Home: home, Now: func() time.Time { return time.Unix(0, 0) }}
	r := Report{Host: Host{Cores: 2}, Apps: []AppReport{{Label: "YouTube", Adapter: "vacuumtube", FlatpakID: "rocks.shy.VacuumTube", Caps: caps, Plan: &plan, Running: true}}}
	d.ApplyReady(&r)
	if r.Apps[0].Status != "pending" {
		t.Fatalf("a running app waits: %+v", r.Apps[0])
	}
	if cur, _ := os.ReadFile(conf); string(cur) != vacuumConf {
		t.Fatal("nothing may be written while the app runs")
	}
	a, ok := d.ApplyApp(&r, "rocks.shy.VacuumTube")
	if !ok || a.Status != "applied" || a.Backup == "" {
		t.Fatalf("after closing: %+v", a)
	}
	if cur, _ := os.ReadFile(conf); !strings.Contains(string(cur), `"pause_on_blur": true`) {
		t.Fatalf("settings not written:\n%s", cur)
	}
	if _, again := d.ApplyApp(&r, "rocks.shy.VacuumTube"); again {
		t.Fatal("only pending apps are applied")
	}
}

func joinNotes(notes []Note) string {
	out := ""
	for _, n := range notes {
		out += n.Level + ":" + n.Text + "\n"
	}
	return out
}
