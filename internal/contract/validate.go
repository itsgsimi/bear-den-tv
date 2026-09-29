// JSON Schema validation of incoming messages against contracts/*.schema.json.

package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	bdtv "bear-den-tv"
)

// Schema base URL; every contracts/*.schema.json declares $id under it, so
// relative $refs such as layout.schema.json#/properties/ui resolve here.
const schemaBase = "https://bear-den-tv.local/contracts/"

// Schema documents compiled from the embedded contracts. Each name maps to the
// file under contracts/ and the location appended to its $id.
const (
	schemaActionRequest = "action.schema.json#/$defs/request"
	schemaActionResult  = "action.schema.json#/$defs/result"
	schemaHoldMessage   = "action.schema.json#/$defs/holdMessage"
	schemaState         = "state.schema.json"
	schemaLayout        = "layout.schema.json"
	schemaConfig        = "config.schema.json"
	schemaTheme         = "theme.schema.json"
	schemaWebHints      = "web-hints.schema.json"
)

// MaxMessageBytes bounds every raw document handed to the validators; larger
// inputs are refused before decoding so an untrusted sender cannot exhaust
// memory through the validator.
const MaxMessageBytes = 262144

// ErrUnsupportedProtocol is returned when a message declares a protocol other
// than Protocol; callers map it to failed/unsupported_protocol.
var ErrUnsupportedProtocol = errors.New("unsupported protocol version")

// ValidationError reports why a document failed structural validation. Details
// are schema-level messages safe to show to a local operator; they never
// contain the document itself.
type ValidationError struct {
	Schema  string
	Details []string
}

// Error implements error.
func (e *ValidationError) Error() string {
	if len(e.Details) == 0 {
		return "does not match " + e.Schema
	}
	return "does not match " + e.Schema + ": " + strings.Join(e.Details, "; ")
}

var (
	compileOnce sync.Once
	compiled    map[string]*jsonschema.Schema
	compileErr  error
)

// compile loads every embedded schema once. A failure here is a build defect,
// so callers surface it as an error rather than panicking at package init.
func compile() (map[string]*jsonschema.Schema, error) {
	compileOnce.Do(func() {
		c := jsonschema.NewCompiler()
		c.DefaultDraft(jsonschema.Draft2020)
		for _, name := range []string{"action.schema.json", "state.schema.json", "layout.schema.json", "config.schema.json", "theme.schema.json", "web-hints.schema.json"} {
			raw, err := fs.ReadFile(bdtv.Contracts, "contracts/"+name)
			if err != nil {
				compileErr = fmt.Errorf("contract: read %s: %w", name, err)
				return
			}
			doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
			if err != nil {
				compileErr = fmt.Errorf("contract: parse %s: %w", name, err)
				return
			}
			if name == "action.schema.json" {
				doc = patchActionTarget(doc)
			}
			if err := c.AddResource(schemaBase+name, doc); err != nil {
				compileErr = fmt.Errorf("contract: register %s: %w", name, err)
				return
			}
		}
		compiled = map[string]*jsonschema.Schema{}
		for _, loc := range []string{schemaActionRequest, schemaActionResult, schemaHoldMessage, schemaState, schemaLayout, schemaConfig, schemaTheme, schemaWebHints} {
			s, err := c.Compile(schemaBase + loc)
			if err != nil {
				compileErr = fmt.Errorf("contract: compile %s: %w", loc, err)
				return
			}
			compiled[loc] = s
		}
	})
	return compiled, compileErr
}

// patchActionTarget rewrites action.schema.json#/$defs/target from oneOf to
// anyOf in memory if a oneOf is present. contracts/action.schema.json already
// uses anyOf ("active"/"shell" would otherwise match both the keyword enum and
// the appId pattern and fail oneOf), so this is a no-op guard against the
// file regressing; the file itself is the source of truth for the other
// language validators.
func patchActionTarget(doc any) any {
	root, ok := doc.(map[string]any)
	if !ok {
		return doc
	}
	defs, ok := root["$defs"].(map[string]any)
	if !ok {
		return doc
	}
	target, ok := defs["target"].(map[string]any)
	if !ok {
		return doc
	}
	if branches, ok := target["oneOf"]; ok {
		delete(target, "oneOf")
		target["anyOf"] = branches
	}
	return doc
}

