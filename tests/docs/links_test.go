// Package docs_test keeps the documentation's links true: every relative link
// in the repository's own Markdown (AGENTS.md guides, docs/, contracts/,
// READMEs) must point at a file that exists, and every #anchor into a Markdown
// file must match one of its headings (GitHub's slug rules). The AGENTS.md
// guides are how people and coding agents find their way around, so a moved
// file or a renamed heading fails the build instead of silently rotting.
package docs_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// skipDirs are generated, vendored or externally provided trees.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "build": true, "dist": true, "toolchain": true,
}

var link = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func markdownFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// slug is GitHub's heading anchor: lower case, keep letters, digits, spaces,
// hyphens and underscores, then spaces become hyphens.
func slug(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

// anchors lists the heading slugs of a Markdown file, skipping fenced code.
func anchors(t *testing.T, path string) map[string]bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string]bool{}
	seen := map[string]int{}
	fenced := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced || !strings.HasPrefix(line, "#") {
			continue
		}
		s := slug(strings.TrimSpace(strings.TrimLeft(line, "#")))
		if n := seen[s]; n > 0 {
			out[s+"-"+strconv.Itoa(n)] = true
		}
		seen[s]++
		out[s] = true
	}
	return out
}

func TestRelativeLinksResolve(t *testing.T) {
	root := repoRoot(t)
	cache := map[string]map[string]bool{}
	checked := 0
	for _, file := range markdownFiles(t, root) {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		fenced := false
		for n, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				fenced = !fenced
				continue
			}
			if fenced {
				continue
			}
			for _, m := range link.FindAllStringSubmatch(line, -1) {
				target := m[1]
				if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
					continue
				}
				pathPart, frag, _ := strings.Cut(target, "#")
				dest := file
				if pathPart != "" {
					dest = filepath.Join(filepath.Dir(file), pathPart)
				}
				rel, _ := filepath.Rel(root, file)
				where := rel + ":" + strconv.Itoa(n+1)
				if _, err := os.Stat(dest); err != nil {
					t.Errorf("%s: link %q: %s does not exist", where, target, dest)
					continue
				}
				checked++
				if frag == "" || !strings.HasSuffix(dest, ".md") {
					continue
				}
				if cache[dest] == nil {
					cache[dest] = anchors(t, dest)
				}
				if !cache[dest][frag] {
					t.Errorf("%s: link %q: no heading with anchor #%s in %s", where, target, frag, dest)
				}
			}
		}
	}
	if checked < 50 {
		t.Fatalf("only %d links checked; is the walk finding the docs?", checked)
	}
}

func TestSlugFollowsGitHub(t *testing.T) {
	for heading, want := range map[string]string{
		"Extending the engine (new building blocks)": "extending-the-engine-new-building-blocks",
		"Built for the floor, scaled up":             "built-for-the-floor-scaled-up",
		"`apps/tv-shell` — the TV":                   "appstv-shell--the-tv",
		"Box tiers":                                  "box-tiers",
	} {
		if got := slug(heading); got != want {
			t.Errorf("slug(%q) = %q, want %q", heading, got, want)
		}
	}
}
