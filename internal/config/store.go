// Store: loads, saves and falls back to last-known-good config.json (spec
// contracts/config.md).

package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
)

// File names inside the configuration directory.
const (
	FileName    = "config.json"
	LKGFileName = "config.last-known-good.json"
	HistoryDir  = "config.history"
)

// DefaultHistoryKeep is how many revisions config.history retains.
const DefaultHistoryKeep = 20

// DefaultConfirmTimeout is how long a risky layout change waits for confirmation.
const DefaultConfirmTimeout = 30 * time.Second

// Options configures a Store.
type Options struct {
	// Dir is the configuration directory ($XDG_CONFIG_HOME/bear-den-tv).
	Dir string
	// Clock drives the pending-change rollback timer.
	Clock clock.Clock
	// Rules supplies adapters and the interface lister for validation.
	Rules  Rules
	Logger *slog.Logger
	// ConfirmTimeout overrides DefaultConfirmTimeout (tests).
	ConfirmTimeout time.Duration
	// HistoryKeep overrides DefaultHistoryKeep.
	HistoryKeep int
	// OnChange is invoked (without the store lock held) after every revision
	// write and pending-state transition so the coordinator can republish.
	OnChange func()
}

// Source names where the running configuration came from.
type Source string

const (
	SourceConfig   Source = "config"
	SourceLKG      Source = "last-known-good"
	SourceDefaults Source = "defaults"
)

// LoadReport describes the recovery decision taken by Load.
type LoadReport struct {
	Source Source
	// Initialized is true when no config.json existed and defaults were written.
	Initialized bool
	// Problems lists why earlier sources in the recovery order were skipped.
	Problems []string
	// Notification is the persistent error shown when config.json was not usable.
	Notification *contract.Notification
	// RemoteBlocked is the host-rule failure keeping the LAN listener down, or "".
	RemoteBlocked string
}

// ErrRevisionConflict is returned when base_revision does not match the
// current revision (optimistic concurrency).
var ErrRevisionConflict = errors.New("revision conflict")

// ErrPendingChange is returned when a layout change is attempted while
// another change awaits confirmation.
var ErrPendingChange = errors.New("a layout change is awaiting confirmation")

// ErrNoUndo is returned by Undo when no previous revision is remembered.
var ErrNoUndo = errors.New("nothing to undo")

// ErrUnknownSection is returned by Reset for a section id without defaults.
var ErrUnknownSection = errors.New("unknown section")

// ErrNoPending is returned by Confirm/Cancel when no matching change is pending.
var ErrNoPending = errors.New("no pending layout change with that revision")

// Store is the single writer of config.json. Every method is safe for
// concurrent use.
type Store struct {
	opts Options
	mu   sync.Mutex
	cur  Config
	// undo is the configuration before the last layout change, one level deep.
	undo    *Config
	pending *pendingChange
	report  LoadReport
	loaded  bool
	// retiredMoved: rows moved off the retired browser while config.json
	// loaded, for UpgradeBrowsers to write (upgrade.go).
	retiredMoved []string
}

type pendingChange struct {
	revision         int64
	previousRevision int64
	previous         Config
	expiresAt        time.Time
	source           string
	timer            clock.Timer
}

