// Package bdtv is the module root. It embeds the shared contracts, the built
// phone remote and the web apps' navigation script so the coordinator binary
// is self-contained.
package bdtv

import "embed"

// Contracts holds every JSON Schema and fixture under contracts/.
//
//go:embed contracts/*.json contracts/fixtures/*.json
var Contracts embed.FS

// WebDist holds the compiled phone remote (apps/remote-web/dist). Build it with
// `make web`; a committed placeholder keeps `go build` working without Node.
//
//go:embed all:apps/remote-web/dist
var WebDist embed.FS

// WebNav holds the navigation script the coordinator injects into web apps
// (apps/web-nav/dist/nav.js, built with `make webnav`) and its per-site
// hint files (apps/web-nav/hints/*.json, contracts/web-hints.schema.json).
//
//go:embed apps/web-nav/dist/nav.js apps/web-nav/hints/*.json
var WebNav embed.FS

// Themes holds the built-in theme packages (themes/<id>/theme.json and art).
// The TV shell compiles the same folder in; see docs/THEMES.md.
//
//go:embed all:themes
var Themes embed.FS

// Ornaments holds the built-in ornaments themes may name (served to phones):
// <name>.svg or <name>.png. The whole folder is embedded so the pattern works
// whichever of the two kinds exist; internal/themes only resolves and serves
// .svg and .png files from it.
//
//go:embed apps/tv-shell/assets/ornaments
var Ornaments embed.FS
