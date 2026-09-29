// Semantic config validation against host facts such as network interfaces
// (spec contracts/config.md).

package config

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"sort"
	"strings"

	"bear-den-tv/internal/contract"
)

// Reserved target words that application and section ids must not use (rule 1).
var reservedIDs = map[string]bool{"active": true, "shell": true}

// forbiddenKeys are JSON member names that indicate a leaked credential; a
// document containing one anywhere is rejected (rule 7).
var forbiddenKeys = map[string]bool{"token": true, "x_plex_token": true, "password": true}

// AdapterSpec is what the validator knows about one adapter name: the only
// Flatpak id it may launch and its approved launch arguments (rule 3).
type AdapterSpec struct {
	FlatpakID    string
	ApprovedArgs []string
}

// DefaultAdapters lists the adapters shipped with this build.
var DefaultAdapters = map[string]AdapterSpec{
	"plex-htpc":  {FlatpakID: "tv.plex.PlexHTPC"},
	"vacuumtube": {FlatpakID: "rocks.shy.VacuumTube", ApprovedArgs: []string{"--fullscreen", "--no-window-decorations"}},
	"moonlight":  {FlatpakID: "com.moonlight_stream.Moonlight"},
}

// Interface is one host network interface as seen by the validator.
type Interface struct {
	Name     string
	Loopback bool
	// Virtual marks docker/bridge/veth/tun/wg style interfaces that must not
	// carry the LAN remote.
	Virtual bool
}

// InterfaceLister reports the host's interfaces so rule 4 can be checked
// without binding anything; tests inject a fixed list.
type InterfaceLister interface {
	Interfaces() ([]Interface, error)
}

// HostInterfaces lists real interfaces through package net.
type HostInterfaces struct{}

// Interfaces implements InterfaceLister.
func (HostInterfaces) Interfaces() ([]Interface, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make([]Interface, 0, len(ifs))
	for _, i := range ifs {
		out = append(out, Interface{Name: i.Name, Loopback: i.Flags&net.FlagLoopback != 0, Virtual: isVirtualName(i.Name)})
	}
	return out, nil
}

// StaticInterfaces is an InterfaceLister over a fixed list.
type StaticInterfaces []Interface

// Interfaces implements InterfaceLister.
func (s StaticInterfaces) Interfaces() ([]Interface, error) { return s, nil }

