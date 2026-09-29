// Tests for cec.go against a fake ioctl layer (fakeBus): the fake decodes
// the same structs the kernel would, so what reaches "the wire" is the
// adapter's real encoding. Covers absence and permissions, no bus traffic
// on Probe, claiming or reusing a logical address, each command's frames,
// acknowledgement and reply handling, deadlines on a stuck adapter, unplug
// and Close.

package cec

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"bear-den-tv/internal/platform"

	"golang.org/x/sys/unix"
)

// fakeBus is one adapter and the devices behind it.
type fakeBus struct {
	mu        sync.Mutex
	caps      uint32
	logAddrs  cecLogAddrs // what G_LOG_ADDRS reports
	physAddr  uint16
	acks      map[byte]bool // logical addresses that acknowledge
	tvPower   byte          // Report Power Status operand
	tvAnswers bool
	sent      [][]byte
	setLog    []cecLogAddrs
	block     chan struct{} // non-nil: CEC_TRANSMIT waits for it
	gone      bool          // unplugged: every ioctl fails with ENODEV
	closed    int
}

func newBus() *fakeBus {
	b := &fakeBus{caps: capPhysAddr | capLogAddrs | capTransmit, physAddr: 0x1000, acks: map[byte]bool{addrTV: true}, tvAnswers: true}
	b.logAddrs.LogAddr = [4]byte{logAddrInvalid, logAddrInvalid, logAddrInvalid, logAddrInvalid}
	return b
}

func (b *fakeBus) ioctl(req uintptr, arg unsafe.Pointer) error {
	b.mu.Lock()
	block, gone := b.block, b.gone
	b.mu.Unlock()
	if gone {
		return unix.ENODEV
	}
	if req == iocTransmit && block != nil {
		<-block
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	switch req {
	case iocAdapGCaps:
		(*cecCaps)(arg).Capabilities = b.caps
	case iocAdapGPhysAddr:
		*(*uint16)(arg) = b.physAddr
	case iocAdapGLogAddrs:
		*(*cecLogAddrs)(arg) = b.logAddrs
	case iocAdapSLogAddrs:
		la := (*cecLogAddrs)(arg)
		b.setLog = append(b.setLog, *la)
		if la.NumLogAddrs > 0 {
			la.LogAddr[0], la.LogAddrMask = 4, 1<<4 // Playback Device 1
		}
		b.logAddrs = *la
	case iocTransmit:
		m := (*cecMsg)(arg)
		frame := append([]byte(nil), m.Msg[:m.Len]...)
		b.sent = append(b.sent, frame)
		dest := frame[0] & 0x0f
		if dest == addrBroadcast || b.acks[dest] {
			m.TxStatus = txStatusOK
		} else {
			m.TxStatus = 1 << 2 // CEC_TX_STATUS_NACK
		}
		if m.Reply != 0 {
			if b.tvAnswers && m.TxStatus == txStatusOK {
				m.RxStatus, m.Len = rxStatusOK, 3
				m.Msg[0], m.Msg[1], m.Msg[2] = dest<<4|frame[0]>>4, m.Reply, b.tvPower
			} else {
				m.RxStatus = rxStatusTimeout
			}
		}
	default:
		return unix.ENOTTY
	}
	return nil
}

func (b *fakeBus) Close() error {
	b.mu.Lock()
	b.closed++
	b.mu.Unlock()
	return nil
}

func (b *fakeBus) frames() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([][]byte(nil), b.sent...)
}

func (b *fakeBus) reset() {
	b.mu.Lock()
	b.sent = nil
	b.mu.Unlock()
}

// adapterOn makes a directory with one fake /dev/cec0 backed by bus.
func adapterOn(t *testing.T, bus *fakeBus) (*Adapter, *int) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cec0"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	opens := 0
	a := New(Options{Glob: filepath.Join(dir, "cec*"), open: func(string) (device, error) { opens++; return bus, nil }})
	return a, &opens
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return c
}

