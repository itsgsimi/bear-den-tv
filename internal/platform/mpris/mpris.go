// Package mpris locates and controls MPRIS2 players on the session bus. It
// only ever calls the idempotent Pause/Play/Seek methods (never PlayPause) and
// refuses a call the player's Can* properties do not permit, so the coordinator
// never reports a delivery the player could not act on. It also reads what a
// player is playing (Metadata, Position, Rate: Player.Info) and watches that
// one player's change signals (Player.Watch) for phones' Now playing card
// (contracts/http.md). Titles are never put in PlayerInfo, errors or logs.
package mpris

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/dbusx"
)

const (
	// Prefix is the MPRIS2 bus-name prefix.
	Prefix      = "org.mpris.MediaPlayer2."
	objectPath  = "/org/mpris/MediaPlayer2"
	rootIface   = "org.mpris.MediaPlayer2"
	playerIface = "org.mpris.MediaPlayer2.Player"

	// Backend is the capability backend name for MPRIS-routed actions.
	Backend = "mpris"
)

// PlayerInfo is what the locator learned about one player; used by the probe.
type PlayerInfo struct {
	BusName        string   `json:"bus_name"`
	Identity       string   `json:"identity"`
	DesktopEntry   string   `json:"desktop_entry"`
	PlaybackStatus string   `json:"playback_status"`
	CanControl     bool     `json:"can_control"`
	CanPause       bool     `json:"can_pause"`
	CanPlay        bool     `json:"can_play"`
	CanSeek        bool     `json:"can_seek"`
	Errors         []string `json:"errors,omitempty"`
}

// Locator implements platform.MediaLocator over a session bus.
type Locator struct {
	bus dbusx.Bus
}

// NewLocator wraps bus. The locator does not own the bus.
func NewLocator(bus dbusx.Bus) *Locator {
	return &Locator{bus: bus}
}

