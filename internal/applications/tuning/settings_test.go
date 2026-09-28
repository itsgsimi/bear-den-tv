// Tests for the playback settings catalog and overrides (settings.go).

package tuning

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func settingByID(t *testing.T, list []Setting, id string) Setting {
	t.Helper()
	for _, s := range list {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no setting %q in %+v", id, list)
	return Setting{}
}

func optionValues(s Setting) string {
	var out []string
	for _, o := range s.Options {
		out = append(out, o.Value)
	}
	return strings.Join(out, ",")
}

func expectSetting(t *testing.T, list []Setting, id, auto, options string) Setting {
	t.Helper()
	s := settingByID(t, list, id)
	if s.Auto != auto || s.Value != auto || s.Overridden || optionValues(s) != options {
		t.Fatalf("%s: auto %q value %q overridden %v options %q; want auto %q options %q", id, s.Auto, s.Value, s.Overridden, optionValues(s), auto, options)
	}
	if !s.Offers(s.Auto) {
		t.Fatalf("%s: auto %q must be one of the options %q", id, s.Auto, optionValues(s))
	}
	return s
}

// The reference box (2 × 1.4 GHz entry, Haswell: H.264 only in Plex and
// Moonlight, nothing in VacuumTube's runtime, 1080p at 120 Hz) is offered only
// what it can handle.
func TestCatalogOnTheReferenceBox(t *testing.T) {
	haswell := ParseDecoders([]byte(haswellInspect))
	ml := Catalog("moonlight", haswell, small, refDisplay)
	expectSetting(t, ml, "codec", "h264", "auto,h264")
	fps := expectSetting(t, ml, "fps", "120", "30,60,120")
	if !strings.Contains(fps.Options[2].Note, "lower it if games stutter") {
		t.Fatalf("120 fps on an entry box carries a caution: %+v", fps.Options[2])
	}
	expectSetting(t, ml, "resolution", "1080p", "720p,1080p")
	expectSetting(t, ml, "framepacing", "on", "on,off")

	yt := Catalog("vacuumtube", Caps{Probed: true, Hardware: map[Codec]bool{}}, small, refDisplay)
	expectSetting(t, yt, "codecs", "h264", "h264")
	expectSetting(t, yt, "super_resolution", "off", "on,off")
	expectSetting(t, yt, "pause_on_blur", "on", "on,off")
	expectSetting(t, yt, "low_memory", "off", "on,off")
	expectSetting(t, Catalog("vacuumtube", Caps{Probed: true}, tiny, refDisplay), "low_memory", "on", "on,off")

	px := Catalog("plex-htpc", haswell, small, refDisplay)
	expectSetting(t, px, "scaling", "fast", "fast,default")
	expectSetting(t, px, "hwdec", "auto-safe", "auto-safe,no")

	if Catalog("kodi", haswell, small, refDisplay) != nil {
		t.Fatal("an unknown app has no settings")
	}
}

// Modern boxes with every decoder on a 4K TV get the big choices; the same box
// on a 1080p output, or an entry box, does not.
func TestCatalogOnModernBoxes(t *testing.T) {
	modern := ParseDecoders([]byte(modernInspect))
	uhd120 := uhd60
	uhd120.Refresh = 120
	for _, host := range []Host{standard, big} {
		ml := Catalog("moonlight", modern, host, uhd120)
		expectSetting(t, ml, "codec", "auto", "auto,h264,hevc,av1")
		expectSetting(t, ml, "fps", "120", "30,60,120")
		expectSetting(t, ml, "resolution", "4k", "720p,1080p,1440p,4k")
		expectSetting(t, Catalog("vacuumtube", modern, host, uhd60), "codecs", "av1", "h264,vp9,av1")
		expectSetting(t, Catalog("plex-htpc", modern, host, uhd60), "scaling", "default", "fast,default,high-quality")
		expectSetting(t, Catalog("vacuumtube", modern, host, uhd60), "super_resolution", "on", "on,off")
	}
	expectSetting(t, Catalog("moonlight", modern, standard, refDisplay), "resolution", "1080p", "720p,1080p")
	expectSetting(t, Catalog("moonlight", modern, small, uhd60), "resolution", "1080p", "720p,1080p")
	expectSetting(t, Catalog("moonlight", ParseDecoders([]byte(haswellInspect)), big, uhd60), "resolution", "1080p", "720p,1080p")
	expectSetting(t, Catalog("moonlight", modern, standard, uhd60), "fps", "60", "30,60")
	expectSetting(t, Catalog("moonlight", modern, standard, Display{}), "fps", "60", "30,60")

	// A desktop without VP9/AV1 hardware may still choose them: its CPU decodes.
	none := Caps{Probed: true, Hardware: map[Codec]bool{}}
	codecs := expectSetting(t, Catalog("vacuumtube", none, big, uhd60), "codecs", "av1", "h264,vp9,av1")
	if !strings.Contains(codecs.Options[1].Note, "CPU") || !strings.Contains(codecs.Options[2].Note, "CPU") {
		t.Fatalf("software choices say where they decode: %+v", codecs.Options)
	}
	expectSetting(t, Catalog("vacuumtube", none, big, refDisplay), "codecs", "h264", "h264,vp9,av1")
	expectSetting(t, Catalog("vacuumtube", none, standard, uhd60), "codecs", "h264", "h264")
	vp9 := Caps{Probed: true, Hardware: map[Codec]bool{H264: true, VP9: true}}
	expectSetting(t, Catalog("vacuumtube", vp9, small, refDisplay), "codecs", "vp9", "h264,vp9")
}

// Overrides steer the plan when they are offered; anything else falls back to
// Auto with a note.
func TestOverridesAreHonouredOnlyWhenOffered(t *testing.T) {
	haswell := ParseDecoders([]byte(haswellInspect))
	conf := write(t, "Moonlight.conf", "[General]\nbitrate=28000\nfps=120\nheight=1080\nwidth=1920\nvideocfg=1\nframepacing=true\n")
	p := must(PlanMoonlight(conf, haswell, small, refDisplay, Overrides{"fps": "60", "resolution": "720p", "framepacing": "off", "codec": "hevc"}))
	got := changed(p)
	want := map[string]string{"fps": "60", "width": "1280", "height": "720", "framepacing": "false", "bitrate": "10000"}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	if _, ok := got["videocfg"]; ok {
		t.Fatalf("HEVC is not decoded in hardware here, so it is not offered and H.264 stays: %v", got)
	}
	if !strings.Contains(strings.Join(p.Notes, "\n"), `Your choice "hevc" for Video codec is not available`) {
		t.Fatalf("a refused override is explained: %v", p.Notes)
	}
	if s := settingByID(t, p.Settings, "fps"); s.Value != "60" || !s.Overridden || s.Auto != "120" {
		t.Fatalf("fps setting = %+v", s)
	}
	if s := settingByID(t, p.Settings, "codec"); s.Value != "h264" || s.Overridden {
		t.Fatalf("refused codec setting = %+v", s)
	}
	// Returning to Auto (no override) restores detection's choice, bitrate too.
	if err := os.WriteFile(conf, p.content, 0o600); err != nil {
		t.Fatal(err)
	}
	back := changed(must(PlanMoonlight(conf, haswell, small, refDisplay, nil)))
	if back["fps"] != "120" || back["height"] != "1080" || back["framepacing"] != "true" || back["bitrate"] != "28000" {
		t.Fatalf("back to Auto: %v", back)
	}

	// YouTube: AV1 is refused on the reference box, honoured on a desktop.
	yt := write(t, "config.json", vacuumConf)
	none := Caps{Probed: true, Hardware: map[Codec]bool{}}
	v := must(PlanVacuumTube(yt, none, small, refDisplay, Overrides{"codecs": "av1", "pause_on_blur": "off", "super_resolution": "on"}))
	if got := changed(v); got["h264ify"] != "true" || got["pause_on_blur"] != "" || got["remove_super_resolution"] != "" {
		t.Fatalf("reference box youtube overrides: %v", got)
	}
	if got := changed(must(PlanVacuumTube(yt, none, big, refDisplay, Overrides{"codecs": "av1"}))); got["h264ify"] != "" {
		t.Fatalf("a desktop may allow AV1 on a 1080p screen: %v", got)
	}
	withSR := write(t, "sr.json", "{\n  \"remove_super_resolution\": true,\n  \"low_memory_mode\": true\n}\n")
	if got := changed(must(PlanVacuumTube(withSR, none, small, refDisplay, Overrides{"super_resolution": "on"}))); got["remove_super_resolution"] != "false" || got["low_memory_mode"] != "false" {
		t.Fatalf("switches go back off when chosen or when Auto says so: %v", got)
	}

	// Plex: high quality is not offered on an entry box.
	mpv := write(t, "mpv.conf", "")
	if text := string(must(PlanPlex(mpv, haswell, small, Overrides{"scaling": "high-quality", "hwdec": "no"})).content); !strings.Contains(text, "profile=fast") || !strings.Contains(text, "hwdec=no") {
		t.Fatalf("entry plex:\n%s", text)
	}
	if text := string(must(PlanPlex(mpv, haswell, standard, Overrides{"scaling": "high-quality"})).content); !strings.Contains(text, "profile=high-quality") || !strings.Contains(text, "hwdec=auto-safe") {
		t.Fatalf("standard plex:\n%s", text)
	}
}

// Retune re-plans one app with the current overrides: applied now when the
// app is closed, left alone when automatic tuning is off.
func TestRetuneAppliesTheOwnersChoice(t *testing.T) {
	home := t.TempDir()
	conf := VacuumTubeConf(home)
	if err := os.MkdirAll(filepath.Dir(conf), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conf, []byte(vacuumConf), 0o600); err != nil {
		t.Fatal(err)
	}
	ov := Overrides{}
	d := &Detector{Home: home, Now: func() time.Time { return time.Unix(0, 0) }, Overrides: func(adapter string) Overrides {
		if adapter == "vacuumtube" {
			return ov
		}
		return nil
	}}
	r := Report{Host: small, Display: refDisplay, Apps: []AppReport{{Label: "YouTube", Adapter: "vacuumtube", FlatpakID: "rocks.shy.VacuumTube", Caps: Caps{Probed: true, Hardware: map[Codec]bool{}}}}}
	ov["pause_on_blur"] = "off"
	a, ok := d.Retune(context.Background(), &r, "vacuumtube", false)
	if !ok || a.Status != "off" {
		t.Fatalf("automatic tuning off: %+v", a)
	}
	if cur, _ := os.ReadFile(conf); string(cur) != vacuumConf {
		t.Fatal("nothing is written while automatic tuning is off")
	}
	a, _ = d.Retune(context.Background(), &r, "vacuumtube", true)
	if a.Status != "applied" {
		t.Fatalf("closed app: %+v", a)
	}
	if cur, _ := os.ReadFile(conf); !strings.Contains(string(cur), `"pause_on_blur": false`) {
		t.Fatalf("override not written:\n%s", cur)
	}
	delete(ov, "pause_on_blur")
	if a, _ = d.Retune(context.Background(), &r, "vacuumtube", true); a.Status != "applied" {
		t.Fatalf("back to Auto: %+v", a)
	}
	if cur, _ := os.ReadFile(conf); !strings.Contains(string(cur), `"pause_on_blur": true`) {
		t.Fatalf("Auto not restored:\n%s", cur)
	}
	if _, ok := d.Retune(context.Background(), &r, "moonlight", true); ok {
		t.Fatal("an app that was not detected cannot be retuned")
	}
}
