// Layout apply with confirm/rollback for risky changes (spec
// contracts/config.md).

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"bear-den-tv/internal/contract"
)

// Risky-change thresholds from contracts/config.md.
const (
	riskyTextScale  = 1.6
	riskySafeMargin = 6
)

// ApplyResult is the outcome of ApplyLayout.
type ApplyResult struct {
	Revision int64
	// Pending is true when the change awaits confirmation and will roll back
	// automatically after the confirmation timeout.
	Pending bool
}

// IsRisky reports whether moving from cur to next needs a timed confirmation:
// a section was disabled leaving no enabled application section, text_scale
// exceeds 1.6, or safe_margin_percent exceeds 6.
func IsRisky(cur, next contract.Layout) bool {
	if next.UI.TextScale > riskyTextScale || next.UI.SafeMarginPercent > riskySafeMargin {
		return true
	}
	curEnabled, nextEnabled := enabledAppSections(cur), enabledAppSections(next)
	if nextEnabled == 0 && curEnabled > 0 {
		return true
	}
	return false
}

func enabledAppSections(l contract.Layout) int {
	n := 0
	for _, s := range l.Sections {
		if s.Kind == "applications" && s.Enabled {
			n++
		}
	}
	return n
}

// ApplyLayout validates layout, checks baseRevision against the running
// revision, writes the new revision, and starts the confirmation timer when
// the change is risky. source is "tv" or "web".
func (s *Store) ApplyLayout(baseRevision int64, layout contract.Layout, source string) (ApplyResult, error) {
	s.mu.Lock()
	if s.pending != nil {
		s.mu.Unlock()
		return ApplyResult{}, ErrPendingChange
	}
	if baseRevision != s.cur.Revision {
		cur := s.cur.Revision
		s.mu.Unlock()
		return ApplyResult{}, fmt.Errorf("%w: base %d, current %d", ErrRevisionConflict, baseRevision, cur)
	}
	if err := ValidateLayoutAgainst(s.cur, layout); err != nil {
		s.mu.Unlock()
		return ApplyResult{}, err
	}
	prev := s.cur.Clone()
	risky := IsRisky(prev.Layout(), layout)
	next := s.cur.Clone()
	next.SetLayout(layout)
	next.Revision = s.cur.Revision + 1
	s.cur = next
	if err := s.writeCurrentLocked(); err != nil {
		s.cur = prev
		s.mu.Unlock()
		return ApplyResult{}, err
	}
	s.undo = &prev
	res := ApplyResult{Revision: next.Revision, Pending: risky}
	if risky {
		p := &pendingChange{
			revision:         next.Revision,
			previousRevision: prev.Revision,
			previous:         prev,
			expiresAt:        s.opts.Clock.Now().Add(s.opts.ConfirmTimeout),
			source:           source,
		}
		rev := next.Revision
		p.timer = s.opts.Clock.AfterFunc(s.opts.ConfirmTimeout, func() { s.expirePending(rev) })
		s.pending = p
	}
	s.mu.Unlock()
	s.notify()
	return res, nil
}

func (s *Store) expirePending(revision int64) {
	if err := s.rollback(revision); err != nil && !errors.Is(err, ErrNoPending) {
		s.opts.Logger.Error("config: pending rollback failed", "error", err.Error())
	}
}

// rollback restores the layout from before the pending change as a new revision.
func (s *Store) rollback(revision int64) error {
	s.mu.Lock()
	p := s.pending
	if p == nil || p.revision != revision {
		s.mu.Unlock()
		return ErrNoPending
	}
	p.timer.Stop()
	s.pending = nil
	restored := s.cur.Clone()
	restored.SetLayout(p.previous.Layout())
	restored.Revision = s.cur.Revision + 1
	before := s.cur
	s.cur = restored
	if err := s.writeCurrentLocked(); err != nil {
		s.cur = before
		s.mu.Unlock()
		return err
	}
	s.undo = nil
	s.mu.Unlock()
	s.notify()
	return nil
}

// Confirm keeps the pending change with the given revision.
func (s *Store) Confirm(revision int64) error {
	s.mu.Lock()
	p := s.pending
	if p == nil || p.revision != revision {
		s.mu.Unlock()
		return ErrNoPending
	}
	p.timer.Stop()
	s.pending = nil
	s.mu.Unlock()
	s.notify()
	return nil
}

// Cancel reverts the pending change with the given revision immediately.
func (s *Store) Cancel(revision int64) error { return s.rollback(revision) }

// Pending describes the change awaiting confirmation, or nil.
func (s *Store) Pending() *contract.LayoutPending {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil {
		return nil
	}
	left := s.pending.expiresAt.Sub(s.opts.Clock.Now())
	if left < 0 {
		left = 0
	}
	return &contract.LayoutPending{
		Revision:         s.pending.revision,
		PreviousRevision: s.pending.previousRevision,
		ExpiresInS:       int(left / time.Second),
		Source:           s.pending.source,
	}
}

