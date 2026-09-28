# Bear Den TV
## Product design and implementation specification

**Version:** 0.1 — research-backed implementation proposal  
**Research date:** September 21, 2026  
**Initial installation:** The user's TV-connected mini PC running Linux Mint  
**Portability target:** Comparable, supported Linux desktop distributions  
**Primary goals:** Ease of use, customization, dependable couch control  
**Initial integrations:** Plex HTPC and VacuumTube  
**Required control surface:** A phone-friendly remote served on the local network

> Bear Den TV turns a Linux computer connected to a television into a welcoming, configurable TV experience. It owns the home screen, setup, remote control, and transitions between applications. Existing applications own Plex and YouTube browsing and playback.

**Status and evidence.** This is a design and implementation handoff, not an implemented application or a hardware-validation report. Third-party facts have numbered source references. Architecture, defaults, performance budgets, and acceptance criteria are proposed decisions. No installation, account connection, playback test, or remote-control test has been performed on the user's mini PC. No CPU, GPU, Mint release, desktop edition, or display session is assumed from other computers the user may own.

---

## 1. Product decisions

### 1.1 Build the experience around existing clients

Use **Plex HTPC** for the complete Plex experience and **VacuumTube** for YouTube's TV-style experience. Keep them independently installed and unmodified initially. Plex HTPC's published application listing describes a TV-oriented client; VacuumTube describes itself as an unofficial wrapper around YouTube Leanback. These are not interchangeable with Plex Desktop and a normal YouTube browser tab. [S01, S03]

Bear Den TV should not contain a new media player, a reconstructed YouTube client, a web scraper for personalized recommendations, or its own streaming-service login forms.

### 1.2 Make the LAN remote part of the first usable build

The phone interface is an operating control surface, not merely a settings page. It must navigate Bear Den TV and, through validated adapters, control the foreground Plex or YouTube application. It should remain reachable while a playback application covers the home screen and during a recoverable TV-shell crash.

No mobile application installation, cloud account, external server, or internet connection is required for local control. Playback and account features still depend on their respective services.

### 1.3 Start a dedicated project rather than forcing a Flex fork

**Recommendation:** Build a standalone Bear Den TV codebase. Flex Launcher remains a useful reference and a possible later configuration-import source, not the mandatory renderer or application architecture.

Flex uses an INI-configured launcher model, and its maintainer states that future feature development is moving to another project. That does not make Flex unusable; it means our content rows, phone interface, account-aware optional integrations, and application coordination are a materially different scope. [S05]

A fork remains reasonable only if an early implementation experiment demonstrates substantial reusable code without constraining the design. Do not spend the initial milestone reproducing Flex internals merely to preserve the word “fork.”

### 1.4 Compatibility means tested capabilities, not a universal promise

Separate three questions: can Bear Den TV run, can it launch the installed clients, and can it reliably control those clients and return Home? Support must be recorded by distribution, desktop, display session, client version, and packaging format.

The inspected Plex HTPC Flatpak manifest requests an X11 socket and comments out Wayland support. Plan for an X11 client or XWayland where necessary; this manifest is not proof of universal Wayland-session compatibility. [S02]

---

## 2. Scope and release boundaries

| Included in the initial product | Intentionally outside the initial product |
|---|---|
| TV home screen with configurable sections and favorites | A new Linux distribution or custom compositor |
| LAN browser remote with pairing and device revocation | Internet-facing remote access or a cloud relay |
| Fullscreen launch, app switching, and return Home | Replacement Plex or YouTube browsing/player engines |
| Guided Plex HTPC and VacuumTube setup | Service-password collection or extraction of client cookies |
| TV settings and a permission-limited web layout editor | Arbitrary shell commands or remote code execution |
| A restrained theme system and safe layout recovery | Executable third-party themes and a plugin marketplace |
| Optional real Plex artwork and library rows | Fabricated cross-service Continue Watching or YouTube history |
| Diagnostics, startup integration, and explicit recovery | Guaranteed HDR, audio passthrough, CEC, or power-on support |

The first vertical slice is a polished home screen plus a functioning phone remote and real application handoffs. The 1.0 target adds mature customization, packaging, reliability tests, and optional Plex home-screen content. Plex content is not required to complete onboarding or use either client.

The essential user journey is:

**Open the remote → choose Plex or YouTube → browse and play in that client → press Bear Den Home → return to the same place in the home screen.**

---

## 3. Visual and interaction design

### 3.1 Visual direction

Use the previously approved cinematic concept as a visual reference, with the product name changed in the actual implementation to **Bear Den TV**. Its artwork-heavy composition, dark background, prominent selection treatment, and horizontal sections are the intended direction. The reference image is not a product screenshot, and the extra services and media shown in it are not implemented integrations.

The identity should feel warm and quiet: charcoal surfaces, an optional forest-green accent, a restrained bear/den mark, and readable typography. Colors are theme tokens, not hard-coded throughout components. Avoid promotional placements in Bear Den TV itself; this makes no promise about advertising inside external clients.

Google's TV navigation guidance emphasizes directional navigation and predictable movement between navigation and content. Adopt those interaction principles without copying proprietary assets or attempting a pixel-identical clone. [S06]

### 3.2 Default home screen

| Region | Design and behavior |
|---|---|
| Header | Bear Den TV, Home, Apps, and Settings; optional Search and local profile switcher |
| Featured area | Optional large artwork panel; personal wallpaper before a content source is connected |
| Favorites | Plex and YouTube prominent; reorderable additional approved apps |
| Plex Continue Watching | Real server-backed items when the optional connector is configured |
| Plex Recently Added | Real library items; separate sections can target different libraries |
| Custom sections | Pinned applications, a selected Plex collection, or supported saved links |
| Status | Unobtrusive clock, connection status, and a Remote/Pair Device shortcut |

Do not duplicate Plex and YouTube as both mandatory header tabs and enormous app tiles. Default to simple navigation. A configurable top shortcut can launch either client directly; it must not imply that Bear Den TV has recreated its internal interface.

