// App installs in the coordinator: the owner-only actions app.install and
// app.install_cancel, their IPC twins (app.install, app.install_info,
// app.install_cancel, apps.configure), state.applications[].install and
// state.apps, rediscovery once an install is done, and the daily update
// while the TV is idle (config apps.auto_update). The installer itself is
// internal/applications/install. Spec: contracts/actions.md "App installs",
// contracts/http.md "App installs", docs/decisions/0011-per-user-flathub-installs.md.

package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"bear-den-tv/internal/applications/install"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
)

// AppInstaller is the seam to install.Installer (a fake in tests).
type AppInstaller interface {
	Available() (bool, string)
	Allowed(flatpakID string) bool
	Status(flatpakID string) install.Status
	Info(ctx context.Context, flatpakID string) (install.Sizes, error)
	Start(flatpakID string) error
	Cancel(flatpakID string) error
	Busy() bool
	Updating() bool
	Update(ctx context.Context, flatpakIDs []string) error
	CancelUpdate() bool
}

// Defaults for the idle update (Options.UpdateCheck, UpdateEvery).
const (
	DefaultUpdateCheck = 30 * time.Minute
	DefaultUpdateEvery = 24 * time.Hour
)

const (
	backendInstall = "flathub"
	reasonSystem   = "Updated by your system"
)

// updateState is the idle update's bookkeeping (c.mu).
type updateState struct {
	last    time.Time
	running bool
}

// InstallChanged is the installer's OnChange: a new state, and once a
// Flatpak is installed, its apps are discovered again so their tiles light
// up.
func (c *Coordinator) InstallChanged() {
	c.publish()
	if c.opts.Installer == nil {
		return
	}
	var ids []string
	c.mu.Lock()
	for _, a := range c.opts.Config.Current().Applications {
		fid := a.Launch.AppID
		if c.opts.Installer.Status(fid).State == contract.InstallDone && !c.appLocked(a.ID).install.Installed && !c.rediscovering[fid] {
			if c.rediscovering == nil {
				c.rediscovering = map[string]bool{}
			}
			c.rediscovering[fid] = true
			ids = append(ids, fid)
		}
	}
	c.mu.Unlock()
	for _, fid := range ids {
		go func(fid string) {
			c.rediscover(context.Background(), fid)
			c.mu.Lock()
			delete(c.rediscovering, fid)
			c.mu.Unlock()
			c.afterInstalled(fid)
		}(fid)
	}
}

// rediscover re-reads the installation of every app using flatpakID.
func (c *Coordinator) rediscover(ctx context.Context, flatpakID string) {
	if c.opts.Launcher == nil {
		return
	}
	for _, a := range c.opts.Config.Current().Applications {
		if a.Launch.AppID != flatpakID {
			continue
		}
		inst, err := c.opts.Launcher.Discover(ctx, flatpakID)
		c.mu.Lock()
		rt := c.appLocked(a.ID)
		if err == nil {
			rt.install, rt.discovered = inst, true
		}
		c.mu.Unlock()
	}
	c.setInstalledApps()
	c.publish()
}

// installForLocked is one app's state.applications[].install. c.mu is held.
func (c *Coordinator) installForLocked(a configApp, rt *appRuntime) *contract.Install {
	in := c.opts.Installer
	if in == nil {
		return nil
	}
	fid := a.Launch.AppID
	st := in.Status(fid)
	out := &contract.Install{State: contract.InstallNone, Phase: contract.PhaseIdle}
	if st.SizeBytes > 0 {
		v := st.SizeBytes
		out.SizeBytes = &v
	}
	if st.DiskBytes > 0 {
		v := st.DiskBytes
		out.DiskBytes = &v
	}
	switch st.State {
	case contract.InstallPreparing, contract.InstallDownloading, contract.InstallInstalling, contract.InstallFailed, contract.InstallDone:
		out.State, out.Phase, out.Progress, out.Message = st.State, st.Phase, st.Progress, st.Message
		return out
	}
	ok, why := in.Available()
	switch {
	case rt.install.Installed:
		if rt.install.Scope == "system" {
			out.Message = reasonSystem
		}
	case !rt.discovered:
		// Not known yet: nothing to offer until discovery has answered.
	case !ok:
		out.Message = why
	case !in.Allowed(fid):
		out.Message = "Bear Den can't install " + a.Label + "."
	default:
		out.State, out.Message = contract.InstallAvailable, st.Message // "Install cancelled" after a cancel
	}
	return out
}

