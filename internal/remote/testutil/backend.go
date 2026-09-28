// FakeBackend: an in-memory remote.Backend for server tests.

package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	bdtv "bear-den-tv"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/remote"
)

// Submission records one Backend.Submit call.
type Submission struct {
	Viewer  remote.Viewer
	Request contract.ActionRequest
}

// HoldCall records one HoldStart/HoldRenew/HoldStop call.
type HoldCall struct {
	Kind   string // start | renew | stop
	Viewer remote.Viewer
	HoldID string
	Action string
	Reason string
}

type subscriber struct {
	deviceID string
	ch       chan contract.State
}

type resultSubscriber struct {
	deviceID string
	ch       chan contract.ActionResult
}

// FakeBackend is an in-memory remote.Backend. It starts from the
// state.phone-controller.valid.json fixture, records submissions and hold
// calls, and lets tests script results, hold statuses, layout errors, and
// diagnostics.
type FakeBackend struct {
	mu sync.Mutex

	state        contract.State
	subs         []subscriber
	resultSubs   []resultSubscriber
	submissions  []Submission
	holdCalls    []HoldCall
	holdSignal   chan HoldCall
	revision     int64
	layout       contract.Layout
	defaults     contract.Layout
	pending      *contract.LayoutPending
	previewing   bool
	undoRevision int64

	// SubmitFn overrides the default delivered result.
	SubmitFn func(v remote.Viewer, req contract.ActionRequest) contract.ActionResult
	// HoldStartFn overrides the default active status.
	HoldStartFn func(v remote.Viewer, m contract.HoldMessage) remote.HoldStatus
	// HoldRenewFn overrides the default active status.
	HoldRenewFn func(v remote.Viewer, holdID string) remote.HoldStatus
	// LayoutErr, when set, is returned by every layout write.
	LayoutErr error
	// DiagnosticsFn overrides the default report.
	DiagnosticsFn func(v remote.Viewer) (map[string]any, error)
	// SubscribeErr, when set, is returned by Subscribe and Results.
	SubscribeErr error
}

// NewFakeBackend loads the phone-controller fixture.
func NewFakeBackend() (*FakeBackend, error) {
	raw, err := bdtv.Contracts.ReadFile("contracts/fixtures/state.phone-controller.valid.json")
	if err != nil {
		return nil, err
	}
	var st contract.State
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("fixture: %w", err)
	}
	layout := contract.Layout{
		UI:       contract.UI{Theme: "den-dark", Accent: "#d9a441", Background: "#101418", TextScale: 1, TileDensity: "comfortable", SafeMarginPercent: 4, HeroEnabled: true, ClockEnabled: true},
		Sections: []contract.Section{{ID: "favorites", Title: "Favorites", Kind: "applications", Enabled: true, ApplicationIDs: []string{"plex-htpc", "youtube"}}},
	}
	return &FakeBackend{
		state:      st,
		holdSignal: make(chan HoldCall, 256),
		revision:   3,
		layout:     layout,
		defaults:   layout,
	}, nil
}

// State returns a copy of the current base state.
func (b *FakeBackend) State() contract.State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// SetState replaces the base state and pushes it to every subscriber.
func (b *FakeBackend) SetState(st contract.State) {
	b.mu.Lock()
	b.state = st
	subs := append([]subscriber(nil), b.subs...)
	b.mu.Unlock()
	for _, s := range subs {
		select {
		case s.ch <- b.view(st, s.deviceID):
		default:
		}
	}
}

// EmitResult delivers a late result to every Results subscriber of deviceID
// ("*" for all).
func (b *FakeBackend) EmitResult(deviceID string, res contract.ActionResult) {
	b.mu.Lock()
	subs := append([]resultSubscriber(nil), b.resultSubs...)
	b.mu.Unlock()
	for _, s := range subs {
		if deviceID == "*" || s.deviceID == deviceID {
			select {
			case s.ch <- res:
			default:
			}
		}
	}
}

// Submissions returns a copy of every recorded Submit call.
func (b *FakeBackend) Submissions() []Submission {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Submission(nil), b.submissions...)
}

// HoldCalls returns a copy of every recorded hold call.
func (b *FakeBackend) HoldCalls() []HoldCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]HoldCall(nil), b.holdCalls...)
}