An empty content source should produce an honest setup card or hide the section according to settings. Do not populate production screens with demo movies. Demo fixtures belong behind a clearly labeled development mode.

### 3.3 Couch-distance usability

Use a logical 1920 × 1080 design reference, responsive layout, and correct device-pixel-ratio handling rather than fixed physical pixels. Initial design values are 28-pixel body labels, 40–48-pixel section headings, and a 64–76-pixel hero title at that logical reference size. Validate them from the actual couch.

Provide a text-size control, tile-density control, reduced motion, high-contrast focus, and adjustable edge-safe margins. A proposed starting margin is 3% of screen width/height, adjustable without changing the desktop resolution. Keep required controls inside the safe region.

Selection uses an outline, a small scale change, and readable labeling—not color alone. Initial transitions should be roughly 120–180 milliseconds and interruptible. No animation may delay the next navigation command. Do not auto-advance a hero while it is focused.

### 3.4 Focus and navigation contract

Each section remembers its selected item and horizontal scroll position. Up/Down changes sections; Left/Right changes items. Returning from an app restores section, item, and scroll position using stable identifiers rather than list indexes.

Back reverses one navigation level. **Bear Den Home always targets Bear Den TV**, not the external application's own home page. Close Application is a separate, explicit command. Back at Bear Den's root does not unexpectedly terminate the shell.

The focus graph must cover settings, modal dialogs, errors, missing artwork, empty rows, and loading states. A content refresh must not move focus to a different item merely because ordering changed. Modal focus returns to its initiating control.

### 3.5 Customization without configuration-file editing

Ship one well-finished layout with configurable sections before adding alternate layout engines. Let the user change section order, visibility, tile sizes, labels, artwork source, accent, background, motion, and safe margins.

TV settings cover common changes with directional buttons. The web layout editor supports larger reorganizations and text entry, with Preview, Apply, Undo, and Reset Section. A draft is not active configuration. Changes that could make navigation unusable require a timed confirmation and automatically roll back when unconfirmed.

Profiles initially mean local appearance and layout preferences. They are **not** authenticated Plex/Google account switching, OS isolation, or parental controls. Do not present a child-safe mode until the external clients' account and restriction behavior is actually integrated and tested.

---

## 4. Recommended architecture

### 4.1 Technology decisions

| Component | Proposed implementation | Reason |
|---|---|---|
| TV shell | Qt 6 / Qt Quick / QML, with a small C++ host | Native visual interface with explicit focus and animation control |
| Session coordinator | Go executable, running as the logged-in desktop user | Application lifecycle, LAN service, authorization, state, and adapters in one component |
| Browser remote/editor | TypeScript, HTML, and CSS compiled into static assets | Ordinary phone browser; no Node service on the mini PC |
| HTTP and events | Go `net/http` plus a maintained WebSocket library | One local web endpoint and bidirectional state updates |
| Local IPC | Versioned JSON messages over a Unix-domain socket | No network exposure of privileged desktop operations |
| Configuration | Versioned JSON and a JSON Schema | Portable, inspectable, validated settings |
| Persistent state | Small SQLite database owned only by the coordinator | Pairing records, recent apps, and optional metadata cache |
| Secrets | Desktop Secret Service where available | Keep connector tokens out of settings and browser payloads |

Qt Quick documents model/view, input, animation, and C++ extension facilities. Go's standard HTTP and embedding packages can serve compiled web assets directly. These capabilities support the choice; no performance comparison has been measured on this machine. [S07, S09, S10]

The tradeoff is two implementation languages plus web UI code. Keep responsibilities strict: QML contains presentation, Go contains coordination and policy, and TypeScript contains the phone experience. No business logic should be reimplemented in all three.

Use a supported, pinned Qt release rather than requiring whatever version is newest. `github.com/coder/websocket` is a concrete WebSocket-library candidate; `godbus/dbus` and `jezek/xgb` are candidates for D-Bus and X11 integration. Pin and audit versions in the implementation milestone. [S11, S12, S13]

Do not add Qt HTTP Server casually: its published open-source license is GPLv3 rather than the same LGPL option documented for Qt Quick. Keeping HTTP in Go avoids making that module an accidental dependency. Review the actual distribution and dependency licenses before shipping; this document does not settle the project's license obligations. [S07, S08]

### 4.2 Runtime shape

```text
Phone / tablet / laptop browser
          │ HTTP(S) + WebSocket on the selected LAN interface
          ▼
Bear Den session coordinator — same desktop user, never root
  ├── Pairing, authorization, action router, and web assets
  ├── Configuration, state store, optional Plex content adapter
  ├── Application lifecycle and desktop-session adapters
  └── Private Unix socket
          │
          ▼
      Qt Quick TV shell
          │
          └── Visible home screen, focus, setup, and settings

Coordinator launches / controls independent applications:
  ├── Plex HTPC
  └── VacuumTube
```

These are two long-running local processes, not a collection of microservices. A CLI entry point can share the coordinator binary. There is no Docker, Redis, reverse proxy, or separate database server requirement. HTTPS can be served by the coordinator itself when a trusted certificate is configured.

### 4.3 State and ownership

The coordinator owns connected-device authorization, active-app routing, launch jobs, configuration revision, and session capability reports. The shell owns its actual rendered focus state and reports changes back. An external app remains authoritative for playback state.

Every foreground-app change increments a `context_epoch`. Commands directed at the current app carry the epoch they were created against. A stale command must be rejected rather than accidentally applied to a newly focused application.

The local socket lives under `$XDG_RUNTIME_DIR/bear-den-tv/`, with owner-only directory/socket permissions and same-user peer checks. Use a protocol-version handshake, bounded message size, request identifiers, and explicit disconnect handling. It is not a tunnel for arbitrary LAN-provided desktop calls.

### 4.4 Session lifetime and recovery

