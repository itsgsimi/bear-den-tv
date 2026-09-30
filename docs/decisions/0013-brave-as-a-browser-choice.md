# ADR 0013 — Brave as a browser choice for the web apps

Date: 2026-09-29. Status: accepted (owner decision). Amends
[ADR 0010](0010-web-apps-over-cdp-pipe.md) ("Chromium, and only that").
**Update 2026-09-29 (later):** [ADR 0014](0014-google-chrome-for-streaming-brave-for-browser.md)
removes Chromium: the choice is now Google Chrome or Brave, with Brave the
Browser tile's default and Chrome the streaming sites'; everything below
about Brave still holds.

The owner wants Brave as an alternative to Chromium for the web apps. This
ADR decides how, without weakening ADR 0010's control channel or ADR 0011's
one-press, no-root installs.

## What we know about Brave on Flathub (read 2026-09-29)

- `com.brave.Browser` on Flathub is published by Brave Software (verified
  publisher, brave.com). Brave itself says on brave.com/linux that the
  Flatpak "is not yet working as well as our native packages", that "it
  modifies Chromium sandboxing in ways which have not been vetted by the
  Brave or Chromium security teams", and recommends its native packages to
  those who can use them. Those need root and a system package source, so
  they cannot be a one-press install (ADR 0011); Bear Den offers the
  Flatpak and says what it is.
- The Flatpak's wrapper (`brave.sh`) ends in `exec cobalt "$@"
  --no-default-browser-check`; cobalt prepends `zypak-wrapper.sh`, and
  zypak execs Brave. None of them closes inherited descriptors, so fds 3
  and 4 (the DevTools pipe) should reach Brave as they reach Chromium.
  **Not verified end to end**: no Flatpak may be installed here.
- Brave's help center lists every switch Bear Den uses
  (`--remote-debugging-pipe` "[in=3, out=4]", `--user-data-dir`, `--class`,
  `--app`, `--start-fullscreen`, `--no-first-run`,
  `--no-default-browser-check`; `--start-maximized`, used for the Browser
  tile, is a Chromium switch not on that page); brave-core only adds its
  welcome page on a first run, which `--no-first-run` turns off. The argv
  is the same as Chromium's.
- The Flatpak's finish-args give it neither `home` nor `xdg-data`: a
  profile under `$XDG_DATA_HOME/bear-den-tv` would be invisible inside it.
- Widevine is opt-in in Brave on Linux: the Local State pref
  `brave.widevine_opted_in` (brave-core `kWidevineEnabled`,
  `components/constants/pref_names.h`, registered in
  `browser/widevine/widevine_utils.cc`); once set, Brave's component
  updater fetches the CDM into `<user-data-dir>/WidevineCdm`. But the
  Flatpak's launcher exposes Widevine to the sandbox only from the default
  profile's folder (`~/.var/app/com.brave.Browser/config/BraveSoftware/Brave-Browser/WidevineCdm`,
  cobalt `ExposeWidevine`), not from a `--user-data-dir`. flathub/com.brave.Browser
  issue #357 ("Widevine not detected even though Widevine is Enabled in
  settings") was closed as fixed in 2024 for the default profile.
- Brave's own features (Rewards, Wallet, VPN, Leo, News) are switched off
  by enterprise policies only from `/etc/brave/policies/managed` (root; the
  Flatpak links the host's `/etc/brave/policies` in). There is no per-user
  policy path. Brave's profile prefs for their buttons and prompts are
  named in brave-core's `pref_names.h` files.

## Decision

- **One table.** Browsers are rows in `internal/applications/adapters`
  (`Browsers()`: name, label, Flatpak id, profile root, whether `flatpak
  run` must grant the profile root, whether streaming is unverified, and
  the prefs Bear Den writes). Chromium stays the default; Brave is the
  second row. The installer may install every table browser
  (`InstallableFlatpakIDs`), nothing else.
- **The setting.** Config `apps.browser` (the Browser tile) and
  `apps.streaming_browser` (Netflix, Disney+, Hulu), both `chromium` when
  absent. Rule 3: each web row's `launch.app_id` is its browser's Flatpak
  id, so discovery, installs, launches, icons and Widevine follow the row
  and nothing else branches on the browser. IPC `apps.browser` stores both
  and moves the rows in one config write. TV: Settings → Streaming sites,
  "Browser tile uses" and "Streaming sites use".
- **Streaming stays on Chromium by default**, and choosing Brave for it
  shows "Unverified for streaming: the sites may not play in Brave
  (Widevine)" on that row.
- **Profiles.** Each browser has its own profile root
  (`$XDG_DATA_HOME/bear-den-tv/web/` for Chromium as before,
  `web-brave/` for Brave): no profile is opened by two browsers. For Brave,
  `flatpak run --filesystem=<that root>` grants the sandbox that folder only,
  for that run only (a path with `:` is refused).
- **Prefs, only in Bear Den's profiles.** Before every start (a web app or
  the quiet Widevine run) Bear Den writes the row's prefs into the
  profile's `Local State` and `Default/Preferences`, keeping every other
  key, and refuses a profile, profile root or file that is a symbolic link.
  For Brave: `brave.widevine_opted_in` and `brave.p3a.notice_acknowledged`
  (Local State); the welcome page seen, the full-screen reminder, and the
  VPN, Wallet, Leo, Rewards and News buttons and new-tab widgets off, and
  the Widevine infobar off (Preferences). Nothing for Chromium.
- **Widevine.** The existing preparation runs in the chosen browser and
  checks `<profile>/WidevineCdm/*/manifest.json`. For Brave that file may
  appear while the Flatpak still cannot load it (above), so "ready" means
  only that the CDM was fetched; the row keeps its unverified note.

## Consequences

- The pipe, the navigation script and the closed input set are unchanged:
  nothing new listens, and phones still send named actions only.
- Unverified until seen on a TV: the pipe through Brave's `flatpak run`
  (zypak), the window class, the kiosk prefs' effect (Brave's
  default-browser prompt was reported in 2022 to ignore
  `--no-default-browser-check`, brave/brave-browser#27146), whether Brave
  fetches Widevine headless after the opt-in, and whether any streaming
  site plays in Brave. Now playing does not follow a web app into Brave
  (its MPRIS match is Chromium's).
- Moving a web app to another browser starts it with an empty profile: the
  sign-ins stay in the old browser's profile, which is kept.
- Brave's Flatpak sandbox caveat is recorded in
  [`security.md`](../security.md); the owner chooses it knowingly.
