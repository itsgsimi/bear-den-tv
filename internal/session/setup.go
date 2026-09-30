// The TV's first-run setup and its "Start with this PC" toggle: IPC
// onboarding.complete (config onboarding.completed) and autostart.configure
// (the user's XDG autostart entry, internal/platform/autostart), and the
// shell-only state.onboarding and state.autostart they change
// (contracts/ipc.md, contracts/state.schema.json).

package session

import (
	"errors"

	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform/autostart"
)

// Autostart is where the user's autostart entry lives and how to find the
// start script it runs (cmd/bear-den-tv/session.go wires autostart.File()
// and autostart.StartScript; tests inject both).
type Autostart struct {
	// Path is the entry, e.g. ~/.config/autostart/bear-den-tv.desktop.
	Path string
	// Script finds start-session.sh; an error means autostart cannot be
	// turned on from here.
	Script func() (string, error)
}

// User-facing reasons (state.autostart.reason and the autostart.configure
// reply). Never a path: the start script's own error names this user's
// folders.
const (
	reasonAutostartSession = "Starting with this PC is not available in this session."
	reasonAutostartScript  = "Bear Den's start script was not found, so it can't be set to start with this PC here."
)

// autostartState is state.autostart: enabled when the entry exists,
// available when the session supports it and the start script is found.
func (c *Coordinator) autostartState() *contract.Autostart {
	a := c.opts.Autostart
	if a == nil {
		return &contract.Autostart{Reason: reasonAutostartSession}
	}
	st := &contract.Autostart{Enabled: autostart.Enabled(a.Path), Available: true}
	if _, err := a.Script(); err != nil {
		st.Available, st.Reason = false, reasonAutostartScript
	}
	return st
}

// completeOnboarding is onboarding.complete: config onboarding.completed
// becomes true (revision bump) and a new state follows. Calling it again is
// harmless.
func (c *Coordinator) completeOnboarding() error {
	if _, err := c.opts.Config.Update(func(cfg *config.Config) error {
		cfg.Onboarding.Completed = true
		return nil
	}); err != nil {
		return err
	}
	c.publish()
	return nil
}

// configureAutostart is autostart.configure: enabled writes the same entry
// as `bear-den-tv autostart enable`, false removes it (nothing there is
// fine). Turning it on fails closed, with nothing written, when the session
// has no autostart support or the start script is not found; turning it off
// needs only the session's support. A new state follows either way.
func (c *Coordinator) configureAutostart(enabled bool) error {
	a := c.opts.Autostart
	if a == nil {
		return errors.New(reasonAutostartSession)
	}
	defer c.publish()
	if !enabled {
		if err := autostart.Disable(a.Path); err != nil {
			c.log.Warn("session: autostart disable", "err", err)
			return errors.New("Bear Den could not remove its autostart entry.")
		}
		return nil
	}
	script, err := a.Script()
	if err != nil {
		c.log.Warn("session: autostart", "err", err)
		return errors.New(reasonAutostartScript)
	}
	if err := autostart.Enable(script, a.Path); err != nil {
		c.log.Warn("session: autostart enable", "err", err)
		return errors.New("Bear Den could not write its autostart entry.")
	}
	return nil
}
