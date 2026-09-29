// What this TV's Plex HTPC is playing, read from the owner's Plex server
// (GET /status/sessions on the chosen server, internal/providers/plex
// sessions.go): Plex HTPC publishes no MPRIS player, so the coordinator
// polls this for phones' Now playing (internal/session/plexplaying.go,
// contracts/http.md "Now playing", docs/security.md). One call is one
// request, made only when the coordinator asks; nothing is kept here, and
// titles are never logged.

package plexlink

import (
	"context"
	"net"
	"net/netip"
	"time"

	"bear-den-tv/internal/providers/plex"
)

// Playing statuses.
const (
	PlayingOff          = "off"          // Plex is not signed in and connected on this TV: nothing was asked
	PlayingIdle         = "idle"         // the server lists no Plex HTPC playback
	PlayingFound        = "found"        // this TV's Plex HTPC session
	PlayingUnidentified = "unidentified" // Plex HTPC playback that cannot be tied to this TV (Reason)
)

// Playing is one reading. Title and Subtitle are private media names.
type Playing struct {
	Status   string
	Reason   string
	Title    string
	Subtitle string        // an episode's show, a track's artist
	State    string        // playing | paused | buffering, as the server says
	Offset   time.Duration // viewOffset
	Duration time.Duration // 0 when unknown
}

// String describes p without its titles.
func (p Playing) String() string {
	return "plex playing{status=" + p.Status + " state=" + p.State + " title=[title]}"
}

// LocalAddrs lists this machine's own addresses (net.InterfaceAddrs).
func LocalAddrs() ([]netip.Addr, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	out := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a.String()); err == nil {
			out = append(out, p.Addr().Unmap())
		}
	}
	return out, nil
}

// NowPlaying asks the chosen server what this TV's Plex HTPC is playing.
// PlayingOff without a request while Plex is not connected; an error when
// the server cannot be asked (unreachable, or a shared account that may not
// list the owner's sessions), which the caller backs off from.
func (m *Manager) NowPlaying(ctx context.Context) (Playing, error) {
	m.mu.Lock()
	prov := m.provider
	m.mu.Unlock()
	if prov == nil {
		return Playing{Status: PlayingOff, Reason: "Plex is not signed in on this TV."}, nil
	}
	sessions, err := prov.Sessions(ctx)
	if err != nil {
		return Playing{}, err
	}
	local, err := m.opts.LocalAddrs()
	if err != nil {
		return Playing{}, err
	}
	mine := func(a netip.Addr) bool {
		for _, l := range local {
			if l == a {
				return true
			}
		}
		return false
	}
	match := plex.ThisPlayer(sessions, mine)
	switch match.Status {
	case plex.PlayerFound:
	case plex.PlayerUnidentified:
		return Playing{Status: PlayingUnidentified, Reason: match.Reason}, nil
	default:
		return Playing{Status: PlayingIdle}, nil
	}
	s := match.Session
	p := Playing{Status: PlayingFound, Title: s.Title, State: s.Player.State,
		Offset: time.Duration(max(s.ViewOffset, 0)) * time.Millisecond, Duration: time.Duration(max(s.Duration, 0)) * time.Millisecond}
	switch s.Type {
	case "episode", "track":
		p.Subtitle = s.GrandparentTitle
	}
	return p, nil
}
