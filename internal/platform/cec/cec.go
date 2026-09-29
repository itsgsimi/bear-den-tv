// Package cec is TV control over HDMI-CEC through the Linux kernel CEC API
// (ADR 0008, docs/operations.md "TV control over HDMI-CEC"): ioctl on the
// first /dev/cecN, no cgo, no libcec. It implements platform.TVControl.
//
// Nothing is sent on the bus until the first command: Probe only opens the
// adapter and reads its capabilities. The first command claims a playback
// device logical address (or uses the one an already configured adapter
// has); Close gives it up again. Every call runs with the caller's deadline:
// the ioctl runs on its own goroutine, so an adapter that hangs cannot hold
// the caller, and calls are serialized so the bus sees one frame at a time.
// Not seen on hardware: the reference box has no CEC device.
package cec

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"time"
	"unsafe"

	"bear-den-tv/internal/platform"
)

// Backend is the capability backend name.
const Backend = "hdmi-cec"

// ReasonNoDevice is the capability reason when no adapter exists.
const ReasonNoDevice = "No HDMI-CEC device (/dev/cec*) — most PCs need a USB CEC adapter"

// OSDName is the name the TV may show for this device (at most 14 bytes).
const OSDName = "Bear Den TV"

// replyTimeoutMs bounds the kernel's wait for a reply opcode.
const replyTimeoutMs = 1000

// Errors callers can tell apart.
var (
	ErrNoDevice     = errors.New("cec: no HDMI-CEC device")
	ErrTimeout      = errors.New("cec: the adapter did not answer in time")
	ErrNotAcked     = errors.New("cec: the TV did not acknowledge the message")
	ErrNoReply      = errors.New("cec: the TV did not answer")
	ErrNoPhysAddr   = errors.New("cec: the HDMI connection reports no physical address (TV unplugged or its input powered down)")
	ErrNoLogAddr    = errors.New("cec: the adapter has no logical address and cannot be given one")
	errUnknownKey   = errors.New("cec: unknown volume key")
	errCannotTxCaps = "The HDMI-CEC adapter %s cannot send messages."
)

// Options configures an Adapter; zero values are the real system.
type Options struct {
	// Glob finds adapters (default "/dev/cec*"); the first in sorted order
	// is used.
	Glob string
	// open opens one adapter (tests pass a fake ioctl layer).
	open func(path string) (device, error)
}

// Adapter is the HDMI-CEC backend.
type Adapter struct {
	glob string
	open func(string) (device, error)

	// sem serializes every ioctl; it is held by the goroutine doing the
	// ioctl, so a call abandoned at its deadline keeps the next one waiting
	// (up to that one's own deadline) instead of racing it on the bus.
	sem chan struct{}

	// Guarded by sem.
	dev      device
	path     string
	caps     cecCaps
	self     byte // our logical address; logAddrInvalid until claimed
	claimed  bool // we configured the logical address (Close releases it)
	audioSys bool // an audio system acknowledged a poll
}

var _ platform.TVControl = (*Adapter)(nil)

// New returns an Adapter; nothing is opened until Probe or a command.
func New(opts Options) *Adapter {
	a := &Adapter{glob: opts.Glob, open: opts.open, sem: make(chan struct{}, 1), self: logAddrInvalid}
	if a.glob == "" {
		a.glob = "/dev/cec*"
	}
	if a.open == nil {
		a.open = openKernel
	}
	return a
}

// run holds the bus for fn, with fn on its own goroutine so the caller
// returns at ctx's deadline even when the kernel does not.
func (a *Adapter) run(ctx context.Context, fn func() error) error {
	select {
	case a.sem <- struct{}{}:
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrTimeout, ctx.Err())
	}
	done := make(chan error, 1)
	go func() {
		defer func() { <-a.sem }()
		done <- fn()
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrTimeout, ctx.Err())
	}
}

// Probe implements platform.TVControl.
func (a *Adapter) Probe(ctx context.Context) platform.Capability {
	var cp platform.Capability
	err := a.run(ctx, func() error {
		cp = a.probeLocked()
		return nil
	})
	if err != nil {
		return platform.Capability{Backend: Backend, Reason: "The HDMI-CEC adapter did not answer in time."}
	}
	return cp
}

