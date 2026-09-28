// `bear-den-tv weather`: show, search and set local weather through the
// coordinator (package internal/weather).

package main

// `bear-den-tv weather status|search|set|off`: local weather on the running
// coordinator over the shell socket (weather.search, weather.configure in
// contracts/ipc.md). The CLI never edits config.json itself.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/shellipc"
	"bear-den-tv/internal/weather"
)

const weatherUsage = `usage: bear-den-tv weather status
       bear-den-tv weather search QUERY
       bear-den-tv weather set [--units celsius|fahrenheit] [--no-scene] QUERY [INDEX]
       bear-den-tv weather off`

func cmdWeather(args []string) error {
	if len(args) == 0 {
		return errors.New(weatherUsage)
	}
	fs := flag.NewFlagSet("weather "+args[0], flag.ExitOnError)
	sock := socketFlag(fs)
	units := fs.String("units", "", "celsius or fahrenheit (default: keep the current units)")
	noScene := fs.Bool("no-scene", false, "show the weather in the header only, not in the Home scene")
	_ = fs.Parse(args[1:])
	rest := fs.Args()
	switch args[0] {
	case "status":
		return weatherStatus(*sock)
	case "search":
		places, err := weatherSearch(*sock, strings.Join(rest, " "))
		if err != nil {
			return err
		}
		if len(places) == 0 {
			fmt.Println("No places found.")
		}
		for i, p := range places {
			fmt.Printf("%d\t%s\n", i+1, placeLabel(p))
		}
		return nil
	case "set":
		return weatherSet(*sock, rest, *units, !*noScene)
	case "off":
		st, err := currentWeather(*sock)
		if err != nil {
			return err
		}
		return weatherConfigure(*sock, false, nil, st.Units, st.Scene)
	}
	return errors.New(weatherUsage)
}

func placeLabel(p contract.WeatherPlace) string {
	parts := []string{p.Name}
	for _, s := range []string{p.Region, p.Country} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

func currentWeather(sock string) (contract.Weather, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := dialCLI(ctx, sock)
	if err != nil {
		return contract.Weather{}, err
	}
	defer conn.Close()
	if conn.State.Weather == nil {
		if conn.State.Session.Locked {
			return contract.Weather{}, errors.New("the TV session is locked; weather is hidden until it is unlocked")
		}
		return contract.Weather{}, errors.New("this coordinator does not report weather")
	}
	return *conn.State.Weather, nil
}

func weatherStatus(sock string) error {
	w, err := currentWeather(sock)
	if err != nil {
		return err
	}
	fmt.Printf("status: %s\n", w.Status)
	if w.Place != "" {
		fmt.Printf("place: %s\n", w.Place)
	}
	fmt.Printf("units: %s\nscene: %v\n", w.Units, w.Scene)
	if c := w.Current; c != nil {
		sym := "°C"
		if w.Units == "fahrenheit" {
			sym = "°F"
		}
		day := "night"
		if c.IsDay {
			day = "day"
		}
		fmt.Printf("now: %d%s, %s (%s), %s, observed %s\n", c.Temperature, sym, c.Condition, c.Intensity, day, c.ObservedAt)
	}
	if w.Message != "" {
		fmt.Printf("message: %s\n", w.Message)
	}
	return nil
}

func weatherSearch(sock, query string) ([]contract.WeatherPlace, error) {
	if err := weather.ValidQuery(query); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := dialCLI(ctx, sock)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	id := "cli-weather-" + fmt.Sprint(time.Now().UnixNano())
	if err := conn.Send(shellipc.WeatherSearch{Type: shellipc.TypeWeatherSearch, RequestID: id, Query: query}); err != nil {
		return nil, err
	}
	res, err := conn.WeatherPlaces(id, 15*time.Second)
	if err != nil {
		return nil, err
	}
	if !res.OK {
		return nil, errors.New(res.Error)
	}
	return res.Places, nil
}

// weatherSet searches QUERY and stores result INDEX (1-based, default 1).
func weatherSet(sock string, rest []string, units string, scene bool) error {
	index := 1
	if len(rest) > 1 {
		if n, err := strconv.Atoi(rest[len(rest)-1]); err == nil {
			index, rest = n, rest[:len(rest)-1]
		}
	}
	if len(rest) == 0 {
		return errors.New(weatherUsage)
	}
	if units != "" && units != "celsius" && units != "fahrenheit" {
		return errors.New("--units must be celsius or fahrenheit")
	}
	places, err := weatherSearch(sock, strings.Join(rest, " "))
	if err != nil {
		return err
	}
	if len(places) == 0 {
		return errors.New("no places found; try another spelling")
	}
	if index < 1 || index > len(places) {
		return fmt.Errorf("INDEX must be 1 to %d (see `bear-den-tv weather search`)", len(places))
	}
	if units == "" {
		units = "celsius"
		if w, err := currentWeather(sock); err == nil {
			units = w.Units
		}
	}
	p := places[index-1]
	if err := weatherConfigure(sock, true, &p, units, scene); err != nil {
		return err
	}
	fmt.Printf("Weather on for %s.\n", placeLabel(p))
	return nil
}

func weatherConfigure(sock string, enabled bool, place *contract.WeatherPlace, units string, scene bool) error {
	id := "cli-weather-" + fmt.Sprint(time.Now().UnixNano())
	_, err := call(sock, shellipc.WeatherConfigure{
		Type: shellipc.TypeWeatherConfigure, RequestID: id, Enabled: enabled, Place: place, Units: units, Scene: scene,
	}, id)
	if err == nil && !enabled {
		fmt.Println("Weather off.")
	}
	return err
}