// Open prepares a store for dir without reading anything; call Load next.
func Open(opts Options) (*Store, error) {
	if opts.Dir == "" {
		return nil, errors.New("config: directory required")
	}
	if opts.Clock == nil {
		opts.Clock = clock.Real{}
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.ConfirmTimeout <= 0 {
		opts.ConfirmTimeout = DefaultConfirmTimeout
	}
	if opts.HistoryKeep <= 0 {
		opts.HistoryKeep = DefaultHistoryKeep
	}
	if opts.Rules.Adapters == nil {
		opts.Rules.Adapters = DefaultAdapters
	}
	return &Store{opts: opts, cur: Defaults()}, nil
}

// Path returns the config.json path.
func (s *Store) Path() string { return filepath.Join(s.opts.Dir, FileName) }

// LKGPath returns the last-known-good path.
func (s *Store) LKGPath() string { return filepath.Join(s.opts.Dir, LKGFileName) }

func (s *Store) historyPath(rev int64) string {
	return filepath.Join(s.opts.Dir, HistoryDir, strconv.FormatInt(rev, 10)+".json")
}

// Load reads the configuration in recovery order config.json →
// config.last-known-good.json → built-in defaults. A missing config.json on
// first run is initialized from defaults. The last-known-good copy is only
// promoted after config.json passed structural and portable semantic checks.
func (s *Store) Load() (LoadReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rep := LoadReport{}
	raw, err := os.ReadFile(s.Path())
	switch {
	case err == nil:
		cfg, moved, perr := parseUpgrading(raw, s.opts.Rules)
		if perr == nil {
			s.cur = cfg
			s.retiredMoved = moved
			rep.Source = SourceConfig
			if lerr := s.promoteLKG(raw); lerr != nil {
				s.opts.Logger.Warn("config: last-known-good promotion failed", "error", lerr.Error())
			}
			s.finishLoad(&rep)
			return rep, nil
		}
		rep.Problems = append(rep.Problems, fmt.Sprintf("%s: %v", FileName, perr))
	case errors.Is(err, os.ErrNotExist):
		if _, lkgErr := os.Stat(s.LKGPath()); errors.Is(lkgErr, os.ErrNotExist) {
			s.cur = Defaults()
			rep.Source = SourceDefaults
			rep.Initialized = true
			if err := s.writeCurrentLocked(); err != nil {
				return rep, fmt.Errorf("config: initialize %s: %w", s.Path(), err)
			}
			s.finishLoad(&rep)
			return rep, nil
		}
		rep.Problems = append(rep.Problems, FileName+": missing")
	default:
		rep.Problems = append(rep.Problems, fmt.Sprintf("%s: %v", FileName, err))
	}

	lkgRaw, lerr := os.ReadFile(s.LKGPath())
	if lerr == nil {
		cfg, perr := Parse(lkgRaw, s.opts.Rules)
		if perr == nil {
			s.cur = cfg
			rep.Source = SourceLKG
			rep.Notification = &contract.Notification{
				ID:        "config-recovered",
				Kind:      "error",
				Text:      "config.json could not be loaded; running the last known good configuration.",
				CreatedMs: s.opts.Clock.Now().UnixMilli(),
			}
			s.finishLoad(&rep)
			return rep, nil
		}
		rep.Problems = append(rep.Problems, fmt.Sprintf("%s: %v", LKGFileName, perr))
	} else {
		rep.Problems = append(rep.Problems, fmt.Sprintf("%s: %v", LKGFileName, lerr))
	}
	s.cur = Defaults()
	rep.Source = SourceDefaults
	rep.Notification = &contract.Notification{
		ID:        "config-defaults",
		Kind:      "error",
		Text:      "No usable configuration was found; running built-in defaults. Fix or remove config.json.",
		CreatedMs: s.opts.Clock.Now().UnixMilli(),
	}
	s.finishLoad(&rep)
	return rep, nil
}

func (s *Store) finishLoad(rep *LoadReport) {
	if err := ValidateHost(s.cur, s.opts.Rules); err != nil {
		rep.RemoteBlocked = err.Error()
		s.opts.Logger.Warn("config: remote listener stays down", "reason", err.Error())
	}
	for _, p := range rep.Problems {
		s.opts.Logger.Error("config: recovery", "problem", p)
	}
	s.report = *rep
	s.loaded = true
}

// Report returns the last Load report.
func (s *Store) Report() LoadReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.report
}

// Current returns a copy of the running configuration.
func (s *Store) Current() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur.Clone()
}

// Revision returns the running revision.
func (s *Store) Revision() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur.Revision
}

// RemoteBlocked returns the host-rule failure keeping the listener down, or "".
func (s *Store) RemoteBlocked() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.report.RemoteBlocked
}

// Update applies mutate to a copy of the running configuration, assigns the
// next revision, validates the portable rules, and persists it. Host rules
// are re-evaluated for RemoteBlocked but do not reject the write; use
// UpdateRemote for changes to the remote section.
func (s *Store) Update(mutate func(*Config) error) (int64, error) {
	return s.update(mutate, false, true)
}

