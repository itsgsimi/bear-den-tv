// Fake media: an in-memory MediaLocator and MPRIS-style player for session
// tests and `bear-den-tv dev --dev-fixtures` (DEMO titles only). Positions
// advance on the injected clock, so tests run on clock.Fake.

package fake

import (
	"context"
	"errors"
	"sync"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/platform"
)

// Media is a MediaLocator over players registered by match string (the
// Flatpak id an adapter's MediaMatch returns).
type Media struct {
	mu      sync.Mutex
	players map[string]*Player
	finds   int
}

// NewMedia returns a locator with no players.
func NewMedia() *Media { return &Media{players: map[string]*Player{}} }

// Add registers p under match, replacing any previous player.
func (m *Media) Add(match string, p *Player) {
	m.mu.Lock()
	m.players[match] = p
	m.mu.Unlock()
}

// Remove forgets the player registered under match.
func (m *Media) Remove(match string) {
	m.mu.Lock()
	delete(m.players, match)
	m.mu.Unlock()
}

// Finds counts Find calls.
func (m *Media) Finds() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.finds
}

// Find implements platform.MediaLocator: an exact match only, like the real
// locator never hands back another app's player.
func (m *Media) Find(_ context.Context, match string) (platform.MediaPlayer, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finds++
	p, ok := m.players[match]
	if !ok {
		return nil, false, nil
	}
	return p, true, nil
}

// Player is a scriptable media player. Its position advances with the clock
// while it is Playing.
type Player struct {
	clock clock.Clock

	mu         sync.Mutex
	info       platform.MediaInfo // Position is the position at since
	since      time.Time
	canControl bool
	fail       error
	reads      int
	calls      []string
	watchers   map[chan struct{}]struct{}
}

// NewPlayer returns a controllable player reporting info (Rate 0 means 1).
func NewPlayer(clk clock.Clock, info platform.MediaInfo) *Player {
	if info.Rate == 0 {
		info.Rate = 1
	}
	return &Player{clock: clk, info: info, since: clk.Now(), canControl: true, watchers: map[chan struct{}]struct{}{}}
}

// Set replaces what the player reports (position counted from now) and
// signals watchers, like PropertiesChanged.
func (p *Player) Set(info platform.MediaInfo) {
	if info.Rate == 0 {
		info.Rate = 1
	}
	p.mu.Lock()
	p.info, p.since = info, p.clock.Now()
	p.mu.Unlock()
	p.Signal()
}

// SetCanControl sets what CanControl reports.
func (p *Player) SetCanControl(v bool) {
	p.mu.Lock()
	p.canControl = v
	p.mu.Unlock()
}

// Fail makes every Info and Status call return err (nil restores).
func (p *Player) Fail(err error) {
	p.mu.Lock()
	p.fail = err
	p.mu.Unlock()
}

// Signal wakes every watcher, as a change signal would.
func (p *Player) Signal() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for ch := range p.watchers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Reads counts Info calls (how often the coordinator re-read the player).
func (p *Player) Reads() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reads
}

// Calls lists the control calls received ("Pause", "Play", "Seek").
func (p *Player) Calls() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.calls...)
}

// nowLocked is the reading at the current clock time.
func (p *Player) nowLocked() platform.MediaInfo {
	info := p.info
	info.Artists = append([]string(nil), p.info.Artists...)
	if info.Status == "Playing" && info.HasPosition {
		info.Position += time.Duration(float64(p.clock.Since(p.since)) * info.Rate)
		if info.Length > 0 && info.Position > info.Length {
			info.Position = info.Length
		}
	}
	return info
}

// Status implements platform.MediaPlayer.
func (p *Player) Status(context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fail != nil {
		return "Unknown", p.fail
	}
	return p.info.Status, nil
}

// Info implements platform.MediaPlayer.
func (p *Player) Info(context.Context) (platform.MediaInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reads++
	if p.fail != nil {
		return platform.MediaInfo{}, p.fail
	}
	return p.nowLocked(), nil
}

func (p *Player) control(call, status string, seek time.Duration) error {
	p.mu.Lock()
	if !p.canControl {
		p.mu.Unlock()
		return errors.New("fake: player refuses control")
	}
	p.calls = append(p.calls, call)
	cur := p.nowLocked()
	p.info.Position, p.since = cur.Position+seek, p.clock.Now()
	if p.info.Position < 0 {
		p.info.Position = 0
	}
	if status != "" {
		p.info.Status = status
	}
	p.mu.Unlock()
	p.Signal()
	return nil
}

// Pause implements platform.MediaPlayer.
func (p *Player) Pause(context.Context) error { return p.control("Pause", "Paused", 0) }

// Play implements platform.MediaPlayer.
func (p *Player) Play(context.Context) error { return p.control("Play", "Playing", 0) }

// SeekRelative implements platform.MediaPlayer.
func (p *Player) SeekRelative(_ context.Context, seconds int) error {
	return p.control("Seek", "", time.Duration(seconds)*time.Second)
}

// CanControl implements platform.MediaPlayer.
func (p *Player) CanControl(context.Context) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.canControl, nil
}

// Watch implements platform.MediaPlayer.
func (p *Player) Watch(ctx context.Context) (<-chan struct{}, error) {
	ch := make(chan struct{}, 1)
	p.mu.Lock()
	p.watchers[ch] = struct{}{}
	p.mu.Unlock()
	go func() {
		<-ctx.Done()
		p.mu.Lock()
		delete(p.watchers, ch)
		close(ch)
		p.mu.Unlock()
	}()
	return ch, nil
}

// Watchers counts live watches.
func (p *Player) Watchers() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.watchers)
}

// DemoMedia is the `dev --dev-fixtures` locator: DEMO players for the apps
// that expose MPRIS on a real box as far as anyone knows (Plex HTPC and
// VacuumTube; unverified, docs/IMPLEMENTATION_STATUS.md). Moonlight gets
// none, so its media controls read as unavailable, as they would.
func DemoMedia(clk clock.Clock, plexID, youtubeID string) *Media {
	m := NewMedia()
	m.Add(plexID, NewPlayer(clk, platform.MediaInfo{
		Status: "Playing", Title: "DEMO Episode 3: The Long Winter", Artists: []string{"DEMO Show"}, Album: "DEMO Season 1",
		Length: 44 * time.Minute, Position: 12*time.Minute + 34*time.Second, HasPosition: true, Rate: 1,
	}))
	m.Add(youtubeID, NewPlayer(clk, platform.MediaInfo{
		Status: "Playing", Title: "DEMO Video: Building a Cabin", Artists: []string{"DEMO Channel"},
		Length: 18*time.Minute + 5*time.Second, Position: 3 * time.Minute, HasPosition: true, Rate: 1,
	}))
	return m
}