// WaitHold blocks until a hold call of kind (and reason, when non-empty)
// arrives or timeout passes; ok=false on timeout.
func (b *FakeBackend) WaitHold(kind, reason string, timeout time.Duration) (HoldCall, bool) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case c := <-b.holdSignal:
			if c.Kind == kind && (reason == "" || c.Reason == reason) {
				return c, true
			}
		case <-deadline.C:
			return HoldCall{}, false
		}
	}
}

// Revision returns the current layout revision.
func (b *FakeBackend) Revision() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.revision
}

func (b *FakeBackend) view(st contract.State, deviceID string) contract.State {
	st.Pairing = nil
	if deviceID == "" {
		st.Me = nil
		st.Devices = nil
		return st
	}
	return st
}

func (b *FakeBackend) recordHold(c HoldCall) {
	b.mu.Lock()
	b.holdCalls = append(b.holdCalls, c)
	b.mu.Unlock()
	select {
	case b.holdSignal <- c:
	default:
	}
}

// Snapshot implements remote.Backend.
func (b *FakeBackend) Snapshot(_ context.Context, v *remote.Viewer) contract.State {
	b.mu.Lock()
	st := b.state
	b.mu.Unlock()
	if v == nil {
		return b.view(st, "")
	}
	st = b.view(st, v.DeviceID)
	st.Me = &contract.Me{DeviceID: v.DeviceID, DeviceName: v.DeviceName, Permissions: v.Permissions, TransportSecure: v.Secure}
	return st
}

// Subscribe implements remote.Backend.
func (b *FakeBackend) Subscribe(ctx context.Context, v *remote.Viewer) (<-chan contract.State, error) {
	if b.SubscribeErr != nil {
		return nil, b.SubscribeErr
	}
	ch := make(chan contract.State, 16)
	sub := subscriber{deviceID: v.DeviceID, ch: ch}
	b.mu.Lock()
	b.subs = append(b.subs, sub)
	b.mu.Unlock()
	go func() {
		<-ctx.Done()
		b.mu.Lock()
		for i, s := range b.subs {
			if s.ch == ch {
				b.subs = append(b.subs[:i], b.subs[i+1:]...)
				break
			}
		}
		b.mu.Unlock()
	}()
	return ch, nil
}

// Submit implements remote.Backend; the default result is delivered/ok.
func (b *FakeBackend) Submit(_ context.Context, v remote.Viewer, req contract.ActionRequest) contract.ActionResult {
	b.mu.Lock()
	b.submissions = append(b.submissions, Submission{Viewer: v, Request: req})
	st := b.state
	fn := b.SubmitFn
	b.mu.Unlock()
	if fn != nil {
		return fn(v, req)
	}
	return contract.Result(req, contract.OutcomeDelivered, st.ContextEpoch, st.Target.Info(), nil)
}

// Results implements remote.Backend.
func (b *FakeBackend) Results(ctx context.Context, v remote.Viewer) (<-chan contract.ActionResult, error) {
	if b.SubscribeErr != nil {
		return nil, b.SubscribeErr
	}
	ch := make(chan contract.ActionResult, 16)
	sub := resultSubscriber{deviceID: v.DeviceID, ch: ch}
	b.mu.Lock()
	b.resultSubs = append(b.resultSubs, sub)
	b.mu.Unlock()
	go func() {
		<-ctx.Done()
		b.mu.Lock()
		for i, s := range b.resultSubs {
			if s.ch == ch {
				b.resultSubs = append(b.resultSubs[:i], b.resultSubs[i+1:]...)
				break
			}
		}
		b.mu.Unlock()
	}()
	return ch, nil
}

// HoldStart implements remote.Backend.
func (b *FakeBackend) HoldStart(_ context.Context, v remote.Viewer, m contract.HoldMessage) remote.HoldStatus {
	b.recordHold(HoldCall{Kind: "start", Viewer: v, HoldID: m.HoldID, Action: m.Action})
	if b.HoldStartFn != nil {
		return b.HoldStartFn(v, m)
	}
	return remote.HoldStatus{HoldID: m.HoldID, State: "active"}
}

