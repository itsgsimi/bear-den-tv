# ADR 0010 — Web apps: Flathub Chromium driven over the DevTools pipe

Date: 2026-09-28. Status: accepted. **Update 2026-09-29:** "Chromium, and
only that" is amended by [ADR 0013](0013-brave-as-a-browser-choice.md): the
owner may choose Brave from Flathub for the Browser tile and, marked
unverified for streaming, for the streaming sites. Chromium stays the
default and everything below still holds for it.

Bear Den replaces a smart TV that stays offline. Netflix, Disney+ and Hulu
have no Linux apps; in a Linux browser they play, capped (Netflix and Disney+
at about 720p, Hulu possibly lower). We accept that cap and say it on the
tiles. This ADR decides how Bear Den runs those sites, a plain Browser tile,
and the phone's touchpad, and how it controls a browser without opening a
control channel anyone else could use.

## The browser: Chromium from Flathub

`org.chromium.Chromium`, and only that (not Google Chrome, not a system
package). Every web app is the same Flatpak started with its own profile
(`--user-data-dir=$XDG_DATA_HOME/bear-den-tv/web/<app-id>`), its own window
class (`--class=BearDenWeb-<adapter>`, how the coordinator tells the windows
apart), full screen in app mode (`--start-fullscreen --app=<url>`) for the
services, and as an ordinary maximized browser for the Browser tile.

### Widevine (researched; seen in a container, not on the TV)

The services need the Widevine CDM. What the Flathub packaging shows
(github.com/flathub/org.chromium.Chromium, read 2026-09-28):

- the build sets `enable_widevine=true` (`build-aux/bootstrap.sh`);
- a Flathub patch, `flatpak-Expose-Widevine-into-the-sandbox.patch`, exposes
  the **component-updated** Widevine directory (`DIR_COMPONENT_UPDATED_WIDEVINE_CDM`,
  i.e. `<user-data-dir>/WidevineCdm`) into Chromium's zygote sandbox;