// validateRaw decodes raw with number precision preserved and validates it
// against the named schema location.
func validateRaw(loc string, raw []byte) (any, error) {
	if len(raw) > MaxMessageBytes {
		return nil, &ValidationError{Schema: loc, Details: []string{fmt.Sprintf("message exceeds %d bytes", MaxMessageBytes)}}
	}
	schemas, err := compile()
	if err != nil {
		return nil, err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, &ValidationError{Schema: loc, Details: []string{"invalid JSON: " + err.Error()}}
	}
	if err := schemas[loc].Validate(doc); err != nil {
		return nil, &ValidationError{Schema: loc, Details: flatten(err)}
	}
	return doc, nil
}

// flatten turns the validator's error tree into short leaf messages.
func flatten(err error) []string {
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return []string{err.Error()}
	}
	var out []string
	for _, u := range ve.BasicOutput().Errors {
		if u.Error == nil {
			continue
		}
		loc := u.InstanceLocation
		if loc == "" {
			loc = "/"
		}
		out = append(out, loc+": "+u.Error.String())
	}
	if len(out) == 0 {
		out = []string{ve.Error()}
	}
	if len(out) > 16 {
		out = out[:16]
	}
	return out
}

// ValidateActionRequest checks raw against action.schema.json#/$defs/request
// and decodes it. A wrong protocol number returns ErrUnsupportedProtocol
// (wrapped) so the caller can answer failed/unsupported_protocol; every other
// mismatch returns a *ValidationError.
func ValidateActionRequest(raw []byte) (ActionRequest, error) {
	var probe struct {
		Protocol *json.Number `json:"protocol"`
	}
	if len(raw) <= MaxMessageBytes {
		if err := json.Unmarshal(raw, &probe); err == nil && probe.Protocol != nil && probe.Protocol.String() != "1" {
			return ActionRequest{}, fmt.Errorf("%w: got %s, supported [1]", ErrUnsupportedProtocol, probe.Protocol.String())
		}
	}
	if _, err := validateRaw(schemaActionRequest, raw); err != nil {
		return ActionRequest{}, err
	}
	var req ActionRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return ActionRequest{}, &ValidationError{Schema: schemaActionRequest, Details: []string{err.Error()}}
	}
	if req.Args == nil {
		req.Args = map[string]any{}
	}
	return req, nil
}

// ValidateActionResult checks raw against action.schema.json#/$defs/result.
func ValidateActionResult(raw []byte) (ActionResult, error) {
	if _, err := validateRaw(schemaActionResult, raw); err != nil {
		return ActionResult{}, err
	}
	var res ActionResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return ActionResult{}, &ValidationError{Schema: schemaActionResult, Details: []string{err.Error()}}
	}
	return res, nil
}

// ValidateHoldMessage checks raw against action.schema.json#/$defs/holdMessage.
func ValidateHoldMessage(raw []byte) (HoldMessage, error) {
	if _, err := validateRaw(schemaHoldMessage, raw); err != nil {
		return HoldMessage{}, err
	}
	var m HoldMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return HoldMessage{}, &ValidationError{Schema: schemaHoldMessage, Details: []string{err.Error()}}
	}
	return m, nil
}

// ValidateState checks a serialized snapshot against state.schema.json.
func ValidateState(raw []byte) error {
	_, err := validateRaw(schemaState, raw)
	return err
}

// ValidateLayout checks raw against layout.schema.json and decodes it.
func ValidateLayout(raw []byte) (Layout, error) {
	if _, err := validateRaw(schemaLayout, raw); err != nil {
		return Layout{}, err
	}
	var l Layout
	if err := json.Unmarshal(raw, &l); err != nil {
		return Layout{}, &ValidationError{Schema: schemaLayout, Details: []string{err.Error()}}
	}
	return l, nil
}

// ValidateTheme checks a theme manifest (theme.json) against
// theme.schema.json. File existence is checked by internal/themes.
func ValidateTheme(raw []byte) error {
	_, err := validateRaw(schemaTheme, raw)
	return err
}

// ValidateWebHints checks a navigation hint file (apps/web-nav/hints/*.json)
// against web-hints.schema.json; internal/applications/web refuses to inject
// hints that fail it.
func ValidateWebHints(raw []byte) error {
	_, err := validateRaw(schemaWebHints, raw)
	return err
}

// ValidateConfigStructure checks raw against config.schema.json only; the
// semantic rules of contracts/config.md live in internal/config.
func ValidateConfigStructure(raw []byte) error {
	_, err := validateRaw(schemaConfig, raw)
	return err
}

// MarshalAndValidateState serializes s and proves it satisfies
// state.schema.json. Only tests call it (tests/contract); the coordinator
// marshals state without re-validating it at runtime.
func MarshalAndValidateState(s State) ([]byte, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if err := ValidateState(raw); err != nil {
		return nil, err
	}
	return raw, nil
}
