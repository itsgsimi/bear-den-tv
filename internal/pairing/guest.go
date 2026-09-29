// Guest passes: when a pass ends (PassEnd) and how an ended pass is revoked
// (spec contracts/http.md#guest-passes, threat rows in docs/security.md).
//
// A guest device carries devices.expires_at_ms (Unix epoch ms, wall clock).
// It is revoked through the ordinary Revoke path, so its sessions are deleted
// and remote.Server closes its sockets with 4001, by whichever comes first:
//   - Authenticate, on every authenticated request, when the pass has ended;
//   - the pass timer on the injected clock, which fires at the earliest end
//     or after PassCheckInterval, whichever is sooner, so a suspended box or
//     a wall-clock change is caught within that interval of waking;
//   - New, for passes that ended while the coordinator was stopped.

package pairing

import (
	"context"
	"fmt"
	"time"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/storage"
)

// PassCheckInterval is the longest the pass timer sleeps between checks.
// Monotonic timers stop while the box is suspended; waking every few minutes
// re-reads the wall clock.
const PassCheckInterval = 5 * time.Minute

// TonightEndsAt is the local hour a "tonight" pass ends, the next morning.
const TonightEndsAt = 4

// PassEnd returns when a guest pass issued at now ends. tonight is 04:00 the
// next morning in loc (or 04:00 the same morning when issued between 00:00
// and 03:59), computed on the calendar so a daylight-saving change in between
// is honoured; 24h and 7d are fixed lengths.
func PassEnd(now time.Time, pass string, loc *time.Location) (time.Time, error) {
	switch pass {
	case contract.PassTonight:
		if loc == nil {
			loc = time.Local
		}
		t := now.In(loc)
		day := t.Day()
		if t.Hour() >= TonightEndsAt {
			day++
		}
		return time.Date(t.Year(), t.Month(), day, TonightEndsAt, 0, 0, 0, loc), nil
	case contract.Pass24h:
		return now.Add(24 * time.Hour), nil
	case contract.Pass7d:
		return now.Add(7 * 24 * time.Hour), nil
	}
	return time.Time{}, fmt.Errorf("pairing: unknown guest pass %q (use tonight, 24h or 7d)", pass)
}

// passEnded reports whether dev is a guest pass whose end has passed.
func (s *Service) passEnded(dev storage.Device) bool {
	return dev.ExpiresAtMs != nil && !s.opts.Clock.Now().Before(time.UnixMilli(*dev.ExpiresAtMs))
}

// revokeEnded revokes a device whose guest pass ended, through Revoke so the
// sessions, sockets and holds go exactly as for a revocation from the TV.
func (s *Service) revokeEnded(ctx context.Context, deviceID string) {
	if err := s.Revoke(ctx, deviceID); err != nil {
		s.opts.Logger.Debug("pairing: ended guest pass already revoked", "device_id", deviceID, "err", err)
		return
	}
	s.opts.Logger.Info("pairing: guest pass ended", "device_id", deviceID)
}

// sweepPasses revokes every ended guest pass and re-arms the pass timer for
// the next one (at most PassCheckInterval away); no timer while no guest is
// paired.
func (s *Service) sweepPasses() {
	ctx := context.Background()
	devs, err := s.opts.DB.ListDevices(ctx)
	if err != nil {
		s.opts.Logger.Warn("pairing: cannot read guest passes", "err", err)
	}
	now := s.opts.Clock.Now()
	wait := PassCheckInterval
	pending := err != nil // a failed read is retried after the interval
	for _, d := range devs {
		if d.ExpiresAtMs == nil {
			continue
		}
		if s.passEnded(d) {
			s.revokeEnded(ctx, d.ID)
			continue
		}
		pending = true
		if left := time.UnixMilli(*d.ExpiresAtMs).Sub(now); left < wait {
			wait = left
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.passTimer != nil {
		s.passTimer.Stop()
		s.passTimer = nil
	}
	if !pending {
		return
	}
	s.passTimer = s.opts.Clock.AfterFunc(wait, s.sweepPasses)
}
