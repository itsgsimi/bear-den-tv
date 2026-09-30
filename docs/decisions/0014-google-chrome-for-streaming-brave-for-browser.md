# ADR 0014 — Google Chrome for the streaming sites, Brave for the Browser tile

Date: 2026-09-29. Status: accepted (owner decision). Supersedes the browser
choice of [ADR 0010](0010-web-apps-over-cdp-pipe.md) ("Chromium, and only
that") and the defaults of [ADR 0013](0013-brave-as-a-browser-choice.md)
(Chromium the default, Brave the alternative). The control channel of ADR
0010 and the one-press installs of [ADR 0011](0011-per-user-flathub-installs.md)
are unchanged.

## What was seen on the TV

On the owner's TV (Linux Mint 21.3, package 0.1.0~git129.9ab5b20), Hulu in
Flathub Chromium (`org.chromium.Chromium` 154) failed with "Error playing
video, Hulu Error Code: RUNUNK13", although Widevine 4.10.3050.0 was in the
profile. The owner also reports Google's bot checks (captchas) in Chromium
and live streams that do not play. The services support Google Chrome, not
open-source Chromium builds (a plausible cause is that only Google's own
builds carry Widevine's verified media path; not proven here).

## What we know about Google Chrome on Flathub (read 2026-09-29)

- `com.google.Chrome` (github.com/flathub/com.google.Chrome) is
  community-packaged. Its manifest downloads Google's official `.deb`
  (`google-chrome-stable_154.0.8037.92-1_amd64.deb`, about 143 MB) as
  Flatpak *extra data* when it is installed, and `apply_extra.sh` unpacks
  `./opt/google/chrome` into `/app/extra`. So one Chrome is installed per
  user, downloaded and updated once, and shared by every streaming site.
- Google's package bundles the Widevine CDM (`WidevineCdm/` beside the
  browser). The Flatpak's launcher (`chrome.sh`) touches
  `${XDG_CONFIG_HOME}/google-chrome/WidevineCdm` as a **file**, so the
  default profile never gets a component-updated copy and Chrome keeps
  using the bundled one. **Not verified** why the packagers do it; we copy
  it for Bear Den's profiles.
- Finish-args: network, X11 and Wayland, PulseAudio, `--own-name=org.mpris.MediaPlayer2.chromium.*`,
  the xdg-download/documents/music/videos/pictures folders and host `/etc`,
  but neither `home` nor `xdg-data`: a profile under
  `$XDG_DATA_HOME/bear-den-tv` must be granted per run, as for Brave.
- `chrome.sh` ends in `exec cobalt "$@"` (zypak), like Chromium's and
  Brave's wrappers; none closes inherited descriptors, so fds 3 and 4 (the
  DevTools pipe) should reach Chrome. **Not verified end to end** (no
  Flatpak may be installed here).
- Chrome honours `--class` for an app window's WM_CLASS class on X11
  (`shell_integration_linux::GetProgramClassClass`), so each web app keeps
  its own class. On Wayland an `--app` window's app_id is derived from the
  site instead; not verified (the owner's TV runs X11).

## Decision

- **The browser table** (`internal/applications/adapters`, `Browsers()`)
  has two rows: **Google Chrome** (`chrome`, `com.google.Chrome`, profiles
  under `web-chrome/`) and **Brave** (`brave`, `com.brave.Browser`,
  `web-brave/`). **Chromium is removed**: not a row, not a config value,
  not installable, not in any test or doc as a choice.
- **Defaults.** `apps.streaming_browser`: `chrome` (absent = chrome);
  `apps.browser`: `brave` (absent = brave). Either may name either browser;
  streaming in Brave stays marked unverified.
- **Widevine for Chrome** is the bundled copy. A table row says so
  (`BundledWidevine`): the streaming sites are ready once
  `<flatpak installation>/app/com.google.Chrome/current/active/files/extra/WidevineCdm/manifest.json`
  exists (per user or system-wide), and no quiet headless run is started.
  Before every start Bear Den puts an empty file at `<profile>/WidevineCdm`
  (`BlockProfileWidevine`), as `chrome.sh` does for Chrome's default profile.
  Brave keeps its opt-in and quiet run (ADR 0013).
- **Prefs in Bear Den's Chrome profiles only** (`prefs.go`, the row's
  `Preferences`): no default-browser check, the welcome page seen, signing
  in to Chrome itself off (`signin.allowed`), no password, address or card
  saving, no translate bubbles, and a clean exit recorded so no "Restore
  pages?" bubble after a forced close. The sites' own sign-in is untouched.
  Never written into `~/.var/app/com.google.Chrome` or any other folder.
- **Notes are data.** Each row carries plain words the TV shows beside the
  choice (`state.apps.browsers[].notes`): for Chrome "Google Chrome, made by
  Google: it shares usage data with Google.", "Streaming sites play at up to
  720p on Linux.", "A community-packaged Flatpak of Google's official
  Chrome."
- **Each streaming site is its own app.** Its own profile, its own window
  class (`--class=BearDenWeb-<adapter>`), its own `flatpak run` (its own
  sandbox and process tree, which Now playing and Close follow), its own
  icon and notes; one Chrome install, no shared data.
- **Upgrading.** A `config.json` of the Chromium era (`apps.browser` or
  `apps.streaming_browser` `chromium`, web rows running
  `org.chromium.Chromium`) is moved when it is read: streaming rows to
  Chrome, the Browser tile to Brave (or to the browser the config already
  names when that is not Chromium), written once through the normal save
  path and logged (`config.Store.UpgradeBrowsers`). The old Chromium
  profiles in `$XDG_DATA_HOME/bear-den-tv/web/` are left in place (Chrome
  and Brave profiles differ; sign in again), and Chromium itself is never
  uninstalled by Bear Den ([`operations.md`](../operations.md#upgrading)
  says how to remove both).

## Consequences

- Google Chrome is Google's browser: it shares usage data with Google. The
  TV says so beside the choice; the owner chose it knowingly
  ([`security.md`](../security.md)).
- Streaming stays capped at about 720p on Linux.
- Not yet seen on the TV: the pipe through Chrome's `flatpak run`, the
  window class, the prefs' effect, the bundled CDM path, and whether Hulu,
  Netflix and Disney+ play in Chrome.
- Two streaming sites open at once run two Chrome sandboxes; each may try
  the same MPRIS bus name (Chrome names it after its own pid, and each
  sandbox has its own pid namespace), so the second site's Now playing may
  be missing. Not verified.