func wantFrames(t *testing.T, got [][]byte, want ...[]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("frames % x, want % x", got, want)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("frame %d: % x, want % x (all: % x)", i, got[i], want[i], got)
		}
	}
}

func TestNoDeviceIsUnavailableWithTheReason(t *testing.T) {
	opened := false
	a := New(Options{Glob: filepath.Join(t.TempDir(), "cec*"), open: func(string) (device, error) { opened = true; return nil, errors.New("no") }})
	cp := a.Probe(ctx(t))
	if cp.Available || cp.Reason != ReasonNoDevice || cp.Backend != Backend {
		t.Fatalf("no device: %+v", cp)
	}
	if err := a.PowerOn(ctx(t)); !errors.Is(err, ErrNoDevice) {
		t.Fatalf("PowerOn without a device: %v", err)
	}
	if st, err := a.PowerStatus(ctx(t)); st != platform.TVPowerUnknown || err == nil {
		t.Fatalf("PowerStatus without a device: %s %v", st, err)
	}
	if opened {
		t.Fatal("something was opened without a device")
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close without a device: %v", err)
	}
}

func TestPermissionDeniedPointsAtTheVideoGroup(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "cec0"), nil, 0o600)
	a := New(Options{Glob: filepath.Join(dir, "cec*"), open: func(p string) (device, error) {
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrPermission}
	}})
	cp := a.Probe(ctx(t))
	if cp.Available || !strings.Contains(cp.Reason, "permission denied") || !strings.Contains(cp.Reason, "video group") {
		t.Fatalf("permission: %+v", cp)
	}
}

func TestAdapterThatCannotTransmitIsUnavailable(t *testing.T) {
	bus := newBus()
	bus.caps = capLogAddrs
	a, _ := adapterOn(t, bus)
	if cp := a.Probe(ctx(t)); cp.Available || !strings.Contains(cp.Reason, "cannot send") {
		t.Fatalf("receive-only adapter: %+v", cp)
	}
	if err := a.PowerOn(ctx(t)); err == nil || len(bus.frames()) != 0 {
		t.Fatalf("a receive-only adapter was used: %v % x", err, bus.frames())
	}
}

// Probe opens the adapter and reads its capabilities, and nothing else:
// no logical address is claimed and no frame is sent.
func TestProbeSendsNothingOnTheBus(t *testing.T) {
	bus := newBus()
	a, opens := adapterOn(t, bus)
	for range 3 {
		if cp := a.Probe(ctx(t)); !cp.Available || cp.Backend != Backend {
			t.Fatalf("probe: %+v", cp)
		}
	}
	if len(bus.frames()) != 0 || len(bus.setLog) != 0 || *opens != 1 {
		t.Fatalf("probe touched the bus: frames % x, claims %d, opens %d", bus.frames(), len(bus.setLog), *opens)
	}
}

// The first command claims one playback device address named Bear Den TV,
// polls for an audio system once, then sends; later commands reuse it.
func TestFirstCommandClaimsAPlaybackAddress(t *testing.T) {
	bus := newBus()
	a, _ := adapterOn(t, bus)
	if err := a.PowerOn(ctx(t)); err != nil {
		t.Fatal(err)
	}
	if len(bus.setLog) != 1 {
		t.Fatalf("claims: %d", len(bus.setLog))
	}
	la := bus.setLog[0]
	if la.NumLogAddrs != 1 || la.LogAddrType[0] != logAddrTypePlayback || la.PrimaryDeviceType[0] != primDevTypePlayback ||
		la.AllDeviceTypes[0] != allDevTypePlayback || la.CECVersion != cecVersion14 || la.VendorID != vendorIDNone ||
		string(bytes.TrimRight(la.OSDName[:], "\x00")) != OSDName {
		t.Fatalf("claim: %+v", la)
	}
	wantFrames(t, bus.frames(), []byte{0x45}, []byte{0x40, 0x04})
	bus.reset()
	if err := a.Standby(ctx(t)); err != nil {
		t.Fatal(err)
	}
	wantFrames(t, bus.frames(), []byte{0x40, 0x36})
	if len(bus.setLog) != 1 {
		t.Fatal("claimed twice")
	}
}

