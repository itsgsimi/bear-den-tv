// PhoneBackend: the coordinator as seen by the LAN remote (package
// internal/remote).

package session

import (
	"bytes"
	"context"
	"errors"

	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/remote"
	"bear-den-tv/internal/shellipc"
)

// PhoneBackend adapts the coordinator to remote.Backend.
type PhoneBackend struct{ c *Coordinator }

// Phones returns the remote.Backend view of the coordinator.
func (c *Coordinator) Phones() *PhoneBackend { return &PhoneBackend{c: c} }

var _ remote.Backend = (*PhoneBackend)(nil)

// Snapshot implements remote.Backend.
func (b *PhoneBackend) Snapshot(_ context.Context, v *remote.Viewer) contract.State {
	if v == nil {
		return b.c.buildStateFor(viewAnonymous, nil)
	}
	return b.c.buildStateFor(viewPhone, v)
}

// Subscribe implements remote.Backend; snapshots are coalesced per wake-up.
func (b *PhoneBackend) Subscribe(ctx context.Context, v *remote.Viewer) (<-chan contract.State, error) {
	wake := b.c.subscribe(ctx)
	if v != nil && v.Has(contract.PermController) {
		b.c.phoneStreamOpened(ctx) // keeps a playing player's position fresh
	}
	out := make(chan contract.State, 1)
	go func() {
		defer close(out)
		var last []byte
		for {
			select {
			case <-ctx.Done():
				return
			case <-wake:
			}
			st := b.Snapshot(ctx, v)
			key := stateKey(st)
			if bytes.Equal(key, last) {
				continue
			}
			last = key
			select {
			case out <- st:
			default:
				// Replace a stale undelivered snapshot with the newest one.
				select {
				case <-out:
				default:
				}
				out <- st
			}
		}
	}()
	return out, nil
}

// Submit implements remote.Backend.
func (b *PhoneBackend) Submit(ctx context.Context, v remote.Viewer, req contract.ActionRequest) contract.ActionResult {
	return b.c.submit(ctx, phoneSender(v), req)
}

// Results implements remote.Backend.
func (b *PhoneBackend) Results(ctx context.Context, v remote.Viewer) (<-chan contract.ActionResult, error) {
	ch := make(chan contract.ActionResult, 16)
	b.c.mu.Lock()
	b.c.results[ch] = v.DeviceID
	b.c.mu.Unlock()
	go func() {
		<-ctx.Done()
		b.c.mu.Lock()
		delete(b.c.results, ch)
		b.c.mu.Unlock()
	}()
	return ch, nil
}

// HoldStart implements remote.Backend. Holds obey the same authorization,
// lock, epoch, and capability rules as single taps.
func (b *PhoneBackend) HoldStart(ctx context.Context, v remote.Viewer, m contract.HoldMessage) remote.HoldStatus {
	if !v.Has(contract.PermController) {
		return remote.HoldStatus{HoldID: m.HoldID, State: "rejected", Reason: "forbidden"}
	}
	epoch, _, locked := b.c.current()
	switch {
	case locked:
		return remote.HoldStatus{HoldID: m.HoldID, State: "rejected", Reason: "locked"}
	case m.ContextEpoch != epoch:
		return remote.HoldStatus{HoldID: m.HoldID, State: "rejected", Reason: "stale_epoch"}
	}
	if cp, _ := b.c.capability(m.Action); !cp.Available || !cp.Holdable {
		return remote.HoldStatus{HoldID: m.HoldID, State: "rejected", Reason: "not_holdable"}
	}
	st := b.c.holds.Start(v.DeviceID, m.HoldID, m.Action, m.ContextEpoch)
	return remote.HoldStatus{HoldID: st.HoldID, State: st.State, Reason: st.Reason}
}

// HoldRenew implements remote.Backend.
func (b *PhoneBackend) HoldRenew(_ context.Context, v remote.Viewer, holdID string) remote.HoldStatus {
	st := b.c.holds.Renew(v.DeviceID, holdID)
	return remote.HoldStatus{HoldID: st.HoldID, State: st.State, Reason: st.Reason}
}

// HoldStop implements remote.Backend.
func (b *PhoneBackend) HoldStop(_ context.Context, v remote.Viewer, holdID, reason string) {
	b.c.holds.Stop(v.DeviceID, holdID, reason)
}

// Layout implements remote.Backend.
func (b *PhoneBackend) Layout(_ context.Context, v remote.Viewer) (remote.LayoutView, error) {
	if !v.Has(contract.PermLayoutEditor) {
		return remote.LayoutView{}, remote.ErrForbidden
	}
	cfg := b.c.opts.Config.Current()
	return remote.LayoutView{Revision: cfg.Revision, Layout: cfg.Layout(), Defaults: config.Defaults().Layout(), Pending: b.c.opts.Config.Pending()}, nil
}

// layoutAllowed re-checks permission and the transport rule in the backend.
func (b *PhoneBackend) layoutAllowed(v remote.Viewer) error {
	if !v.Has(contract.PermLayoutEditor) {
		return remote.ErrForbidden
	}
	cfg := b.c.opts.Config.Current()
	if !v.Secure && cfg.Remote.Transport != "https" && !cfg.Remote.LayoutEditingOverHTTP() && !b.c.opts.DevMode {
		return remote.ErrTransport
	}
	if _, _, locked := b.c.current(); locked {
		return remote.ErrForbidden
	}
	return nil
}