// Names returns the MPRIS bus names currently owned, sorted.
func (l *Locator) Names(ctx context.Context) ([]string, error) {
	all, err := l.bus.Names(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, n := range all {
		if strings.HasPrefix(n, Prefix) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names, nil
}

// List describes every player; per-player property failures are recorded in
// Errors rather than failing the listing.
func (l *Locator) List(ctx context.Context) ([]PlayerInfo, error) {
	names, err := l.Names(ctx)
	if err != nil {
		return nil, err
	}
	infos := make([]PlayerInfo, 0, len(names))
	for _, n := range names {
		p := &Player{bus: l.bus, Name: n}
		info := PlayerInfo{BusName: n}
		record := func(what string, err error) {
			if err != nil {
				info.Errors = append(info.Errors, what+": "+err.Error())
			}
		}
		var e error
		info.Identity, e = p.stringProp(ctx, rootIface, "Identity")
		record("Identity", e)
		info.DesktopEntry, e = p.stringProp(ctx, rootIface, "DesktopEntry")
		record("DesktopEntry", e)
		info.PlaybackStatus, e = p.Status(ctx)
		record("PlaybackStatus", e)
		info.CanControl, e = p.boolProp(ctx, "CanControl")
		record("CanControl", e)
		info.CanPause, e = p.boolProp(ctx, "CanPause")
		record("CanPause", e)
		info.CanPlay, e = p.boolProp(ctx, "CanPlay")
		record("CanPlay", e)
		info.CanSeek, e = p.boolProp(ctx, "CanSeek")
		record("CanSeek", e)
		infos = append(infos, info)
	}
	return infos, nil
}

// Find implements platform.MediaLocator: the first player (in sorted bus-name
// order) whose DesktopEntry or bus-name suffix matches wins.
func (l *Locator) Find(ctx context.Context, match string) (platform.MediaPlayer, bool, error) {
	names, err := l.Names(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, n := range names {
		p := &Player{bus: l.bus, Name: n}
		entry, _ := p.stringProp(ctx, rootIface, "DesktopEntry")
		if Matches(n, entry, match) {
			return p, true, nil
		}
	}
	return nil, false, nil
}

// Matches reports whether a player identified by busName and desktopEntry
// belongs to match (a Flatpak app id or desktop entry). Comparison is
// case-insensitive; the bus-name suffix may carry an ".instanceNNN" tail.
func Matches(busName, desktopEntry, match string) bool {
	if match == "" {
		return false
	}
	m := strings.ToLower(match)
	if strings.ToLower(desktopEntry) == m {
		return true
	}
	if !strings.HasPrefix(busName, Prefix) {
		return false
	}
	suffix := strings.ToLower(strings.TrimPrefix(busName, Prefix))
	if suffix == m {
		return true
	}
	if rest, ok := strings.CutPrefix(suffix, m+"."); ok && strings.HasPrefix(rest, "instance") {
		return true
	}
	return false
}

// Player is one MPRIS2 player, implementing platform.MediaPlayer.
type Player struct {
	bus dbusx.Bus
	// Name is the player's bus name.
	Name string
}

// NewPlayer addresses busName on bus without checking that it exists.
func NewPlayer(bus dbusx.Bus, busName string) *Player {
	return &Player{bus: bus, Name: busName}
}

func (p *Player) stringProp(ctx context.Context, iface, name string) (string, error) {
	v, err := p.bus.Property(ctx, p.Name, objectPath, iface, name)
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("mpris: %s is %T, not string", name, v)
	}
	return s, nil
}

func (p *Player) boolProp(ctx context.Context, name string) (bool, error) {
	v, err := p.bus.Property(ctx, p.Name, objectPath, playerIface, name)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("mpris: %s is %T, not bool", name, v)
	}
	return b, nil
}

// Status returns Playing, Paused, Stopped, or Unknown.
func (p *Player) Status(ctx context.Context) (string, error) {
	s, err := p.stringProp(ctx, playerIface, "PlaybackStatus")
	if err != nil {
		return "Unknown", err
	}
	switch s {
	case "Playing", "Paused", "Stopped":
		return s, nil
	}
	return "Unknown", nil
}

// Pause calls Player.Pause after confirming CanPause.
func (p *Player) Pause(ctx context.Context) error {
	return p.guarded(ctx, "CanPause", "Pause")
}

// Play calls Player.Play after confirming CanPlay.
func (p *Player) Play(ctx context.Context) error {
	return p.guarded(ctx, "CanPlay", "Play")
}

// SeekRelative calls Player.Seek with the offset in microseconds after
// confirming CanSeek.
func (p *Player) SeekRelative(ctx context.Context, seconds int) error {
	return p.guarded(ctx, "CanSeek", "Seek", int64(seconds)*1_000_000)
}

func (p *Player) guarded(ctx context.Context, canProp, method string, args ...any) error {
	ok, err := p.boolProp(ctx, canProp)
	if err != nil {
		return fmt.Errorf("mpris: %s on %s: %w", canProp, p.Name, err)
	}
	if !ok {
		return fmt.Errorf("%w: %s reports %s=false", platform.ErrUnsupported, p.Name, canProp)
	}
	_, err = p.bus.Call(ctx, p.Name, objectPath, playerIface+"."+method, args...)
	return err
}

// CanControl returns the player's CanControl property.
func (p *Player) CanControl(ctx context.Context) (bool, error) {
	return p.boolProp(ctx, "CanControl")
}

// CanPause returns the player's CanPause property.
func (p *Player) CanPause(ctx context.Context) (bool, error) {
	return p.boolProp(ctx, "CanPause")
}

// Info implements platform.MediaPlayer. Only PlaybackStatus is required;
// Metadata, Position and Rate are optional in practice (players omit or fail
// them), so a missing one leaves its fields zero rather than failing the read.
func (p *Player) Info(ctx context.Context) (platform.MediaInfo, error) {
	info := platform.MediaInfo{Rate: 1}
	st, err := p.Status(ctx)
	if err != nil {
		return info, fmt.Errorf("mpris: PlaybackStatus on %s: %w", p.Name, err)
	}
	info.Status = st
	if v, err := p.bus.Property(ctx, p.Name, objectPath, playerIface, "Metadata"); err == nil {
		if md, ok := dbusx.Dict(v); ok {
			applyMetadata(&info, md)
		}
	}
	if v, err := p.bus.Property(ctx, p.Name, objectPath, playerIface, "Position"); err == nil {
		if us, ok := toInt64(dbusx.Unwrap(v)); ok && us >= 0 {
			info.Position, info.HasPosition = time.Duration(us)*time.Microsecond, true
		}
	}
	if v, err := p.bus.Property(ctx, p.Name, objectPath, playerIface, "Rate"); err == nil {
		if r, ok := toFloat(dbusx.Unwrap(v)); ok && r >= 0 && !math.IsInf(r, 0) && !math.IsNaN(r) {
			info.Rate = r
		}
	}
	return info, nil
}

// applyMetadata maps the MPRIS Metadata keys the phone shows: xesam:title,
// xesam:artist (a list; some players send one string), xesam:album and
// mpris:length (microseconds; players variously send int64, uint64 or int32).
func applyMetadata(info *platform.MediaInfo, md map[string]any) {
	if s, ok := md["xesam:title"].(string); ok {
		info.Title = strings.TrimSpace(s)
	}
	switch a := md["xesam:artist"].(type) {
	case []string:
		for _, s := range a {
			if s = strings.TrimSpace(s); s != "" {
				info.Artists = append(info.Artists, s)
			}
		}
	case []any:
		for _, x := range a {
			if s, ok := dbusx.Unwrap(x).(string); ok && strings.TrimSpace(s) != "" {
				info.Artists = append(info.Artists, strings.TrimSpace(s))
			}
		}
	case string:
		if s := strings.TrimSpace(a); s != "" {
			info.Artists = []string{s}
		}
	}
	if s, ok := md["xesam:album"].(string); ok {
		info.Album = strings.TrimSpace(s)
	}
	if us, ok := toInt64(md["mpris:length"]); ok && us > 0 {
		info.Length = time.Duration(us) * time.Microsecond
	}
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case int:
		return int64(n), true
	case uint64:
		if n > math.MaxInt64 {
			return 0, false
		}
		return int64(n), true
	case uint32:
		return int64(n), true
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		return int64(n), true
	}
	return 0, false
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	}
	if i, ok := toInt64(v); ok {
		return float64(i), true
	}
	return 0, false
}

