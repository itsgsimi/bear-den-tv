# Bear Den TV — implementation handoff

## Mission

Build Bear Den TV: a polished, configurable Linux TV shell for a TV-connected mini PC, initially running Linux Mint and portable to comparable supported Linux desktop distributions. Ease of use and customization are the primary goals. Use Plex HTPC and VacuumTube as independent TV clients, not replacements to be rewritten. A LAN browser remote is a first-class requirement from the first usable build.

Read `Bear_Den_TV_Design_and_Implementation.md` before making architectural decisions. Its source register distinguishes documentation-backed capabilities from proposed design and untested integrations. The included picture is an earlier visual concept, not working software. Bear Den TV is the actual product name.

## Starting architecture

Use a Qt Quick/QML television shell with a small C++ host, a Go session coordinator, and a static TypeScript/HTML/CSS remote. The coordinator serves the remote itself and owns authorization, action routing, app lifecycle, and validated configuration. Keep local UI IPC private to the desktop user. No Docker, cloud account, reverse proxy, or separate database server is required.

Treat the standalone-project recommendation as the baseline. Do not force a Flex Launcher fork unless an experiment establishes a concrete implementation advantage. Inspect and preserve any existing repository before creating a scaffold. Pin dependencies and document meaningful changes to the proposed architecture.

## First implementation work

Begin with the integration risks, not a large mock UI. Identify the actual distribution/release, desktop/session, installed client versions and formats, GPU, display, portal backends, and usable media-control interfaces. Do not assume this mini PC is one of the user's other known machines.

Prove the following on the target session: a phone can pair with the local remote, navigate a basic Bear Den screen, launch each real client, navigate/select/back within that client, and return to the same Bear Den selection. Test remote-triggered Home separately from a physical desktop shortcut. Record actual evidence and unavailability reasons.

A successful process launch is not a successful window activation. A delivered key is not proof of changed playback. A client-control API described by a library is not proof of compatibility with the installed Plex HTPC build.

Only then implement the polished visual layout, full settings flow, web layout editor, and optional Plex content rows. Keep both clients' browsing, login, account management, and playback inside the existing applications. Do not scrape their private storage or expose Chromium debugging over the network.

## Invariants

The remote sends named, schema-validated actions; it never sends shell commands, arbitrary operating-system keycodes, arbitrary compositor scripts, or executable theme content. Controller, Layout Editor, and Owner permissions are distinct. The LAN listener is enabled through onboarding consent and bound only to selected interfaces. No unauthenticated control endpoint or automatic port forwarding is acceptable.

Use secure same-origin authentication, revocation, request limits, CSRF/Origin/Host protections, and restrictive asset loading. HTTPS is preferred. Explicitly accepted trusted-LAN HTTP is a reduced-security mode, not encrypted pairing; it cannot offer sensitive administration or connector credential entry. Do not claim complete PWA capabilities on plain LAN HTTP.

Use context epochs and bounded repeats. Drop stale input on app changes and reconnection. Release all holds on disconnect, revocation, or lock. Do not route input to unknown foreground windows. Do not convert unimplemented actions into success responses.

Plex home-screen rows are an optional metadata integration. Real Play/Resume buttons require exact-item handoff tests; otherwise say Open Plex. No production placeholder movies, fake YouTube watch history, or invented playback progress.

Use validated, atomic settings writes and last-known-good recovery. Themes are data-only. Account tokens and paired-device secrets are not portable layout configuration. Do not install, overwrite desktop settings, or change firewall policy without explicit authorization.

## Deliverables and evidence

Deliver a small runnable vertical slice first, then proceed through the milestones in the main specification. Maintain a capability matrix, automated contract/security tests, QML navigation tests, browser remote tests, and real-client smoke tests. Include installation/removal instructions and a local diagnostic command.

The first product demonstration should show an actual phone controlling the shell and both clients, including a lost-connection recovery and a return-Home loop. Label simulations and fixture data clearly. A static screenshot alone does not satisfy the milestone.

Report completed behavior, tests actually run, failures observed, and remaining capability gaps. Where hardware or a desktop is unavailable, state that limitation and supply a reproducible test instead of claiming cross-distribution certification.
