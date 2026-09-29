// Action routing: submit, validate against the foreground and finish (spec
// contracts/actions.md).

package session

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"bear-den-tv/internal/actions"
	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/remote"
	"bear-den-tv/internal/shellipc"
)

type configApp = config.Application

// shellSender is the dedup/result key for shell-originated requests.
const shellSender = "\x00shell"

// sender identifies who submitted an action.
type sender struct {
	key    string // device id, or shellSender
	viewer *remote.Viewer
}

func (s sender) isShell() bool { return s.key == shellSender }

func phoneSender(v remote.Viewer) sender { return sender{key: v.DeviceID, viewer: &v} }

// submit is the single entry point for every action: replay cache, then route.
func (c *Coordinator) submit(ctx context.Context, s sender, req contract.ActionRequest) contract.ActionResult {
	switch verdict, prior := c.dedup.Check(s.key, req); verdict {
	case actions.Replay:
		return prior
	case actions.Mismatch:
		return c.fail(req, contract.CodeDuplicateMismatch, "This request id was already used for a different action.")
	}
	res := c.route(ctx, s, req)
	c.dedup.Remember(s.key, req, res)
	return res
}

// finish records and delivers a late (post-accepted) result.
func (c *Coordinator) finish(s sender, req contract.ActionRequest, res contract.ActionResult) {
	c.dedup.Remember(s.key, req, res)
	if s.isShell() {
		if sh := c.shellClient(); sh != nil {
			_ = sh.Send(shellipc.ActionResult{Type: shellipc.TypeActionResult, Result: res})
		}
		return
	}
	c.emitResult(s.key, res)
}

func (c *Coordinator) current() (int64, contract.Target, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.epoch, c.target, c.locked
}

func (c *Coordinator) fail(req contract.ActionRequest, code contract.Code, msg string) contract.ActionResult {
	epoch, target, _ := c.current()
	return contract.Failed(req, epoch, target.Info(), code, msg)
}

func (c *Coordinator) result(req contract.ActionRequest, outcome contract.Outcome, detail map[string]any) contract.ActionResult {
	epoch, target, _ := c.current()
	return contract.Result(req, outcome, epoch, target.Info(), detail)
}

func (c *Coordinator) route(ctx context.Context, s sender, req contract.ActionRequest) contract.ActionResult {
	if req.Protocol != 1 {
		return c.fail(req, contract.CodeUnsupportedProtocol, "This remote speaks a different protocol version.")
	}
	if !s.isShell() {
		if s.viewer == nil {
			return c.fail(req, contract.CodeForbidden, "This device is not allowed to do that.")
		}
		if ok, msg := c.phoneMay(*s.viewer, req); !ok {
			return c.fail(req, contract.CodeForbidden, msg)
		}
	}
	// Any authorized press wakes an off display and cancels a sleep
	// warning; the press that woke the display is swallowed (power.go).
	if c.powerGate(req.Action) {
		res := c.fail(req, contract.CodeDisplayOff, "The screen was off. This press turned it on; press again.")
		res.Detail = map[string]any{"display": contract.DisplayOn}
		return res
	}
	epoch, target, locked := c.current()
	if locked {
		return c.fail(req, contract.CodeLocked, "The TV session is locked. Unlock it on the TV.")
	}
	if !s.isShell() && !contract.IgnoresStaleEpoch(req.Action) && req.ContextEpoch != epoch {
		return c.fail(req, contract.CodeStaleEpoch, "The TV changed since this button was shown; try again.")
	}
	switch req.Action {
	case contract.ActionNavUp, contract.ActionNavDown, contract.ActionNavLeft, contract.ActionNavRight,
		contract.ActionSelect, contract.ActionBack, contract.ActionTextSubmit:
		return c.routeInput(ctx, req, target)
	case contract.ActionHome:
		c.tvWake("home") // HDMI-CEC: the TV on and on Bear Den's input (cec.go)
		return c.async(s, req, c.doHome)
	case contract.ActionAppLaunch:
		return c.async(s, req, c.doLaunch)
	case contract.ActionAppClose:
		return c.doClose(ctx, req)
	case contract.ActionMediaPlay, contract.ActionMediaPause, contract.ActionMediaSeek:
		return c.doMedia(ctx, req, target)
	case contract.ActionAudioVolume, contract.ActionAudioMute:
		return c.doAudio(ctx, req)
	case contract.ActionSleepTimer:
		return c.doSleepTimer(req)
	case contract.ActionDisplayOff:
		return c.doDisplayOff(ctx, req)
	case contract.ActionTVPower:
		return c.doTVPower(ctx, req)
	case contract.ActionPointerMove, contract.ActionPointerClick, contract.ActionPointerScroll:
		return c.doPointer(ctx, s, req, target)
	case contract.ActionAppInstall, contract.ActionAppInstallCancel:
		if cp, _ := c.capability(req.Action); !cp.Available {
			return c.fail(req, contract.CodeUnsupported, cp.Reason)
		}
		if req.Action == contract.ActionAppInstall {
			return c.doInstall(req)
		}
		return c.doInstallCancel(req)
	case contract.ActionShellRestart:
		if c.opts.Supervisor == nil {
			return c.fail(req, contract.CodeUnsupported, "The shell is not supervised by this coordinator.")
		}
		if err := c.opts.Supervisor.Restart(); err != nil {
			return c.fail(req, contract.CodeInternal, "The shell could not be restarted.")
		}
		return c.result(req, contract.OutcomeDelivered, nil)
	}
	return c.fail(req, contract.CodeUnsupported, fmt.Sprintf("%q is not supported.", req.Action))
}