- the manifest does not bundle Widevine; the maintainers have discussed a
  separate `org.chromium.Chromium.Widevine` extension (issue #193).

So Flathub Chromium is built to use a Widevine CDM that Chromium's component
updater downloads into the profile at run time. Community reports on whether
that download happens for this unbranded build are mixed. Nothing may be
installed on the development machine and the TV was not used, but a
container was (2026-09-28, [ADR 0011](0011-per-user-flathub-installs.md)):
in `ubuntu:24.04`, as an unprivileged user, Flathub Chromium 154.0.8037.57
installed with `bear-den-tv apps install netflix --here` fetched Widevine
4.10.3050.0 into a fresh profile (`<profile>/WidevineCdm/4.10.3050.0/manifest.json`)
65 s after starting, both with `--headless=new` and in a window on Xvfb.
Whether the sites then play was not tried (no real site may be automated).
Consequences:

- Bear Den never downloads or copies Widevine itself.
- Each web app's profile gets its own `WidevineCdm` folder when Chromium
  fetches one (`chrome://components` → "Widevine Content Decryption Module"
  shows a version other than 0.0.0.0).
- What Widevine setup Flathub Chromium needs, for a future installer: none
  beyond running it. After `flatpak install --user flathub
  org.chromium.Chromium` (per user, no root), Chromium's component updater
  is expected to download the CDM into each profile's `WidevineCdm/<version>/`
  the first time that profile runs online (a few minutes; the
  `chrome://components` page's "Check for update" forces it). That is per
  user and needs no root. Bear Den automates exactly that
  (`web.Widevine`, `internal/session/widevine.go`): after Chromium is
  installed, and whenever a streaming site is turned on, it starts that
  site's profile once, headless (`--headless=new --no-first-run
  --no-default-browser-check about:blank`, no DevTools channel), waits up to
  5 minutes for `<profile>/WidevineCdm/*/manifest.json`, the one
  machine-checkable sign, and stops it. The state says `drm`: `preparing`,
  `ready`, or `pending` ("Still setting up playback support"); opening the
  site stops the quiet run and the real run fetches it instead. Nothing Bear
  Den ships downloads or copies the CDM itself (Google's licence; the
  community scripts that fetch Google Chrome's copy into the Flatpak's folder
  are not used). Seen in the container only, as above.
- Chromium missing is machine-readable, not only UI text: every web app's
  `launch.app_id` is `org.chromium.Chromium` (defined once,
  `adapters.ChromiumFlatpakID`), so discovery reports it in
  `state.applications[].installed: false` and `installation: "none"` for the
  four web apps, and `hidden: true` while their rows carry
  `hide_when_missing`.
- If Chromium on the TV never gets it, the streaming tiles open the sites but
  playback fails: the status says **streaming needs Widevine and is not
  verified**, and the Browser tile still works. We do not switch to Google
  Chrome.

## The control channel

Options considered, against these criteria: nothing listens on the LAN; no
unauthenticated local channel into a logged-in streaming session; phones
send only named actions (never URLs, scripts or selectors); page addresses
live in the owner's config on the TV.

| Option | For | Against |
|---|---|---|
| **CDP over `--remote-debugging-pipe`** | Two anonymous fds (Chromium reads fd 3, writes fd 4) that only the coordinator and its child hold: no socket, no port, no token to steal. Full DevTools: isolated worlds, trusted `Input.*` events, per-page sessions. | The coordinator must start Chromium itself and keep the pipe; a coordinator restart loses control of a running browser (it has to be reopened). Must survive `flatpak run`. |
| CDP over a loopback port | Easy to attach from anywhere | Any local process, and any Flatpak with network access, can connect and drive a signed-in session (read cookies, navigate, type). Mitigations (random port, `--remote-allow-origins`) do not authenticate. Only with a strong reason: we have none. |
| Bundled extension via `--load-extension` + an authenticated channel | Works without DevTools | Needs its own channel to the coordinator (native messaging from inside the Flatpak, or a loopback socket with a token the extension must read from disk); synthetic events are untrusted (`isTrusted` false), so sites may ignore keys and clicks; more moving parts. |

**Decision: CDP over `--remote-debugging-pipe`.**

Does the pipe survive `flatpak run`? From the source of Flatpak 1.14
(`common/flatpak-run.c`): a foreground `flatpak run` ends in
`flatpak_bwrap_child_setup(bwrap->fds, FALSE)` and `execvpe(bwrap)`, with
the comment "this does not close fds that are not already marked
O_CLOEXEC, because we do want to allow inheriting fds into flatpak run".
Bubblewrap passes inherited fds to its child (checked here:
`bwrap … ls -l /proc/self/fd 3</dev/null 4>/dev/null` lists 3 and 4). The
Flathub wrapper (`chromium.sh`) `exec`s `cobalt`, which execs Chromium, so
fds 3 and 4 reach the browser process. **Not verified end to end**: no
Flatpak app is installed on this machine and none may be. The same code path
(Go `exec.Cmd.ExtraFiles` → fds 3 and 4, `--remote-debugging-pipe`) is tested
against Playwright's Chromium started directly.

## How the page is driven

- The coordinator attaches to every page (`Target.setAutoAttach`, flat
  sessions) and injects the navigation script (`apps/web-nav`, embedded in
  the binary) with `Page.addScriptToEvaluateOnNewDocument` into an **isolated
  world** named `bearden`. The site's own scripts cannot see it; the DOM is
  shared. Status comes back through `Runtime.addBinding` limited to that
  world (`executionContextName`), and reports from any other context are
  ignored, so a page cannot forge them.
- A phone's named action becomes a fixed expression
  (`__bdtv.apply("nav.left")`, built from a closed switch; an integer for a
  seek). The script moves its own focus ring (spatial navigation) and reports
  where focus landed: that is `observed`.
- Anything that needs trusted input comes back as an effect that Bear Den
  checks and performs with `Input.dispatchMouseEvent`/`dispatchKeyEvent`/
  `insertText`: a click inside the viewport (Chromium's own size, not the
  page's claim) for OK or Back, Escape for Back, keys from a closed set
  (`space k j l f ← → Escape Enter`) for media, the phone's text into a
  focused field. Anything else is refused. Clicks and keys are `delivered`;
  pause and play are `observed` only when the page then reports the video
  paused or playing.
- Media uses each site's documented shortcuts through per-site hint files
  (data, `contracts/web-hints.schema.json`), never `currentTime` (Netflix
  errors on direct seeks). The hints for Netflix, Disney+ and Hulu are
  UNVERIFIED: no real site may be automated. The generic behaviour works
  without hints.
- Input is sent only after re-reading the active window and finding the web
  app's own window there (the rule XTEST keys follow).
- The touchpad (`pointer.move|click|scroll`) moves a cursor Bear Den keeps
  inside the viewport, with trusted mouse events; web adapters only, rate
  limited per phone (60 moves, 30 scrolls, 5 clicks a second), never on a
  guest pass.
- Home: if the page reports a playing `<video>`, the site's pause key, then
  the shell comes forward.

## Consequences

- Nothing new listens anywhere; the pipe is the only control channel.
- Profiles hold sign-ins: they live under the owner's data directory, one per
  app, and are never exported or sent to phones.
- The coordinator owns the Chromium process: `app.close` asks it to close
  over the pipe; `force` ends the process group Bear Den started.
- Now playing reads Chromium's MPRIS player (its desktop entry is the
  Flatpak id, UNVERIFIED on the TV); media control goes through the page.
  *Update 2026-09-29:* the player is now the one whose owning process
  descends from the browser Bear Den started for that web app
  (`web.Manager.PID`, `platform.MediaMatch.ProcessRoot`), never matched by
  the shared Flatpak id or desktop entry ([`docs/security.md`](../security.md)).
- Not verified: the pipe through `flatpak run`, the window class on Wayland
  (`--class` is an X11 switch), Widevine, the real sites, and performance on
  the 2-core box.
