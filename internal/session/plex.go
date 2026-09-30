// Plex in the session: state.plex for the shell, the Plex rows as
// state.content, the plex.* IPC messages (contracts/ipc.md) and telling the
// connector what is in front so rows refresh only behind Home
// (internal/plexlink).

package session

import (
	"context"
	"errors"
	"sync"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/pairing"
	"bear-den-tv/internal/shellipc"
)

// PlexLink is the Plex sign-in flow and rows (internal/plexlink.Manager).
type PlexLink interface {
	// State is state.plex (shell view only).
	State() contract.Plex
	// Content is the Plex rows, or nil while Plex is off.
	Content() *contract.Content
	// Observe reports what is in front on every publish.
	Observe(shellFront, home bool)
	SignIn(ctx context.Context) error
	Cancel(ctx context.Context) error
	ChooseServer(ctx context.Context, id string) error
	ChooseLibraries(ctx context.Context, ids []string) error
	SignOut(ctx context.Context) error
	// Change reopens the server ("server") or library ("libraries") choice.
	Change(ctx context.Context, what string) error
}

// plexState is state.plex for the shell view, or nil without a connector.
func (c *Coordinator) plexState() *contract.Plex {
	if c.opts.Plex == nil {
		return nil
	}
	st := c.opts.Plex.State()
	if st.LinkURL != nil {
		st.QRModules = linkQR(*st.LinkURL)
	}
	return &st
}

// linkQR encodes the (constant) link URL once; state builds reuse it.
var linkQRCache struct {
	sync.Mutex
	url     string
	modules []string
}

func linkQR(url string) []string {
	linkQRCache.Lock()
	defer linkQRCache.Unlock()
	if linkQRCache.url != url {
		mods, err := pairing.QRModules(url)
		if err != nil {
			return nil
		}
		linkQRCache.url, linkQRCache.modules = url, mods
	}
	return append([]string(nil), linkQRCache.modules...)
}

// plexContent is the Plex rows when no other feed (DEMO fixtures) is set.
func (c *Coordinator) plexContent() *contract.Content {
	if c.opts.Plex == nil {
		return nil
	}
	return c.opts.Plex.Content()
}

// observePlex tells the connector whether the shell, and Home in it, is in
// front of an unlocked session.
func (c *Coordinator) observePlex(st contract.State) {
	if c.opts.Plex == nil {
		return
	}
	front := st.Target.Kind == "shell" && !st.Session.Locked && st.Session.ShellConnected
	c.opts.Plex.Observe(front, front && st.Shell.Screen == "home")
}

// PlexChanged publishes a new state after the Plex flow or rows changed
// (plexlink Options.OnChange).
func (c *Coordinator) PlexChanged() { c.publish() }

// handlePlex answers one plex.* message. Sign-in talks to plex.tv and the
// chosen server (up to several seconds), so it runs off the client's read
// loop; the reply follows, then a new state.
func (h *ShellHandler) handlePlex(cl *shellipc.Client, m shellipc.Message) {
	c := h.c
	var requestID string
	var do func(context.Context, PlexLink) error
	switch msg := m.(type) {
	case shellipc.PlexSignIn:
		requestID, do = msg.RequestID, func(ctx context.Context, p PlexLink) error { return p.SignIn(ctx) }
	case shellipc.PlexCancel:
		requestID, do = msg.RequestID, func(ctx context.Context, p PlexLink) error { return p.Cancel(ctx) }
	case shellipc.PlexChooseServer:
		requestID, do = msg.RequestID, func(ctx context.Context, p PlexLink) error { return p.ChooseServer(ctx, msg.ServerID) }
	case shellipc.PlexChooseLibraries:
		requestID, do = msg.RequestID, func(ctx context.Context, p PlexLink) error { return p.ChooseLibraries(ctx, msg.LibraryIDs) }
	case shellipc.PlexSignOut:
		requestID, do = msg.RequestID, func(ctx context.Context, p PlexLink) error { return p.SignOut(ctx) }
	case shellipc.PlexChange:
		requestID, do = msg.RequestID, func(ctx context.Context, p PlexLink) error { return p.Change(ctx, msg.What) }
	default:
		return
	}
	if c.opts.Plex == nil {
		h.reply(cl, requestID, errors.New("Plex is not available in this session"), nil)
		return
	}
	go func() {
		err := do(context.Background(), c.opts.Plex)
		h.reply(cl, requestID, err, nil)
		c.publish()
	}()
}