// UpdateRemote is Update with the host-bound rules enforced as well, for
// remote.configure and other network changes.
func (s *Store) UpdateRemote(mutate func(*Config) error) (int64, error) {
	return s.update(mutate, true, true)
}

func (s *Store) update(mutate func(*Config) error, host bool, rememberUndo bool) (int64, error) {
	s.mu.Lock()
	next := s.cur.Clone()
	if err := mutate(&next); err != nil {
		s.mu.Unlock()
		return 0, err
	}
	next.SchemaVersion = SchemaVersion
	next.Revision = s.cur.Revision + 1
	if err := ValidatePortable(next, s.opts.Rules); err != nil {
		s.mu.Unlock()
		return 0, err
	}
	if host {
		if err := ValidateHost(next, s.opts.Rules); err != nil {
			s.mu.Unlock()
			return 0, err
		}
	}
	prev := s.cur.Clone()
	s.cur = next
	if err := s.writeCurrentLocked(); err != nil {
		s.cur = prev
		s.mu.Unlock()
		return 0, err
	}
	if rememberUndo {
		s.undo = &prev
	}
	if err := ValidateHost(s.cur, s.opts.Rules); err != nil {
		s.report.RemoteBlocked = err.Error()
	} else {
		s.report.RemoteBlocked = ""
	}
	rev := s.cur.Revision
	s.mu.Unlock()
	s.notify()
	return rev, nil
}

// writeCurrentLocked persists s.cur: history entry, atomic config.json, then
// last-known-good promotion and history pruning.
func (s *Store) writeCurrentLocked() error {
	raw, err := Marshal(s.cur)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(s.opts.Dir, HistoryDir), 0o700); err != nil {
		return err
	}
	if err := writeFileAtomic(s.historyPath(s.cur.Revision), raw, 0o600); err != nil {
		return fmt.Errorf("history: %w", err)
	}
	if err := writeFileAtomic(s.Path(), raw, 0o600); err != nil {
		return err
	}
	if err := s.promoteLKG(raw); err != nil {
		return fmt.Errorf("last-known-good: %w", err)
	}
	s.pruneHistory()
	return nil
}

// promoteLKG copies validated bytes to the last-known-good file when they differ.
func (s *Store) promoteLKG(raw []byte) error {
	existing, err := os.ReadFile(s.LKGPath())
	if err == nil && string(existing) == string(raw) {
		return nil
	}
	return writeFileAtomic(s.LKGPath(), raw, 0o600)
}

func (s *Store) pruneHistory() {
	entries, err := os.ReadDir(filepath.Join(s.opts.Dir, HistoryDir))
	if err != nil {
		return
	}
	var revs []int64
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSuffix(name, ".json"), 10, 64)
		if err != nil {
			continue
		}
		revs = append(revs, n)
	}
	sort.Slice(revs, func(i, j int) bool { return revs[i] < revs[j] })
	for len(revs) > s.opts.HistoryKeep {
		_ = os.Remove(s.historyPath(revs[0]))
		revs = revs[1:]
	}
}

// HistoryRevisions lists the revisions retained under config.history, ascending.
func (s *Store) HistoryRevisions() []int64 {
	entries, err := os.ReadDir(filepath.Join(s.opts.Dir, HistoryDir))
	if err != nil {
		return nil
	}
	var revs []int64
	for _, e := range entries {
		n, err := strconv.ParseInt(strings.TrimSuffix(e.Name(), ".json"), 10, 64)
		if err == nil {
			revs = append(revs, n)
		}
	}
	sort.Slice(revs, func(i, j int) bool { return revs[i] < revs[j] })
	return revs
}

// LKGStatus reports whether a last-known-good copy exists and its revision.
func (s *Store) LKGStatus() (exists bool, revision int64) {
	raw, err := os.ReadFile(s.LKGPath())
	if err != nil {
		return false, 0
	}
	cfg, err := Parse(raw, s.opts.Rules)
	if err != nil {
		return true, 0
	}
	return true, cfg.Revision
}

func (s *Store) notify() {
	if s.opts.OnChange != nil {
		s.opts.OnChange()
	}
}

// writeFileAtomic writes data to a temp file in the same directory, fsyncs it,
// renames it over path, and fsyncs the directory so a crash leaves either the
// old or the new file, never a torn one.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
