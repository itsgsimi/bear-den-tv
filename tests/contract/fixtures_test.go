// Package contract_test proves every contracts/fixtures/*.valid.json document
// validates against its schema and every *.invalid.json is rejected, using the
// embedded copies so the binary and the repository agree.
package contract_test

import (
	"encoding/json"
	"io/fs"
	"path"
	"strings"
	"testing"

	bdtv "bear-den-tv"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
)

func fixtures(t *testing.T) map[string][]byte {
	t.Helper()
	entries, err := fs.ReadDir(bdtv.Contracts, "contracts/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := fs.ReadFile(bdtv.Contracts, path.Join("contracts/fixtures", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = raw
	}
	if len(out) < 12 {
		t.Fatalf("expected the full fixture set, found %d", len(out))
	}
	return out
}

// validate dispatches a fixture to the validator its name prefix selects and
// returns the validation error, if any.
func validate(t *testing.T, name string, raw []byte) error {
	t.Helper()
	switch {
	case strings.HasPrefix(name, "action.request."):
		_, err := contract.ValidateActionRequest(raw)
		return err
	case strings.HasPrefix(name, "action.result."):
		_, err := contract.ValidateActionResult(raw)
		return err
	case strings.HasPrefix(name, "state."):
		return contract.ValidateState(raw)
	case strings.HasPrefix(name, "layout."):
		_, err := contract.ValidateLayout(raw)
		return err
	case strings.HasPrefix(name, "config."):
		// Structural schema plus the semantic rules of contracts/config.md;
		// config.*.invalid.json fixtures are rejected semantically.
		_, err := config.Parse(raw, config.Rules{Interfaces: config.StaticInterfaces{}})
		return err
	}
	t.Fatalf("no validator for fixture %s", name)
	return nil
}

func TestValidFixturesValidate(t *testing.T) {
	for name, raw := range fixtures(t) {
		if !strings.HasSuffix(name, ".valid.json") {
			continue
		}
		if err := validate(t, name, raw); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestInvalidFixturesAreRejected(t *testing.T) {
	for name, raw := range fixtures(t) {
		if !strings.HasSuffix(name, ".invalid.json") {
			continue
		}
		if err := validate(t, name, raw); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestStateFixturesRoundTrip proves the Go State type carries every field of
// the shell and phone views without loss.
func TestStateFixturesRoundTrip(t *testing.T) {
	for name, raw := range fixtures(t) {
		if !strings.HasPrefix(name, "state.") || !strings.HasSuffix(name, ".valid.json") {
			continue
		}
		var st contract.State
		if err := json.Unmarshal(raw, &st); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out, err := contract.MarshalAndValidateState(st)
		if err != nil {
			t.Fatalf("%s: re-marshal: %v", name, err)
		}
		var a, b any
		_ = json.Unmarshal(raw, &a)
		_ = json.Unmarshal(out, &b)
		ja, _ := json.Marshal(a)
		jb, _ := json.Marshal(b)
		if string(ja) != string(jb) {
			t.Errorf("%s: round trip changed the document\n%s\n%s", name, ja, jb)
		}
	}
}

func TestDefaultsFixtureIsTheBuiltInDefault(t *testing.T) {
	raw := fixtures(t)["config.default.valid.json"]
	var fixture, builtin any
	_ = json.Unmarshal(raw, &fixture)
	out, err := config.Marshal(config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(out, &builtin)
	a, _ := json.Marshal(fixture)
	b, _ := json.Marshal(builtin)
	if string(a) != string(b) {
		t.Fatalf("built-in defaults differ from the fixture\n%s\n%s", a, b)
	}
}
