// Tests for reading what a player is playing (Player.Info) and watching its
// change signals (Player.Watch) over the in-memory bus (mpris.go).

package mpris

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"bear-den-tv/internal/platform/dbusx"
)

const plexBus = "org.mpris.MediaPlayer2.plexhtpc"

// ownedBus is fakeBus plus GetNameOwner: each player's unique name is
// ":1.<index>" in owners.
func ownedBus(players map[string]*fakePlayer, owners map[string]string) *dbusx.Fake {
	f := fakeBus(players)
	inner := f.CallFn
	f.CallFn = func(ctx context.Context, dest, path, method string, args ...any) ([]any, error) {
		if method == "org.freedesktop.DBus.GetNameOwner" {
			if o, ok := owners[args[0].(string)]; ok {
				return []any{o}, nil
			}
			return nil, errors.New("name has no owner")
		}
		return inner(ctx, dest, path, method, args...)
	}
	return f
}

func TestInfoMapsMetadata(t *testing.T) {
	plex := &fakePlayer{props: map[string]any{
		"DesktopEntry": "tv.plex.PlexHTPC", "PlaybackStatus": "Playing",
		// As godbus decodes a{sv}: variants inside, int64 microseconds.
		"Metadata": map[string]dbus.Variant{
			"xesam:title":   dbus.MakeVariant("  DEMO Episode 3 "),
			"xesam:artist":  dbus.MakeVariant([]string{"DEMO Show", " ", "DEMO Guest"}),
			"xesam:album":   dbus.MakeVariant("DEMO Season 1"),
			"mpris:length":  dbus.MakeVariant(int64(2_640_000_000)),
			"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath("/demo/1")),
		},
		"Position": int64(754_000_000),
		"Rate":     1.5,
	}}
	p := NewPlayer(fakeBus(map[string]*fakePlayer{plexBus: plex}), plexBus)
	info, err := p.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != "Playing" || info.Title != "DEMO Episode 3" || info.Album != "DEMO Season 1" {
		t.Fatalf("info %+v", info)
	}
	if strings.Join(info.Artists, "|") != "DEMO Show|DEMO Guest" {
		t.Fatalf("artists %q", info.Artists)
	}
	if info.Length != 2640*time.Second || !info.HasPosition || info.Position != 754*time.Second || info.Rate != 1.5 {
		t.Fatalf("times %+v", info)
	}
}

func TestInfoToleratesMissingAndOddFields(t *testing.T) {
	cases := map[string]struct {
		props map[string]any
	}{
		"only a status": {
			props: map[string]any{"PlaybackStatus": "Paused"},
		},
		"artist as one string, uint64 length, int32 position": {
			props: map[string]any{"PlaybackStatus": "Paused", "Metadata": map[string]any{
				"xesam:title": "DEMO", "xesam:artist": "DEMO Artist", "mpris:length": uint64(5_000_000),
			}, "Position": int32(1_000_000)},
		},
		"wrong types are ignored": {
			props: map[string]any{"PlaybackStatus": "Stopped", "Metadata": "nope", "Position": "later", "Rate": "fast"},
		},
		"negative position and zero length are unknown": {
			props: map[string]any{"PlaybackStatus": "Playing", "Metadata": map[string]any{"xesam:title": "DEMO", "mpris:length": int64(0)}, "Position": int64(-5)},
		},
	}
	for name, tc := range cases {
		p := NewPlayer(fakeBus(map[string]*fakePlayer{plexBus: {props: tc.props}}), plexBus)
		info, err := p.Info(context.Background())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		switch name {
		case "only a status":
			if info.Status != "Paused" || info.Title != "" || info.Artists != nil || info.Length != 0 || info.HasPosition || info.Rate != 1 {
				t.Errorf("%s: %+v", name, info)
			}
		case "artist as one string, uint64 length, int32 position":
			if info.Title != "DEMO" || len(info.Artists) != 1 || info.Artists[0] != "DEMO Artist" || info.Length != 5*time.Second || !info.HasPosition || info.Position != time.Second {
				t.Errorf("%s: %+v", name, info)
			}
		case "wrong types are ignored":
			if info.Status != "Stopped" || info.Title != "" || info.HasPosition || info.Rate != 1 {
				t.Errorf("%s: %+v", name, info)
			}
		case "negative position and zero length are unknown":
			if info.HasPosition || info.Length != 0 || info.Title != "DEMO" {
				t.Errorf("%s: %+v", name, info)
			}
		}
	}
	// No status at all is a failed read.
	p := NewPlayer(fakeBus(map[string]*fakePlayer{plexBus: {props: map[string]any{"Metadata": map[string]any{"xesam:title": "DEMO"}}}}), plexBus)
	if _, err := p.Info(context.Background()); err == nil {
		t.Fatal("a player without PlaybackStatus read as fine")
	}
}

