// What the owner's Plex Media Server says is playing (GET /status/sessions),
// and which of those sessions is this TV's Plex HTPC. Plex HTPC publishes no
// MPRIS player (seen on a real TV, 2026-09-29), so this is the only honest
// source of Now playing for it (docs/security.md "Now playing",
// contracts/http.md "Now playing").
//
// Fields, from the public API as python-plexapi reads it (plexapi/client.py
// PlexClient._loadData for the <Player> element; plexapi/media.py for the
// session): each Metadata item carries type, title, grandparentTitle
// (the show of an episode, the artist of a track), parentTitle, viewOffset
// and duration in milliseconds, and a Player with address (the client's
// address as the server sees it), machineIdentifier (the client's
// X-Plex-Client-Identifier), product, platform, title (the device name),
// state (playing | paused | buffering) and local. User and Session are not
// decoded: nothing here needs the account names.
//
// Identifying this TV: Plex HTPC's client identifier would be exact, but
// where Plex HTPC keeps it on disk is undocumented and not verified, so it is
// not read. Instead a session is this TV's when its Player.product is Plex
// HTPC's (HTPCProduct, UNVERIFIED on the TV) and its Player.address is one of
// this machine's own addresses. Nothing is guessed: no such session is
// "idle"; Plex HTPC sessions from other addresses only, or more than one
// from this machine, are "unidentified" with the reason.
package plex

import (
	"context"
	"net/netip"
	"strings"
)

// HTPCProduct is the X-Plex-Product Plex HTPC reports (UNVERIFIED on the
// TV: the app's own name; compared case-insensitively).
const HTPCProduct = "Plex HTPC"

// Player is a session's <Player>.
type Player struct {
	Address           string `json:"address"`
	MachineIdentifier string `json:"machineIdentifier"`
	Product           string `json:"product"`
	Platform          string `json:"platform"`
	Title             string `json:"title"`
	State             string `json:"state"` // playing | paused | buffering
	Local             bool   `json:"local"`
}

// Session is one /status/sessions Metadata item. Titles are private media
// names: they go to phones only (never logs; String redacts them).
type Session struct {
	Type             string `json:"type"` // movie | episode | track | clip ...
	Title            string `json:"title"`
	ParentTitle      string `json:"parentTitle"`
	GrandparentTitle string `json:"grandparentTitle"`
	ViewOffset       int64  `json:"viewOffset"`
	Duration         int64  `json:"duration"`
	Player           Player `json:"Player"`
}

// String describes s without its titles.
func (s Session) String() string {
	return "plex session{type=" + s.Type + " product=" + s.Player.Product + " state=" + s.Player.State + " title=[title]}"
}

type sessionsEnvelope struct {
	MediaContainer struct {
		Size     int       `json:"size"`
		Metadata []Session `json:"Metadata"`
	} `json:"MediaContainer"`
}

// Sessions is GET /status/sessions: every playback the server knows of. The
// server answers it only for its admin (owner) account; a shared user's
// token gets 401 or 403.
func (c *Client) Sessions(ctx context.Context) ([]Session, error) {
	var env sessionsEnvelope
	if err := c.getJSON(ctx, "/status/sessions", nil, &env); err != nil {
		return nil, err
	}
	return env.MediaContainer.Metadata, nil
}

// Identification results (ThisPlayer).
const (
	PlayerFound        = "found"        // exactly one Plex HTPC session from this machine
	PlayerIdle         = "idle"         // no Plex HTPC session at all
	PlayerUnidentified = "unidentified" // Plex HTPC sessions exist, but none (or several) is certainly this TV's
)

// Match is ThisPlayer's answer. Reason says why in plain words for
// "unidentified" (diagnostics and the phone's capability reason).
type Match struct {
	Status  string
	Reason  string
	Session Session
}

// ThisPlayer picks this TV's Plex HTPC session: product HTPCProduct and an
// address local reports as this machine's own.
func ThisPlayer(sessions []Session, local func(netip.Addr) bool) Match {
	var htpc, mine []Session
	for _, s := range sessions {
		if !strings.EqualFold(strings.TrimSpace(s.Player.Product), HTPCProduct) {
			continue
		}
		htpc = append(htpc, s)
		if a, err := netip.ParseAddr(strings.TrimSpace(s.Player.Address)); err == nil && local != nil && local(a.Unmap()) {
			mine = append(mine, s)
		}
	}
	switch {
	case len(mine) == 1:
		return Match{Status: PlayerFound, Session: mine[0]}
	case len(mine) > 1:
		return Match{Status: PlayerUnidentified, Reason: "Your Plex server lists more than one Plex HTPC playing from this TV's address, so Bear Den cannot tell which one is on screen."}
	case len(htpc) > 0:
		return Match{Status: PlayerUnidentified, Reason: "Your Plex server lists Plex HTPC playing, but not from this TV's addresses, so Bear Den cannot tell whether it is this TV."}
	}
	return Match{Status: PlayerIdle}
}
