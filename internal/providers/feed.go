// Feed: fetches and caches home-screen sections from a ContentProvider.

package providers

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
)

// FeedOptions tunes a Feed. Zero values take the defaults documented on each
// field.
type FeedOptions struct {
	// Logger receives refresh outcomes; item titles are never logged. Nil
	// discards.
	Logger *slog.Logger
	// Clock drives Run's interval and section timestamps. Nil uses clock.Real.
	Clock clock.Clock
	// RefreshTimeout bounds one whole refresh (connect + every section +
	// artwork). Default 45 s.
	RefreshTimeout time.Duration
	// ArtworkWorkers bounds concurrent artwork downloads per refresh. Default 4.
	ArtworkWorkers int
}

// SectionState is the per-section outcome of the last refresh, for
// diagnostics; contract.Content only carries the merged status.
type SectionState struct {
	ID        string
	Status    string // ready | stale | error
	Message   string
	Items     int
	UpdatedAt time.Time // last successful fetch; zero when never
}

type sectionRun struct {
	cfg   SectionConfig
	items []contract.ContentItem
	err   error
}

type sectionMemo struct {
	items     []contract.ContentItem
	updatedAt time.Time
	lastErr   error
}

// Feed drives one ContentProvider for the configured sections. Refresh runs
// sections concurrently with independent failure states: a failing section
// keeps its last good items and the merged status becomes "stale"; only a
// refresh with no good data at all reports "error". Snapshot returns the last
// good contract.Content immediately and never touches the network.
type Feed struct {
	provider ContentProvider
	opts     FeedOptions
	logger   *slog.Logger
	clk      clock.Clock

	mu        sync.Mutex
	sections  []SectionConfig
	running   bool
	connected bool
	memo      map[string]*sectionMemo
	content   contract.Content
}

// NewFeed returns a Feed whose Snapshot reports "connecting" until the first
// Refresh completes. sections are the enabled provider-backed sections in
// layout order.
func NewFeed(provider ContentProvider, sections []SectionConfig, opts FeedOptions) *Feed {
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	if opts.Clock == nil {
		opts.Clock = clock.Real{}
	}
	if opts.RefreshTimeout <= 0 {
		opts.RefreshTimeout = 45 * time.Second
	}
	if opts.ArtworkWorkers <= 0 {
		opts.ArtworkWorkers = 4
	}
	f := &Feed{provider: provider, opts: opts, logger: opts.Logger, clk: opts.Clock, memo: map[string]*sectionMemo{}}
	f.sections = append([]SectionConfig(nil), sections...)
	f.content = contract.Content{Provider: provider.Name(), Status: StatusConnecting, Message: "", Sections: f.emptySections()}
	return f
}

// SetSections replaces the configured sections (layout change). Memoized
// items of sections that disappear are dropped on the next Refresh.
func (f *Feed) SetSections(sections []SectionConfig) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sections = append([]SectionConfig(nil), sections...)
}

// Snapshot returns the last published content. It never blocks on the
// provider or the network.
func (f *Feed) Snapshot() contract.Content {
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneContent(f.content)
}

// SectionStates reports the per-section outcome of the last refresh in
// configured order.
func (f *Feed) SectionStates() []SectionState {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]SectionState, 0, len(f.sections))
	for _, s := range f.sections {
		st := SectionState{ID: s.ID, Status: StatusError}
		if m, ok := f.memo[s.ID]; ok {
			st.Items = len(m.items)
			st.UpdatedAt = m.updatedAt
			switch {
			case m.lastErr == nil:
				st.Status = StatusReady
			case !m.updatedAt.IsZero():
				st.Status = StatusStale
				st.Message = m.lastErr.Error()
			default:
				st.Message = m.lastErr.Error()
			}
		}
		out = append(out, st)
	}
	return out
}

// Run refreshes immediately and then every interval until ctx is done. It is
// the session's refresh cadence; Refresh may also be called directly.
func (f *Feed) Run(ctx context.Context, interval time.Duration) {
	for {
		f.Refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-f.clk.After(interval):
		}
	}
}

// Refresh connects if needed, fetches every configured section concurrently,
// resolves artwork and open actions, and publishes a new snapshot. It returns
// false without doing anything when a refresh is already running; otherwise
// it blocks the caller until the refresh completes or RefreshTimeout expires.
func (f *Feed) Refresh(ctx context.Context) bool {
	f.mu.Lock()
	if f.running {
		f.mu.Unlock()
		return false
	}
	f.running = true
	sections := append([]SectionConfig(nil), f.sections...)
	connected := f.connected
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.running = false
		f.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(ctx, f.opts.RefreshTimeout)
	defer cancel()

	if !connected {
		if err := f.provider.Connect(ctx); err != nil {
			f.logger.Warn("content provider connect failed", "provider", f.provider.Name(), "error", err.Error())
			f.publish(sections, nil, err)
			return true
		}
		f.mu.Lock()
		f.connected = true
		f.mu.Unlock()
	}

	runs := make([]sectionRun, len(sections))
	var wg sync.WaitGroup
	for i, cfg := range sections {
		runs[i].cfg = cfg
		wg.Add(1)
		go func(i int, cfg SectionConfig) {
			defer wg.Done()
			items, _, err := f.provider.FetchItems(ctx, cfg.Kind, cfg, "")
			if err != nil {
				runs[i].err = err
				f.logger.Warn("content section fetch failed", "provider", f.provider.Name(), "section", cfg.ID, "error", err.Error())
				return
			}
			runs[i].items = f.decorate(ctx, items)
		}(i, cfg)
	}
	wg.Wait()
	f.publish(sections, runs, nil)
	return true
}