func isVirtualName(name string) bool {
	for _, p := range []string{"docker", "br-", "br", "veth", "tun", "tap", "wg", "virbr", "vmnet", "lxc", "cni", "flannel", "tailscale", "utun"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// Errors collects every violated rule of one document.
type Errors struct {
	Items []string
}

// Error implements error.
func (e *Errors) Error() string { return "config: " + strings.Join(e.Items, "; ") }

func (e *Errors) add(format string, a ...any) { e.Items = append(e.Items, fmt.Sprintf(format, a...)) }

// Rules configures the semantic validator.
type Rules struct {
	Adapters   map[string]AdapterSpec
	Interfaces InterfaceLister
	// LoadKeyPair parses the HTTPS files for rule 5; nil uses crypto/tls.
	LoadKeyPair func(certFile, keyFile string) error
}

func (r Rules) adapters() map[string]AdapterSpec {
	if r.Adapters == nil {
		return DefaultAdapters
	}
	return r.Adapters
}

// ErrSchemaVersion marks a document whose schema_version this build refuses (rule 9).
var ErrSchemaVersion = errors.New("unsupported schema_version")

// Parse checks a raw document structurally, scans it for leaked credential
// keys, and decodes it. Host-independent semantic rules are applied too; host
// rules (4, 5) are applied by Validate.
func Parse(raw []byte, rules Rules) (Config, error) {
	var head struct {
		SchemaVersion *json.Number `json:"schema_version"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return Config{}, &Errors{Items: []string{"invalid JSON: " + err.Error()}}
	}
	if head.SchemaVersion == nil || head.SchemaVersion.String() != "1" {
		got := "missing"
		if head.SchemaVersion != nil {
			got = head.SchemaVersion.String()
		}
		return Config{}, fmt.Errorf("%w: got %s, this build accepts 1", ErrSchemaVersion, got)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Config{}, &Errors{Items: []string{err.Error()}}
	}
	if key, found := findForbiddenKey(doc, ""); found {
		return Config{}, &Errors{Items: []string{fmt.Sprintf("credential key %q is not allowed in config.json; secrets belong in the secret store", key)}}
	}
	if err := contract.ValidateConfigStructure(raw); err != nil {
		var ve *contract.ValidationError
		if errors.As(err, &ve) {
			return Config{}, &Errors{Items: ve.Details}
		}
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return Config{}, &Errors{Items: []string{err.Error()}}
	}
	if err := ValidatePortable(c, rules); err != nil {
		return Config{}, err
	}
	return c, nil
}

func findForbiddenKey(v any, path string) (string, bool) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if forbiddenKeys[strings.ToLower(k)] {
				return path + "/" + k, true
			}
			if p, ok := findForbiddenKey(t[k], path+"/"+k); ok {
				return p, true
			}
		}
	case []any:
		for i, e := range t {
			if p, ok := findForbiddenKey(e, fmt.Sprintf("%s/%d", path, i)); ok {
				return p, true
			}
		}
	}
	return "", false
}

// ValidatePortable applies the host-independent semantic rules (1, 2, 3, 7,
// 8-on-disk, 9, 10) to an already decoded configuration.
func ValidatePortable(c Config, rules Rules) error {
	errs := &Errors{}
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: got %d", ErrSchemaVersion, c.SchemaVersion)
	}
	if c.Revision < 1 {
		errs.add("revision must be >= 1")
	}
	appIDs := map[string]bool{}
	for _, a := range c.Applications {
		if reservedIDs[a.ID] {
			errs.add("application id %q is reserved", a.ID)
		}
		if appIDs[a.ID] {
			errs.add("duplicate application id %q", a.ID)
		}
		appIDs[a.ID] = true
		spec, ok := rules.adapters()[a.Adapter]
		if !ok {
			errs.add("application %q uses unknown adapter %q", a.ID, a.Adapter)
			continue
		}
		if a.Launch.AppID != spec.FlatpakID {
			errs.add("application %q launch.app_id %q must be %q for adapter %s", a.ID, a.Launch.AppID, spec.FlatpakID, a.Adapter)
		}
		for _, arg := range a.Launch.Args {
			if !contains(spec.ApprovedArgs, arg) {
				errs.add("application %q launch argument %q is not approved for adapter %s", a.ID, arg, a.Adapter)
			}
		}
	}
	sectionIDs := map[string]bool{}
	for _, s := range c.Sections {
		if reservedIDs[s.ID] {
			errs.add("section id %q is reserved", s.ID)
		}
		if sectionIDs[s.ID] {
			errs.add("duplicate section id %q", s.ID)
		}
		sectionIDs[s.ID] = true
		for _, ref := range s.ApplicationIDs {
			if !appIDs[ref] {
				errs.add("section %q references unknown application %q", s.ID, ref)
			}
		}
	}
	if c.PlexContent.Enabled {
		if c.PlexContent.ConnectionRef == nil || *c.PlexContent.ConnectionRef == "" {
			errs.add("plex_content.enabled requires connection_ref")
		}
		if c.PlexContent.ServerURL == nil || *c.PlexContent.ServerURL == "" {
			errs.add("plex_content.enabled requires server_url")
		}
	}
	if c.Remote.Enabled {
		if !c.Onboarding.LANConsent {
			errs.add("remote.enabled requires onboarding.lan_consent")
		}
		if len(c.Remote.Interfaces) == 0 {
			errs.add("remote.enabled requires at least one interface")
		}
		if c.Remote.Transport == "https" && (c.Remote.HTTPS.CertificateFile == nil || c.Remote.HTTPS.PrivateKeyFile == nil) {
			errs.add("remote.transport https requires certificate_file and private_key_file")
		}
	}
	if w := c.Weather; w != nil {
		if w.Enabled && w.Place == nil {
			errs.add("weather.enabled requires a place")
		}
		if w.Units != UnitsCelsius && w.Units != UnitsFahrenheit {
			errs.add("weather.units must be celsius or fahrenheit")
		}
		if p := w.Place; p != nil {
			if p.Name == "" {
				errs.add("weather.place.name is required")
			}
			if p.Latitude < -90 || p.Latitude > 90 || p.Longitude < -180 || p.Longitude > 180 {
				errs.add("weather.place coordinates are out of range")
			}
			if !TwoDecimals(p.Latitude) || !TwoDecimals(p.Longitude) {
				errs.add("weather.place coordinates must carry at most 2 decimals")
			}
		}
	}
	validateCEC(c, errs)
	if len(errs.Items) > 0 {
		return errs
	}
	return nil
}

// RoundCoordinate rounds a latitude or longitude to 2 decimals (about 1 km),
// the most precise location Bear Den stores (rule 10).
func RoundCoordinate(v float64) float64 { return math.Round(v*100) / 100 }

// TwoDecimals reports whether v carries at most 2 decimals (rule 10).
func TwoDecimals(v float64) bool { return math.Abs(v*100-math.Round(v*100)) < 1e-6 }

// ValidateHost applies the host-bound rules: every enabled interface exists
// and is neither loopback nor virtual (rule 4), and HTTPS files parse as a
// certificate/key pair (rule 5). Failures keep the listener down without
// invalidating the rest of the configuration.
func ValidateHost(c Config, rules Rules) error {
	if !c.Remote.Enabled {
		return nil
	}
	errs := &Errors{}
	if rules.Interfaces == nil {
		errs.add("no interface lister configured; refusing to bind")
	} else {
		ifs, err := rules.Interfaces.Interfaces()
		if err != nil {
			errs.add("cannot list interfaces: %v", err)
		} else {
			byName := map[string]Interface{}
			for _, i := range ifs {
				byName[i.Name] = i
			}
			for _, name := range c.Remote.Interfaces {
				i, ok := byName[name]
				switch {
				case !ok:
					errs.add("interface %s not present", name)
				case i.Loopback:
					errs.add("interface %s is loopback", name)
				case i.Virtual:
					errs.add("interface %s is a virtual/bridge/tunnel interface", name)
				}
			}
		}
	}
	if c.Remote.Transport == "https" && c.Remote.HTTPS.CertificateFile != nil && c.Remote.HTTPS.PrivateKeyFile != nil {
		load := rules.LoadKeyPair
		if load == nil {
			load = func(cert, key string) error {
				_, err := tls.LoadX509KeyPair(cert, key)
				return err
			}
		}
		if err := load(*c.Remote.HTTPS.CertificateFile, *c.Remote.HTTPS.PrivateKeyFile); err != nil {
			errs.add("https certificate/key unusable: %v", err)
		}
	}
	if len(errs.Items) > 0 {
		return errs
	}
	return nil
}

// Validate applies every semantic rule, portable and host-bound.
func Validate(c Config, rules Rules) error {
	if err := ValidatePortable(c, rules); err != nil {
		return err
	}
	return ValidateHost(c, rules)
}

// ValidateLayoutAgainst checks a layout draft against the registered
// applications: unique, non-reserved section ids and resolvable references.
func ValidateLayoutAgainst(c Config, l contract.Layout) error {
	draft := c.Clone()
	draft.SetLayout(l)
	raw, err := json.Marshal(l)
	if err != nil {
		return err
	}
	if _, err := contract.ValidateLayout(raw); err != nil {
		var ve *contract.ValidationError
		if errors.As(err, &ve) {
			return &Errors{Items: ve.Details}
		}
		return err
	}
	return ValidatePortable(draft, Rules{})
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
