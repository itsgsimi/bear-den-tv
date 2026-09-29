// Session tests for Plex: the plex.* IPC messages from the shell, state.plex
// in the shell view only (never phones, never while locked), the Plex rows as
// state.content once signed in and Home is in front, and sign-out. Uses the
// real internal/plexlink manager against the loopback plexfake.
package session

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/plexlink"
	"bear-den-tv/internal/providers"
	"bear-den-tv/internal/providers/plex"
	"bear-den-tv/internal/providers/plex/plexfake"
	"bear-den-tv/internal/secrets"
	"bear-den-tv/internal/shellipc"
)

func plexHarness(t *testing.T) (*harness, *plexfake.Fake) {
	t.Helper()
	f, err := plexfake.New(plexfake.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	var mgr *plexlink.Manager
	h := newHarness(t, func(o *Options) {
		mgr, err = plexlink.New(plexlink.Options{
			Config: o.Config, Secrets: secrets.NewMemory(), AccountURL: f.URL,
			ClientIdentifier: "0123456789abcdef0123456789abcdef", ArtworkDir: filepath.Join(t.TempDir(), "art"),
			PollInterval: 10 * time.Millisecond, ClientOptions: plex.ClientOptions{Retries: -1},
		})
		if err != nil {
			t.Fatal(err)
		}
		o.Plex = mgr
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { mgr.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return h, f
}

func (h *harness) plexSend(m shellipc.Message, id string) shellipc.Result {
	h.t.Helper()
	if err := h.shell.Send(m); err != nil {
		h.t.Fatal(err)
	}
	return h.shellResult(id)
}

func TestPlexSignInOverIPCAndRowsOnHome(t *testing.T) {
	h, f := plexHarness(t)
	ctx := context.Background()
	if p := h.c.buildState(viewShell).Plex; p == nil || p.Status != contract.PlexSignedOut {
		t.Fatalf("shell view plex = %+v", p)
	}
	if r := h.plexSend(shellipc.PlexSignIn{Type: shellipc.TypePlexSignIn, RequestID: "p1"}, "p1"); !r.OK {
		t.Fatalf("sign_in refused: %+v", r)
	}
	st := h.c.buildState(viewShell)
	if st.Plex == nil || st.Plex.Status != contract.PlexLinking || st.Plex.Code == nil {
		t.Fatalf("linking = %+v", st.Plex)
	}
	if len(st.Plex.QRModules) < 21 {
		t.Fatalf("no QR code of the link while linking: %d rows", len(st.Plex.QRModules))
	}
	code := *st.Plex.Code
	if _, err := contract.MarshalAndValidateState(st); err != nil {
		t.Fatalf("shell state invalid: %v", err)
	}
	// Phones (guest passes included) never see the flow or the code.
	guest := guestViewer(time.Now().Add(time.Hour))
	for name, v := range map[string]contract.State{
		"guest":      h.phones.Snapshot(ctx, &guest),
		"controller": h.phones.Snapshot(ctx, &h.ctl),
		"owner":      h.phones.Snapshot(ctx, &h.owner),
		"anonymous":  h.c.buildState(viewAnonymous),
	} {
		raw, _ := json.Marshal(v)
		if v.Plex != nil || strings.Contains(string(raw), code) {
			t.Fatalf("%s view carries the Plex flow: %s", name, raw)
		}
	}

	f.Link()
	h.eventually("choose_libraries", func() bool {
		p := h.c.buildState(viewShell).Plex
		return p != nil && p.Status == contract.PlexChooseLibraries
	})
	if r := h.plexSend(shellipc.PlexChooseLibraries{Type: shellipc.TypePlexChooseLibraries, RequestID: "p2", LibraryIDs: []string{"1", "2"}}, "p2"); !r.OK {
		t.Fatalf("choose_libraries refused: %+v", r)
	}
	if !h.c.opts.Config.Current().PlexContent.Enabled {
		t.Fatal("plex_content not enabled")
	}

	// Home in front: the rows fill.
	_ = h.shell.Send(shellipc.Focus{Type: shellipc.TypeFocus, Screen: "home"})
	h.eventually("Plex rows", func() bool {
		c := h.c.buildState(viewShell).Content
		return c != nil && c.Provider == "plex" && c.Status == providers.StatusReady
	})
	shell := h.c.buildState(viewShell)
	if len(shell.Content.Sections) != 2 || len(shell.Content.Sections[0].Items) != 4 {
		t.Fatalf("content = %+v", shell.Content)
	}
	if _, err := contract.MarshalAndValidateState(shell); err != nil {
		t.Fatalf("shell state with rows invalid: %v", err)
	}
	phone := h.phones.Snapshot(ctx, &h.ctl)
	if phone.Content == nil || phone.Plex != nil {
		t.Fatalf("phone: content %v plex %v; phones see rows but not the sign-in flow", phone.Content != nil, phone.Plex)
	}
	if _, err := contract.MarshalAndValidateState(phone); err != nil {
		t.Fatalf("phone state invalid: %v", err)
	}
	for _, q := range f.Requests() {
		if q.TokenInQuery {
			t.Fatalf("token in a query string: %+v", q)
		}
	}

	h.lock.set(true)
	h.eventually("locked", func() bool { return h.c.Target().Kind == "locked" })
	if st := h.c.buildState(viewShell); st.Plex != nil || st.Content != nil {
		t.Fatal("a locked session omits plex and content")
	}
	h.lock.set(false)
	h.eventually("unlocked", func() bool { return h.c.Target().Kind == "shell" })

	if r := h.plexSend(shellipc.PlexSignOut{Type: shellipc.TypePlexSignOut, RequestID: "p3"}, "p3"); !r.OK {
		t.Fatalf("sign_out refused: %+v", r)
	}
	st = h.c.buildState(viewShell)
	if st.Plex.Status != contract.PlexSignedOut || st.Content != nil {
		t.Fatalf("after sign-out: plex %+v content %+v", st.Plex, st.Content)
	}
}

func TestPlexRefusedWithoutAConnector(t *testing.T) {
	h := newHarness(t)
	if h.c.buildState(viewShell).Plex != nil {
		t.Fatal("state.plex without a connector")
	}
	r := h.plexSend(shellipc.PlexSignIn{Type: shellipc.TypePlexSignIn, RequestID: "n1"}, "n1")
	if r.OK || r.Error == "" {
		t.Fatalf("reply = %+v, want a refusal with a reason", r)
	}
}

func TestPlexRowsOnlyRefreshBehindHome(t *testing.T) {
	h, f := plexHarness(t)
	_ = h.shell.Send(shellipc.Focus{Type: shellipc.TypeFocus, Screen: "settings"})
	_ = h.plexSend(shellipc.PlexSignIn{Type: shellipc.TypePlexSignIn, RequestID: "s1"}, "s1")
	f.Link()
	h.eventually("choose_libraries", func() bool {
		p := h.c.buildState(viewShell).Plex
		return p != nil && p.Status == contract.PlexChooseLibraries
	})
	// An app comes to the front before sign-in finishes.
	res := h.submit(h.ctl, h.req("app.launch", map[string]any{"app_id": "plex-htpc"}))
	if res.Outcome == contract.OutcomeFailed {
		t.Fatalf("launch: %+v", res)
	}
	h.eventually("app in front", func() bool { return h.c.Target().Kind == "app" })
	_ = h.plexSend(shellipc.PlexChooseLibraries{Type: shellipc.TypePlexChooseLibraries, RequestID: "s2", LibraryIDs: []string{"1"}}, "s2")
	time.Sleep(200 * time.Millisecond)
	if n := f.Count("/hubs/continueWatching"); n != 0 {
		t.Fatalf("%d row fetches while an app was in front", n)
	}
	// Home again: the rows load.
	h.desk.SetActive(h.shellW)
	_ = h.shell.Send(shellipc.Focus{Type: shellipc.TypeFocus, Screen: "home"})
	h.eventually("rows after returning Home", func() bool { return f.Count("/hubs/continueWatching") >= 1 })
}
