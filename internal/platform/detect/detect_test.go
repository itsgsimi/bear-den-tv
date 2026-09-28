// Tests for display-session classification (detect.go).

package detect

import (
	"context"
	"errors"
	"testing"

	"bear-den-tv/internal/platform"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"xfce x11 session", map[string]string{"XDG_SESSION_TYPE": "x11", "DISPLAY": ":0"}, SessionX11},
		{"ssh with DISPLAY exported", map[string]string{"XDG_SESSION_TYPE": "tty", "DISPLAY": ":0"}, SessionX11},
		{"wayland", map[string]string{"XDG_SESSION_TYPE": "wayland", "WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}, SessionWayland},
		{"wayland by socket only", map[string]string{"WAYLAND_DISPLAY": "wayland-1"}, SessionWayland},
		{"nothing", map[string]string{}, SessionUnknown},
		{"tty only", map[string]string{"XDG_SESSION_TYPE": "tty"}, SessionUnknown},
	}
	for _, tc := range cases {
		if got := Classify(tc.env); got != tc.want {
			t.Errorf("%s: got %s want %s", tc.name, got, tc.want)
		}
	}
}

func TestSessionEnvImportsOnlyAllowedVars(t *testing.T) {
	src := map[string]string{"DISPLAY": ":0", "XAUTHORITY": "/home/u/.Xauthority", "DEEPSEEK_API_KEY": "secret", "PATH": "/bin", "XDG_SESSION_ID": ""}
	env := SessionEnvFrom(func(k string) (string, bool) { v, ok := src[k]; return v, ok })
	if len(env) != 2 || env["DISPLAY"] != ":0" || env["XAUTHORITY"] != "/home/u/.Xauthority" {
		t.Fatalf("%v", env)
	}
	if _, leaked := env["DEEPSEEK_API_KEY"]; leaked {
		t.Fatal("non-session variable imported")
	}
}

func TestNewDesktopAdapterNeverPanicsWithoutDisplay(t *testing.T) {
	ctx := context.Background()
	a := NewDesktopAdapter(ctx, Options{Env: map[string]string{}})
	if a.Name() != UnavailableName || a.DisplaySession() != SessionUnknown {
		t.Fatalf("%s %s", a.Name(), a.DisplaySession())
	}
	for k, c := range a.Capabilities() {
		if c.Available || c.Reason == "" {
			t.Fatalf("%s: %+v", k, c)
		}
	}
	if err := a.DeliverKey(ctx, 1, platform.KeyUp); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	// An X11 verdict with a display nobody serves degrades to unavailable.
	a = NewDesktopAdapter(ctx, Options{Env: map[string]string{"DISPLAY": ":99", "XDG_SESSION_TYPE": "x11"}})
	if a.Name() != UnavailableName || a.DisplaySession() != SessionX11 {
		t.Fatalf("%s %s", a.Name(), a.DisplaySession())
	}
	if r := a.(*Unavailable).Reason(); r == "" {
		t.Fatal("empty reason")
	}
	if fg, err := a.ObserveForeground(ctx); fg.Known || err == nil {
		t.Fatal("unavailable adapter reported a foreground")
	}
	a = NewDesktopAdapter(ctx, Options{Env: map[string]string{"WAYLAND_DISPLAY": "wayland-0"}})
	if a.Name() != "wayland-limited" {
		t.Fatal(a.Name())
	}
	_ = a.Close()
}
