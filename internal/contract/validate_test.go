// Tests for contract validation against fixtures (validate.go; fixtures in
// contracts/fixtures).

package contract

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestValidateActionRequestAccepts(t *testing.T) {
	raw := []byte(`{"protocol":1,"request_id":"b1ed0b5e-e592-4b33-b7a7-9029a434a818","context_epoch":47,"target":"active","action":"nav.left","args":{}}`)
	req, err := ValidateActionRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if req.Action != ActionNavLeft || req.ContextEpoch != 47 || req.Target != "active" {
		t.Fatalf("decoded %+v", req)
	}
}

func TestValidateActionRequestRejects(t *testing.T) {
	cases := map[string]string{
		"unknown action": `{"protocol":1,"request_id":"b1ed0b5e-e592-4b33-b7a7-9029a434a818","context_epoch":47,"target":"active","action":"exec","args":{}}`,
		"extra args":     `{"protocol":1,"request_id":"b1ed0b5e-e592-4b33-b7a7-9029a434a818","context_epoch":47,"target":"active","action":"select","args":{"keycode":36}}`,
		"bad uuid":       `{"protocol":1,"request_id":"nope","context_epoch":47,"target":"active","action":"select","args":{}}`,
		"extra key":      `{"protocol":1,"request_id":"b1ed0b5e-e592-4b33-b7a7-9029a434a818","context_epoch":47,"target":"active","action":"select","args":{},"cmd":"x"}`,
		"zero seek":      `{"protocol":1,"request_id":"b1ed0b5e-e592-4b33-b7a7-9029a434a818","context_epoch":47,"target":"active","action":"media.seek_relative","args":{"seconds":0}}`,
		"control text":   `{"protocol":1,"request_id":"b1ed0b5e-e592-4b33-b7a7-9029a434a818","context_epoch":47,"target":"active","action":"text.submit","args":{"text":"a\u0007b"}}`,
		"bad target":     `{"protocol":1,"request_id":"b1ed0b5e-e592-4b33-b7a7-9029a434a818","context_epoch":47,"target":"Shell!","action":"select","args":{}}`,
		"not json":       `{`,
	}
	for name, raw := range cases {
		_, err := ValidateActionRequest([]byte(raw))
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s: want ValidationError, got %v", name, err)
		}
	}
}

func TestValidateActionRequestProtocol(t *testing.T) {
	raw := []byte(`{"protocol":2,"request_id":"b1ed0b5e-e592-4b33-b7a7-9029a434a818","context_epoch":47,"target":"active","action":"select","args":{}}`)
	_, err := ValidateActionRequest(raw)
	if !errors.Is(err, ErrUnsupportedProtocol) {
		t.Fatalf("want ErrUnsupportedProtocol, got %v", err)
	}
}

func TestValidateActionRequestOversized(t *testing.T) {
	raw := []byte(`{"protocol":1,"request_id":"b1ed0b5e-e592-4b33-b7a7-9029a434a818","context_epoch":47,"target":"active","action":"text.submit","args":{"text":"` + strings.Repeat("a", MaxMessageBytes) + `"}}`)
	if _, err := ValidateActionRequest(raw); err == nil {
		t.Fatal("oversized request accepted")
	}
}

func TestResultHelpersValidate(t *testing.T) {
	req := ActionRequest{Protocol: 1, RequestID: "b1ed0b5e-e592-4b33-b7a7-9029a434a818", Target: "active", Action: ActionSelect, Args: map[string]any{}}
	target := TargetInfo{Kind: "shell", AppID: nil, Label: "Bear Den TV"}
	for _, r := range []ActionResult{
		Failed(req, 3, target, CodeLocked, "locked"),
		Result(req, OutcomeObserved, 3, target, map[string]any{"section_id": "favorites"}),
		Result(req, OutcomeAccepted, 3, target, nil),
	} {
		raw, _ := json.Marshal(r)
		if _, err := ValidateActionResult(raw); err != nil {
			t.Errorf("%s: %v", raw, err)
		}
	}
}

func TestValidateHoldMessage(t *testing.T) {
	if _, err := ValidateHoldMessage([]byte(`{"type":"hold.start","hold_id":"b1ed0b5e-e592-4b33-b7a7-9029a434a818","action":"nav.right","context_epoch":4}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateHoldMessage([]byte(`{"type":"hold.start","hold_id":"b1ed0b5e-e592-4b33-b7a7-9029a434a818"}`)); err == nil {
		t.Fatal("hold.start without action accepted")
	}
	if _, err := ValidateHoldMessage([]byte(`{"type":"hold.start","hold_id":"b1ed0b5e-e592-4b33-b7a7-9029a434a818","action":"select","context_epoch":4}`)); err == nil {
		t.Fatal("non-nav hold accepted")
	}
}

func TestValidateLayoutAndConfigStructure(t *testing.T) {
	layout := []byte(`{"ui":{"theme":"den-dark","accent":"#79A889","background":"den-gradient","text_scale":1.0,"tile_density":"comfortable","safe_margin_percent":3,"reduced_motion":false,"high_contrast_focus":false,"hero_enabled":true,"clock_enabled":true},"sections":[{"id":"favorites","title":"Your Apps","kind":"applications","enabled":true,"application_ids":["plex-htpc"],"hide_when_empty":false}]}`)
	if _, err := ValidateLayout(layout); err != nil {
		t.Fatal(err)
	}
	bad := []byte(`{"ui":{"theme":"den-dark","accent":"#79A889","background":"den-gradient","text_scale":1.0,"tile_density":"comfortable","safe_margin_percent":3,"reduced_motion":false,"high_contrast_focus":false,"hero_enabled":true,"clock_enabled":true},"sections":[{"id":"plex","title":"Plex","kind":"plex-collection","enabled":true,"application_ids":[],"hide_when_empty":false}]}`)
	if _, err := ValidateLayout(bad); err == nil {
		t.Fatal("application_ids on a non-application section accepted")
	}
	if err := ValidateConfigStructure([]byte(`{"schema_version":1}`)); err == nil {
		t.Fatal("incomplete config accepted")
	}
}
