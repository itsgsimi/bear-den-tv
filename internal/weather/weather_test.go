// Tests for internal/weather: the Open-Meteo client against an httptest
// server, the WMO mapping, and the poller's cadence, backoff, expiry and
// unit conversion on clock.Fake. No test touches the network.
package weather

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
)

var zagreb = contract.WeatherPlace{Name: "Zagreb", Region: "City of Zagreb", Country: "Croatia", Latitude: 45.81, Longitude: 15.98}

func TestClientCurrentAndSearch(t *testing.T) {
	var gotQuery, gotUA, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery, gotUA, gotPath = r.URL.RawQuery, r.UserAgent(), r.URL.Path
		switch r.URL.Path {
		case "/v1/forecast":
			_, _ = w.Write([]byte(`{"current":{"time":"2026-09-23T14:00","temperature_2m":11.6,"weather_code":65,"is_day":0}}`))
		case "/v1/search":
			_, _ = w.Write([]byte(`{"results":[{"name":"Zagreb","admin1":"City of Zagreb","country":"Croatia","latitude":45.81444,"longitude":15.97798},{"name":"No coords"}]}`))
		}
	}))
	defer srv.Close()
	c := NewClient(ClientOptions{HTTP: srv.Client(), ForecastURL: srv.URL + "/v1/forecast", GeocodingURL: srv.URL + "/v1/search", UserAgent: "bear-den-tv/test"})

	r, err := c.Current(context.Background(), contract.WeatherPlace{Name: "Z", Latitude: 45.8144, Longitude: 15.9779})
	if err != nil {
		t.Fatal(err)
	}
	if r.TemperatureC != 11.6 || r.Code != 65 || r.IsDay || !r.ObservedAt.Equal(time.Date(2026, 9, 23, 14, 0, 0, 0, time.UTC)) {
		t.Fatalf("reading = %+v", r)
	}
	for _, want := range []string{"latitude=45.81", "longitude=15.98", "current=temperature_2m%2Cweather_code%2Cis_day", "timezone=UTC"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("forecast query %q lacks %q", gotQuery, want)
		}
	}
	if strings.Contains(gotQuery, "45.8144") || gotUA != "bear-den-tv/test" {
		t.Errorf("forecast must send rounded coordinates and the user agent: %q %q", gotQuery, gotUA)
	}

	places, err := c.Search(context.Background(), "Zagreb")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/search" || !strings.Contains(gotQuery, "name=Zagreb") || !strings.Contains(gotQuery, "count=8") || !strings.Contains(gotQuery, "language=en") {
		t.Errorf("geocoding query = %q", gotQuery)
	}
	if len(places) != 1 || places[0] != zagreb {
		t.Fatalf("places = %+v, want one rounded Zagreb", places)
	}
}

