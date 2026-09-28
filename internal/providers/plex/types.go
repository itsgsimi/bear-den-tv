// Plex API response types.

package plex

import (
	"encoding/json"
	"strconv"
)

// flexString decodes a JSON string or number as a string; Plex emits ids as
// either depending on the endpoint.
type flexString string

func (f *flexString) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*f = ""
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*f = flexString(n.String())
	return nil
}

// Identity is GET /identity.
type Identity struct {
	MachineIdentifier string `json:"machineIdentifier"`
	Version           string `json:"version"`
}

// Library is one GET /library/sections directory.
type Library struct {
	Key   flexString `json:"key"`
	Title string     `json:"title"`
	Type  string     `json:"type"` // movie | show | artist | photo
	UUID  string     `json:"uuid"`
}

// Metadata is one media item as returned by hub, onDeck, recentlyAdded, and
// collection endpoints. Only the fields the connector maps are decoded.
type Metadata struct {
	RatingKey        flexString `json:"ratingKey"`
	Key              string     `json:"key"`
	Type             string     `json:"type"` // movie | episode | show | season | track | ...
	Title            string     `json:"title"`
	ParentTitle      string     `json:"parentTitle"`
	GrandparentTitle string     `json:"grandparentTitle"`
	Index            int        `json:"index"`
	ParentIndex      int        `json:"parentIndex"`
	Year             int        `json:"year"`
	Thumb            string     `json:"thumb"`
	ParentThumb      string     `json:"parentThumb"`
	GrandparentThumb string     `json:"grandparentThumb"`
	Art              string     `json:"art"`
	ViewOffset       int64      `json:"viewOffset"`
	Duration         int64      `json:"duration"`
	AddedAt          int64      `json:"addedAt"`
	LibrarySectionID flexString `json:"librarySectionID"`
}

// Page is one page of Metadata with the container's paging counters.
type Page struct {
	Items  []Metadata
	Offset int
	Size   int
	Total  int
}

// Next reports whether another page exists after this one.
func (p Page) Next() bool { return p.Total > 0 && p.Offset+len(p.Items) < p.Total }

type identityEnvelope struct {
	MediaContainer Identity `json:"MediaContainer"`
}

type librariesEnvelope struct {
	MediaContainer struct {
		Directory []Library `json:"Directory"`
	} `json:"MediaContainer"`
}

type hubsEnvelope struct {
	MediaContainer struct {
		Hub []struct {
			HubIdentifier string     `json:"hubIdentifier"`
			Metadata      []Metadata `json:"Metadata"`
		} `json:"Hub"`
	} `json:"MediaContainer"`
}

type metadataEnvelope struct {
	MediaContainer struct {
		Size      int        `json:"size"`
		TotalSize int        `json:"totalSize"`
		Offset    int        `json:"offset"`
		Metadata  []Metadata `json:"Metadata"`
	} `json:"MediaContainer"`
}

func itoa(n int) string { return strconv.Itoa(n) }