Start the coordinator from the graphical user's session, then start or connect the shell. A per-user single-instance lock prevents duplicate coordinators. The coordinator survives a shell crash and can restart it without killing an independently running player.

Use XDG autostart as the portable entry mechanism. Where supported, integrate a user service with the graphical-session lifecycle. Do not assume every desktop activates the same systemd targets or that a login manager exports the same environment. [S24, S25]

Import only required display/session environment. Stop remote input on session lock or logout. Do not implement remote unlocking. In normal desktop mode, exit means exit; restart supervision must not trap the user in the application.

---

## 5. LAN browser remote: required first-class feature

### 5.1 What the phone shows

The default page has the room/device name, connection status, and a conspicuous **Controlling: Bear Den TV / Plex / YouTube** indicator. Place a large directional pad and Select button centrally, with Back and Bear Den Home always visible.

Below it, show Plex and YouTube launch shortcuts, playback buttons, volume/mute where supported, and a text-entry button. The layout editor is a separate tab, not a cluttered collection of settings around the D-pad. Use accessible button labels, generous touch targets, and portrait/landscape layouts.

Only show actionable capabilities. For example, a timeline requires a trustworthy playback position; a volume slider requires a working volume backend; typing into an external search box requires a validated text-entry adapter. Unknown playback state is labeled unknown rather than represented by a guessed progress bar.

The remote is **not a desktop video stream**. No screen capture, VNC server, or remote rendering is needed for the default design.

### 5.2 Discovery and onboarding

Installation starts without a LAN listener. During first-run setup, explain local network control and enable it after the user selects a network interface and transport mode. This is a consent step, not an optional afterthought in implementation scope.

Show a QR code, a literal IP-based address, and an optional `.local` hostname. Suggested example addresses are `http://<mini-pc-ip>:8090/` in trusted-LAN mode or `https://<configured-host>:8090/` in HTTPS mode. These are proposed formats, not running endpoints.

Advertise an HTTP service through mDNS when the host supports it. The browser does not perform arbitrary mDNS discovery itself; discovery is handled by the host/network, with the QR/IP fallback always present. Do not require editing a router, opening an internet port, or installing an app on the phone.

A friendly room name is separate from the computer's hostname. Handle hostname collisions and changing addresses explicitly. Each TV is paired independently; never share device tokens across origins.

### 5.3 Pairing and revocation

Opening the site does not confer control. The TV issues a short-lived pairing invitation. A QR flow may carry a single-use random invitation in the URL fragment; remove it from the address immediately after redemption. The fallback is a six-digit code displayed on the TV and entered on the phone.

Proposed limits are a two-minute invitation expiry and five failed code attempts per invitation, with additional per-source and global rate limits. Cancel or rotate invitations from the TV. Do not leave a permanent default PIN.

Successful pairing creates a revocable device session with a name and visible permissions. Default permission is Controller. Layout Editor and Owner are separate grants. Display a pairing notification on the TV and a connected-device list with Revoke One and Revoke All.

The displayed invitation can bootstrap phone-only control after initial LAN enablement; do not require an already-paired phone to approve the first phone. For later owner grants or sensitive changes, require confirmation on the trusted TV/local interface.

### 5.4 Transport modes: choose honestly

| Mode | Intended use | Boundary |
|---|---|---|
| **HTTPS** | Preferred for ongoing use; browser-trusted certificate supplied/configured | Encrypted transport, secure cookies, fuller browser capabilities |
| **Trusted home LAN over HTTP** | Explicitly accepted low-friction setup for a trusted network | Unencrypted; no credential entry, connector-token setup, executable registration, or sensitive administration |
| **Local only** | LAN control disabled or not yet configured | TV controls continue working; no remote browser access |

Do not label HTTP pairing “secure against other LAN devices.” It prevents casual unpaired control, not traffic interception or active modification by a hostile network participant. TLS is needed for that threat model. Do not invent custom encryption to compensate for an untrusted HTTP-delivered page.

A `.local` name does not itself provide HTTPS or certificate trust. A self-signed certificate is not a zero-setup trusted mobile experience. Explain certificate trust requirements instead of training users to ignore browser warnings.

A plain LAN-HTTP webpage is sufficient for basic controls, but do not promise complete installable-PWA, service-worker, microphone, or other secure-context features there. MDN documents the secure-context requirements and the special treatment of localhost; another computer's LAN address is not the phone's localhost. [S20]

### 5.5 Browser and network defenses

Use an authenticated, same-origin API. Validate Host and Origin against configured addresses, reject unauthorized WebSocket upgrades, authorize every message, and close sockets immediately when a device is revoked. Apply message-size, connection, rate, and idle limits. OWASP's WebSocket guidance is the baseline for these defenses. [S21]

For cookie authentication, use host-only, HttpOnly, SameSite=Strict cookies; Secure is mandatory on HTTPS. Use a server-bound CSRF token for state-changing HTTP requests. Do not rely on CORS or SameSite alone, and never put state-changing operations behind GET. Do not log cookies, invitation secrets, or CSRF values. [S22, S23]

Bind only selected host addresses. Exclude VPN, container, and unrelated interfaces unless explicitly chosen. Treat IPv6 deliberately: a globally routable IPv6 address can still be assigned to a LAN interface. Configure listeners and firewall guidance by selected interface and allowed peers, not by the assumption that every LAN address is RFC1918 IPv4.

No UPnP port mapping, internet exposure, automatic firewall disabling, or hidden telemetry. Network changes that invalidate a binding suspend the listener and offer a local reconfiguration route rather than silently exposing it on every interface.

Serve assets locally, with a restrictive Content Security Policy and no third-party scripts or analytics. Sensitive changes require HTTPS plus owner authorization or the local TV interface. Low-risk cosmetic edits may be granted separately in trusted-LAN mode.

### 5.6 Action protocol

Expose one versioned action model to both HTTP and WebSocket transports. Example:

