# ADR 0007 — The Wayland profile

Date: 2026-09-28. Status: accepted.

The reference TV runs X11. Before this ADR, a Wayland session got an adapter
that reported every desktop capability unavailable, so the target was always
unknown: navigation and media were refused, Home worked only while the shell
was already in front, and a launch waited 30 s for a window it could never see
and then reported failure. This ADR decides what Bear Den does on Wayland and
how it says what it cannot do.

## What a Wayland client can do, by compositor family

A normal Wayland client cannot see or control other clients' windows. Anything
more is a protocol the compositor chooses to offer.

| Family | Window list | Which one is active | Activate / close / fullscreen another | What we do |
|---|---|---|---|---|
| wlroots and friends: sway (tested), labwc, Hyprland, Wayfire, niri (not tested) | `zwlr_foreign_toplevel_manager_v1` (wlr-protocols) | yes (`state` = activated) | yes (`activate(seat)`, `close`, `set_fullscreen` from version 2) | **implemented** |
| compositors with only `ext_foreign_toplevel_list_v1` (the newer staging protocol) | yes | no | no | unavailable, reason says so |
| KDE Plasma (KWin) | `org_kde_plasma_window_management`, restricted to clients KWin trusts; KWin scripting over D-Bus | yes | yes | unavailable: a KWin bridge is possible but not built |
| GNOME (Mutter) | none for clients | no | no | unavailable: only a GNOME Shell extension could do it; out of scope |

Input injection:

| Path | Consent | Works on | Decision |
|---|---|---|---|
| xdg-desktop-portal **RemoteDesktop** (`NotifyKeyboardKeycode`, or libei via `ConnectToEIS`) with a restore token stored in the data dir | a one-time dialog on the TV screen | GNOME, KDE (xdg-desktop-portal-wlr has no RemoteDesktop) | not built |
| wlroots **virtual-keyboard** (`zwp_virtual_keyboard_v1`), re-checking the activated toplevel before each tap | none (sway offers it to every client) | wlroots | not built; the most direct next step on wlroots |
| **uinput** | a udev rule, i.e. a system change | everything | out of scope unless the owner asks |

## Decision

1. **The adapter speaks the wire protocol itself.** `internal/platform/wayland`
   has a small hand-written client ([`wire.go`](../../internal/platform/wayland/wire.go),
   [`client.go`](../../internal/platform/wayland/client.go)) for `wl_display`,
   `wl_registry`, `wl_callback`, `wl_seat` and the wlr foreign-toplevel pair.
   No cgo and no new dependency: these five interfaces carry no file
   descriptors and fit in a few hundred lines, less to audit than a
   generated client library.
2. **On wlroots** the adapter is `wayland-wlr`: `observe_foreground` and
   `activate` are available (backend `wlr-foreign-toplevel`). The foreground
   is the one toplevel whose committed state says activated; none or more
   than one is an unknown foreground, never a guess. Changes are reported
   once per compositor flush, so a focus switch is never seen as a moment
   with nothing in front (which would bump the epoch). Close and fullscreen
   use the same handle. Home works: the coordinator activates the shell.
3. **Windows are matched by app_id** where X11 has WM_CLASS
   (`platform.WindowInfo.AppID`). An adapter matches its Flatpak id exactly,
   or its WM_CLASS fragments as substrings (an XWayland window's app_id is its
   X11 class); the shell matches `bear-den-tv-shell`. The real clients' app_ids
   on Wayland are UNVERIFIED.
4. **Everywhere else** the adapter is `wayland-limited` and names the family's
   reason. Things that do not depend on window control stay available on any
   Wayland session: `app.launch` through Flatpak (reported **delivered**, with
   `observed: false`, when the desktop cannot see windows; liveness then comes
   from `flatpak ps`), media through MPRIS, PC volume through `pactl`, and
   lock observation through logind `LockedHint` and
   `org.freedesktop.ScreenSaver`, now passed to the Wayland adapter like the
   X11 one.
5. **Input stays unavailable** with its reason, on every family. A key sent
   to the wrong window is worse than no key. The path, when someone builds it:
   virtual-keyboard on wlroots (upload an xkb keymap through a memfd, tap
   evdev codes, re-read the activated toplevel before each tap, as XTEST
   re-reads `_NET_ACTIVE_WINDOW`); the RemoteDesktop portal on GNOME and KDE
   (one consent on the TV, restore token in the data dir, same re-check
   where the foreground is observable). `physical_home` needs the
   GlobalShortcuts portal and is not built.
6. **The TV shell runs through XWayland.** The toolchain's Qt has no
   `wayland` platform plugin, and the `.deb` launcher keeps
   `QT_QPA_PLATFORM=xcb`: one tested rendering path (xcb + EGL) on every
   session, and on wlroots the XWayland window still reports app_id
   `bear-den-tv-shell`. Revisit if a Wayland-only compositor (no XWayland)
   matters.

## Consequences

- On sway and other wlroots compositors Bear Den launches, observes, brings
  the shell back (Home), closes and fullscreens apps, and refuses remote nav
  and media keys into apps (media through MPRIS still works where the app
  offers it). The shell itself is fully navigable from the phone, because
  shell input goes over IPC, not injection.
- On GNOME and KDE the phone drives the shell and launches apps, but once an
  app is in front the target is unknown until the viewer leaves it on the TV.
- Lock observation on sway depends on the locker: swaylock sets no logind
  `LockedHint` by itself, so a locked sway session may not be seen as locked.
  This is not solved here and is stated in the docs.
- Proof: unit tests against a fake compositor
  ([`wayland_test.go`](../../internal/platform/wayland/wayland_test.go)) and
  `scripts/wayland-container-test.sh`, which runs headless sway in an
  `ubuntu:24.04` container, cross-checks the adapter against `swaymsg`, runs
  `bear-den-tv doctor --probe`, and with `--shell` runs the coordinator with
  the real shell. Nothing here has been seen on real hardware; the reference
  TV is X11.
