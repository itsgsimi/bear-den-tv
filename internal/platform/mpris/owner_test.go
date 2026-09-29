// Tests for verified player ownership (Find with Processes, mpris.go): the
// players are shaped exactly like the ones observed on a real TV (Linux Mint
// 21.3, X11): VacuumTube's Electron player is
// org.mpris.MediaPlayer2.chromium.instance3 with Identity "VacuumTube", no
// DesktopEntry (reading it is an error) and PlaybackStatus "Playing"; Plex
// HTPC publishes no player at all.

package mpris

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/dbusx"
)

const (
	vacuumTubeID = "rocks.shy.VacuumTube"
	plexID       = "tv.plex.PlexHTPC"
	chromiumID   = "org.chromium.Chromium"
)

// observedVacuumTube is the player as the TV showed it.
func observedVacuumTube() *fakePlayer {
	return &fakePlayer{props: map[string]any{
		"Identity": "VacuumTube", "PlaybackStatus": "Playing", // DesktopEntry: absent
		"CanControl": true, "CanPause": true, "CanPlay": true, "CanSeek": true,
	}}
}

// fakeProcs maps pids to their Flatpak and parent.
type fakeProcs struct {
	flatpak map[int]string
	parent  map[int]int
	fail    map[int]error
}

func (f fakeProcs) FlatpakID(pid int) (string, error) {
	if err := f.fail[pid]; err != nil {
		return "", err
	}
	return f.flatpak[pid], nil
}

func (f fakeProcs) DescendsFrom(pid, root int) (bool, error) {
	for range 64 {
		if pid == root {
			return true, nil
		}
		next, ok := f.parent[pid]
		if !ok {
			return false, nil
		}
		pid = next
	}
	return false, nil
}

// pidBus is fakeBus plus the bus daemon's GetConnectionUnixProcessID,
// answered from owners (bus name → pid; a missing name is the daemon's error).
func pidBus(players map[string]*fakePlayer, owners map[string]uint32) *dbusx.Fake {
	bus := fakeBus(players)
	player := bus.CallFn
	bus.CallFn = func(ctx context.Context, dest, path, method string, args ...any) ([]any, error) {
		if method == "org.freedesktop.DBus.GetConnectionUnixProcessID" {
			pid, ok := owners[args[0].(string)]
			if !ok {
				return nil, fmt.Errorf("org.freedesktop.DBus.Error.NameHasNoOwner: %v", args[0])
			}
			return []any{pid}, nil
		}
		return player(ctx, dest, path, method, args...)
	}
	return bus
}

// the adapters' matches, as internal/session builds them.
var (
	vacuumTube = platform.MediaMatch{FlatpakID: vacuumTubeID, Names: []string{vacuumTubeID}}
	plexHTPC   = platform.MediaMatch{FlatpakID: plexID, Names: []string{plexID}}
)

func TestObservedVacuumTubeIsFoundByItsProcess(t *testing.T) {
	ctx := context.Background()
	yt := observedVacuumTube()
	players := map[string]*fakePlayer{"org.mpris.MediaPlayer2.chromium.instance3": yt}
	owners := map[string]uint32{"org.mpris.MediaPlayer2.chromium.instance3": 4242}

	// What the TV did: by names alone the player is nobody's.
	if _, ok, err := NewLocator(pidBus(players, owners)).Find(ctx, vacuumTube); ok || err != nil {
		t.Fatalf("names alone matched the observed player: %v %v", ok, err)
	}

	procs := fakeProcs{flatpak: map[int]string{4242: vacuumTubeID}}
	l := NewLocator(pidBus(players, owners)).WithProcesses(procs)
	p, ok, err := l.Find(ctx, vacuumTube)
	if err != nil || !ok {
		t.Fatalf("VacuumTube's own player not found: %v %v", ok, err)
	}
	if p.(*Player).Name != "org.mpris.MediaPlayer2.chromium.instance3" {
		t.Fatalf("found %s", p.(*Player).Name)
	}
	if st, err := p.Status(ctx); st != "Playing" || err != nil {
		t.Fatalf("status %q %v", st, err)
	}
	if can, err := p.CanControl(ctx); !can || err != nil {
		t.Fatalf("CanControl %v %v", can, err)
	}
	if err := p.Pause(ctx); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if len(yt.calls) != 1 || yt.calls[0] != "org.mpris.MediaPlayer2.Player.Pause" {
		t.Fatalf("calls %v", yt.calls)
	}
	// Plex HTPC has no player; VacuumTube's is never handed to it.
	if _, ok, err := l.Find(ctx, plexHTPC); ok || err != nil {
		t.Fatalf("Plex got VacuumTube's player: %v %v", ok, err)
	}

	// The probe listing names the owner and records the missing DesktopEntry.
	infos, err := l.List(ctx)
	if err != nil || len(infos) != 1 {
		t.Fatalf("list %v %v", infos, err)
	}
	if infos[0].OwnerPID != 4242 || infos[0].OwnerFlatpak != vacuumTubeID || infos[0].Identity != "VacuumTube" || infos[0].DesktopEntry != "" || len(infos[0].Errors) != 1 {
		t.Fatalf("listing %+v", infos[0])
	}
}

