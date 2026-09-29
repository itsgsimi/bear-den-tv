// Integration test: a fake MPRIS player exported on a private D-Bus session
// bus (a dbus-daemon started for this test only), read through the real
// dbusx connection and this package (mpris.go). Skipped, with the reason,
// when no dbus-daemon is installed.

package mpris

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"

	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/dbusx"
)

const busConfig = `<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>session</type>
  <listen>unix:dir=%s</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>
`

// privateBus starts a dbus-daemon that only this test uses and returns its
// address; the daemon is killed when the test ends.
func privateBus(t *testing.T) string {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon is not installed here; the real-bus MPRIS test needs it (the in-memory bus tests still ran)")
	}
	dir := t.TempDir()
	cfg := filepath.Join(dir, "bus.conf")
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf(busConfig, dir)), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(daemon, "--config-file="+cfg, "--nofork", "--nopidfile", "--print-address=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Skipf("dbus-daemon could not start here: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	lines := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(out).ReadString('\n')
		lines <- strings.TrimSpace(line)
	}()
	select {
	case addr := <-lines:
		if addr == "" {
			t.Fatal("dbus-daemon printed no address")
		}
		return addr
	case <-time.After(5 * time.Second):
		t.Fatal("dbus-daemon did not print its address")
	}
	return ""
}

// demoPlayer answers the Player methods the coordinator calls.
type demoPlayer struct{ props *prop.Properties }

func (p *demoPlayer) Pause() *dbus.Error {
	p.props.SetMust(playerIface, "PlaybackStatus", "Paused")
	return nil
}

func (p *demoPlayer) Play() *dbus.Error {
	p.props.SetMust(playerIface, "PlaybackStatus", "Playing")
	return nil
}

// exportPlayer puts a minimal MPRIS player on the bus under busName and
// returns its property table (Set emits PropertiesChanged like a real one).
// An empty desktopEntry leaves the property out, as VacuumTube does.
func exportPlayer(t *testing.T, addr, busName, identity, desktopEntry, title string) *prop.Properties {
	t.Helper()
	conn, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	metadata := map[string]dbus.Variant{
		"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath("/demo/track/1")),
		"xesam:title":   dbus.MakeVariant(title),
		"xesam:artist":  dbus.MakeVariant([]string{"DEMO Artist"}),
		"xesam:album":   dbus.MakeVariant("DEMO Album"),
		"mpris:length":  dbus.MakeVariant(int64(300_000_000)),
	}
	root := map[string]*prop.Prop{"Identity": {Value: identity, Emit: prop.EmitTrue}}
	if desktopEntry != "" {
		root["DesktopEntry"] = &prop.Prop{Value: desktopEntry, Emit: prop.EmitTrue}
	}
	props, err := prop.Export(conn, objectPath, prop.Map{
		rootIface: root,
		playerIface: {
			"PlaybackStatus": {Value: "Playing", Emit: prop.EmitTrue},
			"Metadata":       {Value: metadata, Emit: prop.EmitTrue},
			"Position":       {Value: int64(42_000_000), Emit: prop.EmitFalse},
			"Rate":           {Value: 1.0, Emit: prop.EmitTrue},
			"CanControl":     {Value: true, Emit: prop.EmitTrue},
			"CanPause":       {Value: true, Emit: prop.EmitTrue},
			"CanPlay":        {Value: true, Emit: prop.EmitTrue},
			"CanSeek":        {Value: true, Emit: prop.EmitTrue},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Export(&demoPlayer{props: props}, objectPath, playerIface); err != nil {
		t.Fatal(err)
	}
	reply, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("request %s: reply %v err %v", busName, reply, err)
	}
	return props
}

func TestRealBusMetadataAndSignals(t *testing.T) {
	addr := privateBus(t)
	plex := exportPlayer(t, addr, "org.mpris.MediaPlayer2.tv.plex.PlexHTPC", "DEMO player", "tv.plex.PlexHTPC", "DEMO Real Bus Title")
	other := exportPlayer(t, addr, "org.mpris.MediaPlayer2.vlc", "DEMO player", "vlc", "DEMO Other Player")

	t.Setenv("DBUS_SESSION_BUS_ADDRESS", addr)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bus, err := dbusx.ConnectSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()

	mp, found, err := NewLocator(bus).Find(ctx, platform.MediaMatch{FlatpakID: "tv.plex.PlexHTPC", Names: []string{"tv.plex.PlexHTPC"}})
	if err != nil || !found {
		t.Fatalf("find: found=%v err=%v", found, err)
	}
	info, err := mp.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != "Playing" || info.Title != "DEMO Real Bus Title" || len(info.Artists) != 1 || info.Artists[0] != "DEMO Artist" ||
		info.Album != "DEMO Album" || info.Length != 300*time.Second || !info.HasPosition || info.Position != 42*time.Second || info.Rate != 1 {
		t.Fatalf("info over the real bus: %+v (title %q)", info, info.Title)
	}

	ch, err := mp.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The other player's change must not wake this player's watch.
	other.SetMust(playerIface, "PlaybackStatus", "Paused")
	select {
	case <-ch:
		t.Fatal("another player's PropertiesChanged woke the watch")
	case <-time.After(300 * time.Millisecond):
	}
	plex.SetMust(playerIface, "PlaybackStatus", "Paused")
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("this player's PropertiesChanged did not wake the watch")
	}
	if info, err := mp.Info(ctx); err != nil || info.Status != "Paused" {
		t.Fatalf("after the change: %+v %v", info, err)
	}
	if err := mp.Play(ctx); err != nil {
		t.Fatalf("play over the real bus: %v", err)
	}
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("Play's status change did not wake the watch")
	}
	if st, err := mp.Status(ctx); err != nil || st != "Playing" {
		t.Fatalf("after Play: %q %v", st, err)
	}
}

