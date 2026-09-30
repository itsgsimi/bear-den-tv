// The one list of interfaces that are never the home LAN
// (VirtualInterfacePrefixes): the TV shell's copy, which keeps them off the
// Phone remote setup screens (refusedInterface in
// apps/tv-shell/src/ShellController.cpp), must say the same (UX-03: a VPN
// was offered as "Allow on Wired"), and VPNs are among them.

package config

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestShellHidesTheSameInterfaces(t *testing.T) {
	src, err := os.ReadFile("../../apps/tv-shell/src/ShellController.cpp")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	start := strings.Index(body, "bool refusedInterface(const QString &name)")
	if start < 0 {
		t.Fatal("refusedInterface not found in ShellController.cpp")
	}
	end := strings.Index(body[start:], "};")
	var shell []string
	for _, m := range regexp.MustCompile(`QStringLiteral\("([^"]+)"\)`).FindAllStringSubmatch(body[start:start+end], -1) {
		shell = append(shell, m[1])
	}
	if strings.Join(shell, ",") != strings.Join(VirtualInterfacePrefixes, ",") {
		t.Fatalf("shell hides %v, config refuses %v", shell, VirtualInterfacePrefixes)
	}
	for _, vpn := range []string{"tailscale0", "wg0", "tun0", "zt5u4y", "docker0"} {
		if !IsVirtualInterface(vpn) {
			t.Errorf("%s is offered", vpn)
		}
	}
	for _, home := range []string{"enp3s0", "eth0", "wlp2s0"} {
		if IsVirtualInterface(home) {
			t.Errorf("%s is refused", home)
		}
	}
}