// decorate resolves artwork (bounded concurrency) and the open action for
// every item; artwork failures leave the item without artwork.
func (f *Feed) decorate(ctx context.Context, items []contract.ContentItem) []contract.ContentItem {
	out := make([]contract.ContentItem, len(items))
	sem := make(chan struct{}, f.opts.ArtworkWorkers)
	var wg sync.WaitGroup
	for i, it := range items {
		wg.Add(1)
		go func(i int, it contract.ContentItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			it.OpenAction = f.provider.ResolveOpenAction(ctx, it).Kind
			if it.OpenAction == "" {
				it.OpenAction = OpenApp
			}
			it.Artwork = nil
			if path, err := f.provider.ResolveArtwork(ctx, it); err != nil {
				f.logger.Debug("artwork unavailable", "provider", f.provider.Name(), "item", it.ID, "error", err.Error())
			} else if path != "" {
				it.Artwork = &path
			}
			if it.Progress != nil {
				p := *it.Progress
				if p < 0 || p > 1 || p != p {
					it.Progress = nil
				}
			}
			out[i] = it
		}(i, it)
	}
	wg.Wait()
	return out
}

// publish merges the run outcomes with memoized sections and stores the new
// snapshot. connectErr != nil means no section ran.
func (f *Feed) publish(sections []SectionConfig, runs []sectionRun, connectErr error) {
	now := f.clk.Now()
	f.mu.Lock()
	defer f.mu.Unlock()
	keep := map[string]bool{}
	for _, s := range sections {
		keep[s.ID] = true
	}
	for id := range f.memo {
		if !keep[id] {
			delete(f.memo, id)
		}
	}
	for _, r := range runs {
		m := f.memo[r.cfg.ID]
		if m == nil {
			m = &sectionMemo{}
			f.memo[r.cfg.ID] = m
		}
		if r.err != nil {
			m.lastErr = r.err
			continue
		}
		m.items = r.items
		m.updatedAt = now
		m.lastErr = nil
	}
	if connectErr != nil {
		for _, s := range sections {
			m := f.memo[s.ID]
			if m == nil {
				m = &sectionMemo{}
				f.memo[s.ID] = m
			}
			m.lastErr = connectErr
		}
	}

	content := contract.Content{Provider: f.provider.Name(), Sections: make([]contract.ContentSection, 0, len(sections))}
	failed, withData := 0, 0
	for _, s := range sections {
		m := f.memo[s.ID]
		items := []contract.ContentItem{}
		if m != nil {
			if m.lastErr != nil {
				failed++
			}
			if !m.updatedAt.IsZero() {
				withData++
				items = m.items
			}
		}
		content.Sections = append(content.Sections, contract.ContentSection{SectionID: s.ID, Items: append([]contract.ContentItem{}, items...)})
	}
	pstatus, pmessage := f.provider.Status()
	switch {
	case connectErr != nil && withData == 0:
		content.Status, content.Message = StatusError, pmessage
		if content.Message == "" {
			content.Message = connectErr.Error()
		}
	case connectErr != nil:
		content.Status, content.Message = StatusStale, pmessage
		if content.Message == "" {
			content.Message = connectErr.Error()
		}
	case failed == 0:
		content.Status, content.Message = StatusReady, ""
		if pstatus == StatusError || pstatus == StatusStale {
			content.Status, content.Message = pstatus, pmessage
		}
	case withData == 0:
		content.Status = StatusError
		content.Message = firstError(runs)
	default:
		content.Status = StatusStale
		content.Message = fmt.Sprintf("%d of %d rows could not be refreshed", failed, len(sections))
	}
	f.content = content
}

func firstError(runs []sectionRun) string {
	for _, r := range runs {
		if r.err != nil {
			return r.err.Error()
		}
	}
	return ""
}

func (f *Feed) emptySections() []contract.ContentSection {
	out := make([]contract.ContentSection, 0, len(f.sections))
	for _, s := range f.sections {
		out = append(out, contract.ContentSection{SectionID: s.ID, Items: []contract.ContentItem{}})
	}
	return out
}

func cloneContent(c contract.Content) contract.Content {
	out := c
	out.Sections = make([]contract.ContentSection, len(c.Sections))
	for i, s := range c.Sections {
		out.Sections[i] = contract.ContentSection{SectionID: s.SectionID, Items: append([]contract.ContentItem{}, s.Items...)}
	}
	return out
}