```json
{
  "protocol": 1,
  "request_id": "b1ed0b5e-e592-4b33-b7a7-9029a434a818",
  "context_epoch": 47,
  "target": "active",
  "action": "nav.left",
  "args": {}
}
```

The browser sends **named intentions**, never shell strings, operating-system keycodes, arbitrary URLs to fetch, or JavaScript to run. The service resolves the active target from its own state. Registry app IDs are checked against locally approved applications.

Suggested action families are `nav.*`, `select`, `back`, `home`, `app.launch`, `media.play`, `media.pause`, `media.seek_relative`, `audio.volume_delta`, `audio.mute`, and `text.submit`. The server validates each action's argument shape and current capability. Bear Den Home may ignore an obsolete target epoch because it is an explicit escape command; it still requires authorization.

Acknowledgements distinguish **accepted**, **delivered**, **observed**, and **failed**. Injecting an allowed key is delivery, not proof that playback changed. The phone must not display a success claim based solely on a launched subprocess exiting zero.

| Proposed endpoint | Purpose |
|---|---|
| `GET /` | Static remote application |
| `POST /api/v1/pair/claim` | Redeem a TV-issued invitation; rate limited |
| `GET /api/v1/state` | Authorized state snapshot and context epoch |
| `GET /api/v1/capabilities` | Available actions and limitations |
| `POST /api/v1/actions` | Submit one validated action |
| `GET /api/v1/events` | Authenticated WebSocket upgrade |
| `PUT /api/v1/layout` | Permission-limited layout change with revision check |
| `DELETE /api/v1/devices/{id}` | Authorized revocation |

Invitation creation is a trusted local action, not an anonymous endpoint that anyone on the LAN can trigger. Do not expose a general configuration replacement API to Controller devices.

### 5.7 Latency, repeats, and connection loss

Use server-bounded directional holds, emitting discrete key taps rather than leaving a key logically down. Proposed starting behavior is a 350 ms repeat delay, six repeats per second, renewals every 200 ms, and a 600 ms expiry. Tune after actual remote testing.

Cancel holds on disconnect, tab hiding, target change, revocation, and session lock. Bound pending commands and reject stale context epochs. Reconnecting a phone fetches state; it **never replays an offline queue** of D-pad, Select, volume, or playback commands.

Serialize commands from multiple remotes. One device may hold a short directional-input lease; a second device receives a visible busy indication rather than fighting an endless repeat stream. Duplicate request IDs are suppressed within a documented bounded retention window, not promised as permanent exactly-once delivery.

---

## 6. Desktop and application-control adapters

### 6.1 One router, multiple mechanisms

The same logical action must route differently depending on its target:

| Target | Preferred route |
|---|---|
| Bear Den TV | Direct shell IPC/state transition, with no synthetic keyboard |
| Plex HTPC | Validated client-control protocol where supported; mapped input fallback |
| VacuumTube | Validated local media interface where present; mapped input for TV navigation |
| Other approved apps | Only explicitly declared and tested capabilities |
| Bear Den Home | Session/window adapter that brings the shell forward |
| PC audio | Audio backend, separately labeled from TV/receiver volume |

MPRIS provides discovery and basic control for players that implement it. Its existence does not prove that either installed client exposes every desired action, and it is not a general TV-menu navigation protocol. Probe interfaces and capabilities instead of assuming support. [S18]

Never route navigation to an unknown foreground window. Text injection is off by default for external applications and enabled only by a validated adapter. Launching a terminal or a privilege prompt must not turn the web remote into a general system keyboard.

### 6.2 X11 baseline

Implement window discovery and activation through EWMH-compatible mechanisms, and controlled input using XTEST where needed. Match app identity using registered desktop IDs, process/instance provenance, and window metadata together; do not rely on window title alone. Verify foreground state before delivery and stop when focus cannot be established. EWMH and X11 bindings provide the relevant primitives. [S13, S14]

X11 synthetic input does not create an isolation boundary. A focus race is still possible, so keep the allowed action set narrow and do not expose arbitrary text or shortcuts. `xdotool` may help a developer investigate, but its own documentation warns about Wayland limitations; it is not a cross-Linux solution. [S19]

A Flatpak launch wrapper's PID is not necessarily the application's durable identity. Track an observed app instance and window, and recover an already-running instance after coordinator restart instead of launching duplicates.

### 6.3 Wayland support

Use portal/compositor capabilities rather than assuming X11 automation will work against native Wayland windows.

The GlobalShortcuts portal supports shortcuts regardless of application focus. The RemoteDesktop portal supports consented input sessions and documents permission persistence and libei-based input. They solve different problems; InputCapture is not the input-injection API. Backend selection and support depend on the actual desktop installation. [S15, S16, S17, S35]

**A web Home request is not automatically a physical desktop shortcut event.** Do not assume that receiving an HTTP message grants a valid activation token or that a simple `requestActivate()` is accepted. Validate remote-triggered focus restoration independently from physical Home shortcuts.

A full-control Wayland profile may require a small, explicitly installed compositor bridge for application activation and active-window observation. Investigate a narrowly scoped KDE bridge first; record GNOME and other desktops separately. The bridge must accept only registered Bear Den/application identifiers, not arbitrary compositor scripts from the network.

Use consented input sessions when available. If consent cannot be restored, guide the user through reauthorization. Where focus or input is not available, report a reduced-capability profile. Do not grant blanket `/dev/uinput` access or install a root input daemon to conceal the limitation.

### 6.4 Home and playback behavior

Home returns promptly to the shell and preserves its navigation position. Default video behavior is pause-on-home when a reliable pause mechanism exists. Prefer idempotent Pause over blindly toggling Play/Pause. VacuumTube documents a Pause on Blur option; validate it against the installed release. [S03]

If pausing cannot be confirmed, disclose that rather than displaying “Paused.” Distinguish background music policies from video policies. Close requests attempt normal application shutdown; forced termination is a separate confirmed recovery action limited to the tracked app instance.

### 6.5 Volume and power

