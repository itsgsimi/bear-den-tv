# Implementation notes and scope clarifications

These notes translate the existing design into an execution contract. They do not claim that any app-control mechanism has been verified on the user's hardware.

## 1. A real product, delivered incrementally

The objective is an application, not documents about an application. Begin with the smallest live control loop, keep it runnable, and add coherent features. Planning files should record decisions and next steps; they should not consume the project instead of code.

Treat the early integration milestone as an experiment with a concrete result: supported, unsupported, or blocked in an identified environment. When the TV machine is unavailable, do not endlessly research the same problem or make a simulation look live. Implement the architecture, test-double backend, and production backend where feasible; record what must be run on the target. Independent shell, remote, settings, and validation work can proceed, but full-control release status stays unproven.

Do not treat "first vertical slice" as permission to abandon customization, a usable layout editor, or the optional Plex connector. Conversely, do not build a large plugin system before the core control path works.

## 2. Required versus conditional behavior

Required for the initial usable product: a branded running TV UI; safe onboarding; a paired LAN remote; useful configuration without editing JSON; Plex and YouTube discovery/launch; routing and return Home on the initial supported target; diagnostics; and a maintenance exit.

Conditional features include observed playback position, software volume, external text entry, exact-title playback, and compositor-specific integrations. Their capability results must be explicit. Disabled with a reason is preferable to a button that silently does nothing, but failure of the basic Plex/YouTube navigation/Home loop remains a full-control acceptance failure.

The Plex content connector is opt-in for end users. Implement it after the base loop without making credentials a prerequisite for using the product. Account-free tests may exercise the provider contract with fixtures; that is not live verification.

A local appearance profile does not isolate another person's external account or constitute parental controls.

## 3. Contracts before duplicated assumptions

Finalize one versioned action/state contract and generate or validate language-specific representations against it. Suggested implementation artifacts are action and IPC schemas, HTTP definitions, state/capability schemas, and contract fixtures. Their exact file formats are engineering choices; keep them small and testable.

The supplied config schema has structural constraints only. Production code must also verify unique IDs, valid references, approved application identity/arguments, interface ownership, certificate readability/trust instructions, and privilege separation. Do not expose the entire machine configuration through the web editor.

Keep network identity, pairing invitations, authenticated remote devices, local authorization, and media-provider credentials as separate concepts. Demo accounts and fake adapters must be isolated from production credentials and state directories.

A local IPC client must be the correct desktop user; socket location and mode alone are not the entire check. Bound JSON decoding, reject unsupported protocol versions, and use explicit connection lifetimes.

## 4. Foreground routing is a safety boundary, not a guess

Track both the intended app transition and the observed foreground context. Invalidate input when either changes or becomes unknown. An independently opened desktop dialog, session lock, or terminal must never inherit remote keystrokes intended for a TV application.

A stale navigation/text command is rejected, not retargeted to the new foreground app. An authorized Bear Den Home command is a separate escape action and can use the spec's explicit epoch exception. On lock it must not unlock or reveal the private shell. Unsupported observation must not be presented as verified focus.

Repeated command IDs need a bounded per-session deduplication policy. Reusing an ID with a different payload is an error. A retry must not restart a player, toggle playback twice, or repeat destructive operations. Serialize target changes and input delivery and test the failure paths.

Holds use expiring server-side leases with discrete input taps. Use monotonic time for durations. Drop queued navigation on disconnection or target change. Do not let a successful reconnect revive a previous hold.

Application wrapper PIDs are not enough to identify windows or authorize closing them. Correlate the observed instance and approved registry identity. A failed launch must not lead to killing all processes with a matching name.

## 5. Practical LAN access without weaker development shortcuts

A working normal HTTP page is sufficient for the trusted-home-LAN remote mode; do not make a service worker, microphone, installed PWA, or cloud DNS a dependency of core controls. Implement HTTPS with standard libraries and accept owner-configured certificates rather than building certificate infrastructure or custom cryptography.

