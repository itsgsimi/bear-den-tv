// Package webtest finds the Chromium the web-app end-to-end tests drive
// (internal/applications/web/e2e_test.go, internal/session/web_e2e_test.go)
// and checks once that it can start at all, so a machine missing Chromium's
// system libraries fails with Chromium's own error instead of an opaque
// "devtools pipe closed". Test-only; the TV never uses it.
package webtest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// Chromium returns BDTV_TEST_CHROMIUM, else the newest Playwright download,
// else "" (callers skip, saying why).
func Chromium() string {
	if p := os.Getenv("BDTV_TEST_CHROMIUM"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	m, _ := filepath.Glob(filepath.Join(home, ".cache", "ms-playwright", "chromium-*", "chrome-linux*", "chrome"))
	sort.Strings(m)
	if len(m) == 0 {
		return ""
	}
	return m[len(m)-1]
}

var (
	once   sync.Once
	failed string
)

// Require returns the test Chromium, skipping when none is installed and
// failing (not skipping) when one is installed but cannot start.
func Require(t *testing.T) string {
	t.Helper()
	bin := Chromium()
	if bin == "" {
		t.Skip("no Playwright Chromium: run `npx playwright install --with-deps chromium` in apps/web-nav or set BDTV_TEST_CHROMIUM (not a pass)")
	}
	once.Do(func() { failed = startProblem(bin) })
	if failed != "" {
		t.Fatalf("test Chromium %s cannot start (missing system libraries? run `npx playwright install --with-deps chromium` in apps/web-nav):\n%s", bin, failed)
	}
	return bin
}

// startProblem runs Chromium once headless on a blank page and returns the
// tail of its output when it fails, or "" when it starts.
func startProblem(bin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--headless=new", "--no-sandbox", "--disable-gpu", "--dump-dom", "about:blank").CombinedOutput()
	if err == nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}
	return err.Error() + "\n" + strings.Join(lines, "\n")
}