func mapLayoutErr(err error) error {
	var ce *config.Errors
	switch {
	case err == nil:
		return nil
	case errors.As(err, &ce):
		return &remote.LayoutValidationError{Errors: ce.Items}
	case errors.Is(err, config.ErrRevisionConflict), errors.Is(err, config.ErrPendingChange), errors.Is(err, config.ErrNoPending):
		return remote.ErrRevisionConflict
	case errors.Is(err, config.ErrUnknownSection):
		return &remote.LayoutValidationError{Errors: []string{err.Error()}}
	case errors.Is(err, config.ErrNoUndo):
		return remote.ErrRevisionConflict
	}
	return err
}

// PutLayout implements remote.Backend.
func (b *PhoneBackend) PutLayout(_ context.Context, v remote.Viewer, baseRevision int64, layout contract.Layout) (remote.PutLayoutResult, error) {
	if err := b.layoutAllowed(v); err != nil {
		return remote.PutLayoutResult{}, err
	}
	res, err := b.c.opts.Config.ApplyLayout(baseRevision, layout, "web")
	if err != nil {
		return remote.PutLayoutResult{Revision: b.c.opts.Config.Revision()}, mapLayoutErr(err)
	}
	if res.Pending {
		b.c.askConfirm(res.Revision)
	}
	return remote.PutLayoutResult{Revision: res.Revision, Pending: res.Pending}, nil
}

// PreviewLayout implements remote.Backend.
func (b *PhoneBackend) PreviewLayout(_ context.Context, v remote.Viewer, layout contract.Layout) error {
	if err := b.layoutAllowed(v); err != nil {
		return err
	}
	if err := config.ValidateLayoutAgainst(b.c.opts.Config.Current(), layout); err != nil {
		return mapLayoutErr(err)
	}
	sh := b.c.shellClient()
	if sh == nil {
		return errors.New("shell not connected")
	}
	b.c.mu.Lock()
	b.c.previewing = true
	b.c.mu.Unlock()
	return sh.Send(shellipc.LayoutPreview{Type: shellipc.TypeLayoutPreview, Layout: layout, ExpiresInS: 60})
}

// EndPreview implements remote.Backend.
func (b *PhoneBackend) EndPreview(_ context.Context, v remote.Viewer) error {
	if err := b.layoutAllowed(v); err != nil {
		return err
	}
	b.c.mu.Lock()
	b.c.previewing = false
	b.c.mu.Unlock()
	if sh := b.c.shellClient(); sh != nil {
		return sh.Send(shellipc.LayoutPreviewEnd{Type: shellipc.TypeLayoutPreviewEnd})
	}
	return nil
}

// ConfirmLayout implements remote.Backend.
func (b *PhoneBackend) ConfirmLayout(_ context.Context, v remote.Viewer, revision int64) error {
	if err := b.layoutAllowed(v); err != nil {
		return err
	}
	return mapLayoutErr(b.c.opts.Config.Confirm(revision))
}

// CancelLayout implements remote.Backend.
func (b *PhoneBackend) CancelLayout(_ context.Context, v remote.Viewer, revision int64) error {
	if err := b.layoutAllowed(v); err != nil {
		return err
	}
	return mapLayoutErr(b.c.opts.Config.Cancel(revision))
}

// UndoLayout implements remote.Backend.
func (b *PhoneBackend) UndoLayout(_ context.Context, v remote.Viewer) (int64, error) {
	if err := b.layoutAllowed(v); err != nil {
		return 0, err
	}
	rev, err := b.c.opts.Config.Undo()
	return rev, mapLayoutErr(err)
}

// ResetLayout implements remote.Backend.
func (b *PhoneBackend) ResetLayout(_ context.Context, v remote.Viewer, sectionID string) (int64, error) {
	if err := b.layoutAllowed(v); err != nil {
		return 0, err
	}
	rev, err := b.c.opts.Config.Reset(sectionID)
	return rev, mapLayoutErr(err)
}

// Diagnostics implements remote.Backend.
func (b *PhoneBackend) Diagnostics(ctx context.Context, v remote.Viewer) (map[string]any, error) {
	if !v.Has(contract.PermOwner) {
		return nil, remote.ErrForbidden
	}
	if b.c.opts.Diagnostics == nil {
		return map[string]any{}, nil
	}
	return b.c.opts.Diagnostics(ctx), nil
}

// askConfirm shows the TV's timed Keep/Revert dialog for a pending change.
// The reply arrives as confirm_result in Receive; the store's own timer rolls
// back if the dialog is never answered.
func (c *Coordinator) askConfirm(revision int64) {
	sh := c.shellClient()
	p := c.opts.Config.Pending()
	if sh == nil || p == nil || p.Revision != revision {
		return
	}
	id := "layout-" + randomID()
	c.mu.Lock()
	c.confirms[id] = revision
	c.mu.Unlock()
	_ = sh.Send(shellipc.ConfirmRequest{
		Type: shellipc.TypeConfirmRequest, ConfirmID: id, ConfirmKind: "layout",
		Summary: "Keep these display changes?", ExpiresInS: p.ExpiresInS,
	})
}
