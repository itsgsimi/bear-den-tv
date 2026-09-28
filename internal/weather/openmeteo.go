// Package weather owns Bear Den's local weather: the Open-Meteo client
// (forecast and geocoding, no key), the WMO weather-code mapping, the poller
// that keeps one reading in memory for state.weather (shell view only), and a
// DEMO fixture source for `bear-den-tv dev`. The coordinator contacts
// Open-Meteo only while the owner has turned weather on; coordinates are
// rounded to 2 decimals and never logged. Spec: contracts/config.md (weather),
// contracts/ipc.md (weather.search, weather.configure), state.schema.json
// (weather), docs/security.md.
package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
)

// Open-Meteo endpoints. Constants: config.json never carries a URL; tests
// override them through ClientOptions.
const (
	ForecastURL  = "https://api.open-meteo.com/v1/forecast"
	GeocodingURL = "https://geocoding-api.open-meteo.com/v1/search"
)

// RequestTimeout bounds every request to Open-Meteo.
const RequestTimeout = 10 * time.Second

// MaxQueryLen is the longest place search accepted (characters).
const MaxQueryLen = 80

// Reading is one current-conditions observation, always in Celsius.
type Reading struct {
	TemperatureC float64
	Code         int // WMO weather code
	IsDay        bool
	ObservedAt   time.Time
}

// Source fetches readings and searches places. The Open-Meteo Client and the
// DEMO Fixture implement it.
type Source interface {
	Current(ctx context.Context, place contract.WeatherPlace) (Reading, error)
	Search(ctx context.Context, query string) ([]contract.WeatherPlace, error)
}

// ClientOptions configures a Client; zero values take the defaults.
type ClientOptions struct {
	HTTP         *http.Client // default: a client with RequestTimeout
	ForecastURL  string       // default ForecastURL (tests only)
	GeocodingURL string       // default GeocodingURL (tests only)
	UserAgent    string       // default "bear-den-tv"
}

// Client talks to Open-Meteo.
type Client struct{ o ClientOptions }

// NewClient returns an Open-Meteo client.
func NewClient(o ClientOptions) *Client {
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: RequestTimeout}
	}
	if o.ForecastURL == "" {
		o.ForecastURL = ForecastURL
	}
	if o.GeocodingURL == "" {
		o.GeocodingURL = GeocodingURL
	}
	if o.UserAgent == "" {
		o.UserAgent = "bear-den-tv"
	}
	return &Client{o: o}
}

// Errors are user-facing and never carry the URL (it holds coordinates).
var (
	ErrUnreachable = errors.New("The weather service could not be reached.")
	ErrBadResponse = errors.New("The weather service sent an answer Bear Den could not read.")
)

