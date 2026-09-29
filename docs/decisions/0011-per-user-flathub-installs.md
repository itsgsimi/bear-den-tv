# ADR 0011 — One-press app installs: per-user Flathub, owner consent

Date: 2026-09-28. Status: accepted.

Bear Den replaces a smart TV. On a smart TV, an app you don't have is one
press away. Until now Bear Den showed "Not installed" and a command to type
on a keyboard. The owner asked for a Steam-like experience: if an app (or
the browser the streaming sites need) is missing, pressing it installs it.
AGENTS.md rule 9 says nothing is installed without the owner saying so.
This ADR decides how both hold at once.

## Decision

- **Per user, from Flathub, no root.** Bear Den runs the `flatpak` CLI as
  the TV's user with `--user` on every call. Apps land in
  `~/.local/share/flatpak`. No sudo, no polkit prompt, no system helper, no
  system installation touched.
- **One owner press is the consent.** Install starts only from the TV's
  install card (Install is focused, Not now beside it), Settings → Add apps,
  the local CLI (`bear-den-tv apps install`), or the phone action
  `app.install`, which only the owner's phone may send (`contract.OwnerActions`;
  family phones, layout editors and guest passes get `forbidden`, and their
  snapshots carry no install state, so they draw no controls). Nothing
  installs on its own. Opening the card asks Flathub for the size: that
  network call is itself the result of an owner press.
- **What is fixed at compile time** (`internal/applications/install`):
  - the remote: `flathub`, per user, added with
    `flatpak remote-add --user --if-not-exists flathub https://dl.flathub.org/repo/flathub.flatpakrepo`
    when missing; no other remote is ever added or used;
  - the refs: only the Flatpak ids in the adapter table
    (`internal/applications/adapters`, incl. `adapters.ChromiumFlatpakID`);
    a phone or the shell names a config application id, and the id to
    install is looked up from its adapter. The installer refuses any id
    outside the table even if called directly;
  - the architecture: `x86_64` only (the installer refuses to run
    elsewhere);
  - the argv: `flatpak install --user --noninteractive -y flathub <id>`.
    Signature checks are Flatpak's default; nothing that weakens them
    (`--no-gpg-verify`, `--from`, `--sideload-repo`, another URL) can be
    built — tests assert every argv exactly.
- **Apps installed system-wide are installed.** Discovery reads
  `flatpak info --user` then `--system`; a system install is never changed
  and never updated by Bear Den ("Updated by your system").
- **Flatpak missing** → installs are unavailable with the reason "Flatpak
  isn't installed on this box". Installing Flatpak itself needs the system
  package manager, which Bear Den never runs. The owner runs, once:
  `sudo apt install flatpak` (Debian/Ubuntu; see
  [`docs/operations.md`](../operations.md#app-installs)).
- **Before downloading:** sizes from `flatpak remote-info --user flathub <id>`
  (download and installed size) plus the app's runtime when it is
  installed neither for the user nor system-wide. Extensions such as the GL
  drivers and codecs are not listed by `remote-info`, so the card says
  "plus shared parts if needed". Free space is checked with `statfs` on the
  user Flatpak directory: it must hold the app, twice its runtime when the
  runtime is new (the GL and codec extensions that come with it are about
  as big again), plus 512 MiB. Measured in the container: Moonlight's app
  and runtime are 1.1 GB, the real install with its extensions 2.5 GB;
  the estimate this rule gives is 2.2 GB.
- **Progress.** `--noninteractive` makes flatpak use its quiet transaction:
  it prints one line per operation (`Installing runtime/…`,
  `Installing app/…`) and **no percentages**, with or without a terminal
  (checked in the container). So the phase comes from those lines, and the
  percentage from how much the filesystem under the user Flatpak directory
  has filled, against the same estimate as the free-space check (without
  the margin), sampled every 2 s; it never goes backwards, holds at 99 when
  the extensions make the install bigger than the estimate (Moonlight in
  the container reached 99 % at 14 s of 16), and reaches 100 only after
  `flatpak info --user <id>` confirms the install. The UI updates on state
  pushes, not on a timer of its own.
- **Cancel** sends SIGTERM to flatpak's process group, then SIGKILL after
  5 s. Flatpak's transaction leaves the half-downloaded objects in the repo
  (reused by the next try) and deploys nothing partial.
- **Failures in plain words:** no network ("Couldn't reach Flathub. Check
  the TV's internet connection."), no space ("Not enough space on this
  box…"), everything else with flatpak's own last error line.
- **One install at a time.** A second one is `busy`.

## Updates

Config `apps.auto_update`, default **on** (the owner's "seamless" wish),
toggled in Settings → Keep apps up to date. While on, the coordinator runs
`flatpak update --user --noninteractive -y <ids>` for the adapter table's
apps that are installed **for this user**, at most once a day, and only
when all of these hold: nothing is playing, no app is running (never
during an app session), the session is unlocked and Bear Den is in front
(its screensaver may be on). An app starting cancels the update. System
installs are left to the system.

## Web apps and Chromium

Turning a streaming site on (Settings → Streaming sites) while Chromium is
missing opens the same install card for Chromium ("Browser for Netflix,
Disney+, Hulu"). After Chromium is installed, the Widevine step of
[ADR 0010](0010-web-apps-over-cdp-pipe.md) is what decides whether the
sites play; see that ADR and the status for what is automated.

## Consequences

- New outbound traffic, only on an owner press or the daily update while
  idle: `dl.flathub.org` (the remote file, metadata, objects).
- The coordinator can now write to `~/.local/share/flatpak` through the
  flatpak CLI. It still never runs a package manager, sudo or anything as
  root.
- Removing an app is the owner's: `flatpak uninstall --user <id>`
  ([`docs/operations.md`](../operations.md#app-installs)).
- Not verified on the TV. Tested with a fake runner fed with flatpak's real
  output, and for real in an Ubuntu 24.04 container as a non-root user.
