// ArtworkCache: a size-bounded on-disk cache of Plex artwork, per scope.

package plex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // registered so DecodeConfig accepts GIF artwork
	_ "image/jpeg" // registered so DecodeConfig accepts JPEG artwork
	_ "image/png"  // registered so DecodeConfig accepts PNG artwork
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"bear-den-tv/internal/clock"
)

// Artwork bounds.
const (
	// DefaultArtworkBudgetMiB is config cache.artwork_max_mib's default.
	DefaultArtworkBudgetMiB = 512
	// MaxArtworkDimension bounds decoded width and height (spec §9.4).
	MaxArtworkDimension = 4096
	// MaxArtworkFileBytes bounds one artwork download.
	MaxArtworkFileBytes = 8 << 20
	indexFile           = "index.json"
)

var (
	// ErrArtworkTooLarge reports a download above MaxArtworkFileBytes or an
	// image above MaxArtworkDimension in either dimension.
	ErrArtworkTooLarge = errors.New("plex: artwork exceeds size bounds")
	// ErrArtworkUndecodable reports bytes image.DecodeConfig cannot read.
	ErrArtworkUndecodable = errors.New("plex: artwork is not a decodable image")
)

var hexName = regexp.MustCompile(`^[a-f0-9]{16,64}$`)

// Fetcher produces the artwork bytes for a cache miss: content type and body.
type Fetcher func(ctx context.Context) (contentType string, body io.ReadCloser, err error)

type artworkEntry struct {
	Size       int64  `json:"size"`
	AccessedAt int64  `json:"accessed_at"` // unix milliseconds
	File       string `json:"file"`
}

type artworkIndex struct {
	Owner   string                   `json:"owner"`
	Entries map[string]*artworkEntry `json:"entries"`
}

// ArtworkCache stores fetched artwork under root/<scope>/ with a global byte
// budget enforced by least-recently-used eviction. Access times live in a
// per-scope index.json so the LRU order survives restarts. Every accepted
// file passed the content-type, byte, and image.DecodeConfig dimension
// checks. Safe for concurrent use; concurrent misses for one key fetch once.
type ArtworkCache struct {
	root     string
	maxBytes int64
	clk      clock.Clock

	mu       sync.Mutex
	scopes   map[string]*artworkIndex
	inflight map[string]chan struct{}
}

// NewArtworkCache opens (or creates) the cache at root, typically
// $XDG_CACHE_HOME/bear-den-tv/artwork, loading every scope index it finds.
// maxBytes <= 0 uses DefaultArtworkBudgetMiB; clk nil uses clock.Real.
func NewArtworkCache(root string, maxBytes int64, clk clock.Clock) (*ArtworkCache, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultArtworkBudgetMiB << 20
	}
	if clk == nil {
		clk = clock.Real{}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	c := &ArtworkCache{root: root, maxBytes: maxBytes, clk: clk, scopes: map[string]*artworkIndex{}, inflight: map[string]chan struct{}{}}
	dirs, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, d := range dirs {
		if !d.IsDir() || !hexName.MatchString(d.Name()) {
			continue
		}
		idx, err := c.loadIndex(d.Name())
		if err != nil {
			continue
		}
		c.scopes[d.Name()] = idx
	}
	return c, nil
}

func (c *ArtworkCache) loadIndex(scope string) (*artworkIndex, error) {
	data, err := os.ReadFile(filepath.Join(c.root, scope, indexFile))
	if err != nil {
		return nil, err
	}
	var idx artworkIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	if idx.Entries == nil {
		idx.Entries = map[string]*artworkEntry{}
	}
	for key, e := range idx.Entries {
		if _, err := os.Stat(filepath.Join(c.root, scope, e.File)); err != nil {
			delete(idx.Entries, key)
		}
	}
	return &idx, nil
}

func (c *ArtworkCache) saveIndex(scope string) error {
	idx := c.scopes[scope]
	if idx == nil {
		return nil
	}
	data, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	dir := filepath.Join(c.root, scope)
	tmp := filepath.Join(dir, indexFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, indexFile))
}

// Usage is the total bytes of accepted artwork across scopes.
func (c *ArtworkCache) Usage() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.usageLocked()
}

func (c *ArtworkCache) usageLocked() int64 {
	var total int64
	for _, idx := range c.scopes {
		for _, e := range idx.Entries {
			total += e.Size
		}
	}
	return total
}