Label software volume as PC volume or application volume. TV/receiver volume and power require their own verified hardware/protocol integration. Passthrough audio may not have meaningful software volume; hide misleading controls.

The web service cannot receive a wake command while its host is asleep or powered off. Wake-on-LAN would require another awake sender/relay and compatible hardware. Do not ship a fake Power On button. Suspend and shutdown controls must explain that the remote will disconnect and require confirmation.

---

## 7. First-class Plex support without rebuilding Plex

### 7.1 Launch and controls first

Discover the registered Plex HTPC application, including user-installed and system-installed Flatpaks. The published Flatpak ID is `tv.plex.PlexHTPC`; prefer a native host-side launch with a fixed argument vector. [S01, S31]

```text
flatpak run tv.plex.PlexHTPC
```

Do not assume the client accepts VacuumTube's fullscreen flags. Configure fullscreen through the client's own supported setup path and validate the result. Keep the client's own account/session management and UI.

Investigate the installed client's control capabilities. The community PlexAPI client documentation exposes navigation, selection, pause, seek, and item-level playback methods, but that is evidence of a protocol surface—not certification that the installed Plex HTPC build supports all of it. A successful probe is required before advertising a capability. [S28]

### 7.2 Optional home-screen content

Add a separate, optional Plex content adapter for personal media libraries accessible to the connected account. The official Plex Media Server API provides the primary documentation entry point. The community library documents Continue Watching and library search as additional implementation references. [S26, S27]

Keep this adapter narrow: server/library selection, artwork, Recently Added, Continue Watching, and optionally a selected collection. Account connection for these rows is separate from signing into Plex HTPC; no token is extracted from the player's private storage.

Do not mark an item watched or manufacture progress because a launch was attempted. The server remains the source of truth. Scope cached results by account, server, and library; purge or segregate them on sign-out and account changes.

### 7.3 Exact-title playback is a gate

A Play/Resume button on our home screen must open that exact item at the intended position. Verify server identity, media identity, account permissions, and target player. Test both a cold launch and an already-running player.

If an exact-item handoff cannot be verified, label the action **Open Plex** and omit the unsupported Play/Resume affordance. Do not silently open the app and claim successful item playback. This fallback keeps the launcher usable without falsifying integration depth.

---

## 8. First-class YouTube support through VacuumTube

The documented Flatpak ID is `rocks.shy.VacuumTube`. Its documented fullscreen flags provide the initial launch path: [S03, S04]

```text
flatpak run rocks.shy.VacuumTube --fullscreen --no-window-decorations
```

Keep VacuumTube responsible for YouTube sign-in, personalized viewing, navigation, and playback. Bear Den TV provides launch/switch/Home behavior, input mapping, health information, and capability-gated remote controls.

The research release page lists v1.8.2, dated July 30, 2026. Its notes include playback fixes and removal of a controller settings shortcut; this illustrates why release-specific tests are needed instead of trusting every instruction in the moving README. [S04]

No documented general-purpose external remote-control API was established by this research. Treat D-pad/menu control as an adapter test, not an existing HTTP endpoint. Do not expose Chromium's debugging port as the LAN remote, and do not make brittle DOM selectors the default integration.

A specific YouTube URL can be passed to VacuumTube, but its documentation warns that account selection occurs before opening it. Saved video links therefore have a tested handoff requirement, not an assumption of immediate playback. [S03]

Do not rebuild personalized YouTube Home or claim to import existing Watch Later/history into Bear Den TV. Google explicitly documents that Watch Later and watch history cannot be retrieved through the playlist-items API. These remain inside the YouTube client. [S30]

If stock input support cannot meet the remote requirement, prefer a small upstream contribution or an optional, narrowly scoped local bridge. Any private bridge must be an explicit dependency with its own tests; do not quietly turn the project into a long-lived client fork.

---

## 9. Configuration, extensibility, and data safety

### 9.1 Configuration model

The coordinator is the sole configuration writer. Use a schema version, a monotonically increasing revision, validation, atomic replacement, and a last-known-good snapshot. The included example file illustrates the proposed model; it is not configuration for an existing application.

Separate portable preferences from machine-local bindings. Export layout, theme tokens, favorite logical app IDs, and section definitions. Exclude remote sessions, connector tokens, private server addresses by default, and machine-specific executable paths. Import presents a preview and unresolved bindings rather than executing anything.

Theme packages contain data and bounded local artwork, not QML, JavaScript, executable hooks, or unrestricted web content. Reject archive path traversal and oversized/decompression-bomb assets. Prefer system fonts initially; redistributed fonts and artwork require their own permission review.

### 9.2 Integration contracts

Keep these interfaces internal and explicit:

```text
ApplicationAdapter
  discover()
  launchOrActivate()
  capabilities()
  sendAction(action, expectedContext)
  observeState()
  requestClose()

ContentProvider
  connect()
  listSections()
  fetchItems(section, cursor)
  resolveArtwork(item)
  resolveOpenAction(item)

DesktopAdapter
  capabilities()
  observeForeground()
  activateRegisteredApp()
  bindPhysicalHome()
  deliverAllowedInput()
  observeSessionLock()
```

Capability results include available/unavailable/needs-permission, a user-facing explanation, and the detected backend. Third-party app definitions may later become data files, but adding an executable or elevated integration requires local owner approval. No plugin marketplace is needed for version one.

### 9.3 Storage and secrets

Follow the XDG directories: configuration in `$XDG_CONFIG_HOME/bear-den-tv`, persistent application data in `$XDG_DATA_HOME/bear-den-tv`, disposable artwork in `$XDG_CACHE_HOME/bear-den-tv`, and session sockets under `$XDG_RUNTIME_DIR`. [S29]

Store remote-device credentials as cryptographically random secrets with server-side hashes; do not keep recoverable bearer tokens unnecessarily. Use a desktop Secret Service backend for optional connector tokens when usable. That API defines storage and lookup of secrets, but actual availability and locked-keyring behavior require testing. [S32]

