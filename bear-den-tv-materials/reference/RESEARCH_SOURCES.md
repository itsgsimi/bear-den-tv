# Bear Den TV — research sources

Reviewed September 21, 2026. Source review is not hardware or playback validation. See the main specification for proposed decisions and limitations.

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

