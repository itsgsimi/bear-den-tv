// Admin CLI commands that talk to the running coordinator over the shell
// socket: doctor, pair, devices, remote (CLI guide internal/AGENTS.md; spec
// contracts/ipc.md).

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/doctor"
	"bear-den-tv/internal/shellipc"
)

// dialCLI connects to the running coordinator as a trusted local cli peer.
func dialCLI(ctx context.Context, socket string) (*shellipc.Conn, error) {
	if socket == "" {
		socket = doctor.DefaultPaths().Socket
	}
	conn, err := shellipc.Dial(ctx, socket, shellipc.ClientCLI, Version)
	if err != nil {
		return nil, fmt.Errorf("no running coordinator at %s (start `bear-den-tv session`): %w", socket, err)
	}
	return conn, nil
}

// call sends one administrative message and waits for its result.
func call(socket string, m shellipc.Message, requestID string) (shellipc.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := dialCLI(ctx, socket)
	if err != nil {
		return shellipc.Result{}, err
	}
	defer conn.Close()
	if err := conn.Send(m); err != nil {
		return shellipc.Result{}, err
	}
	res, err := conn.Result(requestID, 10*time.Second)
	if err != nil {
		return res, err
	}
	if !res.OK {
		return res, errors.New(res.Error)
	}
	return res, nil
}

func socketFlag(fs *flag.FlagSet) *string {
	return fs.String("socket", "", "coordinator socket (default $XDG_RUNTIME_DIR/bear-den-tv/shell.sock)")
}

func cmdDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	probe := fs.Bool("probe", false, "also probe the desktop session (display, D-Bus, audio, Flatpak)")
	shellBin := fs.String("shell-binary", "", "shell binary to look for")
	sock := socketFlag(fs)
	_ = fs.Parse(args)
	paths := doctor.DefaultPaths()
	if *sock != "" {
		paths.Socket = *sock
	}
	rep := doctor.Report(context.Background(), doctor.Options{Paths: paths, Version: Version, ShellBinary: *shellBin, Probe: *probe})
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func cmdPair(args []string) error {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	sock := socketFlag(fs)
	cancelInv := fs.Bool("cancel", false, "withdraw the displayed invitation")
	guest := fs.String("guest", "", "issue a guest pass instead of a family phone: tonight (until 04:00), 24h or 7d")
	_ = fs.Parse(args)
	if *guest != "" && !validPass(*guest) {
		return fmt.Errorf("--guest must be one of %s", strings.Join(contract.PassDurations, ", "))
	}
	id := "cli-pair-" + fmt.Sprint(time.Now().UnixNano())
	if *cancelInv {
		_, err := call(*sock, shellipc.PairCancel{Type: shellipc.TypePairCancel, RequestID: id}, id)
		return err
	}
	res, err := call(*sock, shellipc.PairIssue{Type: shellipc.TypePairIssue, RequestID: id, Pass: *guest}, id)
	if err != nil {
		return err
	}
	data, _ := res.Data.(map[string]any)
	fmt.Printf("Pairing code: %v\n", data["code"])
	if end, ok := data["pass_expires_at_ms"].(float64); ok {
		fmt.Printf("Guest pass: remote only, ends %s\n", time.UnixMilli(int64(end)).Format("Mon 15:04"))
	}
	if u, _ := data["url"].(string); u != "" {
		fmt.Printf("Open on the phone: %s\n", u)
	} else {
		fmt.Println("The phone remote is not listening; enable it first (bear-den-tv remote enable ...).")
	}
	return nil
}

func cmdDevices(args []string) error {
	fs := flag.NewFlagSet("devices", flag.ExitOnError)
	sock := socketFlag(fs)
	_ = fs.Parse(args)
	rest := fs.Args()
	if len(rest) >= 2 && rest[0] == "revoke" {
		id := "cli-revoke-" + fmt.Sprint(time.Now().UnixNano())
		_, err := call(*sock, shellipc.DevicesRevoke{Type: shellipc.TypeDevicesRevoke, RequestID: id, DeviceID: rest[1]}, id)
		return err
	}
	if len(rest) > 0 {
		return errors.New(`usage: bear-den-tv devices [revoke <id|*>]`)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := dialCLI(ctx, *sock)
	if err != nil {
		return err
	}
	defer conn.Close()
	if conn.State.Devices == nil || len(*conn.State.Devices) == 0 {
		fmt.Println("No paired devices.")
		return nil
	}
	for _, d := range *conn.State.Devices {
		ends := ""
		if d.ExpiresAtMs != nil {
			ends = "\tguest pass ends " + time.UnixMilli(*d.ExpiresAtMs).Format("Mon 15:04")
		}
		fmt.Printf("%s\t%s\t%v\tconnected=%v%s\n", d.ID, d.Name, d.Permissions, d.Connected, ends)
	}
	return nil
}

// validPass reports whether pass is a guest pass duration the coordinator
// accepts (contract.PassDurations); the coordinator checks again.
func validPass(pass string) bool {
	for _, p := range contract.PassDurations {
		if p == pass {
			return true
		}
	}
	return false
}

func cmdRemote(args []string) error {
	if len(args) == 0 || (args[0] != "enable" && args[0] != "disable") {
		return errors.New("usage: bear-den-tv remote enable --interface IF --accept-lan-exposure [--port N] [--transport trusted-lan-http|https] | remote disable")
	}
	enable := args[0] == "enable"
	fs := flag.NewFlagSet("remote", flag.ExitOnError)
	sock := socketFlag(fs)
	iface := fs.String("interface", "", "the one network interface to listen on (for example enp1s0)")
	port := fs.Int("port", 0, "TCP port (default from config)")
	transport := fs.String("transport", "", "trusted-lan-http or https (default from config)")
	layoutHTTP := fs.Bool("http-layout-editing", false, "allow layout editing over trusted-LAN HTTP")
	consent := fs.Bool("accept-lan-exposure", false, "confirm that phones on this LAN may reach the remote")
	_ = fs.Parse(args[1:])
	if enable && (*iface == "" || !*consent) {
		return errors.New("enabling the phone remote exposes it to your LAN: pass --interface and --accept-lan-exposure")
	}
	id := "cli-remote-" + fmt.Sprint(time.Now().UnixNano())
	_, err := call(*sock, shellipc.RemoteConfigure{
		Type: shellipc.TypeRemoteConfigure, RequestID: id, Enabled: enable, Transport: *transport,
		Interface: *iface, Port: *port, HTTPLayoutEditing: *layoutHTTP, LANConsent: enable && *consent,
	}, id)
	if err == nil {
		fmt.Println("ok")
	}
	return err
}
