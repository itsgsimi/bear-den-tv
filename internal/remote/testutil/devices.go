// FakeDevices: in-memory pairing and sessions for server tests.

package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/remote"
)

// MaxAttempts is the per-invitation failure budget (contracts/http.md).
const MaxAttempts = 5

// Invitation is the single active TV-issued invitation.
type Invitation struct {
	Token       string
	Code        string
	Permissions []contract.Permission
	ExpiresAt   time.Time
	Attempts    int
}

// TouchCall records one Devices.Touch call.
type TouchCall struct {
	DeviceID  string
	Connected bool
}

type session struct {
	deviceID string
	csrf     string
}

type device struct {
	rec contract.Device
}

// FakeDevices is an in-memory remote.Devices: one active invitation,
// sessions keyed by token, devices, and a broadcast revocation channel.
type FakeDevices struct {
	clock remote.Clock

	mu         sync.Mutex
	invitation *Invitation
	sessions   map[string]session
	devices    map[string]*device
	revSubs    []chan string
	touches    []TouchCall
	nextID     int

	// ClaimErr, when set, is returned by Claim before any lookup.
	ClaimErr error
	// RevocationsErr, when set, is returned by Revocations.
	RevocationsErr error
}

// NewFakeDevices returns an empty store using clock for invitation expiry.
func NewFakeDevices(clock remote.Clock) *FakeDevices {
	return &FakeDevices{clock: clock, sessions: map[string]session{}, devices: map[string]*device{}}
}

func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// SetInvitation makes inv the active invitation, replacing any previous one.
func (d *FakeDevices) SetInvitation(inv Invitation) {
	d.mu.Lock()
	defer d.mu.Unlock()
	copyInv := inv
	d.invitation = &copyInv
}

// Pair creates a device and session directly (as if already paired) and
// returns the device id, the session cookie value, and the CSRF token.
func (d *FakeDevices) Pair(name string, perms ...contract.Permission) (deviceID, sessionToken, csrf string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(perms) == 0 {
		perms = []contract.Permission{contract.PermController}
	}
	return d.createLocked(name, perms)
}

func (d *FakeDevices) createLocked(name string, perms []contract.Permission) (deviceID, sessionToken, csrf string) {
	d.nextID++
	deviceID = fmt.Sprintf("dev_%02d", d.nextID)
	d.devices[deviceID] = &device{rec: contract.Device{ID: deviceID, Name: name, Permissions: perms, CreatedAt: d.clock.Now().UTC().Format(time.RFC3339)}}
	sessionToken = randomToken()
	csrf = randomToken()
	d.sessions[sessionToken] = session{deviceID: deviceID, csrf: csrf}
	return deviceID, sessionToken, csrf
}

// Claim implements remote.Devices against the single active invitation.
func (d *FakeDevices) Claim(_ context.Context, token, code, deviceName, _ string) (remote.ClaimResult, error) {
	if d.ClaimErr != nil {
		return remote.ClaimResult{}, d.ClaimErr
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	inv := d.invitation
	if inv == nil {
		return remote.ClaimResult{}, remote.ErrInvalidInvitation
	}
	if d.clock.Now().After(inv.ExpiresAt) {
		return remote.ClaimResult{}, remote.ErrInvitationExpired
	}
	if inv.Attempts >= MaxAttempts {
		return remote.ClaimResult{}, remote.ErrTooManyAttempts
	}
	matched := (token != "" && token == inv.Token) || (code != "" && code == inv.Code)
	if !matched {
		inv.Attempts++
		return remote.ClaimResult{}, remote.ErrInvalidInvitation
	}
	d.invitation = nil
	id, sess, csrf := d.createLocked(deviceName, inv.Permissions)
	return remote.ClaimResult{DeviceID: id, DeviceName: deviceName, Permissions: inv.Permissions, SessionToken: sess, CSRFToken: csrf}, nil
}

// Authenticate implements remote.Devices.
func (d *FakeDevices) Authenticate(_ context.Context, sessionToken string) (remote.Viewer, string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[sessionToken]
	if !ok {
		return remote.Viewer{}, "", false
	}
	dev, ok := d.devices[s.deviceID]
	if !ok {
		return remote.Viewer{}, "", false
	}
	return remote.Viewer{DeviceID: dev.rec.ID, DeviceName: dev.rec.Name, Permissions: dev.rec.Permissions}, s.csrf, true
}

// Logout implements remote.Devices.
func (d *FakeDevices) Logout(_ context.Context, sessionToken string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.sessions[sessionToken]; !ok {
		return errors.New("unknown session")
	}
	delete(d.sessions, sessionToken)
	return nil
}

// List implements remote.Devices.
func (d *FakeDevices) List(_ context.Context) ([]contract.Device, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]contract.Device, 0, len(d.devices))
	for _, dev := range d.devices {
		out = append(out, dev.rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Revoke implements remote.Devices; "*" revokes every device.
func (d *FakeDevices) Revoke(_ context.Context, deviceID string) error {
	d.mu.Lock()
	var ids []string
	if deviceID == "*" {
		for id := range d.devices {
			ids = append(ids, id)
		}
	} else {
		if _, ok := d.devices[deviceID]; !ok {
			d.mu.Unlock()
			return fmt.Errorf("device %q not found", deviceID)
		}
		ids = []string{deviceID}
	}
	for _, id := range ids {
		delete(d.devices, id)
		for tok, s := range d.sessions {
			if s.deviceID == id {
				delete(d.sessions, tok)
			}
		}
	}
	subs := append([]chan string(nil), d.revSubs...)
	d.mu.Unlock()
	for _, id := range ids {
		for _, ch := range subs {
			select {
			case ch <- id:
			default:
			}
		}
	}
	return nil
}

// Revocations implements remote.Devices.
func (d *FakeDevices) Revocations(ctx context.Context) (<-chan string, error) {
	if d.RevocationsErr != nil {
		return nil, d.RevocationsErr
	}
	ch := make(chan string, 64)
	d.mu.Lock()
	d.revSubs = append(d.revSubs, ch)
	d.mu.Unlock()
	go func() {
		<-ctx.Done()
		d.mu.Lock()
		for i, c := range d.revSubs {
			if c == ch {
				d.revSubs = append(d.revSubs[:i], d.revSubs[i+1:]...)
				break
			}
		}
		d.mu.Unlock()
	}()
	return ch, nil
}

// Touch implements remote.Devices.
func (d *FakeDevices) Touch(_ context.Context, deviceID string, connected bool) {
	d.mu.Lock()
	d.touches = append(d.touches, TouchCall{DeviceID: deviceID, Connected: connected})
	d.mu.Unlock()
}

// Touches returns a copy of every Touch call.
func (d *FakeDevices) Touches() []TouchCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]TouchCall(nil), d.touches...)
}

// HasSession reports whether sessionToken still authenticates.
func (d *FakeDevices) HasSession(sessionToken string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.sessions[sessionToken]
	return ok
}

var _ remote.Devices = (*FakeDevices)(nil)
