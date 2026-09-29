// App installs through the coordinator (install.go): the owner's phone and
// the shell start and cancel installs of the adapter's Flatpak only,
// state.applications[].install reaches the shell and owner phones and no
// one else, a finished install is discovered again, and the idle update
// touches only this user's installs and only while the TV is idle (spec
// contracts/actions.md and http.md "App installs").

package session

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/install"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/shellipc"
)

// fakeInstaller records what the coordinator asked for; the test moves
// installs along with set.
type fakeInstaller struct {
	mu        sync.Mutex
	available bool
	reason    string
	status    map[string]install.Status
	starts    []string
	cancels   []string
	updates   [][]string
	updating  chan struct{} // closed by CancelUpdate
	onChange  func()
}

func newFakeInstaller() *fakeInstaller {
	return &fakeInstaller{available: true, status: map[string]install.Status{}}
}

func (f *fakeInstaller) Available() (bool, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.available, f.reason
}
func (f *fakeInstaller) Allowed(id string) bool {
	for _, a := range adapters.NewRegistry().All() {
		if a.FlatpakID() == id {
			return true
		}
	}
	return false
}
func (f *fakeInstaller) Status(id string) install.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status[id]
}
func (f *fakeInstaller) Info(_ context.Context, id string) (install.Sizes, error) {
	f.set(id, install.Status{SizeBytes: 417_600_000, DiskBytes: 2_218_600_000})
	return install.Sizes{Download: 417_600_000, Disk: 2_218_600_000}, nil
}
func (f *fakeInstaller) Start(id string) error {
	f.mu.Lock()
	for _, s := range f.status {
		if s.State == contract.InstallDownloading || s.State == contract.InstallPreparing {
			f.mu.Unlock()
			return install.ErrBusy
		}
	}
	f.starts = append(f.starts, id)
	f.status[id] = install.Status{State: contract.InstallPreparing, Phase: contract.PhaseChecking}
	f.mu.Unlock()
	return nil
}
func (f *fakeInstaller) Cancel(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status[id].State != contract.InstallDownloading && f.status[id].State != contract.InstallPreparing {
		return install.ErrNotRunning
	}
	f.cancels = append(f.cancels, id)
	f.status[id] = install.Status{State: contract.InstallAvailable, Message: install.ReasonCancelled}
	return nil
}
func (f *fakeInstaller) Busy() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updating != nil {
		return true
	}
	for _, s := range f.status {
		if s.State == contract.InstallDownloading || s.State == contract.InstallPreparing {
			return true
		}
	}
	return false
}
func (f *fakeInstaller) Updating() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.updating != nil
}
func (f *fakeInstaller) Update(ctx context.Context, ids []string) error {
	f.mu.Lock()
	f.updates = append(f.updates, append([]string(nil), ids...))
	ch := make(chan struct{})
	f.updating = ch
	f.mu.Unlock()
	select {
	case <-ch:
	case <-ctx.Done():
	}
	f.mu.Lock()
	f.updating = nil
	f.mu.Unlock()
	return context.Canceled
}
func (f *fakeInstaller) CancelUpdate() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updating == nil {
		return false
	}
	close(f.updating)
	f.updating = nil
	return true
}
func (f *fakeInstaller) set(id string, st install.Status) {
	f.mu.Lock()
	f.status[id] = st
	cb := f.onChange
	f.mu.Unlock()
	if cb != nil {
		cb()
	}
}
func (f *fakeInstaller) calls() (starts, cancels []string, updates [][]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.starts...), append([]string(nil), f.cancels...), append([][]string(nil), f.updates...)
}

// scopedLauncher answers discovery per Flatpak id (missing = not installed).
type scopedLauncher struct {
	*fakeLauncher
	mu     sync.Mutex
	scopes map[string]string
}

func (l *scopedLauncher) Discover(_ context.Context, id string) (applications.Installation, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch s := l.scopes[id]; s {
	case "user", "system":
		return applications.Installation{Installed: true, Version: "1.0", Scope: s}, nil
	}
	return applications.Installation{Scope: "none"}, nil
}
func (l *scopedLauncher) setScope(id, scope string) {
	l.mu.Lock()
	l.scopes[id] = scope
	l.mu.Unlock()
}