// Watch implements platform.MediaPlayer: it emits on PropertiesChanged of
// the Player interface and on Seeked, from this player only. Signals carry
// the sender's unique name, so the well-known name is resolved first; a
// player that cannot be resolved is an error (never another player's signals).
func (p *Player) Watch(ctx context.Context) (<-chan struct{}, error) {
	owner, err := dbusx.OwnerOf(ctx, p.bus, p.Name)
	if err != nil {
		return nil, fmt.Errorf("mpris: owner of %s: %w", p.Name, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	props, err := p.bus.Subscribe(ctx, "org.freedesktop.DBus.Properties", "PropertiesChanged")
	if err != nil {
		cancel()
		return nil, err
	}
	seeks, err := p.bus.Subscribe(ctx, playerIface, "Seeked")
	if err != nil {
		cancel()
		return nil, err
	}
	out := make(chan struct{}, 1)
	notify := func() {
		select {
		case out <- struct{}{}:
		default: // one pending wake-up is enough; the reader re-reads everything
		}
	}
	mine := func(s dbusx.Signal) bool {
		return (s.Sender == owner || s.Sender == p.Name) && s.Path == objectPath
	}
	go func() {
		defer close(out)
		defer cancel()
		for props != nil || seeks != nil {
			select {
			case <-ctx.Done():
				return
			case s, ok := <-props:
				if !ok {
					props = nil
					continue
				}
				if mine(s) && len(s.Body) > 0 && s.Body[0] == playerIface {
					notify()
				}
			case s, ok := <-seeks:
				if !ok {
					seeks = nil
					continue
				}
				if mine(s) {
					notify()
				}
			}
		}
	}()
	return out, nil
}
