// App removal through the coordinator (install.go startUninstall): the
// owner's phone and the shell remove the adapter's Flatpak for this user
// only; system-wide installs, apps that are not installed and running apps
// are refused in plain words; a removal is logged and the app discovered
// again so its tile hides; a family phone is refused (spec
// contracts/actions.md "App removal").

package session

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/install"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/shellipc"
)

// Removing and Uninstall complete fakeInstaller (install_test.go): a
// removal records its call, shows "removing", runs onUninstall (the test
// makes discovery answer "not installed") and clears the status, or
// fails with failRemove's words.
func (f *fakeInstaller) Removing() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.status {
		if s.State == contract.InstallRemoving {
			return true
		}
	}
	return false
}

func (f *fakeInstaller) Uninstall(_ context.Context, id string, deleteData bool) error {
	f.mu.Lock()
	f.removals = append(f.removals, removal{id, deleteData})
	fail, hook := f.failRemove, f.onUninstall
	f.mu.Unlock()
	f.set(id, install.Status{State: contract.InstallRemoving})
	if fail != "" {
		f.set(id, install.Status{Message: fail})
		return &install.Failure{Reason: fail}
	}
	if hook != nil {
		hook(id)
	}
	f.set(id, install.Status{})
	return nil
}

type removal struct {
	id         string
	deleteData bool
}

func (f *fakeInstaller) removed() []removal {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]removal(nil), f.removals...)
}

func TestOwnerRemovesTheAdaptersFlatpakForThisUserOnly(t *testing.T) {
	logs := &syncBuffer{}
	h, fi, sl := installHarness(t, func(o *Options) { o.Logger = slog.New(slog.NewTextHandler(logs, nil)) })
	fi.onUninstall = func(id string) { sl.setScope(id, "none") }

	// Removal is on offer, with what it frees.
	st := h.c.buildState(viewShell)
	if cp := st.Capabilities[contract.ActionAppUninstall]; !cp.Available {
		t.Fatalf("app.uninstall capability %+v", cp)
	}

	// A family phone may not remove anything.
	expectOutcome(t, h.submit(h.ctl, h.req(contract.ActionAppUninstall, map[string]any{"app_id": "youtube"})), contract.OutcomeFailed, contract.CodeForbidden)
	// System-wide: refused, with what to do instead.
	res := h.submit(h.owner, h.req(contract.ActionAppUninstall, map[string]any{"app_id": "plex-htpc"}))
	expectOutcome(t, res, contract.OutcomeFailed, contract.CodeUnsupported)
	if !strings.Contains(res.Message, "installed for everyone on this PC") {
		t.Fatalf("system-wide refusal %q", res.Message)
	}
	// Not installed.
	res = h.submit(h.owner, h.req(contract.ActionAppUninstall, map[string]any{"app_id": "moonlight"}))
	expectOutcome(t, res, contract.OutcomeFailed, contract.CodeUnsupported)
	if res.Message != "Moonlight is not installed." {
		t.Fatalf("not installed %q", res.Message)
	}
	expectOutcome(t, h.submit(h.owner, h.req(contract.ActionAppUninstall, map[string]any{"app_id": "nope"})), contract.OutcomeFailed, contract.CodeInvalid)
	if got := fi.removed(); len(got) != 0 {
		t.Fatalf("removed %v after refusals", got)
	}

	// YouTube, installed for this user, with its data.
	res = h.submit(h.owner, h.req(contract.ActionAppUninstall, map[string]any{"app_id": "youtube", "delete_data": true}))
	expectOutcome(t, res, contract.OutcomeDelivered, contract.CodeOK)
	h.eventually("youtube removed", func() bool {
		a := appState(h.c.buildState(viewShell), "youtube")
		return !a.Installed && a.Installation == "none"
	})
	if got := fi.removed(); len(got) != 1 || got[0] != (removal{adapters.VacuumTubeFlatpakID, true}) {
		t.Fatalf("removed %v", got)
	}
	h.eventually("logged", func() bool {
		return strings.Contains(logs.String(), "removed from this TV") && strings.Contains(logs.String(), "app=youtube")
	})
}

func TestRemoveRefusesARunningAppAndSaysWhyItFailed(t *testing.T) {
	logs := &syncBuffer{}
	h, _, _ := installHarness(t, func(o *Options) { o.Logger = slog.New(slog.NewTextHandler(logs, nil)) })
	res := h.submit(h.ctl, h.req(contract.ActionAppLaunch, map[string]any{"app_id": "youtube"}))
	if res.Outcome == contract.OutcomeFailed {
		t.Fatalf("launch %+v", res)
	}
	h.eventually("youtube running", func() bool { return appState(h.c.buildState(viewShell), "youtube").Running })
	res = h.submit(h.owner, h.req(contract.ActionAppUninstall, map[string]any{"app_id": "youtube"}))
	expectOutcome(t, res, contract.OutcomeFailed, contract.CodeBusy)
	if res.Message != "Close YouTube first." {
		t.Fatalf("running refusal %q", res.Message)
	}

	// A removal flatpak refuses: the app stays, with the reason, logged.
	h2, fi2, _ := installHarness(t, func(o *Options) { o.Logger = slog.New(slog.NewTextHandler(logs, nil)) })
	fi2.failRemove = install.ReasonNotHere
	if r := h2.shellSend(shellipc.AppUninstall{Type: shellipc.TypeAppUninstall, RequestID: "u1", AppID: "youtube"}, "u1"); !r.OK {
		t.Fatalf("app.uninstall refused: %+v", r)
	}
	h2.eventually("failure shown", func() bool {
		a := appState(h2.c.buildState(viewShell), "youtube")
		return a.Installed && a.Install != nil && a.Install.Message == install.ReasonNotHere
	})
	h2.eventually("logged", func() bool { return strings.Contains(logs.String(), "remove failed") })
	if got := fi2.removed(); len(got) != 1 || got[0].deleteData {
		t.Fatalf("removed %v", got)
	}
}
