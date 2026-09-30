// Tests for pactl parsing and volume control (audio.go).

package audio

import (
	"context"
	"errors"
	"strings"
	"testing"

	"bear-den-tv/internal/platform"
)

const infoHDMI = `Server String: /run/user/1000/pulse/native
Library Protocol Version: 35
Server Protocol Version: 35
Is Local: yes
Client Index: 42
Tile Size: 65472
User Name: alice
Host Name: reference-box
Server Name: pulseaudio
Server Version: 15.99.1
Default Sample Specification: s16le 2ch 44100Hz
Default Channel Map: front-left,front-right
Default Sink: alsa_output.pci-0000_00_03.0.hdmi-stereo
Default Source: alsa_input.pci-0000_00_1b.0.analog-stereo
Cookie: dead:beef
`

const infoNull = `Server Name: pulseaudio
Server Version: 15.99.1
Default Sink: auto_null
Default Source: auto_null.monitor
`

type recorder struct {
	info  string
	err   error
	argvs [][]string
}

func (r *recorder) run(ctx context.Context, argv []string) ([]byte, error) {
	r.argvs = append(r.argvs, argv)
	if r.err != nil {
		return nil, r.err
	}
	if len(argv) == 2 && argv[1] == "info" {
		return []byte(r.info), nil
	}
	return nil, nil
}

func TestParseInfo(t *testing.T) {
	info := ParseInfo([]byte(infoHDMI))
	if info.DefaultSink != "alsa_output.pci-0000_00_03.0.hdmi-stereo" || info.ServerName != "pulseaudio" || info.ServerVersion != "15.99.1" || info.DefaultSource != "alsa_input.pci-0000_00_1b.0.analog-stereo" {
		t.Fatalf("%+v", info)
	}
	if c := CapabilityFor(info); !c.Available || c.Backend != BackendName {
		t.Fatalf("%+v", c)
	}
	if c := CapabilityFor(ParseInfo([]byte(infoNull))); c.Available || !strings.Contains(c.Reason, "HDMI audio not active") {
		t.Fatalf("%+v", c)
	}
	if c := CapabilityFor(ParseInfo(nil)); c.Available || c.Reason == "" {
		t.Fatalf("%+v", c)
	}
}

func TestArgvFixedAndClamped(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{5, "+5%"}, {-5, "-5%"}, {100, "+25%"}, {-100, "-25%"}, {25, "+25%"}, {-1, "-1%"},
	}
	for _, tc := range cases {
		argv := VolumeArgv(tc.in)
		if strings.Join(argv[:3], " ") != "pactl set-sink-volume @DEFAULT_SINK@" || argv[3] != tc.want {
			t.Errorf("VolumeArgv(%d)=%v", tc.in, argv)
		}
	}
	if got := strings.Join(MuteArgv(true), " "); got != "pactl set-sink-mute @DEFAULT_SINK@ 1" {
		t.Error(got)
	}
	if got := strings.Join(MuteArgv(false), " "); got != "pactl set-sink-mute @DEFAULT_SINK@ 0" {
		t.Error(got)
	}
}

func TestBackendRefusesWithoutSink(t *testing.T) {
	r := &recorder{info: infoNull}
	b := New(r.run)
	ctx := context.Background()
	if err := b.VolumeDelta(ctx, 5); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("err=%v", err)
	}
	if err := b.SetMute(ctx, true); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("err=%v", err)
	}
	for _, a := range r.argvs {
		if a[1] != "info" {
			t.Fatalf("mutating call issued without a sink: %v", a)
		}
	}
	if c := b.Capability(); c.Available {
		t.Fatalf("%+v", c)
	}
}

func TestBackendAppliesWithSink(t *testing.T) {
	r := &recorder{info: infoHDMI}
	b := New(r.run)
	ctx := context.Background()
	if err := b.VolumeDelta(ctx, 0); err != nil || len(r.argvs) != 0 {
		t.Fatalf("zero delta ran %v (%v)", r.argvs, err)
	}
	if err := b.VolumeDelta(ctx, 60); err != nil {
		t.Fatal(err)
	}
	if err := b.SetMute(ctx, true); err != nil {
		t.Fatal(err)
	}
	last := r.argvs[len(r.argvs)-1]
	if strings.Join(last, " ") != "pactl set-sink-mute @DEFAULT_SINK@ 1" {
		t.Fatalf("%v", last)
	}
	if strings.Join(r.argvs[1], " ") != "pactl set-sink-volume @DEFAULT_SINK@ +25%" {
		t.Fatalf("%v", r.argvs[1])
	}
}

func TestPactlMissing(t *testing.T) {
	r := &recorder{err: errors.New("exec: \"pactl\": executable file not found in $PATH")}
	c := New(r.run).Capability()
	if c.Available || !strings.Contains(c.Reason, "pactl unavailable") {
		t.Fatalf("%+v", c)
	}
}

// pactl's output format (PipeWire-pulse; the mute line captured as is, the
// volume line at 65%): the mute flag and the volume of the default sink.
const (
	muteNo   = "Mute: no\n"
	muteYes  = "Mute: yes\n"
	volume65 = "Volume: front-left: 42598 /  65% / -11.23 dB,   front-right: 42598 /  65% / -11.23 dB\n        balance 0.00\n"
)

func TestLevelReadsMuteAndVolumeWithFixedArgv(t *testing.T) {
	var argvs [][]string
	answers := map[string]string{"get-sink-mute": muteYes, "get-sink-volume": volume65}
	b := New(func(_ context.Context, argv []string) ([]byte, error) {
		argvs = append(argvs, argv)
		return []byte(answers[argv[1]]), nil
	})
	lvl, err := b.Level(context.Background())
	if err != nil || !lvl.Muted || lvl.Percent != 65 {
		t.Fatalf("level = %+v, %v", lvl, err)
	}
	if got := strings.Join(argvs[0], " ") + " | " + strings.Join(argvs[1], " "); got != "pactl get-sink-mute @DEFAULT_SINK@ | pactl get-sink-volume @DEFAULT_SINK@" {
		t.Fatalf("argv %s", got)
	}
	if m, err := ParseMute([]byte(muteNo)); err != nil || m {
		t.Fatalf("mute no = %v, %v", m, err)
	}
	if _, err := ParseMute([]byte("garbage")); err == nil {
		t.Fatal("parsed a mute flag from garbage")
	}
	// The volume is optional: unreadable is -1, the mute flag still counts.
	answers["get-sink-volume"] = "nothing useful"
	if lvl, err := b.Level(context.Background()); err != nil || lvl.Percent != -1 || !lvl.Muted {
		t.Fatalf("level without volume = %+v, %v", lvl, err)
	}
}