// phoneMay is the permission gate for a phone's action. A guest pass may send
// only contract.GuestActions (so an action added later is refused to guests
// until someone puts it on that list) and nothing once its pass has ended;
// everyone else needs controller, and owner for contract.OwnerActions
// (shell.restart, app.install, app.install_cancel) and a forced app.close.
func (c *Coordinator) phoneMay(v remote.Viewer, req contract.ActionRequest) (bool, string) {
	if v.Guest() {
		if c.passEnded(v) {
			return false, "Your guest pass has ended."
		}
		if !contract.GuestMayUse(req.Action) {
			return false, "Guest passes can't do that."
		}
		return true, ""
	}
	if contract.OwnerActions[req.Action] || (req.Action == contract.ActionAppClose && boolArg(req.Args, "force")) {
		if !v.Has(contract.PermOwner) {
			return false, "Only the owner's phone can do that."
		}
		return true, ""
	}
	if !v.Has(contract.PermController) {
		return false, "This device is not allowed to do that."
	}
	return true, ""
}

// passEnded reports whether v's guest pass has ended by the coordinator's
// clock. pairing revokes ended passes on its own timer and on every HTTP
// request; this also covers actions on a socket opened before the end.
func (c *Coordinator) passEnded(v remote.Viewer) bool {
	return v.ExpiresAtMs != nil && !c.clock.Now().Before(time.UnixMilli(*v.ExpiresAtMs))
}

// capability returns the current capability for action plus the failure code
// that an unavailable capability maps to.
func (c *Coordinator) capability(action string) (contract.Capability, contract.Code) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := c.capabilitiesLocked()[action]
	switch c.target.Kind {
	case "unknown":
		return cp, contract.CodeUnknownForeground
	case "none":
		return cp, contract.CodeNoTarget
	case "locked":
		return cp, contract.CodeLocked
	case "shell":
		if c.shellClient() == nil {
			return cp, contract.CodeNoTarget
		}
	}
	return cp, contract.CodeUnsupported
}

