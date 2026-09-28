// Poller: refreshes the configured place's weather on a schedule.

package weather

// The poller: one reading in memory, refreshed every 30 min while weather is
// on with a place, retried after failures at 5, 10, 20, 30 min, dropped after
// 6 h. Snapshot never touches the network, so state building stays instant.

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
)

// Cadence (docs/security.md, contracts/ipc.md).
const (
	PollInterval = 30 * time.Minute
	RetryFirst   = 5 * time.Minute
	RetryMax     = 30 * time.Minute
	MaxAge       = 6 * time.Hour
)

// Status words (state.weather.status, the same words as content.status).
const (
	StatusDisabled   = "disabled"
	StatusConnecting = "connecting"
	StatusReady      = "ready"
	StatusStale      = "stale"
	StatusError      = "error"
)

// Options configures a Poller.
type Options struct {
	Source Source
	Clock  clock.Clock  // nil: clock.Real
	Logger *slog.Logger // nil: discard. Never given coordinates.
	// OnChange runs after every fetch (success or failure), outside locks.
	OnChange func()
}

// Poller keeps the latest reading for the configured place.
type Poller struct {
	o    Options
	kick chan struct{}

	mu       sync.Mutex
	settings config.Weather
	reading  *Reading
	gotAt    time.Time // clock time the reading arrived
	lastErr  error
	failures int
}

// New returns a poller that stays off until Configure turns it on.
func New(o Options) *Poller {
	if o.Clock == nil {
		o.Clock = clock.Real{}
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	return &Poller{o: o, kick: make(chan struct{}, 1), settings: config.Weather{Units: config.UnitsCelsius, Scene: true}}
}

func active(w config.Weather) bool { return w.Enabled && w.Place != nil }

func samePlace(a, b *contract.WeatherPlace) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// Configure applies the config.weather block. Turning weather on or changing
// the place forgets the old reading and wakes Run at once; units and scene
// changes need no fetch.
func (p *Poller) Configure(w config.Weather) {
	p.mu.Lock()
	old := p.settings
	p.settings = w
	if w.Place != nil {
		pl := *w.Place
		p.settings.Place = &pl
	}
	changed := active(old) != active(w) || !samePlace(old.Place, w.Place)
	if changed {
		p.reading, p.lastErr, p.failures = nil, nil, 0
	}
	p.mu.Unlock()
	if changed {
		p.Kick()
	}
}

// Kick asks Run to fetch now (weather.configure refreshes immediately).
func (p *Poller) Kick() {
	select {
	case p.kick <- struct{}{}:
	default:
	}
}

// Search looks a place up through the source (weather.search).
func (p *Poller) Search(ctx context.Context, query string) ([]contract.WeatherPlace, error) {
	if err := ValidQuery(query); err != nil {
		return []contract.WeatherPlace{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()
	places, err := p.o.Source.Search(ctx, query)
	if places == nil {
		places = []contract.WeatherPlace{}
	}
	if err != nil {
		p.o.Logger.Warn("weather: place search failed", "err", err.Error())
		return []contract.WeatherPlace{}, err
	}
	return places, nil
}

// Refresh fetches once when weather is active and returns how long to wait
// before the next fetch: PollInterval after success, RetryFirst doubling to
// RetryMax after consecutive failures. When inactive it fetches nothing and
// returns 0 (wait for Configure).
func (p *Poller) Refresh(ctx context.Context) time.Duration {
	p.mu.Lock()
	w := p.settings
	p.mu.Unlock()
	if !active(w) {
		return 0
	}
	ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
	r, err := p.o.Source.Current(ctx, *w.Place)
	cancel()

	p.mu.Lock()
	if !active(p.settings) || !samePlace(p.settings.Place, w.Place) {
		p.mu.Unlock() // reconfigured mid-fetch: Configure already kicked Run
		return 0
	}
	var next time.Duration
	if err != nil {
		p.lastErr = err
		p.failures++
		next = RetryMax
		if p.failures <= 3 {
			next = min(RetryFirst<<(p.failures-1), RetryMax)
		}
	} else {
		p.reading, p.gotAt, p.lastErr, p.failures = &r, p.o.Clock.Now(), nil, 0
		next = PollInterval
	}
	failures := p.failures
	p.mu.Unlock()
	if err != nil {
		p.o.Logger.Warn("weather: refresh failed", "err", err.Error(), "failures", failures, "retry_in", next)
	} else {
		p.o.Logger.Debug("weather: refreshed")
	}
	if p.o.OnChange != nil {
		p.o.OnChange()
	}
	return next
}

// Run refreshes on the cadence Refresh returns until ctx is done; Configure
// and Kick wake it early. It never blocks callers of Snapshot.
func (p *Poller) Run(ctx context.Context) {
	for {
		select { // a kick from before this fetch is answered by it
		case <-p.kick:
		default:
		}
		wait := p.Refresh(ctx)
		var timer clock.Timer
		var fire <-chan time.Time
		if wait > 0 {
			timer = p.o.Clock.NewTimer(wait)
			fire = timer.C()
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-p.kick:
		case <-fire:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

// Snapshot is state.weather from memory: status, message, place, units,
// scene and the current reading converted to the configured units.
func (p *Poller) Snapshot() contract.Weather {
	p.mu.Lock()
	defer p.mu.Unlock()
	w := p.settings
	units := w.Units
	if units != config.UnitsFahrenheit {
		units = config.UnitsCelsius
	}
	out := contract.Weather{Status: StatusDisabled, Units: units, Scene: w.Scene}
	if w.Place != nil {
		out.Place = w.Place.Name
	}
	if !active(w) {
		if w.Enabled {
			out.Message = "Choose a place to see the weather."
		}
		return out
	}
	fresh := p.reading != nil && p.o.Clock.Since(p.gotAt) <= MaxAge
	switch {
	case p.reading == nil && p.lastErr == nil:
		out.Status = StatusConnecting
	case !fresh:
		out.Status = StatusError
		out.Message = "No recent weather reading."
		if p.lastErr != nil {
			out.Message = p.lastErr.Error()
		}
	case p.lastErr != nil:
		out.Status = StatusStale
		out.Message = "Could not refresh the weather; showing the last reading."
	default:
		out.Status = StatusReady
	}
	if fresh {
		cond, intensity := Condition(p.reading.Code)
		t := p.reading.TemperatureC
		if units == config.UnitsFahrenheit {
			t = t*9/5 + 32
		}
		out.Current = &contract.WeatherCurrent{
			Temperature: int(math.Round(t)), Condition: cond, Intensity: intensity,
			IsDay: p.reading.IsDay, ObservedAt: p.reading.ObservedAt.UTC().Format(time.RFC3339),
		}
	}
	return out
}
