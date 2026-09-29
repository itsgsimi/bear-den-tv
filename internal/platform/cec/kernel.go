// The Linux kernel CEC API (include/uapi/linux/cec.h,
// Documentation/userspace-api/media/cec): ioctl request numbers, the three
// structs this package passes to the kernel laid out byte for byte as the C
// compiler lays them out, and the real /dev/cecN device. kernel_test.go
// checks sizes, offsets and request numbers against the values linux/cec.h
// gives on x86-64 and arm64 (ADR 0008).

package cec

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ioctl request numbers: _IOC(dir, 'a', nr, size).
const (
	iocAdapGCaps     = 0xc04c6100 // CEC_ADAP_G_CAPS      _IOWR('a', 0, struct cec_caps)
	iocAdapGPhysAddr = 0x80026101 // CEC_ADAP_G_PHYS_ADDR _IOR('a', 1, __u16)
	iocAdapGLogAddrs = 0x805c6103 // CEC_ADAP_G_LOG_ADDRS _IOR('a', 3, struct cec_log_addrs)
	iocAdapSLogAddrs = 0xc05c6104 // CEC_ADAP_S_LOG_ADDRS _IOWR('a', 4, struct cec_log_addrs)
	iocTransmit      = 0xc0386105 // CEC_TRANSMIT         _IOWR('a', 5, struct cec_msg)
)

// Constants from linux/cec.h.
const (
	capPhysAddr = 1 << 0 // CEC_CAP_PHYS_ADDR: userspace sets the physical address
	capLogAddrs = 1 << 1 // CEC_CAP_LOG_ADDRS: userspace configures logical addresses
	capTransmit = 1 << 2 // CEC_CAP_TRANSMIT

	txStatusOK = 1 << 0 // CEC_TX_STATUS_OK

	rxStatusOK           = 1 << 0 // CEC_RX_STATUS_OK
	rxStatusTimeout      = 1 << 1 // CEC_RX_STATUS_TIMEOUT
	rxStatusFeatureAbort = 1 << 2 // CEC_RX_STATUS_FEATURE_ABORT

	logAddrTypePlayback  = 3    // CEC_LOG_ADDR_TYPE_PLAYBACK
	primDevTypePlayback  = 4    // CEC_OP_PRIM_DEVTYPE_PLAYBACK
	allDevTypePlayback   = 0x10 // CEC_OP_ALL_DEVTYPE_PLAYBACK
	cecVersion14         = 5    // CEC_OP_CEC_VERSION_1_4
	vendorIDNone         = 0xffffffff
	logAddrsFlUnregFback = 1 << 0 // CEC_LOG_ADDRS_FL_ALLOW_UNREG_FALLBACK

	logAddrInvalid = 0xff   // CEC_LOG_ADDR_INVALID
	physAddrNone   = 0xffff // CEC_PHYS_ADDR_INVALID
)

// errGone is what an ioctl on an unplugged adapter returns.
var errGone = unix.ENODEV

// cecMsg is struct cec_msg (56 bytes).
type cecMsg struct {
	TxTs          uint64
	RxTs          uint64
	Len           uint32
	Timeout       uint32 // ms to wait for the reply opcode
	Sequence      uint32
	Flags         uint32
	Msg           [16]byte
	Reply         byte // opcode to wait for; 0 = none
	RxStatus      byte
	TxStatus      byte
	TxArbLostCnt  byte
	TxNackCnt     byte
	TxLowDriveCnt byte
	TxErrorCnt    byte
	_             byte // the compiler's tail padding
}

// cecLogAddrs is struct cec_log_addrs (92 bytes).
type cecLogAddrs struct {
	LogAddr           [4]byte
	LogAddrMask       uint16
	CECVersion        byte
	NumLogAddrs       byte
	VendorID          uint32
	Flags             uint32
	OSDName           [15]byte
	PrimaryDeviceType [4]byte
	LogAddrType       [4]byte
	AllDeviceTypes    [4]byte
	Features          [4][12]byte
	_                 byte // tail padding
}

// cecCaps is struct cec_caps (76 bytes).
type cecCaps struct {
	Driver            [32]byte
	Name              [32]byte
	AvailableLogAddrs uint32
	Capabilities      uint32
	Version           uint32
}

// device is one open adapter: an ioctl on it, and closing it.
type device interface {
	ioctl(req uintptr, arg unsafe.Pointer) error
	Close() error
}

// kernelDevice is a real /dev/cecN opened read-write in blocking mode, so
// CEC_TRANSMIT returns once the frame was sent (and its reply arrived) and
// CEC_ADAP_S_LOG_ADDRS once the address is claimed.
type kernelDevice struct{ f *os.File }

func openKernel(path string) (device, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &kernelDevice{f: f}, nil
}

func (d *kernelDevice) ioctl(req uintptr, arg unsafe.Pointer) error {
	conn, err := d.f.SyscallConn()
	if err != nil {
		return err
	}
	var errno unix.Errno
	if cerr := conn.Control(func(fd uintptr) {
		_, _, errno = unix.Syscall(unix.SYS_IOCTL, fd, req, uintptr(arg))
	}); cerr != nil {
		return cerr
	}
	if errno != 0 {
		return errno
	}
	return nil
}

func (d *kernelDevice) Close() error { return d.f.Close() }