func (c *Coordinator) routeInput(ctx context.Context, req contract.ActionRequest, target contract.Target) contract.ActionResult {
	switch req.Target {
	case "active":
	case "shell":
		if target.Kind != "shell" {
			return c.fail(req, contract.CodeTargetUnfocused, ShellLabel+" is not in the foreground.")
		}
	default:
		if target.Kind != "app" || strOr(target.AppID) != req.Target {
			return c.fail(req, contract.CodeTargetUnfocused, "That application is not in the foreground.")
		}
	}
	cp, code := c.capability(req.Action)
	if !cp.Available {
		return c.fail(req, code, cp.Reason)
	}
	switch target.Kind {
	case "shell":
		sh := c.shellClient()
		if sh == nil {
			return c.fail(req, contract.CodeNoTarget, "The TV shell is not connected.")
		}
		args := req.Args
		if args == nil {
			args = map[string]any{}
		}
		reply, err := sh.Call(ctx, req.RequestID, shellipc.Input{Type: shellipc.TypeInput, RequestID: req.RequestID, Action: req.Action, Args: args, ContextEpoch: req.ContextEpoch})
		return c.shellReply(req, reply, err)
	case "app":
		if _, isWeb := c.webSpecFor(strOr(target.AppID)); isWeb {
			return c.routeWeb(ctx, req, target)
		}
		c.mu.Lock()
		win := c.targetWindow
		c.mu.Unlock()
		ad, ok := c.adapterFor(strOr(target.AppID))
		if !ok {
			return c.fail(req, contract.CodeUnsupported, "No adapter is registered for "+target.Label+".")
		}
		key, mapped := ad.KeyFor(req.Action)
		if !mapped {
			return c.fail(req, contract.CodeUnsupported, target.Label+" has no verified mapping for this action.")
		}
		if err := c.opts.Desktop.DeliverKey(ctx, win, key); err != nil {
			if errors.Is(err, platform.ErrNotForeground) {
				return c.fail(req, contract.CodeTargetUnfocused, target.Label+" lost focus before the key was sent.")
			}
			return c.fail(req, contract.CodeInternal, "The key could not be delivered.")
		}
		return c.result(req, contract.OutcomeDelivered, nil)
	}
	return c.fail(req, code, cp.Reason)
}

func (c *Coordinator) shellReply(req contract.ActionRequest, reply shellipc.Message, err error) contract.ActionResult {
	switch {
	case errors.Is(err, shellipc.ErrReplyTimeout):
		return c.fail(req, contract.CodeTimeout, "The TV shell did not answer in time.")
	case err != nil:
		return c.fail(req, contract.CodeNoTarget, "The TV shell is not connected.")
	}
	ir, ok := reply.(shellipc.InputResult)
	if !ok {
		return c.fail(req, contract.CodeInternal, "The TV shell sent an unexpected reply.")
	}
	if ir.Outcome == string(contract.OutcomeObserved) {
		return c.result(req, contract.OutcomeObserved, ir.Detail)
	}
	code := contract.Code(ir.Code)
	if code == "" || code == contract.CodeOK {
		code = contract.CodeInternal
	}
	res := c.fail(req, code, "The TV shell could not apply the action.")
	res.Detail = ir.Detail
	return res
}

// async returns accepted now and runs work in the background; its terminal
// result is recorded for replays and pushed to the sender.
func (c *Coordinator) async(s sender, req contract.ActionRequest, work func(context.Context, sender, contract.ActionRequest) contract.ActionResult) contract.ActionResult {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), LaunchObserveTimeout+10*time.Second)
		defer cancel()
		c.finish(s, req, work(ctx, s, req))
	}()
	return c.result(req, contract.OutcomeAccepted, nil)
}

// waitTarget waits until pred holds for the current target or the timeout passes.
func (c *Coordinator) waitTarget(ctx context.Context, timeout time.Duration, pred func(contract.Target) bool) bool {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	wake := c.subscribe(ctx)
	timer := c.clock.NewTimer(timeout)
	defer timer.Stop()
	for {
		if _, t, _ := c.current(); pred(t) {
			return true
		}
		select {
		case <-wake:
		case <-timer.C():
			_, t, _ := c.current()
			return pred(t)
		case <-ctx.Done():
			return false
		}
	}
}