// installCapsLocked fills app.install and app.install_cancel. c.mu is held.
func (c *Coordinator) installCapsLocked(caps map[string]contract.Capability) {
	in := c.opts.Installer
	if in == nil {
		caps[contract.ActionAppInstall] = unavailable("App installs are not available in this session.")
		caps[contract.ActionAppInstallCancel] = caps[contract.ActionAppInstall]
		return
	}
	if ok, why := in.Available(); !ok {
		caps[contract.ActionAppInstall] = unavailable(why)
		caps[contract.ActionAppInstallCancel] = unavailable(why)
		return
	}
	caps[contract.ActionAppInstall] = available(backendInstall)
	if in.Busy() && !in.Updating() {
		caps[contract.ActionAppInstallCancel] = available(backendInstall)
	} else {
		caps[contract.ActionAppInstallCancel] = unavailable("Nothing is installing.")
	}
}

// installError is a refused install, with the result code a phone gets.
type installError struct {
	code contract.Code
	msg  string
}

func (e *installError) Error() string { return e.msg }

// startInstall is app.install from a phone or the shell: the Flatpak comes
// from the app's adapter row, never from the sender.
func (c *Coordinator) startInstall(appID string) (map[string]any, error) {
	in := c.opts.Installer
	if in == nil {
		return nil, &installError{contract.CodeUnsupported, "App installs are not available in this session."}
	}
	if ok, why := in.Available(); !ok {
		return nil, &installError{contract.CodeUnsupported, why}
	}
	app, ok := c.opts.Config.Current().Application(appID)
	if !ok {
		return nil, &installError{contract.CodeInvalid, "That application is not registered."}
	}
	fid := app.Launch.AppID
	ad, known := c.opts.Adapters.ForName(app.Adapter)
	if !known || ad.FlatpakID() != fid || !in.Allowed(fid) {
		return nil, &installError{contract.CodeUnsupported, "Bear Den can't install " + app.Label + "."}
	}
	c.mu.Lock()
	installed := c.appLocked(appID).install.Installed
	c.mu.Unlock()
	if installed {
		return map[string]any{"already_installed": true}, nil
	}
	switch in.Status(fid).State {
	case contract.InstallPreparing, contract.InstallDownloading, contract.InstallInstalling:
		return map[string]any{"already_running": true}, nil
	}
	// The owner's press wins over the idle update.
	if in.Updating() && in.CancelUpdate() {
		deadline := c.clock.Now().Add(8 * time.Second)
		for in.Busy() && c.clock.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
	}
	switch err := in.Start(fid); {
	case errors.Is(err, install.ErrBusy):
		return nil, &installError{contract.CodeBusy, "Another app is installing. Wait for it to finish."}
	case err != nil:
		return nil, &installError{contract.CodeUnsupported, "Bear Den can't install " + app.Label + "."}
	}
	c.log.Info("session: installing from Flathub", "app", appID, "flatpak", fid)
	c.publish()
	return map[string]any{"install_state": contract.InstallPreparing}, nil
}

func (c *Coordinator) cancelInstall(appID string) error {
	in := c.opts.Installer
	app, ok := c.opts.Config.Current().Application(appID)
	if in == nil || !ok {
		return &installError{contract.CodeUnsupported, "Nothing is installing."}
	}
	if err := in.Cancel(app.Launch.AppID); err != nil {
		return &installError{contract.CodeUnsupported, "Nothing is installing."}
	}
	return nil
}

func (c *Coordinator) installResult(req contract.ActionRequest, detail map[string]any, err error) contract.ActionResult {
	var ie *installError
	if errors.As(err, &ie) {
		return c.fail(req, ie.code, ie.msg)
	}
	if err != nil {
		return c.fail(req, contract.CodeInternal, "The install could not start.")
	}
	if detail["already_installed"] == true {
		return c.result(req, contract.OutcomeObserved, detail)
	}
	return c.result(req, contract.OutcomeDelivered, detail)
}

// doInstall routes app.install (owner phones; the permission was checked).
func (c *Coordinator) doInstall(req contract.ActionRequest) contract.ActionResult {
	appID, _ := req.Args["app_id"].(string)
	detail, err := c.startInstall(appID)
	return c.installResult(req, detail, err)
}

