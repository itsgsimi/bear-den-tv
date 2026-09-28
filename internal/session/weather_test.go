// Session tests for local weather: weather.search and weather.configure over
// the shell socket, state.weather per view (shell only; never phones,
// anonymous viewers or a locked session), and text.submit gated on the
// shell's text_field focus report.
package session

import (
	"context"
	"testing"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/shellipc"
	"bear-den-tv/internal/weather"
)

func weatherHarness(t *testing.T) (*harness, *weather.Poller) {
	t.Helper()
	p := weather.New(weather.Options{Source: weather.Fixture{}})
	h := newHarness(t, func(o *Options) { o.Weather = p })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go p.Run(ctx)
	return h, p
}

func TestWeatherSearchConfigureAndState(t *testing.T) {
	h, _ := weatherHarness(t)
	if err := h.shell.Send(shellipc.WeatherSearch{Type: shellipc.TypeWeatherSearch, RequestID: "ws-1", Query: "Demo"}); err != nil {
		t.Fatal(err)
	}
	var places shellipc.WeatherPlaces
	h.eventually("weather_places", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		var ok bool
		places, ok = h.places["ws-1"]
		return ok
	})
	if !places.OK || len(places.Places) != 1 || places.Places[0] != weather.DemoPlace || places.Error != "" {
		t.Fatalf("weather_places = %+v", places)
	}
	_ = h.shell.Send(shellipc.WeatherSearch{Type: shellipc.TypeWeatherSearch, RequestID: "ws-2", Query: "a\nb"})
	h.eventually("refused search", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		r, ok := h.places["ws-2"]
		return ok && !r.OK && r.Error != "" && r.Places != nil && len(r.Places) == 0
	})

	if st := h.c.buildState(viewShell); st.Weather == nil || st.Weather.Status != weather.StatusDisabled {
		t.Fatalf("off by default: %+v", st.Weather)
	}
	send := func(id string, m shellipc.WeatherConfigure) shellipc.Result {
		m.Type, m.RequestID = shellipc.TypeWeatherConfigure, id
		if err := h.shell.Send(m); err != nil {
			t.Fatal(err)
		}
		return h.shellResult(id)
	}
	if r := send("wc-0", shellipc.WeatherConfigure{Enabled: true, Units: "celsius", Scene: true}); r.OK || r.Error == "" {
		t.Fatalf("enabled without a place must fail with a reason: %+v", r)
	}
	if r := send("wc-k", shellipc.WeatherConfigure{Enabled: false, Units: "kelvin"}); r.OK {
		t.Fatalf("unknown units accepted: %+v", r)
	}
	rev := h.c.opts.Config.Revision()
	precise := contract.WeatherPlace{Name: "Zagreb", Region: "City of Zagreb", Country: "Croatia", Latitude: 45.81444, Longitude: 15.97798}
	if r := send("wc-1", shellipc.WeatherConfigure{Enabled: true, Place: &precise, Units: "fahrenheit", Scene: false}); !r.OK {
		t.Fatalf("configure refused: %+v", r)
	}
	cfg := h.c.opts.Config.Current()
	if cfg.Revision != rev+1 || cfg.Weather == nil || !cfg.Weather.Enabled || cfg.Weather.Place.Latitude != 45.81 || cfg.Weather.Place.Longitude != 15.98 || cfg.Weather.Units != "fahrenheit" || cfg.Weather.Scene {
		t.Fatalf("stored weather = %+v (rev %d→%d)", cfg.Weather, rev, cfg.Revision)
	}
	h.eventually("a ready reading in the shell view", func() bool {
		w := h.c.buildState(viewShell).Weather
		return w != nil && w.Status == weather.StatusReady && w.Current != nil
	})
	st := h.c.buildState(viewShell)
	if w := st.Weather; w.Place != "Zagreb" || w.Units != "fahrenheit" || w.Scene || w.Current.Temperature != 54 || w.Current.Condition != "rain" {
		t.Fatalf("shell weather = %+v %+v", w, w.Current)
	}
	if _, err := contract.MarshalAndValidateState(st); err != nil {
		t.Fatalf("shell state with weather is invalid: %v", err)
	}
	for name, v := range map[string]*struct{ st contract.State }{
		"controller": {h.phones.Snapshot(context.Background(), &h.ctl)},
		"owner":      {h.phones.Snapshot(context.Background(), &h.owner)},
		"anonymous":  {h.c.buildState(viewAnonymous)},
	} {
		if v.st.Weather != nil {
			t.Errorf("%s view carries weather", name)
		}
	}
	// place null keeps the stored place (toggles from the shell and the CLI).
	if r := send("wc-2", shellipc.WeatherConfigure{Enabled: false, Units: "celsius", Scene: true}); !r.OK {
		t.Fatalf("turning off refused: %+v", r)
	}
	if w := h.c.opts.Config.Current().Weather; w.Enabled || w.Place == nil || w.Place.Name != "Zagreb" {
		t.Fatalf("off keeps the place: %+v", w)
	}
	if w := h.c.buildState(viewShell).Weather; w.Status != weather.StatusDisabled || w.Current != nil {
		t.Fatalf("off: %+v", w)
	}
	if r := send("wc-3", shellipc.WeatherConfigure{Enabled: true, Units: "celsius", Scene: true}); !r.OK {
		t.Fatalf("on again with the stored place refused: %+v", r)
	}
	h.eventually("ready again", func() bool { return h.c.buildState(viewShell).Weather.Status == weather.StatusReady })
	h.lock.set(true)
	h.eventually("locked", func() bool { return h.c.Target().Kind == "locked" })
	if h.c.buildState(viewShell).Weather != nil {
		t.Fatal("a locked session omits weather")
	}
}

func TestTextSubmitNeedsAFocusedShellTextField(t *testing.T) {
	h := newHarness(t)
	text := func() contract.Capability {
		return h.phones.Snapshot(context.Background(), &h.ctl).Capabilities["text.submit"]
	}
	if cp := text(); cp.Available || cp.Reason != "No text field is focused." {
		t.Fatalf("no text field: %+v", cp)
	}
	expectOutcome(t, h.submit(h.ctl, h.req("text.submit", map[string]any{"text": "Zagreb"})), contract.OutcomeFailed, contract.CodeUnsupported)

	_ = h.shell.Send(shellipc.Focus{Type: shellipc.TypeFocus, Screen: "settings", TextField: true})
	h.eventually("text.submit available", func() bool { return text().Available })
	if cp := text(); cp.Backend != "shell" {
		t.Fatalf("backend = %q", cp.Backend)
	}
	expectOutcome(t, h.submit(h.ctl, h.req("text.submit", map[string]any{"text": "Zagreb"})), contract.OutcomeObserved, contract.CodeOK)
	h.mu.Lock()
	last := h.inputs[len(h.inputs)-1]
	h.mu.Unlock()
	if last.Action != "text.submit" || last.Args["text"] != "Zagreb" {
		t.Fatalf("shell received %+v", last)
	}

	_ = h.shell.Send(shellipc.Focus{Type: shellipc.TypeFocus, Screen: "settings"})
	h.eventually("text.submit unavailable again", func() bool { return !text().Available })
}