func TestClientErrorsAreSafe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "latitude") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"current":`))
	}))
	c := NewClient(ClientOptions{HTTP: srv.Client(), ForecastURL: srv.URL, GeocodingURL: srv.URL})
	if _, err := c.Current(context.Background(), zagreb); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("500 = %v", err)
	}
	if _, err := c.Search(context.Background(), "x"); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("truncated JSON = %v", err)
	}
	srv.Close()
	_, err := c.Current(context.Background(), zagreb)
	if err == nil || strings.Contains(err.Error(), "45.81") || strings.Contains(err.Error(), "http") {
		t.Fatalf("unreachable error must not leak the URL or coordinates: %v", err)
	}
	for _, bad := range []string{"", "   ", strings.Repeat("a", 81), "a\nb", "a\x00"} {
		if _, err := c.Search(context.Background(), bad); err == nil {
			t.Errorf("query %q accepted", bad)
		}
	}
}

func TestConditionMapping(t *testing.T) {
	for code, want := range map[int]string{
		0: "clear/moderate", 2: "partly-cloudy/moderate", 3: "cloudy/moderate", 48: "fog/moderate",
		51: "drizzle/light", 55: "drizzle/heavy", 56: "drizzle/light", 61: "rain/light", 63: "rain/moderate",
		67: "rain/heavy", 71: "snow/light", 75: "snow/heavy", 77: "snow/light", 80: "rain/light", 82: "rain/heavy",
		85: "snow/light", 86: "snow/heavy", 95: "thunder/moderate", 96: "thunder/heavy", 99: "thunder/heavy",
		4: "cloudy/moderate", -1: "cloudy/moderate",
	} {
		c, i := Condition(code)
		if c+"/"+i != want {
			t.Errorf("code %d = %s/%s, want %s", code, c, i, want)
		}
	}
}

// scripted is a Source whose answers the test controls.
type scripted struct {
	mu    sync.Mutex
	calls int
	err   error
	r     Reading
}

func (s *scripted) Current(context.Context, contract.WeatherPlace) (Reading, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.r, s.err
}

func (s *scripted) Search(context.Context, string) ([]contract.WeatherPlace, error) {
	return []contract.WeatherPlace{zagreb}, nil
}

func (s *scripted) set(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

func on(units string) config.Weather {
	p := zagreb
	return config.Weather{Enabled: true, Place: &p, Units: units, Scene: true}
}

func TestPollerStatusesBackoffAndExpiry(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 9, 23, 14, 5, 0, 0, time.UTC))
	src := &scripted{r: Reading{TemperatureC: 11.6, Code: 63, IsDay: true, ObservedAt: time.Date(2026, 9, 23, 14, 0, 0, 0, time.UTC)}}
	var logs bytes.Buffer
	p := New(Options{Source: src, Clock: fc, Logger: slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))})
	ctx := context.Background()

	if s := p.Snapshot(); s.Status != StatusDisabled || s.Current != nil {
		t.Fatalf("off by default: %+v", s)
	}
	if p.Refresh(ctx) != 0 || src.calls != 0 {
		t.Fatal("nothing is fetched while weather is off")
	}
	p.Configure(on(config.UnitsCelsius))
	if s := p.Snapshot(); s.Status != StatusConnecting || s.Place != "Zagreb" {
		t.Fatalf("on, no reading yet: %+v", s)
	}
	if d := p.Refresh(ctx); d != PollInterval {
		t.Fatalf("after success wait %v, want %v", d, PollInterval)
	}
	s := p.Snapshot()
	want := contract.WeatherCurrent{Temperature: 12, Condition: "rain", Intensity: "moderate", IsDay: true, ObservedAt: "2026-09-23T14:00:00Z"}
	if s.Status != StatusReady || s.Message != "" || s.Current == nil || *s.Current != want {
		t.Fatalf("ready = %+v %+v", s, s.Current)
	}
	// Units change converts without a refetch.
	p.Configure(on(config.UnitsFahrenheit))
	if s := p.Snapshot(); s.Units != "fahrenheit" || s.Current == nil || s.Current.Temperature != 53 || src.calls != 1 {
		t.Fatalf("fahrenheit = %+v calls %d", s.Current, src.calls)
	}

	// Failures: stale while the reading is young, backing off 5, 10, 20, 30, 30.
	src.set(errors.New("The weather service could not be reached."))
	for _, want := range []time.Duration{5 * time.Minute, 10 * time.Minute, 20 * time.Minute, 30 * time.Minute, 30 * time.Minute} {
		if d := p.Refresh(ctx); d != want {
			t.Fatalf("backoff = %v, want %v", d, want)
		}
	}
	if s := p.Snapshot(); s.Status != StatusStale || s.Current == nil || s.Message == "" {
		t.Fatalf("stale = %+v", s)
	}
	fc.Advance(MaxAge + time.Minute)
	if s := p.Snapshot(); s.Status != StatusError || s.Current != nil || !strings.Contains(s.Message, "could not be reached") {
		t.Fatalf("a reading older than 6 h is dropped: %+v", s)
	}
	src.set(nil)
	if d := p.Refresh(ctx); d != PollInterval || p.Snapshot().Status != StatusReady {
		t.Fatal("a success resets the backoff")
	}
	// A new place forgets the old reading.
	other := on(config.UnitsCelsius)
	other.Place.Name, other.Place.Latitude = "Split", 43.51
	p.Configure(other)
	if s := p.Snapshot(); s.Status != StatusConnecting || s.Current != nil || s.Place != "Split" {
		t.Fatalf("new place: %+v", s)
	}
	p.Configure(config.Weather{Units: config.UnitsCelsius})
	if s := p.Snapshot(); s.Status != StatusDisabled || s.Current != nil {
		t.Fatalf("off again: %+v", s)
	}
	if strings.Contains(logs.String(), "45.81") || strings.Contains(logs.String(), "15.98") {
		t.Fatalf("logs carry coordinates:\n%s", logs.String())
	}
}

func TestPollerRunFollowsTheClock(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 9, 23, 14, 0, 0, 0, time.UTC))
	src := &scripted{err: errors.New("down")}
	fetched := make(chan struct{}, 16)
	p := New(Options{Source: src, Clock: fc, OnChange: func() { fetched <- struct{}{} }})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)
	p.Configure(on(config.UnitsCelsius))
	waitFetch := func(what string) {
		t.Helper()
		select {
		case <-fetched:
		case <-time.After(3 * time.Second):
			t.Fatalf("no fetch: %s", what)
		}
	}
	armed := func() {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for fc.Pending() == 0 {
			if time.Now().After(deadline) {
				t.Fatal("no retry timer armed")
			}
			time.Sleep(time.Millisecond)
		}
	}
	waitFetch("configure kicks an immediate fetch")
	armed()
	fc.Advance(4 * time.Minute)
	select {
	case <-fetched:
		t.Fatal("retried before 5 minutes")
	case <-time.After(20 * time.Millisecond):
	}
	fc.Advance(time.Minute)
	waitFetch("retry after 5 minutes")
	armed()
	fc.Advance(10 * time.Minute)
	waitFetch("retry after 10 more minutes")
	src.set(nil)
	armed()
	p.Kick()
	waitFetch("kick fetches at once")
	if p.Snapshot().Status != StatusReady {
		t.Fatalf("status = %+v", p.Snapshot())
	}
}

func TestFixtureIsDemoAndOffline(t *testing.T) {
	f := Fixture{Now: func() time.Time { return time.Date(2026, 9, 23, 14, 7, 0, 0, time.UTC) }}
	r, err := f.Current(context.Background(), DemoPlace)
	if c, i := Condition(r.Code); err != nil || r.TemperatureC != 12 || c != "rain" || i != "moderate" {
		t.Fatalf("fixture reading = %+v %v", r, err)
	}
	places, _ := f.Search(context.Background(), "anything")
	if len(places) != 1 || !strings.Contains(places[0].Name, "DEMO") {
		t.Fatalf("fixture places = %+v", places)
	}
}
