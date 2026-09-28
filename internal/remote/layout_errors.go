// LayoutValidationError: a layout rejected with per-field reasons (spec
// contracts/http.md).

package remote

import "strings"

// LayoutValidationError is returned by Backend layout writes when the
// submitted layout violates contracts/layout.schema.json or the semantic
// rules in internal/config. The server answers 422 invalid with Errors.
type LayoutValidationError struct {
	Errors []string
}

// Error joins the individual messages.
func (e *LayoutValidationError) Error() string {
	return "invalid layout: " + strings.Join(e.Errors, "; ")
}
