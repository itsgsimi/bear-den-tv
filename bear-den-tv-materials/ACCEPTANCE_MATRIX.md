# Bear Den TV — implementation acceptance matrix

All items below begin **NOT RUN**. The original full checklist remains at `reference/ACCEPTANCE_CHECKLIST.md`. This matrix adds stable identifiers for tracing requirements to actual tests.

For each item, record implementation paths, test name/command, environment, result, and evidence location. Allowed outcomes: PASS, FAIL, BLOCKED, NOT RUN, or NOT APPLICABLE with rationale. "Test double passed" and "live passed" are separate observations. Do not present an unavailable required behavior as a successful product gate.

| ID | Requirement and required observation | Verification |
|---|---|---|
| UI-01 | Running Bear Den TV shell uses real components, correct branding, and a useful no-account home | Rendered screenshots + manual navigation |
| UI-02 | D-pad reaches all screens, empty states, dialogs, and errors; Back/Home/Close stay distinct | QML tests + live inspection |
| UI-03 | Item-ID focus and scroll restore after refresh, reorder, modal dismissal, and app return | Automated state tests + live loop |
| UI-04 | Text scale, density, safe margins, contrast, and reduced motion are usable at TV sizes | Screenshots + directional-navigation tests |
| CFG-01 | TV settings and authorized web editor change and persist ordinary preferences without JSON editing | Browser/QML integration + restart |
| CFG-02 | Preview/apply/undo/reset and failed/timed-out-change rollback work; conflicting revisions fail cleanly | Automated persistence and concurrency tests |
| CFG-03 | Schema/semantic errors, duplicate IDs, dangling references, corrupt files, and unknown versions cannot replace good settings | Negative tests + recovery launch |
| CFG-04 | Export/theme import cannot leak secrets or execute content; unsafe archives/images fail safely | Security tests |
| NET-01 | No LAN listener before consent; selected interface/address only; IPv6 and changed bindings handled | Socket inspection + isolated network tests |
| NET-02 | QR/IP entry and code fallback work; mDNS failure is nonfatal | Phone/manual + browser test |
| AUTH-01 | Unpaired devices cannot control/read private state; invitation expiry and brute-force limits enforced | HTTP/WebSocket negative tests |
| AUTH-02 | Controller/editor/owner permissions are separate; controller cannot alter launch definitions or self-promote | Authorization tests |
| AUTH-03 | Device revocation closes active sockets and invalidates HTTP sessions; logout/lock stops private state | Integration + desktop-lock test |
| AUTH-04 | Host/Origin/CSRF protections, oversized inputs, command floods, and restrictive assets are verified | Security suite |
| AUTH-05 | HTTPS path works; explicit HTTP mode blocks sensitive changes and displays its limitations | Transport-mode tests + real phone trust test |
| REM-01 | Phone shows the true controlling target and operates shell D-pad/Select/Back/Home | Browser automation + real phone |
| REM-02 | Lost Wi-Fi, tab hiding, phone sleep, revocation, lock, and target change expire held input | Fake-clock tests + disconnect experiment |
| REM-03 | Reconnect never replays queued actions; stale epochs and altered duplicate IDs rejected | Contract/concurrency tests |
| REM-04 | Two remotes remain predictable; busy/lease behavior and bounds visible | Two-session automated test |
| APP-01 | Discover actual client installations; missing/slow/failed launch gives actionable feedback | Integration + real installation tests |
| APP-02 | Cold and warm launch activate the correct instance without duplicate windows or unrelated process termination | Real desktop client tests |
| APP-03 | Real phone navigates Plex HTPC, plays content, and returns Home to the prior selection | Live hardware/client gate |
| APP-04 | Real phone navigates VacuumTube, plays content, and returns Home to the prior selection | Live hardware/client gate |
| APP-05 | Browser-triggered Home works separately from physical-shortcut Home; unknown foreground gets no input | Desktop live test + negative tests |
| APP-06 | Pausing/state/text/audio controls reflect verified capabilities; delivery is not falsely called observation | Adapter contract + live capability probes |
| PLEX-01 | Opt-in provider fetches authorized real rows; outages and sign-out cannot break core controls or leak another cache | Provider tests + live authorized server |
| PLEX-02 | Exact-title/resume verified for cold/warm player; otherwise action is accurately labeled Open Plex | Real-client gate |
| REL-01 | Shell crash recovery preserves coordinator and remote; intentional exit is respected; repeated crashes are bounded | Process-lifecycle tests |
| REL-02 | TV off/on, input changes, sleep/resume, lock, and network changes recover as documented | Real hardware/session tests |
| PORT-01 | Clean native build/install/run on actual Mint baseline; removal leaves other clients/data intact | Clean installation test |
| PORT-02 | At least one non-Mint and one Wayland profile record real capabilities, limitations, and versions | Separate environment reports |
| OPS-01 | Doctor, redacted logs, source/dependency versions, install/run/test/uninstall instructions actually work | Command/documentation checks |
| PERF-01 | Startup, warm Home, action latency, memory, and UI smoothness measured with method/environment | Measurements; no invented target-as-result |

## Minimum integrated demonstration

Start the real shell/coordinator. Pair a phone using an invitation. Navigate the home screen. Launch Plex, browse and play authorized content, return Bear Den Home, and verify restored focus. Repeat with VacuumTube. Change an allowed layout preference from the web editor and confirm persistence after restart. Interrupt the phone connection during a held direction and show no stuck/replayed input. Finally, revoke the device and verify existing and new requests fail.

A video or screenshot alone is not proof of every step; accompany demonstrations with an environment report and clear observations. Keep live secrets, private content, and invitation/session values out of exported evidence.
