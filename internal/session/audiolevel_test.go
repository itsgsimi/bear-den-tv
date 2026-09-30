// state.audio (coordinator.go readAudioLevel, audioStateLocked): phones see
// the PC's real mute state and volume, read back right after a mute and on
// the audio probe, whoever changed it; the shell never gets it.

package session

import (
	"context"
	"sync"
	"testing"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
)

// levelAudio is a PC sink that remembers its mute flag and volume and can
// read them back (platform.AudioReader).
type levelAudio struct {
	mu      sync.Mutex
	muted   bool
	percent int
}

func (a *levelAudio) Capability() platform.Capability {
	return platform.Capability{Available: true, Backend: "fake-pulse"}
}
func (a *levelAudio) VolumeDelta(_ context.Context, p int) error {
	a.mu.Lock()
	a.percent += p
	a.mu.Unlock()
	return nil
}
func (a *levelAudio) SetMute(_ context.Context, m bool) error {
	a.mu.Lock()
	a.muted = m
	a.mu.Unlock()
	return nil
}
func (a *levelAudio) Level(context.Context) (platform.AudioLevel, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return platform.AudioLevel{Muted: a.muted, Percent: a.percent}, nil
}

func TestPhonesSeeThePCsRealMuteState(t *testing.T) {
	la := &levelAudio{percent: 40}
	h := newHarness(t, func(o *Options) { o.Audio = la })
	ctx := context.Background()
	audioOf := func() *contract.AudioState { return h.phones.Snapshot(ctx, &h.ctl).Audio }
	h.eventually("the first reading", func() bool {
		a := audioOf()
		return a != nil && a.VolumePercent != nil && *a.VolumePercent == 40
	})
	if a := audioOf(); a.Muted {
		t.Fatalf("audio = %+v", a)
	}
	if h.c.buildState(viewShell).Audio != nil {
		t.Fatal("the shell got state.audio")
	}
	// The phone's mute is read back at once.
	res := h.submit(h.ctl, h.req(contract.ActionAudioMute, map[string]any{"muted": true}))
	expectOutcome(t, res, contract.OutcomeDelivered, contract.CodeOK)
	if a := audioOf(); a == nil || !a.Muted {
		t.Fatalf("after mute: %+v", a)
	}
	// Someone else unmutes (the keyboard, another app): the next reading shows it.
	_ = la.SetMute(ctx, false)
	h.c.readAudioLevel(ctx)
	if a := audioOf(); a == nil || a.Muted {
		t.Fatalf("after an unmute elsewhere: %+v", a)
	}
	if _, err := contract.MarshalAndValidateState(h.phones.Snapshot(ctx, &h.ctl)); err != nil {
		t.Fatal(err)
	}
}