func (a *Adapter) probeLocked() platform.Capability {
	if a.dev != nil {
		var caps cecCaps
		if err := a.dev.ioctl(iocAdapGCaps, unsafe.Pointer(&caps)); err == nil {
			a.caps = caps
			return a.capabilityLocked()
		}
		a.dropLocked() // unplugged: look again
	}
	paths, _ := filepath.Glob(a.glob)
	sort.Strings(paths)
	if len(paths) == 0 {
		return platform.Capability{Backend: Backend, Reason: ReasonNoDevice}
	}
	path := paths[0]
	dev, err := a.open(path)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return platform.Capability{Backend: Backend, Reason: fmt.Sprintf("Cannot open %s: permission denied. The session user usually needs to be in the video group.", path)}
		}
		return platform.Capability{Backend: Backend, Reason: fmt.Sprintf("Cannot open %s: %v", path, err)}
	}
	var caps cecCaps
	if err := dev.ioctl(iocAdapGCaps, unsafe.Pointer(&caps)); err != nil {
		_ = dev.Close()
		return platform.Capability{Backend: Backend, Reason: fmt.Sprintf("%s is not an HDMI-CEC adapter the kernel understands: %v", path, err)}
	}
	a.dev, a.path, a.caps = dev, path, caps
	return a.capabilityLocked()
}

func (a *Adapter) capabilityLocked() platform.Capability {
	if a.caps.Capabilities&capTransmit == 0 {
		return platform.Capability{Backend: Backend, Reason: fmt.Sprintf(errCannotTxCaps, a.path)}
	}
	return platform.Capability{Available: true, Backend: Backend}
}

// dropLocked forgets a device that stopped answering (never releases the
// logical address: the device is gone).
func (a *Adapter) dropLocked() {
	if a.dev != nil {
		_ = a.dev.Close()
	}
	a.dev, a.path, a.self, a.claimed, a.audioSys = nil, "", logAddrInvalid, false, false
}

// readyLocked opens the adapter if needed and makes sure we have a logical
// address, claiming a playback device address the first time.
func (a *Adapter) readyLocked() error {
	if a.dev == nil {
		if cp := a.probeLocked(); !cp.Available {
			if a.dev == nil {
				return fmt.Errorf("%w: %s", ErrNoDevice, cp.Reason)
			}
			return errors.New("cec: " + cp.Reason)
		}
	}
	if cp := a.capabilityLocked(); !cp.Available {
		return errors.New("cec: " + cp.Reason)
	}
	if a.self != logAddrInvalid {
		return nil
	}
	var la cecLogAddrs
	if err := a.dev.ioctl(iocAdapGLogAddrs, unsafe.Pointer(&la)); err != nil {
		a.dropLocked()
		return fmt.Errorf("cec: reading logical addresses: %w", err)
	}
	if la.NumLogAddrs > 0 && la.LogAddr[0] != logAddrInvalid {
		a.self = la.LogAddr[0] // configured already (another program, or the driver)
	} else {
		if a.caps.Capabilities&capLogAddrs == 0 {
			return ErrNoLogAddr
		}
		want := playbackLogAddrs()
		if err := a.dev.ioctl(iocAdapSLogAddrs, unsafe.Pointer(&want)); err != nil {
			return fmt.Errorf("cec: claiming a playback address: %w", err)
		}
		if want.NumLogAddrs == 0 || want.LogAddr[0] == logAddrInvalid {
			return ErrNoLogAddr
		}
		a.self, a.claimed = want.LogAddr[0], true
	}
	// Volume keys go to an audio system (a soundbar or receiver) when one
	// acknowledges a poll, else to the TV.
	m := newMsg(poll(a.self, addrAudioSystem))
	a.audioSys = a.dev.ioctl(iocTransmit, unsafe.Pointer(m)) == nil && m.TxStatus&txStatusOK != 0
	return nil
}

// playbackLogAddrs asks for one playback device address, CEC 1.4.
func playbackLogAddrs() cecLogAddrs {
	var la cecLogAddrs
	la.CECVersion = cecVersion14
	la.NumLogAddrs = 1
	la.VendorID = vendorIDNone
	la.Flags = logAddrsFlUnregFback
	copy(la.OSDName[:], OSDName)
	la.PrimaryDeviceType[0] = primDevTypePlayback
	la.LogAddrType[0] = logAddrTypePlayback
	la.AllDeviceTypes[0] = allDevTypePlayback
	return la
}

