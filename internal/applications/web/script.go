// The navigation script and its hints, embedded in the binary (embed.go
// WebNav): apps/web-nav/dist/nav.js plus apps/web-nav/hints/<id>.json,
// validated against contracts/web-hints.schema.json before use.

package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"

	bdtv "bear-den-tv"
	"bear-den-tv/internal/contract"
)

var hintID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// NavScript returns the built navigation script.
func NavScript() ([]byte, error) {
	return fs.ReadFile(bdtv.WebNav, "apps/web-nav/dist/nav.js")
}

// Hints returns the named hint file, compacted, after checking it against
// the schema; "" returns JSON null (the generic behaviour).
func Hints(id string) ([]byte, error) {
	if id == "" {
		return []byte("null"), nil
	}
	if !hintID.MatchString(id) {
		return nil, fmt.Errorf("web: invalid hints id %q", id)
	}
	raw, err := fs.ReadFile(bdtv.WebNav, "apps/web-nav/hints/"+id+".json")
	if err != nil {
		return nil, fmt.Errorf("web: hints %s: %w", id, err)
	}
	if err := contract.ValidateWebHints(raw); err != nil {
		return nil, fmt.Errorf("web: hints %s: %w", id, err)
	}
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Source is what the coordinator hands Page.addScriptToEvaluateOnNewDocument:
// the script, then its install call with the hints as a JSON literal. Go's
// encoder escapes U+2028/U+2029 and "<", so the literal is plain JavaScript.
func Source(hintsID string) (string, error) {
	nav, err := NavScript()
	if err != nil {
		return "", err
	}
	h, err := Hints(hintsID)
	if err != nil {
		return "", err
	}
	var v any
	if err := json.Unmarshal(h, &v); err != nil {
		return "", err
	}
	lit, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(nav) + "\n;__bdtvNav.install(" + string(lit) + ");\n", nil
}
