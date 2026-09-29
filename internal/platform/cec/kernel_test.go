// Tests for kernel.go: the Go structs match the kernel's layout and the
// ioctl request numbers match linux/cec.h. The expected numbers were printed
// by a C program including <linux/cec.h> (kernel 7.0 headers, x86-64): sizeof
// and offsetof for struct cec_msg, cec_log_addrs and cec_caps, and the
// CEC_* request macros. The layout has no pointers or longs, so it is the
// same on arm64.

package cec

import (
	"testing"
	"unsafe"
)

// ioc is the kernel's _IOC(dir, type, nr, size).
func ioc(dir, typ, nr, size uintptr) uintptr { return dir<<30 | size<<16 | typ<<8 | nr }

func TestStructLayoutMatchesLinuxCECH(t *testing.T) {
	var m cecMsg
	var la cecLogAddrs
	var c cecCaps
	checks := []struct {
		name      string
		got, want uintptr
	}{
		{"sizeof(struct cec_msg)", unsafe.Sizeof(m), 56},
		{"cec_msg.len", unsafe.Offsetof(m.Len), 16},
		{"cec_msg.timeout", unsafe.Offsetof(m.Timeout), 20},
		{"cec_msg.sequence", unsafe.Offsetof(m.Sequence), 24},
		{"cec_msg.flags", unsafe.Offsetof(m.Flags), 28},
		{"cec_msg.msg", unsafe.Offsetof(m.Msg), 32},
		{"cec_msg.reply", unsafe.Offsetof(m.Reply), 48},
		{"cec_msg.rx_status", unsafe.Offsetof(m.RxStatus), 49},
		{"cec_msg.tx_status", unsafe.Offsetof(m.TxStatus), 50},
		{"sizeof(struct cec_log_addrs)", unsafe.Sizeof(la), 92},
		{"cec_log_addrs.log_addr_mask", unsafe.Offsetof(la.LogAddrMask), 4},
		{"cec_log_addrs.cec_version", unsafe.Offsetof(la.CECVersion), 6},
		{"cec_log_addrs.num_log_addrs", unsafe.Offsetof(la.NumLogAddrs), 7},
		{"cec_log_addrs.vendor_id", unsafe.Offsetof(la.VendorID), 8},
		{"cec_log_addrs.flags", unsafe.Offsetof(la.Flags), 12},
		{"cec_log_addrs.osd_name", unsafe.Offsetof(la.OSDName), 16},
		{"cec_log_addrs.primary_device_type", unsafe.Offsetof(la.PrimaryDeviceType), 31},
		{"cec_log_addrs.log_addr_type", unsafe.Offsetof(la.LogAddrType), 35},
		{"cec_log_addrs.all_device_types", unsafe.Offsetof(la.AllDeviceTypes), 39},
		{"cec_log_addrs.features", unsafe.Offsetof(la.Features), 43},
		{"sizeof(struct cec_caps)", unsafe.Sizeof(c), 76},
		{"cec_caps.available_log_addrs", unsafe.Offsetof(c.AvailableLogAddrs), 64},
		{"cec_caps.capabilities", unsafe.Offsetof(c.Capabilities), 68},
		{"cec_caps.version", unsafe.Offsetof(c.Version), 72},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, linux/cec.h says %d", c.name, c.got, c.want)
		}
	}
}

func TestIoctlNumbersMatchLinuxCECH(t *testing.T) {
	const r, rw = 2, 3 // _IOC_READ, _IOC_READ|_IOC_WRITE
	checks := []struct {
		name      string
		got, want uintptr
		derived   uintptr
	}{
		{"CEC_ADAP_G_CAPS", iocAdapGCaps, 0xc04c6100, ioc(rw, 'a', 0, unsafe.Sizeof(cecCaps{}))},
		{"CEC_ADAP_G_PHYS_ADDR", iocAdapGPhysAddr, 0x80026101, ioc(r, 'a', 1, 2)},
		{"CEC_ADAP_G_LOG_ADDRS", iocAdapGLogAddrs, 0x805c6103, ioc(r, 'a', 3, unsafe.Sizeof(cecLogAddrs{}))},
		{"CEC_ADAP_S_LOG_ADDRS", iocAdapSLogAddrs, 0xc05c6104, ioc(rw, 'a', 4, unsafe.Sizeof(cecLogAddrs{}))},
		{"CEC_TRANSMIT", iocTransmit, 0xc0386105, ioc(rw, 'a', 5, unsafe.Sizeof(cecMsg{}))},
	}
	for _, c := range checks {
		if c.got != c.want || c.derived != c.want {
			t.Errorf("%s = %#x (from _IOC %#x), linux/cec.h says %#x", c.name, c.got, c.derived, c.want)
		}
	}
}