Initially bind nothing on the LAN. Onboarding lets the user select the intended address/interface and explicitly choose the mode. Tests should start temporary loopback listeners or isolated namespaces where allowed, not silently expose the development host.

Pairing invitation creation is local/trusted. A public pair-claim endpoint only redeems an already-issued invitation. Authentication must precede private state access, and revocation must terminate existing WebSocket control as well as future requests. A controller cannot promote itself, change executable definitions, or replace general system settings through a cosmetic editor.

Trust does not automatically follow RFC1918 addresses, `.local` names, LAN membership, or a reverse-proxy header. Handle Host/Origin and intentional IPv6 bindings. Ignore forwarded identity/address headers unless an explicit, restricted trusted-proxy configuration is actually implemented.

Logging must not capture pairing fragments, session cookies, connector tokens, typed searches, or private content titles. Tests and screenshots shared in the repo must redact them too.

## 6. Visual implementation, not a screenshot wallpaper

The reference is for hierarchy, spacing, card treatment, focus, and atmosphere. Implement those as real components. Do not display the mockup as a background and overlay a few invisible click regions. Do not ship movie artwork cropped from the reference or fabricate additional service availability.

Capture at least: home before connection; populated demo home marked as demo; settings; app unavailable/error; pairing with a nonlive test invitation; phone remote in portrait and landscape; and the web layout editor. Capture the actual running renderer, not new generated mockups.

Test navigation after row refresh/reorder/deletion, with no content, with a long title, while a modal opens/closes, and after returning from a client. Timed settings confirmation must have a usable escape and automatic recovery.

The app should look intentional without a Plex account: branded wallpaper/gradient, large useful favorites, clear setup, and optional empty rows. Metadata fetches must not sit on the critical path for navigation or Home.

## 7. Build, installation, and recovery

Use actual supported dependencies and record versions. Prefer source builds into the repository output directory before touching the host. Keep native packaging scripts deterministic and document the tested baseline. A generated `.deb` or `.rpm` alone does not establish portability.

Do not install clients, edit autostart, change firewall policy, enable automatic OS login, or add a compositor bridge without permission. Build/test actions remain subject to the sandbox policy. Runtime components run as the logged-in desktop user, not root.

Make crash recovery and intentional exit different states. Add bounded restart backoff and a circuit breaker for a repeatedly crashing shell. The remote should explain a shell failure and offer the permitted recovery rather than spawning an infinite restart loop. Do not close an external player just because the shell restarts.

Provide commands or equivalent targets for dependency checks, build, unit/contract tests, development launch, doctor, package, and uninstall. Only document commands after implementing them. No placeholder `make test` that returns success without running tests.

## 8. Evidence and continuity

Every compatibility claim must say whether it came from documentation, a unit test, a virtual desktop, a physical desktop, or a phone controlling the real TV machine. Templates start with unknown/null values deliberately.

Review the diff for unsafe shortcuts, secret leaks, duplicated state machines, fake integration success, TODO-only core features, and dead buttons. Run relevant checks after fixes. Keep the status file short enough to read at the start of the next session.

A test marked SKIP/NOT RUN/BLOCKED is never counted as passed. Optional or conditional behavior can be legitimately unavailable; required gates cannot be waived by renaming the feature.

## 9. Guidance references

The original technical source register is preserved in `reference/RESEARCH_SOURCES.md`; use it as a navigation aid and re-check changing details before depending on them.

Two official agent-workflow references were consulted for this handoff. They support using explicit file/image context, implementation constraints, verification commands, and concise repository instructions. They do not validate the Bear Den TV architecture or any client integration:

- OpenAI, prompting guidance: `https://developers.openai.com/codex/prompting/`
- OpenAI, repository instructions: `https://developers.openai.com/codex/guides/agents-md/`

The agent should use its available features and approval policy; this handoff does not require a specific model, paid tier, experimental workflow mode, or unavailable delegation tool.
