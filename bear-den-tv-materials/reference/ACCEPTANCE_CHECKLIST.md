# Bear Den TV — release acceptance checklist

This checklist is unexecuted. Check an item only after recording its environment and evidence. A passed mock test does not substitute for a passed real-client test.

## Target and setup
- [ ] Actual mini PC, OS release, desktop, display session, GPU/driver, and app versions recorded.
- [ ] Fresh installation opens a usable UI with no hand-edited settings.
- [ ] Display selection, safe margins, text scaling, and maintenance exit work.
- [ ] Autostart is opt-in and removable; no duplicate coordinator appears.
- [ ] At least one non-Mint target and one Wayland profile have explicit results.

## LAN access and pairing
- [ ] Only selected network interfaces expose the service, including intentional IPv6 handling.
- [ ] QR/IP access works; failure of mDNS does not prevent connection.
- [ ] Unpaired devices cannot read private state or control anything.
- [ ] Pairing expiry, failed-attempt limits, and notification work.
- [ ] Controller cannot grant itself Editor or Owner privileges.
- [ ] Revoking a device terminates existing connections and future access.
- [ ] HTTPS certificate/trust behavior is tested; HTTP limitations are visible.
- [ ] No internet port forwarding, unauthenticated API, or third-party web script exists.

## Remote behavior
- [ ] D-pad, Select, Back, and Bear Den Home operate the shell.
- [ ] The remote indicates the real current control target.
- [ ] Plex HTPC and VacuumTube each pass a real phone-only navigation/play/Home loop.
- [ ] Remote-triggered Home works independently of physical-key shortcut tests.
- [ ] Unsupported playback/text/volume controls are disabled with a reason.
- [ ] Long holds, Wi-Fi loss, tab hiding, and phone sleep cannot leave stuck input.
- [ ] Reconnection does not replay old button presses.
- [ ] Two remotes have bounded, predictable command arbitration.
- [ ] Lock/logout disables input and stops private state exposure.

## Application integration
- [ ] Already-running clients activate without unwanted duplicate windows.
- [ ] Cold launch, failed launch, normal exit, and crash have distinct behavior.
- [ ] Wrong-focus or unknown-window input is refused.
- [ ] Pause-on-home is verified or clearly reported as unavailable/unconfirmed.
- [ ] Recovering an app cannot kill unrelated processes.
- [ ] Optional Plex artwork/rows use real, authorized data.
- [ ] Exact item, account/server identity, and resume offset are tested before Play/Resume is enabled.
- [ ] No YouTube personalized history/recommendation integration is falsely advertised.

## Customization and safety
- [ ] TV and permitted web editing require no configuration-file knowledge.
- [ ] Preview, Apply, Undo, Reset, and failed-change rollback work.
- [ ] Duplicate IDs, broken references, bad schema versions, and corrupt files are rejected.
- [ ] Malicious theme archives and unsafe image assets are rejected.
- [ ] Portable exports contain no tokens, paired-device secrets, or hidden executable changes.
- [ ] Provider outage and low disk space do not break app launch or navigation.

## Security and reliability
- [ ] Cross-origin requests, WebSocket hijacking attempts, and bad Host values fail.
- [ ] CSRF, oversized messages, command floods, and stale epochs are tested.
- [ ] Metadata fetching cannot become an arbitrary network proxy.
- [ ] Credentials, typed searches, and private URLs are absent from ordinary logs.
- [ ] Shell crash recovery leaves the coordinator and remote usable.
- [ ] TV off/on, HDMI input changes, sleep/resume, and network address changes are tested.
- [ ] Volume/power labels do not promise unsupported TV, receiver, or wake functions.
- [ ] Performance budgets are measured with a documented method, not inferred from design.
- [ ] Dependency versions, licenses, and known limitations accompany the release.