func TestAnotherFlatpakCannotClaimAnApp(t *testing.T) {
	ctx := context.Background()
	// A player that names VacuumTube exactly, owned by Chromium's Flatpak.
	impostor := &fakePlayer{props: map[string]any{"Identity": "x", "DesktopEntry": vacuumTubeID, "PlaybackStatus": "Playing", "CanControl": true}}
	players := map[string]*fakePlayer{"org.mpris.MediaPlayer2." + vacuumTubeID: impostor}
	owners := map[string]uint32{"org.mpris.MediaPlayer2." + vacuumTubeID: 500}
	l := NewLocator(pidBus(players, owners)).WithProcesses(fakeProcs{flatpak: map[int]string{500: chromiumID}})
	if _, ok, _ := l.Find(ctx, vacuumTube); ok {
		t.Fatal("a player owned by another Flatpak matched by its names")
	}
}

func TestNamesAreOnlyAnExactFallbackOutsideFlatpaks(t *testing.T) {
	ctx := context.Background()
	players := map[string]*fakePlayer{
		"org.mpris.MediaPlayer2.chromium.instance3": observedVacuumTube(), // host process: no DesktopEntry, not exact
		"org.mpris.MediaPlayer2.spotify":            {props: map[string]any{"Identity": "Spotify", "PlaybackStatus": "Paused"}},
	}
	owners := map[string]uint32{"org.mpris.MediaPlayer2.chromium.instance3": 700, "org.mpris.MediaPlayer2.spotify": 701}
	l := NewLocator(pidBus(players, owners)).WithProcesses(fakeProcs{flatpak: map[int]string{}})
	if _, ok, _ := l.Find(ctx, vacuumTube); ok {
		t.Fatal("a host player without an exact name matched")
	}
	p, ok, _ := l.Find(ctx, platform.MediaMatch{FlatpakID: "com.spotify.Client", Names: []string{"spotify"}})
	if !ok || p.(*Player).Name != "org.mpris.MediaPlayer2.spotify" {
		t.Fatalf("exact bus-name fallback for a host player: %v", ok)
	}
}

func TestUnreadableOwnersFailClosed(t *testing.T) {
	ctx := context.Background()
	players := map[string]*fakePlayer{
		"org.mpris.MediaPlayer2.chromium.instance3": observedVacuumTube(),
		"org.mpris.MediaPlayer2." + plexID:          {props: map[string]any{"DesktopEntry": plexID, "PlaybackStatus": "Playing"}},
	}
	// Plex's name has no owner the daemon can tell; VacuumTube's process is another user's.
	owners := map[string]uint32{"org.mpris.MediaPlayer2.chromium.instance3": 800}
	l := NewLocator(pidBus(players, owners)).WithProcesses(fakeProcs{fail: map[int]error{800: errors.New("proc: process belongs to another user")}})
	if _, ok, _ := l.Find(ctx, vacuumTube); ok {
		t.Fatal("an unreadable owner matched")
	}
	if _, ok, _ := l.Find(ctx, plexHTPC); ok {
		t.Fatal("a player without a known owner matched by its names")
	}
}

func TestWebAppsMatchOnlyTheirOwnBrowserTree(t *testing.T) {
	ctx := context.Background()
	// Two web apps in one Chromium Flatpak: Netflix's browser is pid 10
	// (its player's D-Bus proxy is pid 11), the Browser tile's is pid 20.
	players := map[string]*fakePlayer{
		"org.mpris.MediaPlayer2.chromium.instance11": {props: map[string]any{"Identity": "Chromium", "DesktopEntry": chromiumID, "PlaybackStatus": "Playing"}},
		"org.mpris.MediaPlayer2.chromium.instance21": {props: map[string]any{"Identity": "Chromium", "DesktopEntry": chromiumID, "PlaybackStatus": "Paused"}},
	}
	owners := map[string]uint32{"org.mpris.MediaPlayer2.chromium.instance11": 11, "org.mpris.MediaPlayer2.chromium.instance21": 21}
	procs := fakeProcs{
		flatpak: map[int]string{11: chromiumID, 21: chromiumID},
		parent:  map[int]int{11: 10, 10: 1, 21: 20, 20: 1},
	}
	l := NewLocator(pidBus(players, owners)).WithProcesses(procs)
	for root, want := range map[int]string{10: "org.mpris.MediaPlayer2.chromium.instance11", 20: "org.mpris.MediaPlayer2.chromium.instance21"} {
		p, ok, err := l.Find(ctx, platform.MediaMatch{FlatpakID: chromiumID, ProcessRoot: root})
		if !ok || err != nil || p.(*Player).Name != want {
			t.Fatalf("root %d: %v %v", root, ok, err)
		}
	}
	if _, ok, _ := l.Find(ctx, platform.MediaMatch{FlatpakID: chromiumID, ProcessRoot: 30}); ok {
		t.Fatal("a web app with no player got another web app's")
	}
	// Without process reading a web app can never be told apart: no match.
	if _, ok, _ := NewLocator(pidBus(players, owners)).Find(ctx, platform.MediaMatch{FlatpakID: chromiumID, Names: []string{chromiumID}, ProcessRoot: 10}); ok {
		t.Fatal("a web app matched without process reading")
	}
}