A locked or absent keyring must not prevent ordinary launch/remote control. Disable the optional connector or ask for an explicit storage choice; do not silently write Plex tokens into layout JSON. Avoid fake encryption with the encryption key saved beside the ciphertext.

### 9.4 Networking and caching

Run provider requests asynchronously with cancellation, short deadlines, bounded retries, and independent failure states. App launch and Home never wait for artwork, metadata, internet access, or a Plex server.

Use a proposed 512 MiB artwork disk budget, configurable downward, with eviction and reduced fetching under low disk space. Decode/resize images off the presentation path and bound decoded image dimensions. Cache staleness must be visible without making every row display an alarming error.

Keep credentials in service-side requests rather than image URLs sent to browsers. A metadata/artwork proxy may access only explicitly connected service endpoints and approved paths; validate redirects and resolved destinations. Do not expose a generic `fetch_url` endpoint. OWASP's SSRF guidance informs this boundary. [S33]

---

## 10. Linux portability and packaging

### 10.1 Compatibility levels

| Level | Meaning |
|---|---|
| Core | TV shell, local settings, and LAN remote can operate Bear Den TV |
| Launcher | Installed external clients can be launched and observed |
| Full couch control | Both clients support verified remote navigation and return Home |
| Enhanced content | Optional content rows and exact-item handoffs pass their tests |

Target the actual Mint installation first, then at least one non-Mint distribution and a Wayland desktop. Initial investigation should cover Mint/Cinnamon, an Ubuntu or Debian desktop, and KDE Plasma on a supported Fedora or openSUSE release. These are test targets, not validated compatibility claims.

Record OS release, architecture, desktop/version, X11 or Wayland, portal backend, Qt version, app versions, packaging, GPU/driver, and proven capabilities. Do not infer the display session just because a distribution is called Mint or Fedora.

### 10.2 Distribution strategy

Package Bear Den TV as a native desktop application/coordinator pair: a `.deb` for the initial Mint/Ubuntu/Debian family and an `.rpm` for additional targets. Provide reproducible builds and a documented developer/source-install path. A portable bundle or AppImage can follow after its integration is tested.

Keep Plex HTPC and VacuumTube as separate dependencies, normally installed through Flatpak. Discover availability, show a supported installation action, and require user authorization for installation. Do not install clients silently or ship their account state.

Do not make a Flatpak-only Bear Den TV the first packaging strategy. Host app launching, window control, and session integration interact with sandbox boundaries. Flatpak documentation explicitly describes restricted host access; a future sandboxed shell would need a deliberately designed host coordinator, not broad escape permissions. [S34]

Build portable artifacts against an explicit oldest-supported runtime baseline and test the result; an executable built on a recent Mint installation is not automatically compatible with every older distribution.

### 10.3 Setup, updates, and removal

First-run setup detects the display/session, selects the TV screen, calibrates text/margins, enables and pairs the LAN remote, discovers both clients, and runs a guided launch/Home test. Offer automatic startup only after explicit consent. Automatic OS login is a separate, security-relevant host setting, not something the application silently enables.

Updates must preserve validated configuration and disclose when an integration needs retesting. Capture third-party versions in diagnostics; do not indefinitely freeze a vulnerable browser runtime merely to preserve old behavior. Removal unregisters only Bear Den's own autostart/service/bridge entries and never removes the user's clients or media libraries.

---

## 11. Implementation plan and release gates

### Milestone 0 — Validate the risky integration paths

Inspect the actual mini PC and produce a capability report. Install neither system changes nor clients without the user's authorization. Record available apps, app IDs, session type, native input options, and media interfaces.

Build a minimal coordinator and remote experiment capable of navigating a test shell, launching each real client, sending directional/select/back commands, and returning Home. Probe Plex item playback separately. Repeat the critical Home experiment on one Wayland desktop or a representative test host.

**Exit gate:** Evidence for every supported action, explicit unsupported states, and a decision on the desktop adapters. Do not create a visually elaborate frontend around an unproven Home mechanism.

### Milestone 1 — Polished shell and LAN remote together

Implement the branded home screen, explicit focus graph, local IPC, paired browser remote, connected-app indicator, configuration schema, and first-run setup. Include screenshot fixtures with original or licensed artwork.

**Exit gate:** From a phone, pair, navigate all shell screens, select favorites, return Home, reconnect after Wi-Fi loss, and recover after a shell restart. No terminal is required for ordinary configuration.

### Milestone 2 — Plex HTPC and VacuumTube adapters

Implement discovery, single-instance launch/activation, bounded app-control actions, capability reporting, and normal/failed exit handling. Confirm playback controls and pause-on-home rather than deriving them from process state.

**Exit gate:** Both clients pass a complete phone-only browse/play/Home loop on the reference machine. Missing commands are visible and disabled, not silently ignored. Application account entry remains in the applications.

### Milestone 3 — Customization and optional real Plex rows

Implement the permission-limited web layout editor, TV editing, preview/undo/rollback, theme tokens, section ordering, and safe export/import. Add the optional Plex connector with real artwork and state. Enable exact-item Play/Resume only after its gate passes.

**Exit gate:** A nontechnical user can rearrange the home screen and recover defaults without editing a file. Disconnected providers do not break launch or remote control. No mock content appears in production.

### Milestone 4 — Cross-distribution operation and appliance reliability

Build native packages, add capability-specific desktop bridges where justified, test X11 and Wayland profiles, and validate autostart, session locking, TV input changes, sleep/resume, network transitions, and low disk conditions.

**Exit gate:** Publish a compatibility matrix based on observations, including reduced-capability profiles. Packaging installation success alone is not sufficient.

### Milestone 5 — Security, performance, and release

Threat-model the LAN listener and every route into desktop control. Test malformed actions, cross-origin requests, stolen/revoked sessions, brute-force pairing, path traversal, and metadata proxy abuse. Audit bundled licenses and dependencies and create a release manifest.

