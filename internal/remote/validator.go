// StrictValidator: checks action requests against the known action names (spec
// contracts/actions.md).

package remote

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"

	"bear-den-tv/internal/contract"
)

// ErrUnsupportedProtocol is contract.ErrUnsupportedProtocol: returned by every
// validator when the request's protocol field is not contract.Protocol.
var ErrUnsupportedProtocol = contract.ErrUnsupportedProtocol

var (
	uuidPattern  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	appIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
)

// StrictValidator is a schema-free RequestValidator: strict JSON decoding with
// unknown fields refused, protocol pinned to 1, a UUID request_id, a
// non-negative context_epoch, a contract target, a known action name, and an
// args object. It does not check per-action argument shapes; the default
// validator (contract.ValidateActionRequest) does.
type StrictValidator struct{}

// ValidateActionRequest implements RequestValidator.
func (StrictValidator) ValidateActionRequest(raw []byte) (contract.ActionRequest, error) {
	var req contract.ActionRequest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return contract.ActionRequest{}, fmt.Errorf("invalid request: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return contract.ActionRequest{}, errors.New("invalid request: trailing data after the JSON object")
	}
	if req.Protocol != contract.Protocol {
		return contract.ActionRequest{}, ErrUnsupportedProtocol
	}
	if !uuidPattern.MatchString(req.RequestID) {
		return contract.ActionRequest{}, errors.New("invalid request: request_id must be a UUID")
	}
	if req.ContextEpoch < 0 {
		return contract.ActionRequest{}, errors.New("invalid request: context_epoch must be >= 0")
	}
	if req.Target != "active" && req.Target != "shell" && !appIDPattern.MatchString(req.Target) {
		return contract.ActionRequest{}, errors.New("invalid request: target must be active, shell, or an application id")
	}
	if !knownAction(req.Action) {
		return contract.ActionRequest{}, fmt.Errorf("invalid request: unknown action %q", req.Action)
	}
	if req.Args == nil {
		return contract.ActionRequest{}, errors.New("invalid request: args must be an object")
	}
	return req, nil
}

func knownAction(name string) bool {
	for _, a := range contract.AllActions {
		if a == name {
			return true
		}
	}
	return false
}

// isUUID reports whether s matches the contract UUID pattern.
func isUUID(s string) bool { return uuidPattern.MatchString(s) }
