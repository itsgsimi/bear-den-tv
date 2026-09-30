// Package copy_test keeps the words on the TV and the phone plain (the
// honest-UX rule; UX audit 2026-09-29, UX-09, UX-15, UX-22): no translated
// string in the TV shell's QML (qsTr) or in the phone's copy
// (apps/remote-web/src/i18n.ts) may use the builders' words for things:
// the coordinator, MPRIS, the context epoch, IPC, HTTP(S), GPU or CPU.
// Codes and raw values shown for troubleshooting are data, not copy, and
// are not checked here.
package copy_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// jargon are whole words that must not appear in on-screen copy.
var jargon = regexp.MustCompile(`\b(?i:coordinator|mpris|epoch|ipc|https?|gpu|cpu)\b`)

var (
	qsTr      = regexp.MustCompile(`qsTr\("((?:[^"\\]|\\.)*)"`)
	tsLiteral = regexp.MustCompile("'((?:[^'\\\\]|\\\\.)*)'|`((?:[^`\\\\]|\\\\.)*)`")
)

func check(t *testing.T, file string, strs []string) {
	t.Helper()
	for _, s := range strs {
		if m := jargon.FindString(s); m != "" {
			t.Errorf("%s: %q says %q; say it in plain words", file, s, m)
		}
	}
}

func TestTVCopyIsPlain(t *testing.T) {
	files, err := filepath.Glob("../../apps/tv-shell/qml/*.qml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no QML found: %v", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var strs []string
		for _, m := range qsTr.FindAllStringSubmatch(string(b), -1) {
			strs = append(strs, m[1])
		}
		check(t, filepath.Base(f), strs)
	}
}

func TestPhoneCopyIsPlain(t *testing.T) {
	b, err := os.ReadFile("../../apps/remote-web/src/i18n.ts")
	if err != nil {
		t.Fatal(err)
	}
	var strs []string
	for _, line := range strings.Split(string(b), "\n") {
		code := line
		if i := strings.Index(code, "//"); i >= 0 && !strings.Contains(code[:i], "'") && !strings.Contains(code[:i], "`") {
			code = code[:i]
		}
		if strings.HasPrefix(strings.TrimSpace(code), "*") || strings.HasPrefix(strings.TrimSpace(code), "/*") {
			continue
		}
		for _, m := range tsLiteral.FindAllStringSubmatch(code, -1) {
			s := m[1] + m[2]
			// Identifiers and keys ('https', 'trusted-lan-http') compare
			// values; only sentences are copy.
			if !strings.Contains(s, " ") {
				continue
			}
			strs = append(strs, s)
		}
	}
	if len(strs) < 50 {
		t.Fatalf("found only %d phone strings; the scan is broken", len(strs))
	}
	check(t, "i18n.ts", strs)
}