// Undo restores the layout of the revision before the last layout change and
// returns the new revision. One level deep; a second Undo redoes.
func (s *Store) Undo() (int64, error) {
	s.mu.Lock()
	if s.pending != nil {
		s.mu.Unlock()
		return 0, ErrPendingChange
	}
	if s.undo == nil {
		s.mu.Unlock()
		return 0, ErrNoUndo
	}
	target := s.undo.Layout()
	s.mu.Unlock()
	return s.update(func(c *Config) error { c.SetLayout(target); return nil }, false, true)
}

// Reset restores built-in defaults for one section id, or for the whole
// layout when sectionID is empty, and returns the new revision.
func (s *Store) Reset(sectionID string) (int64, error) {
	s.mu.Lock()
	if s.pending != nil {
		s.mu.Unlock()
		return 0, ErrPendingChange
	}
	s.mu.Unlock()
	defaults := Defaults().Layout()
	return s.update(func(c *Config) error {
		if sectionID == "" {
			c.SetLayout(defaults)
			return nil
		}
		var def *contract.Section
		for i := range defaults.Sections {
			if defaults.Sections[i].ID == sectionID {
				def = &defaults.Sections[i]
			}
		}
		if def == nil {
			return fmt.Errorf("%w: %s", ErrUnknownSection, sectionID)
		}
		replaced := false
		for i := range c.Sections {
			if c.Sections[i].ID == sectionID {
				c.Sections[i] = cloneSection(*def)
				replaced = true
			}
		}
		if !replaced {
			c.Sections = append(c.Sections, cloneSection(*def))
		}
		return nil
	}, false, true)
}

// Portable is the export document: the layout and nothing machine-local.
type Portable struct {
	Format        string          `json:"format"`
	SchemaVersion int             `json:"schema_version"`
	Layout        contract.Layout `json:"layout"`
}

// PortableFormat identifies export documents.
const PortableFormat = "bear-den-tv-layout"

// Export returns the portable subset (ui + sections). It never includes
// launch definitions, remote settings, devices, or credentials.
func (s *Store) Export() ([]byte, error) {
	cur := s.Current()
	raw, err := json.MarshalIndent(Portable{Format: PortableFormat, SchemaVersion: SchemaVersion, Layout: cur.Layout()}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// ImportPreview is what an import would change; nothing is written.
type ImportPreview struct {
	Layout contract.Layout `json:"layout"`
	// UnresolvedApplicationIDs lists referenced app ids not registered here.
	UnresolvedApplicationIDs []string `json:"unresolved_application_ids"`
	// Changes summarizes the differences from the running layout.
	Changes []string `json:"changes"`
	// Risky reports whether applying would require a timed confirmation.
	Risky bool `json:"risky"`
}

// ImportPreview parses an export document and reports what applying it would
// change and which application references cannot be resolved.
func (s *Store) ImportPreview(raw []byte) (ImportPreview, error) {
	if len(raw) > contract.MaxMessageBytes {
		return ImportPreview{}, errors.New("import document too large")
	}
	var doc Portable
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ImportPreview{}, fmt.Errorf("import: %w", err)
	}
	if doc.Format != PortableFormat || doc.SchemaVersion != SchemaVersion {
		return ImportPreview{}, fmt.Errorf("import: not a %s schema_version %d document", PortableFormat, SchemaVersion)
	}
	layoutRaw, err := json.Marshal(doc.Layout)
	if err != nil {
		return ImportPreview{}, err
	}
	layout, err := contract.ValidateLayout(layoutRaw)
	if err != nil {
		return ImportPreview{}, fmt.Errorf("import: %w", err)
	}
	cur := s.Current()
	known := map[string]bool{}
	for _, a := range cur.Applications {
		known[a.ID] = true
	}
	pv := ImportPreview{Layout: layout, UnresolvedApplicationIDs: []string{}, Changes: []string{}}
	seen := map[string]bool{}
	for _, sec := range layout.Sections {
		for _, id := range sec.ApplicationIDs {
			if !known[id] && !seen[id] {
				seen[id] = true
				pv.UnresolvedApplicationIDs = append(pv.UnresolvedApplicationIDs, id)
			}
		}
	}
	curLayout := cur.Layout()
	if curLayout.UI != layout.UI {
		pv.Changes = append(pv.Changes, "ui")
	}
	curSections := map[string]contract.Section{}
	for _, sec := range curLayout.Sections {
		curSections[sec.ID] = sec
	}
	for _, sec := range layout.Sections {
		old, ok := curSections[sec.ID]
		if !ok {
			pv.Changes = append(pv.Changes, "add section "+sec.ID)
			continue
		}
		a, _ := json.Marshal(old)
		b, _ := json.Marshal(sec)
		if string(a) != string(b) {
			pv.Changes = append(pv.Changes, "change section "+sec.ID)
		}
		delete(curSections, sec.ID)
	}
	for id := range curSections {
		pv.Changes = append(pv.Changes, "remove section "+id)
	}
	pv.Risky = IsRisky(curLayout, layout)
	return pv, nil
}
