// `bear-den-tv apps install|install-cancel`: install an app Bear Den knows
// from Flathub for this user (docs/decisions/0011-per-user-flathub-installs.md,
// docs/operations.md "App installs"). By default it asks the running
// coordinator over IPC (app.install, then follows state until the install
// ends); with --here it runs the installer in this process, for a box
// where Bear Den is not running (and the container proof).

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/install"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/doctor"
	"bear-den-tv/internal/shellipc"
)

// cmdAppsInstall runs:
//
//	apps install APP-ID [--here]     install the app's Flatpak from Flathub for this user
//	apps install-cancel APP-ID       stop a running install (through the coordinator)
//
// APP-ID is a config application id (plex-htpc, youtube, moonlight, netflix,
// …); the Flatpak comes from that app's adapter row, never from the command
// line.
func cmdAppsInstall(args []string) error {
	fs := flag.NewFlagSet("apps "+args[0], flag.ExitOnError)
	here := fs.Bool("here", false, "run the installer in this process instead of asking the running coordinator")
	sock := socketFlag(fs)
	// Flags may come after the app id too ("apps install moonlight --here").
	var flags, rest []string
	for _, a := range args[1:] {
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
		} else {
			rest = append(rest, a)
		}
	}
	_ = fs.Parse(append(flags, rest...))
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: bear-den-tv apps %s APP-ID [--here]", args[0])
	}
	appID := fs.Arg(0)
	if args[0] == "install-cancel" {
		id := "cli-install-cancel"
		_, err := call(socketPath(*sock), shellipc.AppInstallCancel{Type: shellipc.TypeAppInstallCancel, RequestID: id, AppID: appID}, id)
		return err
	}
	if *here {
		return installHere(appID)
	}
	return installViaCoordinator(socketPath(*sock), appID)
}

func socketPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return doctor.DefaultPaths().Socket
}

// installViaCoordinator sends app.install and prints the app's install
// state from every state push until it ends.
func installViaCoordinator(socket, appID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := dialCLI(ctx, socket)
	if err != nil {
		return fmt.Errorf("is Bear Den running? (use --here to install without it): %w", err)
	}
	defer conn.Close()
	id := "cli-install"
	if err := conn.Send(shellipc.AppInstall{Type: shellipc.TypeAppInstall, RequestID: id, AppID: appID}); err != nil {
		return err
	}
	res, err := conn.Result(id, 10*time.Second)
	if err != nil {
		return err
	}
	if !res.OK {
		return errors.New(res.Error)
	}
	if d, _ := res.Data.(map[string]any); d["already_installed"] == true {
		fmt.Println("Already installed.")
		return nil
	}
	last := ""
	for {
		m, err := conn.Recv()
		if err != nil {
			return err
		}
		st, ok := m.(shellipc.State)
		if !ok {
			continue
		}
		for _, a := range st.State.Applications {
			if a.ID != appID || a.Install == nil {
				continue
			}
			line := describe(a.Install.State, a.Install.Phase, a.Install.Progress, a.Install.Message)
			if line != last {
				fmt.Println(line)
				last = line
			}
			switch a.Install.State {
			case contract.InstallDone:
				return nil
			case contract.InstallFailed:
				return errors.New(a.Install.Message)
			case contract.InstallAvailable:
				return errors.New("the install stopped: " + a.Install.Message)
			}
		}
	}
}

// installHere runs the installer in this process, resolving APP-ID through
// config.json and the adapter table like the coordinator does.
func installHere(appID string) error {
	cfg, err := readConfig(doctor.DefaultPaths().ConfigDir)
	if err != nil {
		return err
	}
	app, ok := cfg.Application(appID)
	if !ok {
		return fmt.Errorf("%q is not an app in config.json", appID)
	}
	ad, ok := adapters.ForName(app.Adapter)
	if !ok || !adapters.RunsIn(ad, app.Launch.AppID) {
		return fmt.Errorf("Bear Den can't install %s", app.Label)
	}
	ids := adapters.NewRegistry().InstallableFlatpakIDs()
	done := make(chan struct{}, 1)
	in := install.New(install.Options{Allowed: ids, OnChange: func() {
		select {
		case done <- struct{}{}:
		default:
		}
	}})
	if ok, why := in.Available(); !ok {
		return errors.New(why)
	}
	fid := app.Launch.AppID
	if in.Installed(context.Background(), fid) {
		fmt.Println("Already installed.")
		return nil
	}
	fmt.Printf("Installing %s (%s) from Flathub into %s\n", app.Label, fid, in.UserDir())
	start := time.Now()
	if err := in.Start(fid); err != nil {
		return err
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	last, sized := "", false
	for {
		select {
		case <-sig:
			_ = in.Cancel(fid)
		case <-done:
		case <-time.After(time.Second):
		}
		st := in.Status(fid)
		if !sized && st.SizeBytes > 0 {
			sized = true
			fmt.Printf("About %s to download, %s on disk (plus shared parts if needed)\n", install.HumanSize(st.SizeBytes), install.HumanSize(st.DiskBytes))
		}
		line := describe(st.State, st.Phase, st.Progress, st.Message)
		if line != last {
			fmt.Printf("%6.1fs  %s\n", time.Since(start).Seconds(), line)
			last = line
		}
		switch st.State {
		case contract.InstallDone:
			return nil
		case contract.InstallFailed:
			return errors.New(st.Message)
		case contract.InstallAvailable:
			return errors.New(st.Message)
		}
	}
}

func describe(state, phase string, progress int, message string) string {
	parts := []string{state}
	if phase != "" {
		parts = append(parts, phase)
	}
	if state == contract.InstallDownloading || state == contract.InstallDone {
		parts = append(parts, fmt.Sprintf("%d%%", progress))
	}
	if message != "" {
		parts = append(parts, message)
	}
	return strings.Join(parts, " · ")
}