func TestListNeverCarriesTitles(t *testing.T) {
	plex := &fakePlayer{props: map[string]any{
		"Identity": "Plex", "DesktopEntry": "tv.plex.PlexHTPC", "PlaybackStatus": "Playing",
		"CanControl": true, "CanPause": true, "CanPlay": true, "CanSeek": true,
		"Metadata": map[string]any{"xesam:title": "DEMO Secret Title", "xesam:artist": []string{"DEMO Secret Artist"}},
	}}
	infos, err := NewLocator(fakeBus(map[string]*fakePlayer{plexBus: plex})).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(infos)
	if strings.Contains(string(raw), "Secret") {
		t.Fatalf("the probe listing names what is playing: %s", raw)
	}
}

func TestWatchOnlyThisPlayersSignals(t *testing.T) {
	players := map[string]*fakePlayer{plexBus: {props: map[string]any{"PlaybackStatus": "Playing"}}, "org.mpris.MediaPlayer2.other": {props: map[string]any{}}}
	bus := ownedBus(players, map[string]string{plexBus: ":1.10", "org.mpris.MediaPlayer2.other": ":1.11"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := NewPlayer(bus, plexBus).Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	changed := func(sender, path, iface string) dbusx.Signal {
		return dbusx.Signal{Sender: sender, Path: path, Name: "org.freedesktop.DBus.Properties.PropertiesChanged", Body: []any{iface, map[string]dbus.Variant{}, []string{}}}
	}
	expectNone := func(what string) {
		t.Helper()
		select {
		case <-ch:
			t.Fatalf("%s woke the watch", what)
		case <-time.After(30 * time.Millisecond):
		}
	}
	expectOne := func(what string) {
		t.Helper()
		select {
		case _, ok := <-ch:
			if !ok {
				t.Fatalf("%s: watch closed", what)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s did not wake the watch", what)
		}
	}
	bus.Emit(changed(":1.11", objectPath, playerIface))
	expectNone("another player's change")
	bus.Emit(changed(":1.10", "/org/other", playerIface))
	expectNone("a change on another object")
	bus.Emit(changed(":1.10", objectPath, rootIface))
	expectNone("a change of the root interface")
	bus.Emit(dbusx.Signal{Sender: ":1.11", Path: objectPath, Name: playerIface + ".Seeked", Body: []any{int64(1)}})
	expectNone("another player's seek")

	bus.Emit(changed(":1.10", objectPath, playerIface))
	expectOne("this player's change")
	bus.Emit(dbusx.Signal{Sender: ":1.10", Path: objectPath, Name: playerIface + ".Seeked", Body: []any{int64(1)}})
	expectOne("this player's seek")
	// A burst nobody has read yet coalesces into one pending wake-up.
	for i := 0; i < 5; i++ {
		bus.Emit(changed(":1.10", objectPath, playerIface))
	}
	time.Sleep(50 * time.Millisecond)
	expectOne("a burst")
	expectNone("the rest of a burst")

	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			<-ch
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watch not closed after cancel")
	}
}

func TestWatchFailsClosedWithoutAnOwner(t *testing.T) {
	bus := ownedBus(map[string]*fakePlayer{plexBus: {props: map[string]any{}}}, map[string]string{})
	if _, err := NewPlayer(bus, plexBus).Watch(context.Background()); err == nil {
		t.Fatal("a player whose owner is unknown was watched (its signals cannot be told apart)")
	}
}
