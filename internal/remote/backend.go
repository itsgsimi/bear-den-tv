// Package remote serves the LAN phone remote: static assets, pairing, cookie
// sessions, CSRF, per-action authorization, WebSocket events, rate limits,
// Host/Origin checks, and revocation. It talks to the coordinator through the
// Backend interface so it can be tested against a fake.
package remote

import (
	"context"

	"bear-den-tv/internal/contract"
)

// Viewer identifies the authenticated device submitting a request.
type Viewer struct {
	DeviceID    string
	DeviceName  string
	Permissions []contract.Permission
	Secure      bool // transport is HTTPS
	// ExpiresAtMs is when a guest pass ends (Unix epoch ms, wall clock); nil
	// for family phones (contracts/http.md#guest-passes).
	ExpiresAtMs *int64
}

// Guest reports whether the viewer holds a guest pass (the guest permission).
func (v Viewer) Guest() bool {
	for _, q := range v.Permissions {
		if q == contract.PermGuest {
			return true
		}
	}
	return false
}

// Has reports whether the viewer holds at least the given permission. A
// viewer holding guest is a guest pass whatever else it lists (fail closed):
// it has guest and nothing above it.
func (v Viewer) Has(p contract.Permission) bool {
	if v.Guest() {
		return p.Rank() > 0 && p.Rank() <= contract.PermGuest.Rank()
	}
	best := 0
	for _, q := range v.Permissions {
		if q.Rank() > best {
			best = q.Rank()
		}
	}
	return best >= p.Rank()
}

// Backend is what the coordinator exposes to the remote server.
type Backend interface {
	// Snapshot returns the state redacted for the viewer (nil viewer = anonymous /api/v1/info only).
	Snapshot(ctx context.Context, v *Viewer) contract.State
	// Subscribe returns a channel that receives a new snapshot whenever state
	// changes, until ctx is done. Implementations may coalesce.
	Subscribe(ctx context.Context, v *Viewer) (<-chan contract.State, error)
	// Submit validates authorization for the viewer and routes the action.
	// The returned result is the first terminal or accepted result; later
	// results for asynchronous actions are delivered via Results.
	Submit(ctx context.Context, v Viewer, req contract.ActionRequest) contract.ActionResult
	// Results streams late results (accepted → delivered/observed/failed) for
	// the viewer's requests.
	Results(ctx context.Context, v Viewer) (<-chan contract.ActionResult, error)
	// Holds: server-side leases (contracts/actions.md#holds).
	HoldStart(ctx context.Context, v Viewer, m contract.HoldMessage) HoldStatus
	HoldRenew(ctx context.Context, v Viewer, holdID string) HoldStatus
	HoldStop(ctx context.Context, v Viewer, holdID string, reason string)
	// Layout editing (layout_editor permission checked by the server; the
	// backend enforces transport rules again).
	Layout(ctx context.Context, v Viewer) (LayoutView, error)
	PutLayout(ctx context.Context, v Viewer, baseRevision int64, layout contract.Layout) (PutLayoutResult, error)
	PreviewLayout(ctx context.Context, v Viewer, layout contract.Layout) error
	EndPreview(ctx context.Context, v Viewer) error
	ConfirmLayout(ctx context.Context, v Viewer, revision int64) error
	CancelLayout(ctx context.Context, v Viewer, revision int64) error
	UndoLayout(ctx context.Context, v Viewer) (int64, error)
	ResetLayout(ctx context.Context, v Viewer, sectionID string) (int64, error)
	// Diagnostics returns the redacted doctor report (owner only).
	Diagnostics(ctx context.Context, v Viewer) (map[string]any, error)
}

// HoldStatus is reported to the phone as a hold event.
type HoldStatus struct {
	HoldID string `json:"hold_id"`
	State  string `json:"state"` // active | expired | cancelled | busy | rejected
	Reason string `json:"reason,omitempty"`
}

// LayoutView is GET /api/v1/layout.
type LayoutView struct {
	Revision int64                   `json:"revision"`
	Layout   contract.Layout         `json:"layout"`
	Defaults contract.Layout         `json:"defaults"`
	Pending  *contract.LayoutPending `json:"pending"`
}

// PutLayoutResult is the success body of PUT /api/v1/layout.
type PutLayoutResult struct {
	Revision int64 `json:"revision"`
	Pending  bool  `json:"pending"`
}

// Devices is the pairing/device store the server needs.
type Devices interface {
	// Claim redeems an invitation (fragment token or six-digit code) and creates
	// a device + session. Errors: ErrInvalidInvitation, ErrInvitationExpired,
	// ErrTooManyAttempts.
	Claim(ctx context.Context, token, code, deviceName, sourceAddr string) (ClaimResult, error)
	// Authenticate resolves a session token to a viewer; ok=false when unknown or revoked.
	Authenticate(ctx context.Context, sessionToken string) (Viewer, string /*csrf*/, bool)
	// Logout ends one session.
	Logout(ctx context.Context, sessionToken string) error
	// List, Revoke: owner operations. Revoke("*") revokes every device.
	List(ctx context.Context) ([]contract.Device, error)
	Revoke(ctx context.Context, deviceID string) error
	// Revocations emits device ids as they are revoked so the server can close sockets.
	Revocations(ctx context.Context) (<-chan string, error)
	// Touch records activity/connection state for the device list.
	Touch(ctx context.Context, deviceID string, connected bool)
}

// ClaimResult is the success body of POST /api/v1/pair/claim plus the cookie value.
type ClaimResult struct {
	DeviceID     string
	DeviceName   string
	Permissions  []contract.Permission
	SessionToken string
	CSRFToken    string
}
