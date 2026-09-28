// Package providers defines the optional home-screen content seam
// (ContentProvider, spec §9.2) and the Feed that drives one provider for the
// configured sections, keeping the last good contract.Content available
// without ever blocking a caller on the network.
//
// Providers: plex (real server, token from internal/secrets) and fixtures
// (DEMO-labeled items behind --dev-fixtures). App launch and Home never wait
// on anything in this package (spec §9.4).
package providers

import (
	"context"

	"bear-den-tv/internal/contract"
)

// Content status values (contracts/state.schema.json content.status).
const (
	StatusDisabled   = "disabled"
	StatusConnecting = "connecting"
	StatusReady      = "ready"
	StatusStale      = "stale"
	StatusError      = "error"
)

// Open action values (contracts/state.schema.json content.sections[].items[].open_action).
const (
	// OpenApp launches or activates the provider's client application; the
	// item itself is not opened.
	OpenApp = "open_app"
	// PlayExact opens that exact item at its resume position. Only a provider
	// with a verified client handoff may return it (spec §7.3).
	PlayExact = "play_exact"
)

// Section kinds a provider can serve (contracts/layout.schema.json sections[].kind).
const (
	KindContinueWatching = "plex-continue-watching"
	KindRecentlyAdded    = "plex-recently-added"
	KindCollection       = "plex-collection"
)

// SectionDescriptor is one row kind the connected provider can serve, with
// the provider key (library or collection id) a configured section may name.
type SectionDescriptor struct {
	Kind  string
	Key   string
	Title string
}

// SectionConfig is the configured section a provider fetches items for. ID
// and Kind come from sections[]; Key names the provider-side collection for
// KindCollection; LibraryIDs restricts libraries (plex_content.library_ids,
// empty = every video library); Limit caps items (0 = provider default).
type SectionConfig struct {
	ID         string
	Kind       string
	Key        string
	LibraryIDs []string
	Limit      int
}

// OpenAction is what selecting an item does. Kind is the contract value;
// AppID names the application to launch for OpenApp; Reason explains why
// PlayExact is not offered.
type OpenAction struct {
	Kind   string
	AppID  string
	Reason string
}

// ContentProvider is the spec §9.2 seam. Implementations are safe for
// concurrent use; every network call honors ctx and returns within the
// provider's own deadlines. Items never carry credentials or upstream URLs.
type ContentProvider interface {
	// Name is the contract content.provider value ("plex", "fixtures").
	Name() string
	// Connect verifies credentials and server identity. It is called before
	// the first fetch and again after a failure; errors are user-safe text.
	Connect(ctx context.Context) error
	// ListSections reports which section kinds and keys the provider serves.
	ListSections(ctx context.Context) ([]SectionDescriptor, error)
	// FetchItems returns one page of items for a section and the cursor for
	// the next page ("" when there is none).
	FetchItems(ctx context.Context, sectionKind string, cfg SectionConfig, cursor string) (items []contract.ContentItem, next string, err error)
	// ResolveArtwork returns a coordinator-local file path for the item's
	// artwork, "" when the item has none, or an error when it could not be
	// fetched; callers show the item without artwork on error.
	ResolveArtwork(ctx context.Context, item contract.ContentItem) (localPath string, err error)
	// ResolveOpenAction reports what selecting the item does.
	ResolveOpenAction(ctx context.Context, item contract.ContentItem) OpenAction
	// Status is the provider's own state: a content status and a user-facing
	// message ("" when nothing needs saying).
	Status() (status, message string)
}