func newMsg(b []byte) *cecMsg {
	m := &cecMsg{Len: uint32(len(b))}
	copy(m.Msg[:], b)
	return m
}

// transmitLocked sends one frame and checks that it was acknowledged
// (broadcasts are acknowledged by the bus); with reply set it waits for that
// opcode and returns the reply.
func (a *Adapter) transmitLocked(b []byte, reply byte) ([]byte, error) {
	m := newMsg(b)
	if reply != 0 {
		m.Reply, m.Timeout = reply, replyTimeoutMs
	}
	if err := a.dev.ioctl(iocTransmit, unsafe.Pointer(m)); err != nil {
		if errors.Is(err, errGone) {
			a.dropLocked() // unplugged; the next Probe looks again
		}
		return nil, fmt.Errorf("cec: transmit: %w", err)
	}
	if m.TxStatus&txStatusOK == 0 {
		return nil, ErrNotAcked
	}
	if reply == 0 {
		return nil, nil
	}
	if m.RxStatus&rxStatusOK == 0 || m.RxStatus&(rxStatusTimeout|rxStatusFeatureAbort) != 0 {
		return nil, ErrNoReply
	}
	n := min(int(m.Len), len(m.Msg))
	return append([]byte(nil), m.Msg[:n]...), nil
}

// send readies the adapter and sends each frame built from our address.
func (a *Adapter) send(ctx context.Context, frames ...func(self byte) []byte) error {
	return a.run(ctx, func() error {
		if err := a.readyLocked(); err != nil {
			return err
		}
		for _, f := range frames {
			if _, err := a.transmitLocked(f(a.self), 0); err != nil {
				return err
			}
		}
		return nil
	})
}

// PowerOn implements platform.TVControl.
func (a *Adapter) PowerOn(ctx context.Context) error {
	return a.send(ctx, imageViewOn)
}

// Standby implements platform.TVControl.
func (a *Adapter) Standby(ctx context.Context) error {
	return a.send(ctx, standby)
}

// ActiveSource implements platform.TVControl.
func (a *Adapter) ActiveSource(ctx context.Context) error {
	return a.run(ctx, func() error {
		if err := a.readyLocked(); err != nil {
			return err
		}
		var pa uint16
		if err := a.dev.ioctl(iocAdapGPhysAddr, unsafe.Pointer(&pa)); err != nil {
			return fmt.Errorf("cec: reading the physical address: %w", err)
		}
		if pa == physAddrNone {
			return ErrNoPhysAddr
		}
		_, err := a.transmitLocked(activeSource(a.self, pa), 0)
		return err
	})
}

// VolumeKey implements platform.TVControl.
func (a *Adapter) VolumeKey(ctx context.Context, k platform.TVKey) error {
	code, ok := uiCode(k)
	if !ok {
		return errUnknownKey
	}
	return a.run(ctx, func() error {
		if err := a.readyLocked(); err != nil {
			return err
		}
		to := byte(addrTV)
		if a.audioSys {
			to = addrAudioSystem
		}
		if _, err := a.transmitLocked(userControlPressed(a.self, to, code), 0); err != nil {
			return err
		}
		_, err := a.transmitLocked(userControlReleased(a.self, to), 0)
		return err
	})
}

// PowerStatus implements platform.TVControl.
func (a *Adapter) PowerStatus(ctx context.Context) (platform.TVPower, error) {
	st := platform.TVPowerUnknown
	err := a.run(ctx, func() error {
		if err := a.readyLocked(); err != nil {
			return err
		}
		reply, err := a.transmitLocked(giveDevicePowerStatus(a.self), opReportPowerStatus)
		if err != nil {
			return err
		}
		st = parsePowerStatus(reply)
		return nil
	})
	if err != nil {
		return platform.TVPowerUnknown, err
	}
	return st, nil
}

// closeTimeout bounds Close.
var closeTimeout = 2 * time.Second

// Close implements platform.TVControl: it releases a logical address this
// process claimed (an empty CEC_ADAP_S_LOG_ADDRS) and closes the adapter.
func (a *Adapter) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	return a.run(ctx, func() error {
		if a.dev == nil {
			return nil
		}
		var err error
		if a.claimed {
			var none cecLogAddrs
			err = a.dev.ioctl(iocAdapSLogAddrs, unsafe.Pointer(&none))
		}
		a.dropLocked()
		return err
	})
}