// An adapter that already has an address (another program, or a driver
// that configures itself) is used as it is and never reconfigured.
func TestUsesAnAlreadyConfiguredAddress(t *testing.T) {
	bus := newBus()
	bus.logAddrs.NumLogAddrs, bus.logAddrs.LogAddr[0] = 1, 8 // Playback Device 2
	a, _ := adapterOn(t, bus)
	if err := a.PowerOn(ctx(t)); err != nil {
		t.Fatal(err)
	}
	wantFrames(t, bus.frames(), []byte{0x85}, []byte{0x80, 0x04})
	if len(bus.setLog) != 0 {
		t.Fatal("reconfigured an adapter that had an address")
	}
	if err := a.Close(); err != nil || len(bus.setLog) != 0 {
		t.Fatalf("Close released an address it did not claim: %v %d", err, len(bus.setLog))
	}
}

func TestAdapterWithoutAnAddressThatCannotBeConfigured(t *testing.T) {
	bus := newBus()
	bus.caps = capTransmit
	a, _ := adapterOn(t, bus)
	if err := a.PowerOn(ctx(t)); !errors.Is(err, ErrNoLogAddr) {
		t.Fatalf("PowerOn: %v", err)
	}
}

func TestEachCommandOnTheWire(t *testing.T) {
	bus := newBus()
	a, _ := adapterOn(t, bus)
	if err := a.ActiveSource(ctx(t)); err != nil {
		t.Fatal(err)
	}
	wantFrames(t, bus.frames(), []byte{0x45}, []byte{0x4f, 0x82, 0x10, 0x00})

	keys := []struct {
		k    platform.TVKey
		code byte
	}{{platform.TVVolumeUp, 0x41}, {platform.TVVolumeDown, 0x42}, {platform.TVMute, 0x65}, {platform.TVUnmute, 0x66}}
	for _, k := range keys {
		bus.reset()
		if err := a.VolumeKey(ctx(t), k.k); err != nil {
			t.Fatal(err)
		}
		// No audio system acknowledged the poll: the TV gets the keys.
		wantFrames(t, bus.frames(), []byte{0x40, 0x44, k.code}, []byte{0x40, 0x45})
	}
	if err := a.VolumeKey(ctx(t), "power"); err == nil {
		t.Fatal("an unknown key was sent")
	}

	for status, want := range map[byte]platform.TVPower{0: platform.TVPowerOn, 1: platform.TVPowerStandby, 2: platform.TVPowerOn, 3: platform.TVPowerStandby} {
		bus.reset()
		bus.tvPower = status
		got, err := a.PowerStatus(ctx(t))
		if err != nil || got != want {
			t.Fatalf("status %d: %s %v, want %s", status, got, err, want)
		}
		wantFrames(t, bus.frames(), []byte{0x40, 0x8f})
	}
}

func TestVolumeGoesToAnAudioSystemThatAnswers(t *testing.T) {
	bus := newBus()
	bus.acks[addrAudioSystem] = true
	a, _ := adapterOn(t, bus)
	if err := a.VolumeKey(ctx(t), platform.TVVolumeUp); err != nil {
		t.Fatal(err)
	}
	wantFrames(t, bus.frames(), []byte{0x45}, []byte{0x45, 0x44, 0x41}, []byte{0x45, 0x45})
}

