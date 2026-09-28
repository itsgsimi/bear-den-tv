// Fixture: a fixed weather source for --dev-fixtures and tests.

package weather

// Fixture is the DEMO weather source for `bear-den-tv dev --dev-fixtures`
// and the sandbox: no network, a fixed rainy reading, one DEMO place.

import (
	"context"
	"time"

	"bear-den-tv/internal/contract"
)

// DemoPlace is the only place the fixture source knows.
var DemoPlace = contract.WeatherPlace{Name: "Demo Town (DEMO)", Region: "", Country: "DEMO", Latitude: 0, Longitude: 0}

// Fixture implements Source without touching the network.
type Fixture struct {
	// Now stamps readings; nil uses time.Now.
	Now func() time.Time
}

// Current implements Source: 12 °C, moderate rain, daytime.
func (f Fixture) Current(context.Context, contract.WeatherPlace) (Reading, error) {
	now := time.Now
	if f.Now != nil {
		now = f.Now
	}
	return Reading{TemperatureC: 12, Code: 63, IsDay: true, ObservedAt: now().UTC().Truncate(15 * time.Minute)}, nil
}

// Search implements Source: DemoPlace for any valid query.
func (f Fixture) Search(_ context.Context, query string) ([]contract.WeatherPlace, error) {
	if err := ValidQuery(query); err != nil {
		return []contract.WeatherPlace{}, err
	}
	return []contract.WeatherPlace{DemoPlace}, nil
}
