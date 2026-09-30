// The TV's bear tips: IPC tips.configure, tips.reset and tips.event
// (contracts/ipc.md) kept in config tips (contracts/config.md "Bear tips"),
// and the shell-only state.tips they change. The shell decides when a tip
// may show and which one (apps/tv-shell/qml/TipBear.qml); the coordinator
// only remembers what happened, on its injected clock: which tips were
// answered (each at most once), the local day of the last one shown (at
// most one a day) and the "Not now" answers in a row (three stop the tips
// until Show tips again). A tip shown and left unanswered (the owner moved
// on or left Home) uses the day but may come back another day.

package session

import (
	"errors"

	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/shellipc"
)

// tipsOf returns a copy of config tips to change (the default when absent).
func tipsOf(cfg *config.Config) *config.Tips {
	if cfg.Tips == nil {
		return &config.Tips{Enabled: true, Done: []string{}}
	}
	t := *cfg.Tips
	t.Done = append([]string{}, cfg.Tips.Done...)
	return &t
}

// configureTips is tips.configure: config tips.enabled.
func (c *Coordinator) configureTips(enabled bool) error {
	if _, err := c.opts.Config.Update(func(cfg *config.Config) error {
		t := tipsOf(cfg)
		t.Enabled = enabled
		cfg.Tips = t
		return nil
	}); err != nil {
		return err
	}
	c.publish()
	return nil
}

// resetTips is tips.reset (Show tips again): nothing shown, no "Not now",
// no day used, and the tips on.
func (c *Coordinator) resetTips() error {
	if _, err := c.opts.Config.Update(func(cfg *config.Config) error {
		cfg.Tips = &config.Tips{Enabled: true, Done: []string{}}
		return nil
	}); err != nil {
		return err
	}
	c.publish()
	return nil
}

// tipEvent is tips.event from the shell: seen uses today (the clock's
// local calendar day); ok ends a run of "Not now" and not_now adds one (at
// most config.TipsStopAfter); both answers mark the tip done. An unknown
// tip or event is refused.
func (c *Coordinator) tipEvent(tip, event string) error {
	if !contract.IsTipID(tip) {
		return errors.New("unknown tip")
	}
	switch event {
	case shellipc.TipEventSeen, shellipc.TipEventOK, shellipc.TipEventNotNow:
	default:
		return errors.New("unknown tip event")
	}
	day := c.clock.Now().Local().Format("2006-01-02")
	if _, err := c.opts.Config.Update(func(cfg *config.Config) error {
		t := tipsOf(cfg)
		done := false
		for _, d := range t.Done {
			done = done || d == tip
		}
		if !done && event != shellipc.TipEventSeen {
			t.Done = append(t.Done, tip)
		}
		switch event {
		case shellipc.TipEventSeen:
			t.LastDay = day
		case shellipc.TipEventOK:
			t.NotNowStreak = 0
		case shellipc.TipEventNotNow:
			t.NotNowStreak = min(config.TipsStopAfter, t.NotNowStreak+1)
		}
		cfg.Tips = t
		return nil
	}); err != nil {
		return err
	}
	c.publish()
	return nil
}

// receiveTips handles the tips.* IPC messages. tips.event comes from the
// shell only and has no reply.
func (h *ShellHandler) receiveTips(cl *shellipc.Client, m shellipc.Message, isShell bool) {
	c := h.c
	switch msg := m.(type) {
	case shellipc.TipsConfigure:
		h.reply(cl, msg.RequestID, c.configureTips(msg.Enabled), nil)
	case shellipc.TipsReset:
		h.reply(cl, msg.RequestID, c.resetTips(), nil)
	case shellipc.TipsEvent:
		if !isShell {
			return
		}
		if err := c.tipEvent(msg.Tip, msg.Event); err != nil {
			c.log.Warn("tips: ignoring a shell event", "tip", msg.Tip, "event", msg.Event, "err", err)
		}
	}
}