func TestUnacknowledgedAndUnanswered(t *testing.T) {
	bus := newBus()
	bus.acks = map[byte]bool{} // no TV on the bus
	a, _ := adapterOn(t, bus)
	if err := a.PowerOn(ctx(t)); !errors.Is(err, ErrNotAcked) {
		t.Fatalf("PowerOn to nobody: %v", err)
	}
	bus.acks[addrTV] = true
	bus.tvAnswers = false
	if st, err := a.PowerStatus(ctx(t)); !errors.Is(err, ErrNoReply) || st != platform.TVPowerUnknown {
		t.Fatalf("a TV that does not answer: %s %v", st, err)
	}
}

func TestNoPhysicalAddressIsNotAnActiveSource(t *testing.T) {
	bus := newBus()
	bus.physAddr = physAddrNone
	a, _ := adapterOn(t, bus)
	if err := a.ActiveSource(ctx(t)); !errors.Is(err, ErrNoPhysAddr) {
		t.Fatalf("ActiveSource: %v", err)
	}
	for _, f := range bus.frames() {
		if len(f) > 1 && f[1] == opActiveSource {
			t.Fatalf("Active Source sent without a physical address: % x", f)
		}
	}
}

// A stuck adapter never holds a caller past its deadline, the next call
// waits for the stuck one only up to its own deadline, and the adapter
// works again once the kernel returns.
func TestDeadlinesOnAStuckAdapter(t *testing.T) {
	bus := newBus()
	a, _ := adapterOn(t, bus)
	if cp := a.Probe(ctx(t)); !cp.Available {
		t.Fatal(cp.Reason)
	}
	block := make(chan struct{})
	bus.mu.Lock()
	bus.block = block
	bus.mu.Unlock()
	for _, call := range []func(context.Context) error{a.PowerOn, a.Standby} {
		c, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		start := time.Now()
		err := call(c)
		cancel()
		if !errors.Is(err, ErrTimeout) {
			t.Fatalf("stuck adapter: %v", err)
		}
		if d := time.Since(start); d > time.Second {
			t.Fatalf("the caller was held %v", d)
		}
	}
	c, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	if cp := a.Probe(c); cp.Available || !strings.Contains(cp.Reason, "did not answer") {
		t.Fatalf("probe while stuck: %+v", cp)
	}
	cancel()
	bus.mu.Lock()
	bus.block = nil
	bus.mu.Unlock()
	close(block)
	if err := a.PowerOn(ctx(t)); err != nil {
		t.Fatalf("after the kernel returned: %v", err)
	}
}

func TestUnpluggedAdapterIsLookedForAgain(t *testing.T) {
	bus := newBus()
	a, opens := adapterOn(t, bus)
	if err := a.PowerOn(ctx(t)); err != nil {
		t.Fatal(err)
	}
	bus.mu.Lock()
	bus.gone = true
	bus.logAddrs = newBus().logAddrs // a replugged adapter starts unconfigured
	bus.mu.Unlock()
	if err := a.Standby(ctx(t)); err == nil {
		t.Fatal("a gone adapter accepted a frame")
	}
	bus.mu.Lock()
	bus.gone = false
	bus.mu.Unlock()
	if cp := a.Probe(ctx(t)); !cp.Available || *opens != 2 {
		t.Fatalf("replugged: %+v, opens %d", cp, *opens)
	}
	if err := a.Standby(ctx(t)); err != nil || len(bus.setLog) != 2 {
		t.Fatalf("after replug: %v, claims %d", err, len(bus.setLog))
	}
}

// Close gives up the address this process claimed and closes the adapter;
// a later Probe opens it again.
func TestCloseReleasesTheClaimedAddress(t *testing.T) {
	bus := newBus()
	a, opens := adapterOn(t, bus)
	if err := a.PowerOn(ctx(t)); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if len(bus.setLog) != 2 || bus.setLog[1].NumLogAddrs != 0 || bus.closed != 1 {
		t.Fatalf("release: claims %+v, closed %d", bus.setLog, bus.closed)
	}
	if cp := a.Probe(ctx(t)); !cp.Available || *opens != 2 {
		t.Fatalf("reopen: %+v, opens %d", cp, *opens)
	}
}
