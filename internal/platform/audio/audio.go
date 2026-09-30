// Package audio controls the PulseAudio/PipeWire-pulse default sink through
// the pactl CLI with a fixed argument vector. The only inputs that vary are a
// clamped signed percentage and a mute flag; nothing from the network reaches
// argv. Level reads the mute flag and volume back (`pactl get-sink-mute` and
// `get-sink-volume @DEFAULT_SINK@`) for state.audio. The capability is
// available only while a real default sink exists —
// with the HDMI sink inactive PulseAudio reports the dummy auto_null sink and
// the control is hidden rather than misleading.
package audio

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"bear-den-tv/internal/platform"
)

const (
	// BackendName is the capability backend name.
	BackendName = "pulseaudio-pactl"
	// MaxStep bounds one VolumeDelta call in percent.
	MaxStep = 25
	// DefaultSink is the pactl alias for the current default sink.
	DefaultSink = "@DEFAULT_SINK@"
	// NullSink is PulseAudio's placeholder sink when no output is active.
	NullSink = "auto_null"

	probeTimeout = 3 * time.Second
)

// Runner executes one fixed argv and returns its standard output. A non-zero
// exit must surface as an error.
type Runner func(ctx context.Context, argv []string) ([]byte, error)

// ExecRunner runs argv with os/exec, appending stderr to the error.
func ExecRunner(ctx context.Context, argv []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return out, fmt.Errorf("%s: %w: %s", argv[0], err, msg)
		}
		return out, fmt.Errorf("%s: %w", argv[0], err)
	}
	return out, nil
}

// Backend implements platform.AudioBackend.
type Backend struct {
	run Runner
}

// New returns a backend using run, or ExecRunner when run is nil.
func New(run Runner) *Backend {
	if run == nil {
		run = ExecRunner
	}
	return &Backend{run: run}
}

// Info is the parsed subset of `pactl info`.
type Info struct {
	ServerName    string `json:"server_name"`
	ServerVersion string `json:"server_version"`
	DefaultSink   string `json:"default_sink"`
	DefaultSource string `json:"default_source"`
}

// ParseInfo extracts the "Key: Value" lines of `pactl info` that matter here.
func ParseInfo(out []byte) Info {
	var info Info
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "Server Name":
			info.ServerName = value
		case "Server Version":
			info.ServerVersion = value
		case "Default Sink":
			info.DefaultSink = value
		case "Default Source":
			info.DefaultSource = value
		}
	}
	return info
}

// CapabilityFor derives the capability from parsed server info.
func CapabilityFor(info Info) platform.Capability {
	switch info.DefaultSink {
	case "":
		return platform.Capability{Backend: BackendName, Reason: "pactl reports no default sink"}
	case NullSink:
		return platform.Capability{Backend: BackendName, Reason: "No active audio sink (HDMI audio not active)"}
	}
	return platform.Capability{Available: true, Backend: BackendName}
}

// Probe runs `pactl info` and returns what it said with the derived capability.
func (b *Backend) Probe(ctx context.Context) (Info, platform.Capability) {
	out, err := b.run(ctx, []string{"pactl", "info"})
	if err != nil {
		return Info{}, platform.Capability{Backend: BackendName, Reason: "pactl unavailable: " + err.Error()}
	}
	info := ParseInfo(out)
	return info, CapabilityFor(info)
}

// Capability implements platform.AudioBackend with a bounded probe.
func (b *Backend) Capability() platform.Capability {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	_, c := b.Probe(ctx)
	return c
}

// ClampStep bounds percent to [-MaxStep, MaxStep].
func ClampStep(percent int) int {
	if percent > MaxStep {
		return MaxStep
	}
	if percent < -MaxStep {
		return -MaxStep
	}
	return percent
}

// VolumeArgv is the fixed pactl argv for a clamped relative volume change.
func VolumeArgv(percent int) []string {
	p := ClampStep(percent)
	sign := "+"
	if p < 0 {
		sign = "-"
		p = -p
	}
	return []string{"pactl", "set-sink-volume", DefaultSink, sign + strconv.Itoa(p) + "%"}
}

// MuteArgv is the fixed pactl argv for setting the default sink's mute flag.
func MuteArgv(muted bool) []string {
	flag := "0"
	if muted {
		flag = "1"
	}
	return []string{"pactl", "set-sink-mute", DefaultSink, flag}
}

// VolumeDelta implements platform.AudioBackend: a zero delta is a no-op, the
// magnitude is clamped to MaxStep, and the call is refused while the
// capability is unavailable.
func (b *Backend) VolumeDelta(ctx context.Context, percent int) error {
	if percent == 0 {
		return nil
	}
	if err := b.ensureAvailable(ctx); err != nil {
		return err
	}
	_, err := b.run(ctx, VolumeArgv(percent))
	return err
}

// SetMute implements platform.AudioBackend.
func (b *Backend) SetMute(ctx context.Context, muted bool) error {
	if err := b.ensureAvailable(ctx); err != nil {
		return err
	}
	_, err := b.run(ctx, MuteArgv(muted))
	return err
}

// GetMuteArgv and GetVolumeArgv are the fixed argv that read the sink.
var (
	GetMuteArgv   = []string{"pactl", "get-sink-mute", DefaultSink}
	GetVolumeArgv = []string{"pactl", "get-sink-volume", DefaultSink}
)

// ParseMute reads "Mute: yes" / "Mute: no".
func ParseMute(out []byte) (bool, error) {
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "Mute:"); ok {
			switch strings.TrimSpace(v) {
			case "yes":
				return true, nil
			case "no":
				return false, nil
			}
		}
	}
	return false, fmt.Errorf("pactl: no mute state in %q", strings.TrimSpace(string(out)))
}

// ParseVolume reads the first channel's percent from
// "Volume: front-left: 65536 / 100% / 0.00 dB, ...".
func ParseVolume(out []byte) (int, error) {
	s := string(out)
	if i := strings.Index(s, "Volume:"); i >= 0 {
		s = s[i:]
		if j := strings.Index(s, "%"); j > 0 {
			k := j
			for k > 0 && s[k-1] >= '0' && s[k-1] <= '9' {
				k--
			}
			if n, err := strconv.Atoi(s[k:j]); err == nil {
				return n, nil
			}
		}
	}
	return -1, fmt.Errorf("pactl: no volume in %q", strings.TrimSpace(string(out)))
}

// Level implements platform.AudioReader: the mute flag (required) and the
// volume (-1 when unreadable).
func (b *Backend) Level(ctx context.Context) (platform.AudioLevel, error) {
	out, err := b.run(ctx, GetMuteArgv)
	if err != nil {
		return platform.AudioLevel{}, err
	}
	muted, err := ParseMute(out)
	if err != nil {
		return platform.AudioLevel{}, err
	}
	lvl := platform.AudioLevel{Muted: muted, Percent: -1}
	if out, err := b.run(ctx, GetVolumeArgv); err == nil {
		if p, err := ParseVolume(out); err == nil {
			lvl.Percent = p
		}
	}
	return lvl, nil
}

func (b *Backend) ensureAvailable(ctx context.Context) error {
	_, c := b.Probe(ctx)
	if !c.Available {
		return fmt.Errorf("%w: %s", platform.ErrUnsupported, c.Reason)
	}
	return nil
}
