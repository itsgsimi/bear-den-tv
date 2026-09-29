// Tests that every app Bear Den can launch is a row in the tuning table
// (detect.go Apps), and that the optional apps with nothing to tune say why
// instead of pretending (NoTuning; docs/APP_PERFORMANCE.md).

package tuning

import (
	"testing"

	"bear-den-tv/internal/applications/adapters"
)

func TestEveryAdapterHasATuningRow(t *testing.T) {
	rows := map[string]App{}
	for _, a := range Apps {
		rows[a.Adapter] = a
	}
	for _, ad := range adapters.NewRegistry().All() {
		row, ok := rows[ad.Name()]
		if !ok {
			t.Errorf("%s has no row in tuning.Apps", ad.Name())
			continue
		}
		if row.FlatpakID != ad.FlatpakID() {
			t.Errorf("%s: tuning row Flatpak id %s, adapter %s", ad.Name(), row.FlatpakID, ad.FlatpakID())
		}
	}
}

func TestOptionalAppsHaveNoTunableSettingsAndSayWhy(t *testing.T) {
	haswell := ParseDecoders([]byte(haswellInspect))
	for _, adapter := range []string{"spotify", "jellyfin", "retroarch"} {
		var row *App
		for i := range Apps {
			if Apps[i].Adapter == adapter {
				row = &Apps[i]
			}
		}
		if row == nil {
			t.Fatalf("%s: no tuning row", adapter)
		}
		for _, host := range []Host{small, big} {
			plan, err := row.Plan(t.TempDir(), haswell, host, refDisplay, Overrides{"anything": "x"})
			if err != nil || len(plan.Changes) != 0 || len(plan.Settings) != 0 || plan.File != "" {
				t.Errorf("%s: a plan with changes or settings: %+v %v", adapter, plan, err)
			}
			if len(plan.Notes) != 1 || plan.Notes[0] == "" || plan.App != adapter || plan.Flatpak != row.FlatpakID {
				t.Errorf("%s: the plan must name the app and say why nothing is tuned: %+v", adapter, plan)
			}
			if got := Catalog(adapter, haswell, host, refDisplay); len(got) != 0 {
				t.Errorf("%s: offers settings %+v", adapter, got)
			}
			if expect, _ := Expectations(adapter, haswell, host, refDisplay); len(expect) == 0 {
				t.Errorf("%s: no expectation", adapter)
			}
		}
	}
}