// Get returns the local path of the artwork for key within scope, fetching
// it once on a miss. owner tags the scope for PurgeOwner. scope and key are
// hex digests (ScopeKey; sha256 of the thumb path and size).
func (c *ArtworkCache) Get(ctx context.Context, scope, owner, key string, fetch Fetcher) (string, error) {
	if !hexName.MatchString(scope) || !hexName.MatchString(key) {
		return "", errors.New("plex: artwork scope and key must be hex digests")
	}
	for attempt := 0; attempt < 2; attempt++ {
		c.mu.Lock()
		if idx := c.scopes[scope]; idx != nil {
			if e := idx.Entries[key]; e != nil {
				path := filepath.Join(c.root, scope, e.File)
				if _, err := os.Stat(path); err == nil {
					e.AccessedAt = c.clk.Now().UnixMilli()
					_ = c.saveIndex(scope)
					c.mu.Unlock()
					return path, nil
				}
				delete(idx.Entries, key)
			}
		}
		flightKey := scope + "/" + key
		if wait, busy := c.inflight[flightKey]; busy {
			c.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		done := make(chan struct{})
		c.inflight[flightKey] = done
		c.mu.Unlock()

		path, err := c.fill(ctx, scope, owner, key, fetch)
		c.mu.Lock()
		delete(c.inflight, flightKey)
		close(done)
		c.mu.Unlock()
		return path, err
	}
	return "", errors.New("plex: artwork fetch did not settle")
}

func extensionFor(format string) string {
	switch format {
	case "jpeg":
		return "jpg"
	default:
		return format
	}
}

// fill downloads, validates, and stores one artwork file, then evicts to
// the budget.
func (c *ArtworkCache) fill(ctx context.Context, scope, owner, key string, fetch Fetcher) (string, error) {
	contentType, body, err := fetch(ctx)
	if err != nil {
		return "", err
	}
	defer body.Close()
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch ct {
	case "image/jpeg", "image/png", "image/gif":
	default:
		return "", fmt.Errorf("%w: content type %q", ErrNotImage, ct)
	}
	dir := filepath.Join(c.root, scope)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	cleanup := func() { tmp.Close(); os.Remove(tmpName) }
	n, err := io.Copy(tmp, io.LimitReader(body, MaxArtworkFileBytes+1))
	if err != nil {
		cleanup()
		return "", fmt.Errorf("plex: artwork download: %w", err)
	}
	if n > MaxArtworkFileBytes {
		cleanup()
		return "", fmt.Errorf("%w: more than %d bytes", ErrArtworkTooLarge, MaxArtworkFileBytes)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return "", err
	}
	cfg, format, err := image.DecodeConfig(tmp)
	if err != nil {
		cleanup()
		return "", fmt.Errorf("%w: %v", ErrArtworkUndecodable, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxArtworkDimension || cfg.Height > MaxArtworkDimension {
		cleanup()
		return "", fmt.Errorf("%w: %dx%d", ErrArtworkTooLarge, cfg.Width, cfg.Height)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	file := key + "." + extensionFor(format)
	final := filepath.Join(dir, file)
	if err := os.Rename(tmpName, final); err != nil {
		os.Remove(tmpName)
		return "", err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.scopes[scope]
	if idx == nil {
		idx = &artworkIndex{Owner: owner, Entries: map[string]*artworkEntry{}}
		c.scopes[scope] = idx
	}
	idx.Owner = owner
	if old := idx.Entries[key]; old != nil && old.File != file {
		os.Remove(filepath.Join(dir, old.File))
	}
	idx.Entries[key] = &artworkEntry{Size: n, AccessedAt: c.clk.Now().UnixMilli(), File: file}
	touched := map[string]bool{scope: true}
	c.evictLocked(scope, key, touched)
	for s := range touched {
		_ = c.saveIndex(s)
	}
	return final, nil
}

// evictLocked removes least-recently-used entries until usage fits the
// budget, never evicting the entry just stored.
func (c *ArtworkCache) evictLocked(keepScope, keepKey string, touched map[string]bool) {
	for c.usageLocked() > c.maxBytes {
		var oldestScope, oldestKey string
		var oldest *artworkEntry
		for scope, idx := range c.scopes {
			for key, e := range idx.Entries {
				if scope == keepScope && key == keepKey {
					continue
				}
				if oldest == nil || e.AccessedAt < oldest.AccessedAt || (e.AccessedAt == oldest.AccessedAt && scope+key < oldestScope+oldestKey) {
					oldest, oldestScope, oldestKey = e, scope, key
				}
			}
		}
		if oldest == nil {
			return
		}
		os.Remove(filepath.Join(c.root, oldestScope, oldest.File))
		delete(c.scopes[oldestScope].Entries, oldestKey)
		touched[oldestScope] = true
	}
}

// Purge removes one scope directory and its index.
func (c *ArtworkCache) Purge(scope string) error {
	if !hexName.MatchString(scope) {
		return errors.New("plex: artwork scope must be a hex digest")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.scopes, scope)
	return os.RemoveAll(filepath.Join(c.root, scope))
}

// PurgeOwner removes every scope tagged with owner (all libraries of one
// server) and leaves other owners' scopes untouched.
func (c *ArtworkCache) PurgeOwner(owner string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var firstErr error
	for scope, idx := range c.scopes {
		if idx.Owner != owner {
			continue
		}
		delete(c.scopes, scope)
		if err := os.RemoveAll(filepath.Join(c.root, scope)); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Scopes lists the scope ids currently indexed (diagnostics and tests).
func (c *ArtworkCache) Scopes() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.scopes))
	for s := range c.scopes {
		out = append(out, s)
	}
	return out
}

// touchTime is exposed for tests that reason about LRU order.
func (c *ArtworkCache) accessedAt(scope, key string) (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.scopes[scope]
	if idx == nil {
		return time.Time{}, false
	}
	e := idx.Entries[key]
	if e == nil {
		return time.Time{}, false
	}
	return time.UnixMilli(e.AccessedAt), true
}
