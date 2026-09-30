// TV Settings → Edit layout from phones (IPC remote.layout_editing,
// setLayoutEditing in ipc.go): the owner's switch stores config
// remote.http_layout_editing, phones see it in state.remote, and it is off
// by default.

package session

import (
	"context"
	"testing"

	"bear-den-tv/internal/shellipc"
)

func TestLayoutEditingSwitchFromTheTV(t *testing.T) {
	h := newHarness(t)
	if h.c.opts.Config.Current().Remote.HTTPLayoutEditing {
		t.Fatal("layout editing over HTTP is on by default")
	}
	set := func(id string, on bool) {
		t.Helper()
		r := h.shellSend(shellipc.RemoteLayoutEditing{Type: shellipc.TypeRemoteLayoutEditing, RequestID: id, Enabled: on}, id)
		if !r.OK {
			t.Fatalf("remote.layout_editing %v: %+v", on, r)
		}
	}
	set("le-on", true)
	if !h.c.opts.Config.Current().Remote.HTTPLayoutEditing {
		t.Fatal("not stored")
	}
	if st := h.phones.Snapshot(context.Background(), &h.owner); !st.Remote.HTTPLayoutEditing {
		t.Fatal("phones do not see it")
	}
	set("le-off", false)
	if h.c.opts.Config.Current().Remote.HTTPLayoutEditing {
		t.Fatal("still on")
	}
}