// HoldRenew implements remote.Backend.
func (b *FakeBackend) HoldRenew(_ context.Context, v remote.Viewer, holdID string) remote.HoldStatus {
	b.recordHold(HoldCall{Kind: "renew", Viewer: v, HoldID: holdID})
	if b.HoldRenewFn != nil {
		return b.HoldRenewFn(v, holdID)
	}
	return remote.HoldStatus{HoldID: holdID, State: "active"}
}

// HoldStop implements remote.Backend.
func (b *FakeBackend) HoldStop(_ context.Context, v remote.Viewer, holdID string, reason string) {
	b.recordHold(HoldCall{Kind: "stop", Viewer: v, HoldID: holdID, Reason: reason})
}

// Layout implements remote.Backend.
func (b *FakeBackend) Layout(_ context.Context, _ remote.Viewer) (remote.LayoutView, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return remote.LayoutView{Revision: b.revision, Layout: b.layout, Defaults: b.defaults, Pending: b.pending}, nil
}

// PutLayout implements remote.Backend with optimistic revision checking.
func (b *FakeBackend) PutLayout(_ context.Context, _ remote.Viewer, baseRevision int64, layout contract.Layout) (remote.PutLayoutResult, error) {
	if b.LayoutErr != nil {
		return remote.PutLayoutResult{}, b.LayoutErr
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if baseRevision != b.revision {
		return remote.PutLayoutResult{Revision: b.revision}, remote.ErrRevisionConflict
	}
	if len(layout.Sections) == 0 {
		return remote.PutLayoutResult{}, &remote.LayoutValidationError{Errors: []string{"sections must not be empty"}}
	}
	b.undoRevision = b.revision
	b.revision++
	b.layout = layout
	b.pending = &contract.LayoutPending{Revision: b.revision, PreviousRevision: b.undoRevision, ExpiresInS: 30, Source: "phone"}
	return remote.PutLayoutResult{Revision: b.revision, Pending: true}, nil
}

// PreviewLayout implements remote.Backend.
func (b *FakeBackend) PreviewLayout(_ context.Context, _ remote.Viewer, _ contract.Layout) error {
	if b.LayoutErr != nil {
		return b.LayoutErr
	}
	b.mu.Lock()
	b.previewing = true
	b.mu.Unlock()
	return nil
}

// EndPreview implements remote.Backend.
func (b *FakeBackend) EndPreview(_ context.Context, _ remote.Viewer) error {
	if b.LayoutErr != nil {
		return b.LayoutErr
	}
	b.mu.Lock()
	b.previewing = false
	b.mu.Unlock()
	return nil
}

// Previewing reports whether a preview is active.
func (b *FakeBackend) Previewing() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.previewing
}

// ConfirmLayout implements remote.Backend.
func (b *FakeBackend) ConfirmLayout(_ context.Context, _ remote.Viewer, revision int64) error {
	if b.LayoutErr != nil {
		return b.LayoutErr
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pending == nil || b.pending.Revision != revision {
		return remote.ErrRevisionConflict
	}
	b.pending = nil
	return nil
}

// CancelLayout implements remote.Backend.
func (b *FakeBackend) CancelLayout(_ context.Context, _ remote.Viewer, revision int64) error {
	if b.LayoutErr != nil {
		return b.LayoutErr
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pending == nil || b.pending.Revision != revision {
		return remote.ErrRevisionConflict
	}
	b.revision = b.pending.PreviousRevision
	b.pending = nil
	return nil
}

// UndoLayout implements remote.Backend.
func (b *FakeBackend) UndoLayout(_ context.Context, _ remote.Viewer) (int64, error) {
	if b.LayoutErr != nil {
		return 0, b.LayoutErr
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.revision++
	b.layout = b.defaults
	return b.revision, nil
}

// ResetLayout implements remote.Backend.
func (b *FakeBackend) ResetLayout(_ context.Context, _ remote.Viewer, _ string) (int64, error) {
	if b.LayoutErr != nil {
		return 0, b.LayoutErr
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.revision++
	b.layout = b.defaults
	return b.revision, nil
}

// Diagnostics implements remote.Backend.
func (b *FakeBackend) Diagnostics(_ context.Context, v remote.Viewer) (map[string]any, error) {
	if b.DiagnosticsFn != nil {
		return b.DiagnosticsFn(v)
	}
	return map[string]any{"ok": true, "display_session": "x11"}, nil
}

var _ remote.Backend = (*FakeBackend)(nil)
