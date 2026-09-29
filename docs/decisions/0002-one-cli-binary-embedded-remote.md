# ADR 0002 — One CLI binary with subcommands; embedded phone remote

Date: 2026-09-22. Status: accepted.

`cmd/bear-den-tv` carries `session`, `dev`, `doctor`, `pair`, `devices`, `remote`, `config`, `autostart`, and `uninstall` subcommands; `cmd/bear-den-tv-session` is a thin alias so the spec's two entry points both exist without duplicating logic. The compiled remote (`apps/remote-web/dist`) is committed and embedded with `//go:embed` so `go build` does not require Node and the shipped binary serves assets from memory; `make web` regenerates the directory deterministically (esbuild, minified, no sourcemaps).

**Update (current tree).** The subcommands today are `session`, `dev`, `doctor`, `pair`, `devices`, `remote`, `autostart`, `shortcut`, `apps`, `themes`, `weather`, `plex`, `badges`, `version` and `help` ([`cmd/bear-den-tv/main.go`](../../cmd/bear-den-tv/main.go)). `config`, `uninstall` and the `cmd/bear-den-tv-session` alias do not exist; the only other binary is `cmd/bdtv-probe`, a read-only probe report. (`artwork fetch`, which cached Flathub icons nothing read any more, was removed on 2026-09-29; see ADR 0012.) The embedding lives in [`embed.go`](../../embed.go), which also embeds `contracts/`, `themes/` and the web apps' navigation script (`apps/web-nav/dist`).
