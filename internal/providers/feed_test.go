// Tests for the section feed with a fake provider (feed.go).

package providers

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
)

type fakeProvider struct {
	mu         sync.Mutex
	connectErr error
	connects   int
	fetches    int
	fetch      func(ctx context.Context, cfg SectionConfig) ([]contract.ContentItem, error)
	artwork    func(item contract.ContentItem) (string, error)
	status     string
	message    string
}

func (p *fakeProvider) Name() string { return "fake" }

func (p *fakeProvider) Connect(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.connects++
	return p.connectErr
}

func (p *fakeProvider) ListSections(ctx context.Context) ([]SectionDescriptor, error) {
	return []SectionDescriptor{{Kind: KindContinueWatching}}, nil
}

func (p *fakeProvider) FetchItems(ctx context.Context, kind string, cfg SectionConfig, cursor string) ([]contract.ContentItem, string, error) {
	p.mu.Lock()
	p.fetches++
	fetch := p.fetch
	p.mu.Unlock()
	items, err := fetch(ctx, cfg)
	return items, "", err
}

func (p *fakeProvider) ResolveArtwork(ctx context.Context, item contract.ContentItem) (string, error) {
	if p.artwork == nil {
		return "", nil
	}
	return p.artwork(item)
}

func (p *fakeProvider) ResolveOpenAction(ctx context.Context, item contract.ContentItem) OpenAction {
	return OpenAction{Kind: OpenApp, AppID: "plex-htpc"}
}

func (p *fakeProvider) Status() (string, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.status == "" {
		return StatusReady, ""
	}
	return p.status, p.message
}

func item(id string) contract.ContentItem {
	return contract.ContentItem{ID: id, Title: "t", Subtitle: "s"}
}

var twoSections = []SectionConfig{
	{ID: "a", Kind: KindContinueWatching},
	{ID: "b", Kind: KindRecentlyAdded},
}

func TestFeedSnapshotBeforeRefresh(t *testing.T) {
	f := NewFeed(&fakeProvider{}, twoSections, FeedOptions{})
	c := f.Snapshot()
	if c.Status != StatusConnecting || c.Provider != "fake" || len(c.Sections) != 2 || c.Sections[1].SectionID != "b" || c.Sections[0].Items == nil {
		t.Fatalf("initial snapshot: %+v", c)
	}
}

func TestFeedIndependentSectionFailure(t *testing.T) {
	failB := false
	var mu sync.Mutex
	p := &fakeProvider{fetch: func(ctx context.Context, cfg SectionConfig) ([]contract.ContentItem, error) {
		mu.Lock()
		defer mu.Unlock()
		if cfg.ID == "b" && failB {
			return nil, errors.New("row b down")
		}
		return []contract.ContentItem{item(cfg.ID + ":1")}, nil
	}}
	f := NewFeed(p, twoSections, FeedOptions{})
	ctx := context.Background()
	if !f.Refresh(ctx) {
		t.Fatal("refresh must run")
	}
	c := f.Snapshot()
	if c.Status != StatusReady || c.Message != "" || len(c.Sections[0].Items) != 1 || len(c.Sections[1].Items) != 1 {
		t.Fatalf("after first refresh: %+v", c)
	}
	if c.Sections[0].Items[0].OpenAction != OpenApp {
		t.Fatalf("open action must come from the provider: %+v", c.Sections[0].Items[0])
	}

	mu.Lock()
	failB = true
	mu.Unlock()
	f.Refresh(ctx)
	c = f.Snapshot()
	if c.Status != StatusStale || !strings.Contains(c.Message, "1 of 2") {
		t.Fatalf("one failing section must be stale: %+v", c)
	}
	if len(c.Sections[1].Items) != 1 || c.Sections[1].Items[0].ID != "b:1" {
		t.Fatalf("failing section keeps its last good items: %+v", c.Sections[1])
	}
	states := f.SectionStates()
	if states[0].Status != StatusReady || states[1].Status != StatusStale || states[1].Message != "row b down" {
		t.Fatalf("section states: %+v", states)
	}
	if p.connects != 1 {
		t.Fatalf("connect once: %d", p.connects)
	}
}

func TestFeedAllSectionsFailWithoutDataIsError(t *testing.T) {
	p := &fakeProvider{fetch: func(ctx context.Context, cfg SectionConfig) ([]contract.ContentItem, error) {
		return nil, errors.New("server unreachable")
	}}
	f := NewFeed(p, twoSections, FeedOptions{})
	f.Refresh(context.Background())
	c := f.Snapshot()
	if c.Status != StatusError || c.Message != "server unreachable" || len(c.Sections) != 2 || len(c.Sections[0].Items) != 0 {
		t.Fatalf("all failing without data: %+v", c)
	}
	for _, s := range f.SectionStates() {
		if s.Status != StatusError || !s.UpdatedAt.IsZero() {
			t.Fatalf("section state: %+v", s)
		}
	}
}

