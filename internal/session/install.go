// App installs in the coordinator: the owner-only actions app.install,
// app.install_cancel and app.uninstall, their IPC twins (app.install,
// app.install_info, app.install_cancel, app.uninstall, apps.configure),
// state.applications[].install and
// state.apps, rediscovery once an install is done (turning on first the app
// whose own card started it, when the shell said so: IPC app.install
// "enable"), and the daily update
// while the TV is idle (config apps.auto_update). The installer itself is
// internal/applications/install. Removal (the owner's confirmed Remove)
// runs `flatpak uninstall --user` for the app's adapter's Flatpak only,
// refuses system-wide installs and running apps with plain words, logs how
// it ended and discovers the app again so its tile hides.
// Spec: contracts/actions.md "App installs" and "App removal",
// contracts/http.md "App installs", docs/decisions/0011-per-user-flathub-installs.md.

package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"bear-den-tv/internal/applications/adapters"
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
	Removing() bool
	Uninstall(ctx context.Context, flatpakID string, deleteData bool) error
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
		c.logInstallOutcomeLocked(a.ID, fid)
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
			// Before the tile lights up, so the shell's open of it finds
			// the app on.
			c.enableAfterInstall(fid)
			c.rediscover(context.Background(), fid)
			c.mu.Lock()
			delete(c.rediscovering, fid)
			c.mu.Unlock()
			c.afterInstalled(fid)
		}(fid)
	}
}

