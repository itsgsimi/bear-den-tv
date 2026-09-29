#!/usr/bin/env bash
# The TV sandbox: the real TV shell running here, offscreen, on demo data, for
# prototyping without the TV. Change QML, rebuild (`make shell`, a few
# seconds), and look at the picture.
#
#   scripts/sandbox.sh shot [options]       one screenshot (prints its path)
#   scripts/sandbox.sh gallery [DIR]        every theme × main screens, plus Plain
#   scripts/sandbox.sh perf [perf options]  frames and CPU per phase (scripts/perf-sandbox.sh)
#
# shot options:
#   --screen NAME   home (default), settings, playback, advanced-playback,
#                   pairing, devices, diagnostics, remote-setup
#   --theme ID      any installed theme (themes/, or BDTV_THEMES_DIR), with its
#                   accent colour, as choosing it in TV Settings does
#   --no-weather    drop the demo's local weather (its rain replaces the
#                   theme's own particles)
#   --weather COND  local weather in the scene: clear, partly-cloudy, cloudy,
#                   fog, drizzle, rain, snow, thunder; add :night for night
#                   and :light or :heavy for the intensity (rain:heavy:night)
#   --apps-only     only the first rail (Your Apps), so Home has room for the
#                   theme's corner scene
#   --reduced-motion  the Motion setting's reduced motion (static scene)
#   --lightning     with --weather thunder: a flash every second
#                   (BDTV_LIGHTNING_SECONDS=1), so the startled bears show
#   --plain         the Plain style
#   --classic       the Classic art style (smooth art instead of pixel art)
#   --bears ACT     a bear visit: walk, peek, hop, parade, chase
#   --after MS      when to take it (default 2600: decorations fully grown)
#   --size WxH      window size (default 1920x1080; try 3840x2160 or 1280x720)
#   --fixture PATH  another state snapshot (default: the shell tests' demo)
#   --out PATH      where to write (default build/shots/<screen>-<theme>.png)
#
# Pictures are drawn by the CPU, not the TV's GPU: layout, colour and
# decorations match the TV; smoothness is judged on the TV.
set -euo pipefail
cd "$(dirname "$0")/.."
SHELL_BIN=build/bin/bear-den-tv-shell
FIXTURE=apps/tv-shell/tests/fixtures/state.demo.json

ensure_shell() { [ -x "$SHELL_BIN" ] || make -s shell >/dev/null; }

shot() {
  local screen=home theme="" plain="" classic="" weather=1 wx="" apps="" still="" lightning="" bears="" after=2600 size=1920x1080 fixture=$FIXTURE out=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --screen) screen=$2; shift 2 ;;
      --theme) theme=$2; shift 2 ;;
      --plain) plain=1; shift ;;
      --classic) classic=1; shift ;;
      --no-weather) weather=""; shift ;;
      --weather) wx=$2; shift 2 ;;
      --apps-only) apps=1; shift ;;
      --reduced-motion) still=1; shift ;;
      --lightning) lightning=1; shift ;;
      --bears) bears=$2; shift 2 ;;
      --after) after=$2; shift 2 ;;
      --size) size=$2; shift 2 ;;
      --fixture) fixture=$2; shift 2 ;;
      --out) out=$2; shift 2 ;;
      *) echo "unknown option $1 (see the header of $0)" >&2; exit 2 ;;
    esac
  done
  ensure_shell
  local tmp; tmp=$(mktemp --suffix=.json)
  python3 - "$fixture" "$tmp" "$theme" "$plain" "$weather" "${BDTV_THEMES_DIR:-}" "$classic" "$wx" "$apps" "$still" <<'EOF'
import json, os, sys
d = json.load(open(sys.argv[1]))
theme, plain, weather, user_dir, classic, wx, apps, still = sys.argv[3:11]
if theme:
    d["layout"]["ui"]["background"] = theme
    for base in (user_dir, "themes"):
        manifest = os.path.join(base, theme, "theme.json") if base else ""
        if manifest and os.path.isfile(manifest):
            accent = json.load(open(manifest)).get("accent")
            if accent: d["layout"]["ui"]["accent"] = accent
            break
if plain: d["layout"]["ui"]["theme"] = "plain-dark"
if classic: d["layout"]["ui"]["art_style"] = "classic"
if not weather: d.pop("weather", None)
if wx and "weather" in d:
    parts = wx.split(":")
    conds = ["clear", "partly-cloudy", "cloudy", "fog", "drizzle", "rain", "snow", "thunder"]
    if parts[0] not in conds or any(p not in ("night", "day", "light", "moderate", "heavy") for p in parts[1:]):
        sys.exit("--weather: CONDITION[:night][:light|:heavy], CONDITION one of " + ", ".join(conds))
    cur = d["weather"]["current"]
    cur["condition"] = parts[0]
    cur["is_day"] = "night" not in parts[1:]
    for p in parts[1:]:
        if p in ("light", "moderate", "heavy"): cur["intensity"] = p
    d["weather"]["scene"] = True
if apps:
    for i, s in enumerate(d["layout"]["sections"]): s["enabled"] = i == 0
if still: d["layout"]["ui"]["reduced_motion"] = True
json.dump(d, open(sys.argv[2], "w"))
EOF
  out=${out:-build/shots/$screen-${theme:-default}${plain:+-plain}${classic:+-classic}${wx:+-${wx//:/-}}${bears:+-$bears}.png}
  mkdir -p "$(dirname "$out")"
  env QT_QPA_PLATFORM=offscreen ${lightning:+BDTV_LIGHTNING_SECONDS=1} ${bears:+BDTV_BEARS_SECONDS=1 BDTV_BEARS_ACT=$bears} \
    "$SHELL_BIN" --dev --fixture "$tmp" --screen "$screen" --windowed --size "$size" \
    --screenshot "$out" --screenshot-after "$after" --exit-after $((after + 600)) 2>/dev/null
  rm -f "$tmp"
  [ -s "$out" ] || { echo "no screenshot written (is '$screen' a screen name?)" >&2; exit 1; }
  echo "$out"
}

gallery() {
  local dir=${1:-build/shots/gallery}
  ensure_shell
  local themes; themes=$(ls themes | grep -v '\.md$')
  for t in $themes; do
    for s in home settings; do shot --screen "$s" --theme "$t" --out "$dir/$s-$t.png" >/dev/null; done
  done
  shot --theme den --plain --out "$dir/home-den-plain.png" >/dev/null
  for s in playback advanced-playback pairing; do shot --screen "$s" --out "$dir/$s.png" >/dev/null; done
  ls "$dir"/*.png
}

case "${1:-}" in
  shot) shift; shot "$@" ;;
  gallery) shift; gallery "$@" ;;
  perf) shift; exec scripts/perf-sandbox.sh "$@" ;;
  *) sed -n '2,34p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
