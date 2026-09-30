// state.applications[].notes (state.go, appNotes): the adapter table's notes
// reach the shell and every phone, and the unverified-browser note appears
// on the streaming sites only while that browser is the streaming browser.

package session

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/contract"
)

const braveNote = "Streaming in Brave is unverified: the sites may not play."

func TestAppNotesInEveryView(t *testing.T) {
	h := newHarness(t)
	shell := h.c.buildState(viewShell)
	for _, a := range shell.Applications {
		ad, ok := adapters.ForName(a.Adapter)
		if !ok {
			t.Fatalf("%s: unknown adapter %s", a.ID, a.Adapter)
		}
		if want := adapters.NotesOf(ad); len(want) == 0 || !reflect.DeepEqual(a.Notes, want) {
			t.Errorf("%s: notes %q, want the adapter's %q", a.ID, a.Notes, want)
		}
	}
	if _, err := contract.MarshalAndValidateState(shell); err != nil {
		t.Fatalf("shell state with notes is invalid: %v", err)
	}
	for name, st := range map[string]contract.State{
		"controller": h.phones.Snapshot(context.Background(), &h.ctl),
		"owner":      h.phones.Snapshot(context.Background(), &h.owner),
	} {
		for i, a := range st.Applications {
			if !reflect.DeepEqual(a.Notes, shell.Applications[i].Notes) {
				t.Errorf("%s view, %s: notes %q, the shell has %q", name, a.ID, a.Notes, shell.Applications[i].Notes)
			}
		}
		if _, err := contract.MarshalAndValidateState(st); err != nil {
			t.Fatalf("%s state with notes is invalid: %v", name, err)
		}
	}
}

func TestBraveNoteOnlyWhileBraveStreams(t *testing.T) {
	h := newHarness(t)
	hasBrave := func(id string) bool { return slices.Contains(appState(h.c.buildState(viewShell), id).Notes, braveNote) }
	for _, id := range []string{"netflix", "disney-plus", "hulu", "browser", "plex-htpc"} {
		if hasBrave(id) {
			t.Fatalf("%s carries the Brave note with Chromium chosen", id)
		}
	}
	if r := setBrowsers(h, "chromium", "brave"); !r.OK {
		t.Fatal(r.Error)
	}
	for _, id := range []string{"netflix", "disney-plus", "hulu"} {
		if !hasBrave(id) {
			t.Errorf("%s: no Brave note while Brave is the streaming browser: %q", id, appState(h.c.buildState(viewShell), id).Notes)
		}
	}
	for _, id := range []string{"browser", "plex-htpc", "youtube"} {
		if hasBrave(id) {
			t.Errorf("%s carries the streaming note", id)
		}
	}
	if !slices.Contains(appState(h.phones.Snapshot(context.Background(), &h.ctl), "netflix").Notes, braveNote) {
		t.Error("phones do not get the Brave note")
	}
	// The Browser tile in Brave is not streaming: no note anywhere.
	if r := setBrowsers(h, "brave", "chromium"); !r.OK {
		t.Fatal(r.Error)
	}
	for _, id := range []string{"netflix", "disney-plus", "hulu", "browser"} {
		if hasBrave(id) {
			t.Errorf("%s keeps the Brave note after the streaming sites moved back to Chromium", id)
		}
	}
}