// logInstallOutcomeLocked logs once how an install ended (on the TV a
// RetroArch install ended with nothing in the log and no tile). c.mu is held.
func (c *Coordinator) logInstallOutcomeLocked(appID, fid string) {
	st := c.opts.Installer.Status(fid)
	outcome := ""
	switch st.State {
	case contract.InstallDone, contract.InstallFailed:
		outcome = st.State
	default:
		if c.installLogged[fid] != "" && st.State != contract.InstallDone && st.State != contract.InstallFailed {
			delete(c.installLogged, fid) // a new job starts: log its outcome too
		}
		return
	}
	if c.installLogged[fid] == outcome {
		return
	}
	if c.installLogged == nil {
		c.installLogged = map[string]string{}
	}
	c.installLogged[fid] = outcome
	if outcome == contract.InstallFailed {
		c.log.Warn("session: install from Flathub failed", "app", appID, "flatpak", fid, "reason", st.Message)
	} else {
		c.log.Info("session: install from Flathub finished", "app", appID, "flatpak", fid)
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
	out.DRM = c.drmForLocked(a, rt.install.Installed)
	switch st.State {
	case contract.InstallPreparing, contract.InstallDownloading, contract.InstallInstalling, contract.InstallFailed, contract.InstallDone, contract.InstallRemoving:
		out.State, out.Phase, out.Progress, out.Message = st.State, st.Phase, st.Progress, st.Message
		if rt.install.Installed && rt.install.SizeBytes > 0 {
			v := rt.install.SizeBytes
			out.InstalledBytes = &v
		}
		return out
	}
	ok, why := in.Available()
	switch {
	case rt.install.Installed:
		if rt.install.SizeBytes > 0 {
			v := rt.install.SizeBytes
			out.InstalledBytes = &v
		}
		if rt.install.Scope == "system" {
			out.Message = reasonSystem
		} else if st.Message != "" {
			out.Message = st.Message // why the last Remove did not work
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

// installCapsLocked fills app.install, app.install_cancel and
// app.uninstall. c.mu is held.
func (c *Coordinator) installCapsLocked(caps map[string]contract.Capability) {
	in := c.opts.Installer
	if in == nil {
		caps[contract.ActionAppInstall] = unavailable("App installs are not available in this session.")
		caps[contract.ActionAppInstallCancel] = caps[contract.ActionAppInstall]
		caps[contract.ActionAppUninstall] = unavailable("Removing apps is not available in this session.")
		return
	}
	if ok, why := in.Available(); !ok {
		caps[contract.ActionAppInstall] = unavailable(why)
		caps[contract.ActionAppInstallCancel] = unavailable(why)
		caps[contract.ActionAppUninstall] = unavailable(why)
		return
	}
	caps[contract.ActionAppInstall] = available(backendInstall)
	caps[contract.ActionAppUninstall] = available(backendInstall)
	if in.Busy() && !in.Updating() && !in.Removing() {
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
// from the app's adapter row, never from the sender. enable (the shell's
// install card opened from that app's own tile or row) turns that app, and
// only that app, on once the install is done (enableAfterInstall); several
// apps may share the Flatpak (the streaming sites share their browser), so
// the press says which one the owner meant.
func (c *Coordinator) startInstall(appID string, enable bool) (map[string]any, error) {
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
	if !known || !adapters.RunsIn(ad, fid) || !in.Allowed(fid) {
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
	c.mu.Lock()
	if c.enableAfter == nil {
		c.enableAfter = map[string]string{}
	}
	if enable {
		c.enableAfter[fid] = appID
	} else {
		delete(c.enableAfter, fid)
	}
	c.mu.Unlock()
	c.log.Info("session: installing from Flathub", "app", appID, "flatpak", fid, "turn_on_after", enable)
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
	detail, err := c.startInstall(appID, false)
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

// startUninstall is app.uninstall from an owner phone or the shell (the
// TV's confirmed Remove): the Flatpak comes from the app's adapter row,
// never from the sender, and is removed for this user only. It returns
// once the removal has started; install.state is "removing" meanwhile.
func (c *Coordinator) startUninstall(appID string, deleteData bool) (map[string]any, error) {
	in := c.opts.Installer
	if in == nil {
		return nil, &installError{contract.CodeUnsupported, "Removing apps is not available in this session."}
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
	if !known || !adapters.RunsIn(ad, fid) || !in.Allowed(fid) {
		return nil, &installError{contract.CodeUnsupported, "Bear Den can't remove " + app.Label + "."}
	}
	c.mu.Lock()
	inst := c.appLocked(appID).install
	var running []string
	for _, a := range c.opts.Config.Current().Applications {
		if a.Launch.AppID != fid {
			continue
		}
		rt := c.appLocked(a.ID)
		if rt.instance != nil || rt.launchState == "running" || rt.launchState == "launching" || (c.opts.Web != nil && c.opts.Web.Running(a.ID)) {
			running = append(running, a.Label)
		}
	}
	c.mu.Unlock()
	switch {
	case !inst.Installed:
		return nil, &installError{contract.CodeUnsupported, app.Label + " is not installed."}
	case inst.Scope == "system":
		return nil, &installError{contract.CodeUnsupported, app.Label + " is installed for everyone on this PC, so it can only be removed with the PC's own software tool."}
	case len(running) > 0:
		return nil, &installError{contract.CodeBusy, "Close " + running[0] + " first."}
	}
	// The owner's press wins over the idle update, as for installs.
	if in.Updating() && in.CancelUpdate() {
		deadline := c.clock.Now().Add(8 * time.Second)
		for in.Busy() && c.clock.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if in.Busy() {
		return nil, &installError{contract.CodeBusy, "Another app is installing or being removed. Wait for it to finish."}
	}
	c.log.Info("session: removing from this TV", "app", appID, "flatpak", fid, "delete_data", deleteData)
	go func() {
		err := in.Uninstall(context.Background(), fid, deleteData)
		var f *install.Failure
		switch {
		case errors.As(err, &f):
			c.log.Warn("session: remove failed", "app", appID, "flatpak", fid, "reason", f.Reason, "detail", f.Detail)
		case err != nil:
			c.log.Warn("session: remove failed", "app", appID, "flatpak", fid, "reason", err.Error())
		default:
			c.log.Info("session: removed from this TV", "app", appID, "flatpak", fid, "data_deleted", deleteData)
		}
		c.rediscover(context.Background(), fid)
	}()
	return map[string]any{"app_id": appID, "removing": true}, nil
}

// doUninstall routes app.uninstall (owner phones; the permission was checked).
func (c *Coordinator) doUninstall(req contract.ActionRequest) contract.ActionResult {
	appID, _ := req.Args["app_id"].(string)
	deleteData, _ := req.Args["delete_data"].(bool)
	detail, err := c.startUninstall(appID, deleteData)
	return c.installResult(req, detail, err)
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

// configureApps is IPC apps.configure (Apps → Keep apps up to date).
func (c *Coordinator) configureApps(autoUpdate bool) error {
	if _, err := c.opts.Config.Update(func(cfg *config.Config) error {
		apps := config.Apps{}
		if cfg.Apps != nil {
			apps = *cfg.Apps // the browser choices stay
		}
		apps.AutoUpdate = autoUpdate
		cfg.Apps = &apps
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

// enableAfterInstall turns on the app whose own card started flatpakID's
// install (startInstall's enable), through the normal config write, once;
// an app already on, or no longer using that Flatpak, is left alone.
func (c *Coordinator) enableAfterInstall(flatpakID string) {
	c.mu.Lock()
	appID := c.enableAfter[flatpakID]
	delete(c.enableAfter, flatpakID)
	c.mu.Unlock()
	if appID == "" {
		return
	}
	app, ok := c.opts.Config.Current().Application(appID)
	if !ok || app.Launch.AppID != flatpakID || app.IsEnabled() || !c.isWebAdapter(app.Adapter) {
		return
	}
	if err := c.setAppEnabled(context.Background(), appID, true); err != nil {
		c.log.Warn("session: could not turn the app on after its install", "app", appID, "err", err)
		return
	}
	c.log.Info("session: turned on after its install (the owner pressed Install on its card)", "app", appID, "flatpak", flatpakID)
}

// afterInstalled runs what a freshly installed Flatpak needs before its
// apps are fully ready: for a web browser, the enabled streaming sites' playback
// support (widevine.go).
func (c *Coordinator) afterInstalled(flatpakID string) {
	c.prepareEnabledStreaming(flatpakID)
}