// TestRealBusObservedVacuumTube exports VacuumTube's player as the TV showed
// it (chromium.instance3, Identity VacuumTube, no DesktopEntry) and finds it
// through the real bus daemon's GetConnectionUnixProcessID; only the
// pid-to-Flatpak lookup is faked (the exporter is this test process).
func TestRealBusObservedVacuumTube(t *testing.T) {
	addr := privateBus(t)
	exportPlayer(t, addr, "org.mpris.MediaPlayer2.chromium.instance3", "VacuumTube", "", "DEMO Video")

	t.Setenv("DBUS_SESSION_BUS_ADDRESS", addr)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bus, err := dbusx.ConnectSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	vt := platform.MediaMatch{FlatpakID: "rocks.shy.VacuumTube", Names: []string{"rocks.shy.VacuumTube"}}

	if _, found, err := NewLocator(bus).Find(ctx, vt); found || err != nil {
		t.Fatalf("names alone: found=%v err=%v", found, err)
	}
	me := os.Getpid()
	l := NewLocator(bus).WithProcesses(fakeProcs{flatpak: map[int]string{me: "rocks.shy.VacuumTube"}})
	if pid, err := l.OwnerPID(ctx, "org.mpris.MediaPlayer2.chromium.instance3"); err != nil || pid != me {
		t.Fatalf("the bus daemon named pid %d (%v), want %d", pid, err, me)
	}
	mp, found, err := l.Find(ctx, vt)
	if err != nil || !found {
		t.Fatalf("by owner: found=%v err=%v", found, err)
	}
	if st, err := mp.Status(ctx); st != "Playing" || err != nil {
		t.Fatalf("status %q %v", st, err)
	}
	if err := mp.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	if st, _ := mp.Status(ctx); st != "Paused" {
		t.Fatalf("after Pause: %q", st)
	}
	other := NewLocator(bus).WithProcesses(fakeProcs{flatpak: map[int]string{me: "org.chromium.Chromium"}})
	if _, found, _ := other.Find(ctx, vt); found {
		t.Fatal("a player owned by another Flatpak matched")
	}
}
