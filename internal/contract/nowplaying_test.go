// Tests for the state.now_playing type (contract.go): it marshals to what
// state.schema.json accepts, and never prints its titles through String or
// slog (docs/security.md).

package contract

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func demoNowPlaying() *NowPlaying {
	length, pos := int64(2_640_000), int64(754_000)
	return &NowPlaying{AppID: "plex-htpc", Title: "DEMO Secret Title", Subtitle: "DEMO Secret Show", Status: NowPlayingPlaying,
		LengthMs: &length, PositionMs: &pos, PositionAt: 203_500, Rate: 1}
}

func TestNowPlayingMarshalsToSchema(t *testing.T) {
	on := true
	st := State{
		Protocol: 1, ContextEpoch: 1, DeviceName: "Bear Den", ConfigRevision: 1,
		Session:       SessionState{DisplaySession: "x11", DesktopAdapter: "fake", ShellState: "running"},
		Target:        Target{Kind: "none", Label: "Nothing"},
		Capabilities:  map[string]Capability{},
		Shell:         ShellState{Screen: "home"},
		Applications:  []AppState{},
		Remote:        RemoteState{Transport: "local-only", Addresses: []string{}, Limits: DefaultLimits, NowPlaying: &on},
		Notifications: []Notification{},
		NowPlaying:    demoNowPlaying(),
	}
	raw, err := MarshalAndValidateState(st)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"title":"DEMO Secret Title"`) || !strings.Contains(string(raw), `"now_playing":true`) {
		t.Fatalf("snapshot lacks the fields: %s", raw)
	}
	// Optional members are omitted, not null.
	st.NowPlaying = &NowPlaying{AppID: "plex-htpc", Title: "DEMO", Status: NowPlayingPaused, PositionAt: 1, Rate: 1}
	raw, err = MarshalAndValidateState(st)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"subtitle", "length_ms", "position_ms"} {
		if strings.Contains(string(raw), `"`+k+`"`) {
			t.Fatalf("%s should be omitted: %s", k, raw)
		}
	}
	st.NowPlaying.Status = "buffering"
	if _, err := MarshalAndValidateState(st); err == nil {
		t.Fatal("status buffering validated")
	}
}

func TestNowPlayingNeverPrintsTitles(t *testing.T) {
	np := demoNowPlaying()
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})).Info("now playing", "np", np, "value", *np)
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("now playing", "np", *np)
	out := buf.String() + fmt.Sprint(np, *np) + fmt.Sprintf("%v %s", *np, np)
	if strings.Contains(out, "Secret") {
		t.Fatalf("a title leaked: %s", out)
	}
	if !strings.Contains(out, "plex-htpc") || !strings.Contains(out, "[title]") {
		t.Fatalf("redacted form lost its useful parts: %s", out)
	}
}