// installHarness: Plex system-wide, YouTube for the user, Moonlight and
// Chromium (the web apps) missing.
func installHarness(t *testing.T, configure ...func(*Options)) (*harness, *fakeInstaller, *scopedLauncher) {
	fi := newFakeInstaller()
	var sl *scopedLauncher
	h := newHarness(t, append([]func(*Options){func(o *Options) {
		sl = &scopedLauncher{fakeLauncher: o.Launcher.(*fakeLauncher), scopes: map[string]string{
			adapters.PlexHTPCFlatpakID: "system", adapters.VacuumTubeFlatpakID: "user",
		}}
		o.Launcher = sl
		o.Installer = fi
	}}, configure...)...)
	fi.onChange = h.c.InstallChanged
	h.eventually("discovery", func() bool {
		return appState(h.c.buildState(viewShell), "moonlight").Installation == "none"
	})
	return h, fi, sl
}

func TestOwnerInstallsTheAdaptersFlatpakOnly(t *testing.T) {
	h, fi, _ := installHarness(t)
	res := h.submit(h.owner, h.req(contract.ActionAppInstall, map[string]any{"app_id": "moonlight"}))
	expectOutcome(t, res, contract.OutcomeDelivered, contract.CodeOK)
	// A second press while it runs is not a second install.
	expectOutcome(t, h.submit(h.owner, h.req(contract.ActionAppInstall, map[string]any{"app_id": "moonlight"})), contract.OutcomeDelivered, contract.CodeOK)
	// A web app installs Chromium, the one browser web apps use; one at a time.
	fi.set(adapters.MoonlightFlatpakID, install.Status{State: contract.InstallDownloading, Phase: contract.PhaseRuntime, Progress: 42})
	expectOutcome(t, h.submit(h.owner, h.req(contract.ActionAppInstall, map[string]any{"app_id": "netflix"})), contract.OutcomeFailed, contract.CodeBusy)
	// Installed apps (for the user or system-wide) are left alone.
	res = h.submit(h.owner, h.req(contract.ActionAppInstall, map[string]any{"app_id": "plex-htpc"}))
	expectOutcome(t, res, contract.OutcomeObserved, contract.CodeOK)
	if res.Detail["already_installed"] != true {
		t.Fatalf("detail %v", res.Detail)
	}
	expectOutcome(t, h.submit(h.owner, h.req(contract.ActionAppInstall, map[string]any{"app_id": "nope"})), contract.OutcomeFailed, contract.CodeInvalid)
	// Cancel.
	expectOutcome(t, h.submit(h.owner, h.req(contract.ActionAppInstallCancel, map[string]any{"app_id": "moonlight"})), contract.OutcomeDelivered, contract.CodeOK)
	expectOutcome(t, h.submit(h.owner, h.req(contract.ActionAppInstallCancel, map[string]any{"app_id": "moonlight"})), contract.OutcomeFailed, contract.CodeUnsupported)
	fi.set(adapters.MoonlightFlatpakID, install.Status{})
	expectOutcome(t, h.submit(h.owner, h.req(contract.ActionAppInstall, map[string]any{"app_id": "browser"})), contract.OutcomeDelivered, contract.CodeOK)

	starts, cancels, _ := fi.calls()
	if fmt.Sprint(starts) != fmt.Sprint([]string{adapters.MoonlightFlatpakID, adapters.ChromiumFlatpakID}) || fmt.Sprint(cancels) != fmt.Sprint([]string{adapters.MoonlightFlatpakID}) {
		t.Fatalf("starts %v cancels %v", starts, cancels)
	}
}

