// Tests for MPRIS player matching and control (mpris.go).

package mpris

import (
	"context"
	"errors"
	"testing"

	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/dbusx"
)

type fakePlayer struct {
	props map[string]any
	calls []string
}

func fakeBus(players map[string]*fakePlayer) *dbusx.Fake {
	return &dbusx.Fake{
		NamesFn: func(context.Context) ([]string, error) {
			names := []string{":1.7", "org.freedesktop.DBus"}
			for n := range players {
				names = append(names, n)
			}
			return names, nil
		},
		PropertyFn: func(ctx context.Context, dest, path, iface, name string) (any, error) {
			p, ok := players[dest]
			if !ok || path != objectPath {
				return nil, errors.New("no such player " + dest)
			}
			v, ok := p.props[name]
			if !ok {
				return nil, errors.New("no property " + name)
			}
			return v, nil
		},
		CallFn: func(ctx context.Context, dest, path, method string, args ...any) ([]any, error) {
			p, ok := players[dest]
			if !ok {
				return nil, errors.New("no such player " + dest)
			}
			call := method
			for _, a := range args {
				call += " " + itoa(a)
			}
			p.calls = append(p.calls, call)
			return nil, nil
		},
	}
}

func itoa(a any) string {
	switch v := a.(type) {
	case int64:
		if v < 0 {
			return "-" + itoa(-v)
		}
		if v == 0 {
			return "0"
		}
		var s string
		for v > 0 {
			s = string(rune('0'+v%10)) + s
			v /= 10
		}
		return s
	}
	return "?"
}

func TestMatches(t *testing.T) {
	cases := []struct {
		bus, entry, match string
		want              bool
	}{
		{"org.mpris.MediaPlayer2.tv.plex.PlexHTPC", "", "tv.plex.PlexHTPC", true},
		{"org.mpris.MediaPlayer2.chromium.instance2345", "rocks.shy.VacuumTube", "rocks.shy.VacuumTube", true},
		{"org.mpris.MediaPlayer2.vlc.instance99", "vlc", "VLC", true},
		{"org.mpris.MediaPlayer2.vlc", "vlc", "tv.plex.PlexHTPC", false},
		{"org.mpris.MediaPlayer2.plexhtpc", "", "plex", false},
		{"org.mpris.MediaPlayer2.plex.other", "", "plex", false},
		{"org.mpris.MediaPlayer2.vlc", "vlc", "", false},
		{"org.other.Name", "tv.plex.PlexHTPC", "tv.plex.PlexHTPC", true},
	}
	for _, tc := range cases {
		if got := Matches(tc.bus, tc.entry, tc.match); got != tc.want {
			t.Errorf("Matches(%q,%q,%q)=%v want %v", tc.bus, tc.entry, tc.match, got, tc.want)
		}
	}
}

func TestFindAndControl(t *testing.T) {
	plex := &fakePlayer{props: map[string]any{
		"Identity": "Plex HTPC", "DesktopEntry": "tv.plex.PlexHTPC", "PlaybackStatus": "Playing",
		"CanControl": true, "CanPause": true, "CanPlay": true, "CanSeek": false,
	}}
	players := map[string]*fakePlayer{"org.mpris.MediaPlayer2.plexhtpc": plex}
	l := NewLocator(fakeBus(players))
	ctx := context.Background()

	if _, ok, err := l.Find(ctx, platform.MediaMatch{FlatpakID: "rocks.shy.VacuumTube", Names: []string{"rocks.shy.VacuumTube"}}); ok || err != nil {
		t.Fatalf("unexpected find ok=%v err=%v", ok, err)
	}
	p, ok, err := l.Find(ctx, platform.MediaMatch{FlatpakID: "tv.plex.PlexHTPC", Names: []string{"tv.plex.PlexHTPC"}})
	if err != nil || !ok {
		t.Fatalf("find err=%v ok=%v", err, ok)
	}
	if s, _ := p.Status(ctx); s != "Playing" {
		t.Fatalf("status %q", s)
	}
	if err := p.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.Play(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.SeekRelative(ctx, -30); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("seek err=%v want ErrUnsupported", err)
	}
	want := []string{"org.mpris.MediaPlayer2.Player.Pause", "org.mpris.MediaPlayer2.Player.Play"}
	if len(plex.calls) != len(want) || plex.calls[0] != want[0] || plex.calls[1] != want[1] {
		t.Fatalf("calls %v", plex.calls)
	}
	plex.props["CanSeek"] = true
	if err := p.SeekRelative(ctx, -30); err != nil {
		t.Fatal(err)
	}
	if got := plex.calls[len(plex.calls)-1]; got != "org.mpris.MediaPlayer2.Player.Seek -30000000" {
		t.Fatalf("seek call %q", got)
	}
	plex.props["CanPause"] = false
	if err := p.Pause(ctx); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("pause err=%v", err)
	}
	plex.props["PlaybackStatus"] = "Weird"
	if s, err := p.Status(ctx); s != "Unknown" || err != nil {
		t.Fatalf("status %q %v", s, err)
	}
}

func TestQuitOnlyWhenThePlayerAllowsIt(t *testing.T) {
	ctx := context.Background()
	players := map[string]*fakePlayer{
		Prefix + "spotify":  {props: map[string]any{"CanQuit": true}},
		Prefix + "stubborn": {props: map[string]any{"CanQuit": false}},
	}
	bus := fakeBus(players)
	if err := NewPlayer(bus, Prefix+"spotify").Quit(ctx); err != nil {
		t.Fatal(err)
	}
	if got := players[Prefix+"spotify"].calls; len(got) != 1 || got[0] != rootIface+".Quit" {
		t.Fatalf("calls %v", got)
	}
	if err := NewPlayer(bus, Prefix+"stubborn").Quit(ctx); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("CanQuit=false: %v", err)
	}
	if len(players[Prefix+"stubborn"].calls) != 0 {
		t.Fatal("Quit was called on a player that does not allow it")
	}
	var _ platform.MediaQuitter = (*Player)(nil)
}

func TestListRecordsPerPlayerErrors(t *testing.T) {
	players := map[string]*fakePlayer{
		"org.mpris.MediaPlayer2.b": {props: map[string]any{"Identity": "B", "DesktopEntry": "b", "PlaybackStatus": "Paused", "CanControl": true, "CanPause": true, "CanPlay": true, "CanSeek": true}},
		"org.mpris.MediaPlayer2.a": {props: map[string]any{"Identity": "A"}},
	}
	infos, err := NewLocator(fakeBus(players)).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 || infos[0].BusName != "org.mpris.MediaPlayer2.a" || infos[1].Identity != "B" {
		t.Fatalf("infos %+v", infos)
	}
	if len(infos[0].Errors) == 0 || infos[0].PlaybackStatus != "Unknown" || len(infos[1].Errors) != 0 {
		t.Fatalf("infos %+v", infos)
	}
}
