// Tests for the shell environment allowlist (supervisor.go).

package shellipc

import (
	"slices"
	"testing"
)

func TestShellEnvironmentPassesOnlyWhatTheShellNeeds(t *testing.T) {
	base := []string{
		"DISPLAY=:0", "HOME=/home/tv", "XDG_DATA_HOME=/data", "QT_SCALE_FACTOR=1",
		"BDTV_THEMES_DIR=/themes", "BDTV_BEARS_ACT=walk", "BDTV_SHELL_SOCKET=/stale",
		"GITHUB_TOKEN=secret", "BDTV_TIER=entry", "broken",
	}
	got := ShellEnvironment(base, map[string]string{"BDTV_SHELL_SOCKET": "/run/sock"})
	for _, want := range []string{"DISPLAY=:0", "HOME=/home/tv", "XDG_DATA_HOME=/data", "QT_SCALE_FACTOR=1", "BDTV_THEMES_DIR=/themes", "BDTV_BEARS_ACT=walk", "BDTV_SHELL_SOCKET=/run/sock"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	for _, banned := range []string{"GITHUB_TOKEN=secret", "BDTV_TIER=entry", "BDTV_SHELL_SOCKET=/stale", "broken"} {
		if slices.Contains(got, banned) {
			t.Errorf("%s must not reach the shell: %v", banned, got)
		}
	}
}