// doInstallCancel routes app.install_cancel.
func (c *Coordinator) doInstallCancel(req contract.ActionRequest) contract.ActionResult {
	appID, _ := req.Args["app_id"].(string)
	if err := c.cancelInstall(appID); err != nil {
		return c.installResult(req, nil, err)
	}
	return c.result(req, contract.OutcomeDelivered, map[string]any{"app_id": appID})
}

// installInfo is IPC app.install_info: how big the app's install is.
func (c *Coordinator) installInfo(ctx context.Context, appID string) (map[string]any, error) {
	in := c.opts.Installer
	if in == nil {
		return nil, errors.New("App installs are not available in this session.")
	}
	if ok, why := in.Available(); !ok {
		return nil, errors.New(why)
	}
	app, ok := c.opts.Config.Current().Application(appID)
	if !ok || !in.Allowed(app.Launch.AppID) {
		return nil, errors.New("Bear Den can't install that app.")
	}
	if _, err := in.Info(ctx, app.Launch.AppID); err != nil {
		var f *install.Failure
		if errors.As(err, &f) {
			return nil, errors.New(f.Reason)
		}
		return nil, fmt.Errorf("Couldn't ask Flathub: %v", err)
	}
	st := in.Status(app.Launch.AppID)
	c.publish()
	return map[string]any{"size_bytes": st.SizeBytes, "disk_bytes": st.DiskBytes}, nil
}

// configureApps is IPC apps.configure (Settings → Keep apps up to date).
func (c *Coordinator) configureApps(autoUpdate bool) error {
	if _, err := c.opts.Config.Update(func(cfg *config.Config) error {
		cfg.Apps = &config.Apps{AutoUpdate: autoUpdate}
		return nil
	}); err != nil {
		return err
	}
	if !autoUpdate && c.opts.Installer != nil {
		c.opts.Installer.CancelUpdate()
	}
	c.publish()
	return nil
}

// watchUpdates checks every UpdateCheck whether the daily update may run.
func (c *Coordinator) watchUpdates(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.clock.After(c.opts.UpdateCheck):
		}
		c.maybeUpdate(ctx)
	}
}

// updateIdleLocked: the update may run only while the session is unlocked,
// Bear Den's shell is in front (its screensaver may be on) and no app is
// running, so nothing is playing. c.mu is held.
func (c *Coordinator) updateIdleLocked() bool {
	return !c.locked && c.target.Kind == "shell" && !c.anyAppRunningLocked()
}

// maybeUpdate starts the idle update of the user's installs when it is due,
// allowed and idle; it reports whether it started one.
func (c *Coordinator) maybeUpdate(ctx context.Context) bool {
	in := c.opts.Installer
	cfg := c.opts.Config.Current()
	if in == nil || !cfg.AutoUpdate() {
		return false
	}
	if ok, _ := in.Available(); !ok || in.Busy() {
		return false
	}
	var ids []string
	seen := map[string]bool{}
	c.mu.Lock()
	if !c.updateIdleLocked() || c.upd.running || (!c.upd.last.IsZero() && c.clock.Since(c.upd.last) < c.opts.UpdateEvery) {
		c.mu.Unlock()
		return false
	}
	for _, a := range cfg.Applications {
		fid := a.Launch.AppID
		rt := c.appLocked(a.ID)
		// Only what this user installed; system installs are the system's.
		if rt.install.Installed && rt.install.Scope == "user" && in.Allowed(fid) && !seen[fid] {
			seen[fid] = true
			ids = append(ids, fid)
		}
	}
	if len(ids) == 0 {
		c.mu.Unlock()
		return false
	}
	c.upd.last, c.upd.running = c.clock.Now(), true
	c.mu.Unlock()
	go func() {
		err := in.Update(ctx, ids)
		c.mu.Lock()
		c.upd.running = false
		c.mu.Unlock()
		if err != nil {
			c.log.Info("session: app update stopped", "err", err)
		}
		for _, fid := range ids {
			c.rediscover(ctx, fid) // new versions
		}
	}()
	c.log.Info("session: updating apps while idle", "flatpaks", ids)
	return true
}

// stopUpdateFor ends a running idle update because the TV is no longer idle
// (an app starting or coming to the front).
func (c *Coordinator) stopUpdateFor(why string) {
	if in := c.opts.Installer; in != nil && in.CancelUpdate() {
		c.log.Info("session: app update cancelled", "why", why)
	}
}

// afterInstalled runs what a freshly installed Flatpak needs before its
// apps are fully ready. Nothing yet.
func (c *Coordinator) afterInstalled(string) {}