func (c *Client) get(ctx context.Context, base string, q url.Values, out any) error {
	ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return ErrBadResponse
	}
	req.Header.Set("User-Agent", c.o.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.o.HTTP.Do(req)
	if err != nil {
		return ErrUnreachable // *url.Error would leak the query string
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return fmt.Errorf("The weather service answered %d.", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(out); err != nil {
		return ErrBadResponse
	}
	return nil
}

func coord(v float64) string { return strconv.FormatFloat(config.RoundCoordinate(v), 'f', 2, 64) }

// Current implements Source: temperature (°C), weather code and day/night.
func (c *Client) Current(ctx context.Context, p contract.WeatherPlace) (Reading, error) {
	q := url.Values{}
	q.Set("latitude", coord(p.Latitude))
	q.Set("longitude", coord(p.Longitude))
	q.Set("current", "temperature_2m,weather_code,is_day")
	q.Set("timezone", "UTC")
	var body struct {
		Current *struct {
			Time        string   `json:"time"`
			Temperature *float64 `json:"temperature_2m"`
			WeatherCode *int     `json:"weather_code"`
			IsDay       *int     `json:"is_day"`
		} `json:"current"`
	}
	if err := c.get(ctx, c.o.ForecastURL, q, &body); err != nil {
		return Reading{}, err
	}
	cur := body.Current
	if cur == nil || cur.Temperature == nil || cur.WeatherCode == nil || cur.IsDay == nil {
		return Reading{}, ErrBadResponse
	}
	at, err := parseTime(cur.Time)
	if err != nil {
		return Reading{}, ErrBadResponse
	}
	return Reading{TemperatureC: *cur.Temperature, Code: *cur.WeatherCode, IsDay: *cur.IsDay == 1, ObservedAt: at}, nil
}

// parseTime reads Open-Meteo's "2026-09-23T14:00" (UTC with timezone=UTC).
func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02T15:04", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, errors.New("bad time")
}

// ValidQuery checks a place search: 1..MaxQueryLen characters, no control
// characters.
func ValidQuery(q string) error {
	n := utf8.RuneCountInString(strings.TrimSpace(q))
	if !utf8.ValidString(q) || n == 0 || utf8.RuneCountInString(q) > MaxQueryLen {
		return fmt.Errorf("Type a place name of 1 to %d characters.", MaxQueryLen)
	}
	for _, r := range q {
		if unicode.IsControl(r) {
			return errors.New("The place name contains characters that are not allowed.")
		}
	}
	return nil
}

// Search implements Source: up to 8 places, English names, coordinates
// rounded to 2 decimals.
func (c *Client) Search(ctx context.Context, query string) ([]contract.WeatherPlace, error) {
	if err := ValidQuery(query); err != nil {
		return []contract.WeatherPlace{}, err
	}
	q := url.Values{}
	q.Set("name", strings.TrimSpace(query))
	q.Set("count", "8")
	q.Set("language", "en")
	q.Set("format", "json")
	var body struct {
		Results []struct {
			Name      string   `json:"name"`
			Admin1    string   `json:"admin1"`
			Country   string   `json:"country"`
			Latitude  *float64 `json:"latitude"`
			Longitude *float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := c.get(ctx, c.o.GeocodingURL, q, &body); err != nil {
		return []contract.WeatherPlace{}, err
	}
	out := []contract.WeatherPlace{}
	for _, r := range body.Results {
		if r.Name == "" || r.Latitude == nil || r.Longitude == nil || len(out) == 8 {
			continue
		}
		p := contract.WeatherPlace{
			Name: clip(r.Name), Region: clip(r.Admin1), Country: clip(r.Country),
			Latitude: config.RoundCoordinate(*r.Latitude), Longitude: config.RoundCoordinate(*r.Longitude),
		}
		if p.Latitude < -90 || p.Latitude > 90 || p.Longitude < -180 || p.Longitude > 180 {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// clip keeps a name within the 80-character contract limit.
func clip(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= 80 {
		return s
	}
	return string([]rune(s)[:80])
}

// Conditions (state.weather.current.condition) and intensities.
const (
	Clear        = "clear"
	PartlyCloudy = "partly-cloudy"
	Cloudy       = "cloudy"
	Fog          = "fog"
	Drizzle      = "drizzle"
	Rain         = "rain"
	Snow         = "snow"
	Thunder      = "thunder"

	Light    = "light"
	Moderate = "moderate"
	Heavy    = "heavy"
)

var wmo = map[int][2]string{
	0: {Clear, Moderate}, 1: {PartlyCloudy, Moderate}, 2: {PartlyCloudy, Moderate}, 3: {Cloudy, Moderate},
	45: {Fog, Moderate}, 48: {Fog, Moderate},
	51: {Drizzle, Light}, 53: {Drizzle, Moderate}, 55: {Drizzle, Heavy}, 56: {Drizzle, Light}, 57: {Drizzle, Heavy},
	61: {Rain, Light}, 63: {Rain, Moderate}, 65: {Rain, Heavy}, 66: {Rain, Light}, 67: {Rain, Heavy},
	71: {Snow, Light}, 73: {Snow, Moderate}, 75: {Snow, Heavy}, 77: {Snow, Light},
	80: {Rain, Light}, 81: {Rain, Moderate}, 82: {Rain, Heavy},
	85: {Snow, Light}, 86: {Snow, Heavy},
	95: {Thunder, Moderate}, 96: {Thunder, Heavy}, 99: {Thunder, Heavy},
}

// Condition maps an Open-Meteo WMO weather code to the contract's condition
// and intensity; unknown codes read as cloudy/moderate.
func Condition(code int) (condition, intensity string) {
	if m, ok := wmo[code]; ok {
		return m[0], m[1]
	}
	return Cloudy, Moderate
}