func TestFeedConnectFailureThenRecovery(t *testing.T) {
	p := &fakeProvider{connectErr: errors.New("keyring locked"), status: StatusError, message: "Keyring is locked.",
		fetch: func(ctx context.Context, cfg SectionConfig) ([]contract.ContentItem, error) {
			return []contract.ContentItem{item(cfg.ID)}, nil
		}}
	f := NewFeed(p, twoSections, FeedOptions{})
	ctx := context.Background()
	f.Refresh(ctx)
	c := f.Snapshot()
	if c.Status != StatusError || c.Message != "Keyring is locked." || p.fetches != 0 {
		t.Fatalf("connect failure: %+v fetches=%d", c, p.fetches)
	}
	p.mu.Lock()
	p.connectErr = nil
	p.status = ""
	p.mu.Unlock()
	f.Refresh(ctx)
	if c := f.Snapshot(); c.Status != StatusReady || len(c.Sections[0].Items) != 1 {
		t.Fatalf("recovery: %+v", c)
	}
	if p.connects != 2 {
		t.Fatalf("connect retried once after failure: %d", p.connects)
	}
	f.Refresh(ctx)
	if p.connects != 2 {
		t.Fatalf("connected feed must not reconnect: %d", p.connects)
	}
}

func TestFeedSnapshotNeverBlocksOnFetch(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	p := &fakeProvider{fetch: func(ctx context.Context, cfg SectionConfig) ([]contract.ContentItem, error) {
		started <- struct{}{}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return []contract.ContentItem{item(cfg.ID)}, nil
		}
	}}
	f := NewFeed(p, twoSections, FeedOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan bool, 1)
	go func() { done <- f.Refresh(ctx) }()
	<-started
	<-started

	snap := make(chan contract.Content, 1)
	go func() { snap <- f.Snapshot() }()
	select {
	case c := <-snap:
		if c.Status != StatusConnecting {
			t.Fatalf("snapshot during a stalled refresh is the previous content: %+v", c)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Snapshot blocked on a stalled fetch")
	}
	if f.Refresh(ctx) {
		t.Fatal("a second Refresh while one runs must return false")
	}
	cancel()
	select {
	case ran := <-done:
		if !ran {
			t.Fatal("first refresh must report that it ran")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Refresh did not finish after cancel")
	}
	if c := f.Snapshot(); c.Status != StatusError {
		t.Fatalf("cancelled first refresh with no data: %+v", c)
	}
}

func TestFeedDecorateArtworkAndProgress(t *testing.T) {
	bad := 1.5
	good := 0.25
	p := &fakeProvider{
		fetch: func(ctx context.Context, cfg SectionConfig) ([]contract.ContentItem, error) {
			a := item("a")
			a.Progress = &bad
			b := item("b")
			b.Progress = &good
			c := item("c")
			return []contract.ContentItem{a, b, c}, nil
		},
		artwork: func(it contract.ContentItem) (string, error) {
			switch it.ID {
			case "a":
				return "/cache/a.jpg", nil
			case "b":
				return "", errors.New("timeout")
			}
			return "", nil
		},
	}
	f := NewFeed(p, twoSections[:1], FeedOptions{ArtworkWorkers: 1})
	f.Refresh(context.Background())
	items := f.Snapshot().Sections[0].Items
	if items[0].Artwork == nil || *items[0].Artwork != "/cache/a.jpg" || items[0].Progress != nil {
		t.Fatalf("item a: %+v", items[0])
	}
	if items[1].Artwork != nil || items[1].Progress == nil || *items[1].Progress != 0.25 {
		t.Fatalf("item b: %+v", items[1])
	}
	if items[2].Artwork != nil || items[2].OpenAction != OpenApp {
		t.Fatalf("item c: %+v", items[2])
	}
}

func TestFeedSetSectionsDropsStaleMemo(t *testing.T) {
	p := &fakeProvider{fetch: func(ctx context.Context, cfg SectionConfig) ([]contract.ContentItem, error) {
		return []contract.ContentItem{item(cfg.ID)}, nil
	}}
	f := NewFeed(p, twoSections, FeedOptions{})
	f.Refresh(context.Background())
	f.SetSections(twoSections[1:])
	f.Refresh(context.Background())
	c := f.Snapshot()
	if len(c.Sections) != 1 || c.Sections[0].SectionID != "b" || len(f.SectionStates()) != 1 {
		t.Fatalf("sections after SetSections: %+v", c)
	}
	f.mu.Lock()
	_, kept := f.memo["a"]
	f.mu.Unlock()
	if kept {
		t.Fatal("memo of a removed section must be dropped")
	}
}

func TestFeedRunUsesClock(t *testing.T) {
	fetched := make(chan struct{}, 10)
	p := &fakeProvider{fetch: func(ctx context.Context, cfg SectionConfig) ([]contract.ContentItem, error) {
		fetched <- struct{}{}
		return nil, nil
	}}
	clk := clock.NewFake(time.Unix(1700000000, 0))
	f := NewFeed(p, twoSections[:1], FeedOptions{Clock: clk})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go f.Run(ctx, time.Minute)
	<-fetched
	deadline := time.Now().Add(5 * time.Second)
	for clk.Pending() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	clk.Advance(time.Minute)
	select {
	case <-fetched:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not refresh after the interval")
	}
	cancel()
}