**Exit gate:** The acceptance suite passes on documented reference profiles, known limitations are visible, and no unresolved failure can leave unrestricted keyboard input or an unauthenticated control endpoint exposed.

---

## 12. Acceptance criteria and engineering budgets

### 12.1 Core release tests

| Test | Required result |
|---|---|
| Fresh install | User reaches a usable shell without writing configuration |
| Phone pairing | Unpaired phone cannot act; valid invitation pairs; expired invitation fails |
| Remote navigation | D-pad, Select, Back, and Home operate every supported target |
| App handoff | Correct app activates; repeated launch does not create unwanted duplicates |
| Return Home | Shell regains focus and restores its original selection |
| Wi-Fi interruption | Holds expire; stale commands are not replayed on reconnection |
| Two phones | Commands remain bounded and predictable; one hold cannot monopolize forever |
| Session lock | External input and private state delivery stop until local unlock |
| App crash | Clear error and controlled recovery; unrelated apps remain untouched |
| Shell crash | Coordinator and remote remain available and can restore the shell |
| Provider unavailable | Cached/empty content is honest; app shortcuts still work |
| Configuration failure | Invalid data never replaces the last-known-good configuration |
| Device revocation | Existing WebSocket and HTTP control access cease |
| Platform limitation | Unsupported capabilities appear as unavailable with a reason |

### 12.2 Proposed performance targets—not measurements

On the eventual reference mini PC, target a usable warm home screen within three seconds of launching Bear Den TV, p95 phone-action-to-visible-shell-response under 150 ms on a healthy LAN, and a warm return-Home transition under one second. External-client startup is measured separately.

Target smooth 60 Hz navigation at 1080p and validate 4K separately. Stop decorative animation and unnecessary fetching while an external app is foregrounded. Initial memory budget for the coordinator plus visible shell is 350 MiB resident memory at 1080p, excluding external players; report graphics allocations and shared-memory accounting separately. These targets may be adjusted with recorded measurements, not quietly represented as benchmark results.

CI should cover Go unit/race/fuzz tests, contract/schema tests, QML focus/navigation tests, and browser remote interaction tests. Mocked app adapters cannot replace manual or hardware-assisted playback tests. Use screenshot regression tests for intentional design changes, but do not require pixel equality across different GPU/font stacks.

---

## 13. Operational diagnostics and maintenance

Expose a local `bear-den-tv doctor` command and equivalent TV diagnostics page. Report capability checks, detected versions, active app, current desktop adapter, LAN binding, paired-device count, and recent errors. Redact private IPs and library titles from shared diagnostic exports by default unless the user includes them deliberately.

Structured logs use request IDs and action names, not typed search terms, media credentials, full private URLs, or secret-bearing headers. Distinguish launcher failure, network failure, player failure, and an unsupported platform operation.

Record integration observations as data under `tests/compatibility/`, with a date and environment. A client update invalidates relevant assumptions until smoke-tested. A small manual regression run is more useful than a long unverified “supported” list.

---

## 14. Proposed repository layout

```text
bear-den-tv/
  apps/
    tv-shell/                 # CMake, C++, QML, embedded UI assets
    remote-web/               # TypeScript, HTML, CSS, browser tests
  cmd/
    bear-den-tv-session/      # Coordinator entry point
    bear-den-tv/              # CLI / user entry point
  internal/
    actions/                  # Typed commands, routing, epochs, acknowledgements
    applications/             # Registry, launch jobs, app adapters
    platform/                 # X11, portals, desktop bridges, audio, session state
    remote/                   # Pairing, HTTP, WebSocket, authorization
    config/                   # Schema, migrations, atomic persistence
    providers/plex/           # Optional metadata integration
    storage/                  # State, secret references, cache
  contracts/                  # JSON Schema, HTTP and IPC definitions
  integrations/               # Built-in Plex and VacuumTube profiles
  themes/                     # Non-executable data-only themes
  packaging/                  # deb, rpm, autostart, user service, bridge packaging
  tests/
    contract/
    integration/
    security/
    compatibility/
  docs/
    design.md
    decisions/
    operations.md
    compatibility.md
    sources.md
```

The agent implementing this plan should first inspect any existing repository and preserve relevant work. Do not fabricate benchmark results, pretend simulated adapters are live integrations, or convert unsupported capabilities into silent success. Change the proposed stack only with a written decision explaining what concrete constraint the alternative resolves.

---

## 15. Evidence, uncertainties, and source register

The verified documentation supports using existing TV clients, implementing a native shell, serving a local remote, and choosing session-specific control mechanisms. It does **not** establish end-to-end compatibility on the user's hardware.

The main unanswered implementation questions are the installed Plex client's usable control protocol, VacuumTube navigation/media behavior, browser-triggered Home on the actual desktop, keyring behavior under the chosen login setup, and graphics/audio/CEC capabilities. Milestone 0 has explicit experiments for these rather than leaving them as general research tasks.

Research access note: the Plex support article pages did not consistently load in this environment. Accordingly, the design does not depend on unseen instructions from those pages; it uses the published application manifest/listing and the clearly labeled community API reference where applicable. A source archive could not be downloaded into the research container; this document is not represented as a full source audit.

### Source register

The following sources were reviewed on September 21, 2026. Moving documentation and repository branches can change; lock implementation dependencies and record tested versions. URLs are provided for the implementing agent, not as claims of deployed Bear Den TV endpoints.

**[S01] Plex HTPC — published application listing**  
`https://flathub.org/en/apps/tv.plex.PlexHTPC`  
Evidence used: TV-client purpose and Flatpak identifier; not an installed-client test.

**[S02] Plex HTPC — Flathub build manifest**  
`https://raw.githubusercontent.com/flathub/tv.plex.PlexHTPC/master/tv.plex.PlexHTPC.yml`  
Evidence used: Observed X11 permission and commented-out Wayland permission; moving branch.