func (c *Coordinator) findShellWindow(ctx context.Context) (platform.WindowID, bool) {
	c.mu.Lock()
	id := c.shellWindow
	c.mu.Unlock()
	wins, err := c.opts.Desktop.ListWindows(ctx)
	if err != nil {
		return 0, false
	}
	for _, w := range wins {
		if w.ID == id {
			return id, true
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, w := range wins {
		if c.isShellWindow(w) {
			c.shellWindow = w.ID
			return w.ID, true
		}
	}
	return 0, false
}

// doHome is the explicit escape: activate the shell window, then ask the shell
// to go home and restore focus. observed only when the shell is the verified
// foreground afterwards.
func (c *Coordinator) doHome(ctx context.Context, _ sender, req contract.ActionRequest) contract.ActionResult {
	sh := c.shellClient()
	if sh == nil {
		return c.fail(req, contract.CodeNoTarget, "The TV shell is not running.")
	}
	var paused map[string]any
	if _, t, _ := c.current(); t.Kind != "shell" {
		if t.Kind == "app" {
			paused = c.pauseWebForHome(ctx, t)
			if paused == nil {
				paused = c.pauseAppForHome(ctx, t) // homepause.go: verified MPRIS only
			}
		}
		win, ok := c.findShellWindow(ctx)
		if !ok {
			return c.fail(req, contract.CodeNoTarget, "The shell window could not be found.")
		}
		if err := c.opts.Desktop.Activate(ctx, win); err != nil {
			return c.fail(req, contract.CodeTargetUnfocused, "The shell window could not be brought forward.")
		}
	}
	reply, err := sh.Call(ctx, req.RequestID, shellipc.Home{Type: shellipc.TypeHome, RequestID: req.RequestID, RestoreFocus: true})
	res := c.shellReply(req, reply, err)
	if res.Outcome != contract.OutcomeObserved {
		return res
	}
	for k, v := range paused {
		if res.Detail == nil {
			res.Detail = map[string]any{}
		}
		res.Detail[k] = v
	}
	if !c.waitTarget(ctx, ActivateObserveTimeout, func(t contract.Target) bool { return t.Kind == "shell" }) {
		res = c.result(req, contract.OutcomeDelivered, res.Detail)
		res.Message = "Home was sent, but the shell was not observed in the foreground."
	}
	return res
}

func (c *Coordinator) adapterFor(appID string) (applicationsAdapter, bool) {
	a, ok := c.opts.Config.Current().Application(appID)
	if !ok {
		return nil, false
	}
	return c.opts.Adapters.ForName(a.Adapter)
}

type applicationsAdapter = applications.Adapter

func (c *Coordinator) setLaunch(appID, state string, errMsg string, inst *applications.Instance) {
	c.mu.Lock()
	rt := c.appLocked(appID)
	rt.launchState = state
	if errMsg != "" {
		rt.lastError = &errMsg
	} else if state == "running" {
		rt.lastError = nil
	}
	if inst != nil {
		rt.instance = inst
	}
	c.mu.Unlock()
	c.publish()
}

// doLaunch launches or activates a registered application.
func (c *Coordinator) doLaunch(ctx context.Context, s sender, req contract.ActionRequest) contract.ActionResult {
	appID, _ := req.Args["app_id"].(string)
	app, ok := c.opts.Config.Current().Application(appID)
	if !ok {
		return c.fail(req, contract.CodeInvalid, "That application is not registered.")
	}
	ad, ok := c.opts.Adapters.ForName(app.Adapter)
	webSpec, isWeb := adapters.WebOf(ad)
	if !ok || c.opts.Launcher == nil || (isWeb && c.opts.Web == nil) {
		return c.fail(req, contract.CodeUnsupported, app.Label+" cannot be launched on this installation.")
	}
	if !app.IsEnabled() {
		return c.fail(req, contract.CodeUnsupported, app.Label+" is turned off. Turn it on in Settings → Streaming sites.")
	}
	isApp := func(t contract.Target) bool { return t.Kind == "app" && strOr(t.AppID) == appID }
	c.stopUpdateFor("an app is starting") // never update during an app session
	c.stopDRMPrep(appID)                  // one Chromium per profile: the real run takes over

	// A second press while the app is still starting waits for that launch
	// instead of starting another instance. Checking and claiming "launching"
	// is one step so two quick presses cannot both start it.
	c.mu.Lock()
	rt0 := c.appLocked(appID)
	starting := rt0.launchState == "launching"
	prevState := rt0.launchState
	if !starting {
		rt0.launchState = "launching"
	}
	c.mu.Unlock()
	if starting {
		if c.waitTarget(ctx, LaunchObserveTimeout, isApp) {
			return c.result(req, contract.OutcomeObserved, map[string]any{"app_id": appID, "already_starting": true})
		}
		return c.fail(req, contract.CodeTimeout, app.Label+" is still starting.")
	}
	// A relaunch right after a close must not have its new window closed.
	c.stopCloseWatch(appID)

	// One app at a time on a TV: opening a different app asks the others to
	// close normally (they get a regular close request and can save state).
	c.closeOtherApps(ctx, appID)

	// Activate an existing window rather than starting a second instance.
	if wins, err := c.opts.Desktop.ListWindows(ctx); err == nil {
		for _, w := range wins {
			if !w.Mapped || !ad.MatchWindow(w) {
				continue
			}
			if err := c.opts.Desktop.Activate(ctx, w.ID); err != nil {
				c.setLaunch(appID, prevState, "", nil)
				return c.fail(req, contract.CodeTargetUnfocused, app.Label+" is running but could not be brought forward.")
			}
			c.setLaunch(appID, "running", "", nil)
			c.noteLaunched(app)
			c.finish(s, req, c.result(req, contract.OutcomeDelivered, map[string]any{"app_id": appID, "activated": true}))
			if c.waitTarget(ctx, ActivateObserveTimeout, isApp) {
				return c.result(req, contract.OutcomeObserved, map[string]any{"app_id": appID})
			}
			return c.fail(req, contract.CodeTimeout, app.Label+" did not come to the foreground.")
		}
	}

	c.mu.Lock()
	rt := c.appLocked(appID)
	notInstalled := rt.discovered && !rt.install.Installed
	c.mu.Unlock()
	if notInstalled {
		c.setLaunch(appID, prevState, "", nil)
		return c.fail(req, contract.CodeLaunchFailed, app.Label+" is not installed.")
	}
	c.setLaunch(appID, "launching", "", nil)
	var inst applications.Instance
	var err error
	if isWeb {
		inst, err = c.opts.Web.Launch(ctx, app, webSpec)
		if err != nil {
			c.log.Warn("session: web app did not start", "app", appID, "err", err)
		}
	} else {
		inst, err = c.opts.Launcher.Launch(ctx, app.Launch.AppID, app.Launch.Args)
	}
	if err != nil {
		c.setLaunch(appID, "failed", app.Label+" could not be started.", nil)
		return c.fail(req, contract.CodeLaunchFailed, app.Label+" could not be started.")
	}
	if !c.opts.Desktop.Capabilities()[platform.CapObserveForeground].Available {
		// A desktop that cannot see windows (GNOME or KDE on Wayland) can
		// only say the app was started: delivered, never observed, and not
		// a failure because no window was seen. Liveness then comes from
		// the launcher's instance list (reconcileApps).
		c.setLaunch(appID, "running", "", &inst)
		c.noteLaunched(app)
		res := c.result(req, contract.OutcomeDelivered, map[string]any{"app_id": appID, "observed": false})
		res.Message = app.Label + " was started; this desktop cannot confirm it came to the front."
		return res
	}
	c.setLaunch(appID, "launching", "", &inst)
	c.finish(s, req, c.result(req, contract.OutcomeDelivered, map[string]any{"app_id": appID}))
	if c.waitTarget(ctx, LaunchObserveTimeout, isApp) {
		c.setLaunch(appID, "running", "", nil)
		c.noteLaunched(app)
		return c.result(req, contract.OutcomeObserved, map[string]any{"app_id": appID})
	}
	c.setLaunch(appID, "failed", app.Label+" started but its window was not seen.", nil)
	return c.fail(req, contract.CodeTimeout, app.Label+" started but its window was not seen.")
}

// closeOtherApps sends a normal close request to every registered app other
// than keep that has a mapped window. Failures are logged, never fatal.
func (c *Coordinator) closeOtherApps(ctx context.Context, keep string) {
	for _, a := range c.opts.Config.Current().Applications {
		if a.ID == keep {
			continue
		}
		ad, ok := c.opts.Adapters.ForName(a.Adapter)
		if !ok {
			continue
		}
		if n, err := c.closeApp(ctx, a.ID, ad); err != nil {
			c.log.Warn("session: could not close app before opening another", "app", a.ID, "err", err)
		} else if n > 0 {
			c.log.Info("session: closing app before opening another", "app", a.ID, "opening", keep)
		}
	}
}

var errListWindows = errors.New("windows could not be listed")

// closeWatch is one app's close follow-through (see CloseFollowFor).
type closeWatch struct{ cancel context.CancelFunc }

// closeApp asks every mapped window of the app to close normally, then keeps
// closing windows the app opens in reply for CloseFollowFor. It returns how
// many windows were asked to close now; 0 means the app has none.
func (c *Coordinator) closeApp(ctx context.Context, appID string, ad applications.Adapter) (int, error) {
	asked := map[platform.WindowID]bool{}
	n, err := c.closeWindows(ctx, ad, asked)
	if n == 0 {
		return 0, err
	}
	follow, cancel := context.WithCancel(context.Background())
	w := &closeWatch{cancel: cancel}
	c.mu.Lock()
	if prev := c.closing[appID]; prev != nil {
		prev.cancel()
	}
	c.closing[appID] = w
	c.mu.Unlock()
	go func() {
		defer func() {
			cancel()
			c.mu.Lock()
			if c.closing[appID] == w {
				delete(c.closing, appID)
			}
			c.mu.Unlock()
		}()
		deadline := c.clock.Now().Add(CloseFollowFor)
		for c.clock.Now().Before(deadline) {
			select {
			case <-follow.Done():
				return
			case <-c.clock.After(closeFollowPoll):
			}
			if more, _ := c.closeWindows(follow, ad, asked); more > 0 {
				c.log.Info("session: closing a window the app opened while closing", "app", appID)
			}
		}
	}()
	return n, err
}

// closeWindows sends a normal close to each mapped window of ad not yet in
// asked, recording it there. It returns the number asked and the first error.
func (c *Coordinator) closeWindows(ctx context.Context, ad applications.Adapter, asked map[platform.WindowID]bool) (int, error) {
	wins, err := c.opts.Desktop.ListWindows(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errListWindows, err)
	}
	n := 0
	var first error
	for _, w := range wins {
		if !w.Mapped || asked[w.ID] || !ad.MatchWindow(w) {
			continue
		}
		asked[w.ID] = true
		if err := c.opts.Desktop.RequestClose(ctx, w.ID); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		n++
	}
	return n, first
}

// stopCloseWatch ends appID's close follow-through, if any.
func (c *Coordinator) stopCloseWatch(appID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if w := c.closing[appID]; w != nil {
		w.cancel()
		delete(c.closing, appID)
	}
}

func (c *Coordinator) doClose(ctx context.Context, req contract.ActionRequest) contract.ActionResult {
	appID, _ := req.Args["app_id"].(string)
	app, ok := c.opts.Config.Current().Application(appID)
	if !ok {
		return c.fail(req, contract.CodeInvalid, "That application is not registered.")
	}
	if boolArg(req.Args, "force") && c.isWebAdapter(app.Adapter) && c.opts.Web != nil && c.opts.Web.Running(appID) {
		// A web app's Chromium is the process Bear Den started: end it
		// (never `flatpak kill`, which could name another web app's
		// instance of the same Chromium).
		if err := c.opts.Web.Close(ctx, appID, true); err != nil {
			return c.fail(req, contract.CodeInternal, app.Label+" could not be stopped.")
		}
		c.setLaunch(appID, "exited", "", nil)
		return c.result(req, contract.OutcomeDelivered, map[string]any{"app_id": appID, "forced": true})
	}
	if boolArg(req.Args, "force") {
		c.mu.Lock()
		inst := c.appLocked(appID).instance
		c.mu.Unlock()
		if inst == nil || c.opts.Launcher == nil {
			return c.fail(req, contract.CodeNoTarget, "No tracked "+app.Label+" instance to stop.")
		}
		if err := c.opts.Launcher.Kill(ctx, *inst); err != nil {
			return c.fail(req, contract.CodeInternal, app.Label+" could not be stopped.")
		}
		c.mu.Lock()
		c.appLocked(appID).instance = nil
		c.mu.Unlock()
		c.setLaunch(appID, "exited", "", nil)
		return c.result(req, contract.OutcomeDelivered, map[string]any{"app_id": appID, "forced": true})
	}
	ad, ok := c.opts.Adapters.ForName(app.Adapter)
	if !ok {
		return c.fail(req, contract.CodeUnsupported, "No adapter is registered for "+app.Label+".")
	}
	n, err := c.closeApp(ctx, appID, ad)
	switch {
	case n > 0:
		return c.result(req, contract.OutcomeDelivered, map[string]any{"app_id": appID})
	case errors.Is(err, errListWindows):
		return c.fail(req, contract.CodeUnknownForeground, "Windows could not be listed.")
	case err != nil:
		return c.fail(req, contract.CodeInternal, app.Label+" could not be asked to close.")
	}
	return c.fail(req, contract.CodeNoTarget, app.Label+" is not running.")
}

// mediaMatchFor says which MPRIS players belong to appID
// (platform.MediaMatch): those whose owning process runs in the adapter's
// Flatpak, with the adapter's MPRIS name as the exact fallback for owners
// outside any Flatpak. A web app shares its browser's Flatpak with the other
// web apps, so its player must descend from the browser process Bear Den
// started for it; without one there is no match (fail closed).
func (c *Coordinator) mediaMatchFor(appID string) (platform.MediaMatch, bool) {
	ad, ok := c.adapterFor(appID)
	if !ok {
		return platform.MediaMatch{}, false
	}
	if _, isWeb := adapters.WebOf(ad); isWeb {
		if c.opts.Web == nil {
			return platform.MediaMatch{}, false
		}
		pid := c.opts.Web.PID(appID)
		if pid <= 1 {
			return platform.MediaMatch{}, false
		}
		return platform.MediaMatch{FlatpakID: ad.FlatpakID(), ProcessRoot: pid}, true
	}
	return platform.MediaMatch{FlatpakID: ad.FlatpakID(), Names: []string{ad.MediaMatch()}}, true
}

// probeMedia looks for an MPRIS player for the foreground application.
func (c *Coordinator) probeMedia(ctx context.Context) {
	_, t, _ := c.current()
	if t.Kind != "app" || c.opts.Media == nil {
		if t.Kind == "app" {
			c.mu.Lock()
			c.media = &mediaProbe{appID: strOr(t.AppID)}
			c.mu.Unlock()
			c.publish()
		}
		return
	}
	appID := strOr(t.AppID)
	probe := &mediaProbe{appID: appID, probedAt: c.clock.Now()}
	if m, ok := c.mediaMatchFor(appID); ok {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if p, found, err := c.opts.Media.Find(ctx, m); err == nil && found {
			probe.player = p
			probe.canCtl, _ = p.CanControl(ctx)
		}
	}
	c.mu.Lock()
	if _, cur, _ := c.currentLocked(); cur.Kind == "app" && strOr(cur.AppID) == appID {
		c.media = probe
	}
	c.mu.Unlock()
	c.kickNowPlaying() // read the new player (or drop the old one) for phones
	c.publish()
}

func (c *Coordinator) currentLocked() (int64, contract.Target, bool) {
	return c.epoch, c.target, c.locked
}

func (c *Coordinator) doMedia(ctx context.Context, req contract.ActionRequest, target contract.Target) contract.ActionResult {
	cp, code := c.capability(req.Action)
	if !cp.Available {
		return c.fail(req, code, cp.Reason)
	}
	if req.Target != "active" && req.Target != strOr(target.AppID) {
		return c.fail(req, contract.CodeTargetUnfocused, "That application is not in the foreground.")
	}
	if _, isWeb := c.webSpecFor(strOr(target.AppID)); isWeb && target.Kind == "app" {
		// The site's own shortcuts through the page, never MPRIS or
		// currentTime (Netflix errors on direct seeks).
		return c.routeWeb(ctx, req, target)
	}
	c.mu.Lock()
	m := c.media
	c.mu.Unlock()
	if m == nil || m.player == nil {
		return c.fail(req, contract.CodeUnsupported, "No verified media controls.")
	}
	var err error
	want := ""
	switch req.Action {
	case contract.ActionMediaPause:
		err, want = m.player.Pause(ctx), "Paused"
	case contract.ActionMediaPlay:
		err, want = m.player.Play(ctx), "Playing"
	case contract.ActionMediaSeek:
		err = m.player.SeekRelative(ctx, intArg(req.Args, "seconds"))
	}
	if err != nil {
		return c.fail(req, contract.CodeUnsupported, target.Label+" refused the media command.")
	}
	// The position and status changed: phones' Now playing re-reads them
	// (not every player signals a seek).
	defer c.kickNowPlaying()
	if want != "" {
		deadline := c.clock.Now().Add(MediaObserveTimeout)
		for c.clock.Now().Before(deadline) {
			if st, err := m.player.Status(ctx); err == nil && st == want {
				return c.result(req, contract.OutcomeObserved, map[string]any{"playback_status": st})
			}
			select {
			case <-ctx.Done():
				return c.result(req, contract.OutcomeDelivered, nil)
			case <-c.clock.After(100 * time.Millisecond):
			}
		}
	}
	return c.result(req, contract.OutcomeDelivered, nil)
}

func (c *Coordinator) doAudio(ctx context.Context, req contract.ActionRequest) contract.ActionResult {
	if c.cecSettings().TVVolume() {
		return c.doTVVolume(ctx, req) // cec.volume_target tv (cec.go)
	}
	cp, _ := c.capability(req.Action)
	if !cp.Available {
		return c.fail(req, contract.CodeUnsupported, cp.Reason)
	}
	var err error
	if req.Action == contract.ActionAudioMute {
		err = c.opts.Audio.SetMute(ctx, boolArg(req.Args, "muted"))
	} else {
		err = c.opts.Audio.VolumeDelta(ctx, intArg(req.Args, "delta"))
	}
	if err != nil {
		return c.fail(req, contract.CodeInternal, "PC volume could not be changed.")
	}
	return c.result(req, contract.OutcomeDelivered, nil)
}

// holdTap delivers one repetition of a held nav action with the lease's epoch.
func (c *Coordinator) holdTap(deviceID, action string, epoch int64) (string, error) {
	req := contract.ActionRequest{Protocol: 1, RequestID: "hold-" + randomID(), ContextEpoch: epoch, Target: "active", Action: action, Args: map[string]any{}}
	if cur, _, locked := c.current(); locked {
		return "locked", errors.New("locked")
	} else if cur != epoch {
		return "target_changed", errors.New("stale epoch")
	}
	_, target, _ := c.current()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res := c.routeInput(ctx, req, target)
	if res.Outcome == contract.OutcomeFailed {
		return string(res.Code), errors.New(res.Message)
	}
	return "", nil
}

func boolArg(args map[string]any, k string) bool {
	b, _ := args[k].(bool)
	return b
}

func intArg(args map[string]any, k string) int {
	switch v := args[k].(type) {
	case float64:
		return int(math.Round(v))
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}