func TestInstallStateOnlyForTheShellAndOwnerPhones(t *testing.T) {
	h, fi, _ := installHarness(t)
	fi.set(adapters.MoonlightFlatpakID, install.Status{State: contract.InstallDownloading, Phase: contract.PhaseApp, Progress: 63})
	ctx := context.Background()
	shell := h.c.buildState(viewShell)
	own := h.phones.Snapshot(ctx, &h.owner)
	for name, st := range map[string]contract.State{"shell": shell, "owner": own} {
		if st.Apps == nil || !st.Apps.AutoUpdate {
			t.Fatalf("%s: apps %+v", name, st.Apps)
		}
		ml := appState(st, "moonlight").Install
		if ml == nil || ml.State != contract.InstallDownloading || ml.Progress != 63 || ml.Phase != contract.PhaseApp {
			t.Fatalf("%s: moonlight install %+v", name, ml)
		}
		if p := appState(st, "plex-htpc").Install; p == nil || p.State != contract.InstallNone || p.Message != "Updated by your system" {
			t.Fatalf("%s: plex install %+v", name, p)
		}
		if n := appState(st, "netflix").Install; n == nil || n.State != contract.InstallAvailable {
			t.Fatalf("%s: netflix install %+v", name, n)
		}
		if _, err := contract.MarshalAndValidateState(st); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	guest := guestViewer(time.Now().Add(time.Hour))
	layout := h.ctl
	layout.Permissions = []contract.Permission{contract.PermController, contract.PermLayoutEditor}
	for name, st := range map[string]contract.State{
		"controller":    h.phones.Snapshot(ctx, &h.ctl),
		"layout editor": h.phones.Snapshot(ctx, &layout),
		"guest":         h.phones.Snapshot(ctx, &guest),
	} {
		if st.Apps != nil {
			t.Fatalf("%s got state.apps", name)
		}
		for _, a := range st.Applications {
			if a.Install != nil {
				t.Fatalf("%s got %s's install", name, a.ID)
			}
		}
		if _, err := contract.MarshalAndValidateState(st); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestFlatpakMissingIsSaidAndRefused(t *testing.T) {
	h, fi, _ := installHarness(t)
	fi.mu.Lock()
	fi.available, fi.reason = false, install.ReasonNoFlatpak
	fi.mu.Unlock()
	st := h.c.buildState(viewShell)
	if cp := st.Capabilities[contract.ActionAppInstall]; cp.Available || cp.Reason != install.ReasonNoFlatpak {
		t.Fatalf("capability %+v", cp)
	}
	if in := appState(st, "moonlight").Install; in.State != contract.InstallNone || in.Message != install.ReasonNoFlatpak {
		t.Fatalf("install %+v", in)
	}
	res := h.submit(h.owner, h.req(contract.ActionAppInstall, map[string]any{"app_id": "moonlight"}))
	expectOutcome(t, res, contract.OutcomeFailed, contract.CodeUnsupported)
	if res.Message != install.ReasonNoFlatpak {
		t.Fatalf("message %q", res.Message)
	}
	if starts, _, _ := fi.calls(); len(starts) != 0 {
		t.Fatalf("started %v", starts)
	}
}

func TestAFinishedInstallIsDiscoveredAgain(t *testing.T) {
	h, fi, sl := installHarness(t)
	if r := h.shellSend(shellipc.AppInstall{Type: shellipc.TypeAppInstall, RequestID: "i1", AppID: "moonlight"}, "i1"); !r.OK {
		t.Fatalf("app.install refused: %+v", r)
	}
	sl.setScope(adapters.MoonlightFlatpakID, "user")
	fi.set(adapters.MoonlightFlatpakID, install.Status{State: contract.InstallDone, Progress: 100})
	h.eventually("moonlight installed", func() bool {
		a := appState(h.c.buildState(viewShell), "moonlight")
		return a.Installed && a.Install.State == contract.InstallDone
	})
}

func (h *harness) shellSend(m shellipc.Message, requestID string) shellipc.Result {
	h.t.Helper()
	if err := h.shell.Send(m); err != nil {
		h.t.Fatal(err)
	}
	return h.shellResult(requestID)
}

func TestInstallIPC(t *testing.T) {
	h, fi, _ := installHarness(t)
	r := h.shellSend(shellipc.AppInstallInfo{Type: shellipc.TypeAppInstallInfo, RequestID: "s1", AppID: "netflix"}, "s1")
	data, _ := r.Data.(map[string]any)
	if !r.OK || data["size_bytes"] != float64(417_600_000) {
		t.Fatalf("app.install_info reply %+v", r)
	}
	if in := appState(h.c.buildState(viewShell), "hulu").Install; in.SizeBytes == nil || *in.SizeBytes != 417_600_000 {
		t.Fatalf("hulu (also Chromium) install %+v", in)
	}
	if r := h.shellSend(shellipc.InstallRequest{Type: shellipc.TypeInstallRequest, RequestID: "s2", AppID: "netflix"}, "s2"); !r.OK {
		t.Fatalf("applications.install_request refused: %+v", r)
	}
	fi.set(adapters.ChromiumFlatpakID, install.Status{State: contract.InstallDownloading, Phase: contract.PhaseRuntime})
	if r := h.shellSend(shellipc.AppInstallCancel{Type: shellipc.TypeAppInstallCancel, RequestID: "s3", AppID: "netflix"}, "s3"); !r.OK {
		t.Fatalf("app.install_cancel refused: %+v", r)
	}
	if r := h.shellSend(shellipc.AppsConfigure{Type: shellipc.TypeAppsConfigure, RequestID: "s4", AutoUpdate: false}, "s4"); !r.OK {
		t.Fatalf("apps.configure refused: %+v", r)
	}
	if h.c.opts.Config.Current().AutoUpdate() || h.c.buildState(viewShell).Apps.AutoUpdate {
		t.Fatal("auto_update still on")
	}
}

func TestIdleUpdateOnlyForUserInstallsAndOnlyWhenIdle(t *testing.T) {
	h, fi, _ := installHarness(t, func(o *Options) { o.UpdateCheck = time.Hour })
	ctx := context.Background()

	// Idle, Bear Den in front: only YouTube (this user's) is updated, never
	// Plex (system-wide) or anything not installed.
	if !h.c.maybeUpdate(ctx) {
		t.Fatal("no update while idle")
	}
	h.eventually("update running", fi.Updating)
	_, _, updates := fi.calls()
	if fmt.Sprint(updates) != fmt.Sprint([][]string{{adapters.VacuumTubeFlatpakID}}) {
		t.Fatalf("updated %v", updates)
	}
	// Once a day at most.
	if h.c.maybeUpdate(ctx) {
		t.Fatal("a second update the same day")
	}
	// An app starting ends it.
	res := h.submit(h.ctl, h.req(contract.ActionAppLaunch, map[string]any{"app_id": "youtube"}))
	if res.Outcome == contract.OutcomeFailed {
		t.Fatalf("launch %+v", res)
	}
	h.eventually("update cancelled", func() bool { return !fi.Updating() })
	h.eventually("youtube in front", func() bool { return h.c.Target().Kind == "app" })

	// Not idle: an app in front, or running behind Home.
	h.c.mu.Lock()
	h.c.upd.last = time.Time{}
	h.c.mu.Unlock()
	if h.c.maybeUpdate(ctx) {
		t.Fatal("updated with an app in front")
	}
	h.desk.SetActive(h.shellW)
	h.eventually("shell in front", func() bool { return h.c.Target().Kind == "shell" })
	if h.c.maybeUpdate(ctx) {
		t.Fatal("updated with an app running behind Home")
	}
	// Turned off.
	// YouTube closes: idle again.
	wins, _ := h.desk.ListWindows(ctx)
	for _, w := range wins {
		if w.ID != h.shellW {
			h.desk.RemoveWindow(w.ID)
		}
	}
	h.eventually("youtube exited", func() bool { return !appState(h.c.buildState(viewShell), "youtube").Running })
	if r := h.shellSend(shellipc.AppsConfigure{Type: shellipc.TypeAppsConfigure, RequestID: "u1", AutoUpdate: false}, "u1"); !r.OK {
		t.Fatal(r.Error)
	}
	if h.c.maybeUpdate(ctx) {
		t.Fatal("updated with auto_update off")
	}
	if r := h.shellSend(shellipc.AppsConfigure{Type: shellipc.TypeAppsConfigure, RequestID: "u2", AutoUpdate: true}, "u2"); !r.OK {
		t.Fatal(r.Error)
	}
	// Locked.
	h.lock.set(true)
	h.eventually("locked", func() bool { return h.c.Target().Kind == "locked" })
	if h.c.maybeUpdate(ctx) {
		t.Fatal("updated behind the lock screen")
	}
	h.lock.set(false)
	h.eventually("unlocked", func() bool { return h.c.Target().Kind == "shell" })
	if !h.c.maybeUpdate(ctx) {
		t.Fatal("no update once idle again")
	}
}