**[S03] VacuumTube — project README**  
`https://github.com/shy1132/VacuumTube`  
Evidence used: Leanback wrapper, launch flags, Pause on Blur, and URL/account-selection behavior.

**[S04] VacuumTube — release history**  
`https://github.com/shy1132/VacuumTube/releases`  
Evidence used: v1.8.2 release dated July 30, 2026; release-specific compatibility observations.

**[S05] Flex Launcher — project README**  
`https://github.com/complexlogic/flex-launcher`  
Evidence used: INI configuration, existing launcher scope, and stated maintenance direction.

**[S06] Android Developers — Navigation on TV**  
`https://developer.android.com/design/ui/tv/guides/foundations/navigation-on-tv`  
Evidence used: Directional TV-navigation design reference.

**[S07] Qt — Qt Quick documentation**  
`https://doc.qt.io/qt-6/qtquick-index.html`  
Evidence used: UI model/view, input, animation, extension facilities, and listed module licenses.

**[S08] Qt — Qt HTTP Server documentation**  
`https://doc.qt.io/qt-6/qthttpserver-index.html`  
Evidence used: Module capabilities and GPLv3/commercial licensing distinction.

**[S09] Go — net/http documentation**  
`https://pkg.go.dev/net/http`  
Evidence used: HTTP serving, request handling, and server configuration.

**[S10] Go — embed documentation**  
`https://pkg.go.dev/embed`  
Evidence used: Compile static web assets into the coordinator.

**[S11] Coder — websocket project**  
`https://github.com/coder/websocket`  
Evidence used: Candidate maintained Go WebSocket implementation; audit and pin before implementation.

**[S12] godbus — D-Bus bindings**  
`https://github.com/godbus/dbus`  
Evidence used: Candidate Go D-Bus transport.

**[S13] XGB — X Go bindings**  
`https://github.com/jezek/xgb`  
Evidence used: X11 protocol bindings, including XTEST-related functionality.

**[S14] freedesktop.org — Extended Window Manager Hints**  
`https://specifications.freedesktop.org/wm/latest-single/`  
Evidence used: X11 window state, identity hints, and activation mechanisms.

**[S15] XDG Desktop Portal — GlobalShortcuts**  
`https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.GlobalShortcuts.html`  
Evidence used: Global shortcut sessions; not general permission for remote-origin focus changes.

**[S16] XDG Desktop Portal — RemoteDesktop**  
`https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.RemoteDesktop.html`  
Evidence used: Consented input sessions, persistence, and EIS/libei integration.

**[S17] XDG Desktop Portal — portals.conf**  
`https://flatpak.github.io/xdg-desktop-portal/docs/portals.conf.html`  
Evidence used: Desktop-specific backend selection.

**[S18] freedesktop.org — MPRIS specification**  
`https://specifications.freedesktop.org/mpris/latest/`  
Evidence used: Discovery and media control for clients implementing the interface.

**[S19] xdotool — project documentation**  
`https://github.com/jordansissel/xdotool`  
Evidence used: X11 automation tool and its Wayland limitations.

**[S20] MDN — Secure contexts**  
`https://developer.mozilla.org/en-US/docs/Web/Security/Defenses/Secure_Contexts`  
Evidence used: HTTPS/secure-context restrictions and localhost distinction.

**[S21] OWASP — WebSocket Security Cheat Sheet**  
`https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html`  
Evidence used: Authentication, origin checking, per-message authorization, and resource limits.

**[S22] OWASP — CSRF Prevention Cheat Sheet**  
`https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html`  
Evidence used: Protection for cookie-authenticated state-changing requests.

**[S23] MDN — Set-Cookie header**  
`https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Set-Cookie`  
Evidence used: Cookie scope and Secure, HttpOnly, and SameSite attributes.

**[S24] freedesktop.org — Desktop Application Autostart**  
`https://specifications.freedesktop.org/autostart/latest/`  
Evidence used: Portable graphical-session autostart baseline.

**[S25] systemd — Desktop Environment Integration**  
`https://systemd.io/DESKTOP_ENVIRONMENTS/`  
Evidence used: Graphical-session lifecycle integration and desktop responsibilities.

**[S26] Plex — Media Server API entry point**  
`https://developer.plex.tv/`  
Evidence used: Official documentation entry point for personal-media server integrations.

**[S27] Python PlexAPI — Server documentation**  
`https://python-plexapi.readthedocs.io/en/latest/modules/server.html`  
Evidence used: Community implementation reference for Continue Watching and search; not an official client certification.

**[S28] Python PlexAPI — Client documentation**  
`https://python-plexapi.readthedocs.io/en/latest/modules/client.html`  
Evidence used: Community client-control reference for navigation and playback; installed-client support must be tested.

**[S29] freedesktop.org — XDG Base Directory Specification**  
`https://specifications.freedesktop.org/basedir/latest/`  
Evidence used: Standard locations for configuration, data, caches, and runtime files.

**[S30] Google — YouTube PlaylistItems: list**  
`https://developers.google.com/youtube/v3/docs/playlistItems/list`  
Evidence used: Explicit Watch Later and watch-history retrieval limitations.

**[S31] Flatpak — Using Flatpak**  
`https://docs.flatpak.org/en/latest/using-flatpak.html`  
Evidence used: Discovery, installation, and launch model.

**[S32] freedesktop.org — Secret Service API**  
`https://specifications.freedesktop.org/secret-service/latest/`  
Evidence used: Secret storage interface; host availability remains a deployment concern.

**[S33] OWASP — SSRF Prevention Cheat Sheet**  
`https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html`  
Evidence used: Allowlisting and validation for server-side content/artwork fetching.

**[S34] Flatpak — Sandbox Permissions**  
`https://docs.flatpak.org/en/latest/sandbox-permissions.html`  
Evidence used: Host-access limitations and permissions influencing packaging choice.

**[S35] XDG Desktop Portal — InputCapture**  
`https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.InputCapture.html`  
Evidence used: Capture is distinct from input injection; do not substitute it for RemoteDesktop.

