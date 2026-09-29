// Tests for the fake media player (media.go): positions follow the injected
// clock while playing, controls signal watchers, Find matches exactly.

package fake

import (
	"context"
	"testing"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/platform"
)

func TestFakePlayerFollowsTheClock(t *testing.T) {
	clk := clock.NewFake(time.Unix(1000, 0))
	p := NewPlayer(clk, platform.MediaInfo{Status: "Playing", Title: "DEMO", Length: time.Minute, Position: 10 * time.Second, HasPosition: true})
	ctx := context.Background()
	ch, _ := p.Watch(ctx)
	clk.Advance(5 * time.Second)
	if info, _ := p.Info(ctx); info.Position != 15*time.Second || info.Rate != 1 {
		t.Fatalf("after 5 s playing: %v", info.Position)
	}
	_ = p.Pause(ctx)
	<-ch
	clk.Advance(30 * time.Second)
	if info, _ := p.Info(ctx); info.Position != 15*time.Second || info.Status != "Paused" {
		t.Fatalf("paused: %+v", info)
	}
	_ = p.SeekRelative(ctx, -20)
	if info, _ := p.Info(ctx); info.Position != 0 {
		t.Fatalf("seek below zero: %v", info.Position)
	}
	_ = p.Play(ctx)
	clk.Advance(2 * time.Minute)
	if info, _ := p.Info(ctx); info.Position != time.Minute {
		t.Fatalf("position past the length: %v", info.Position)
	}
	if got := p.Calls(); len(got) != 3 || got[0] != "Pause" || got[2] != "Play" {
		t.Fatalf("calls %v", got)
	}

	m := NewMedia()
	m.Add("tv.plex.PlexHTPC", p)
	if _, ok, _ := m.Find(ctx, "tv.plex"); ok {
		t.Fatal("a partial match found a player")
	}
	if got, ok, _ := m.Find(ctx, "tv.plex.PlexHTPC"); !ok || got != p {
		t.Fatal("exact match not found")
	}
}
