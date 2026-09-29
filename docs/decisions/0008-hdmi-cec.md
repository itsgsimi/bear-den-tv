# ADR 0008 — TV control over HDMI-CEC through the kernel CEC API

Date: 2026-09-28. Status: accepted.

Bear Den exists so the TV can stay a dumb HDMI screen that is never on the
network. The owner still wants the TV to follow the box: turn off with the
sleep timer, come on and switch to Bear Den's input when someone presses a
key, and, for some, take the phone's volume buttons. HDMI-CEC does that over
the HDMI cable itself: a slow shared bus between the devices on one HDMI
chain, with no network involved.

Most mini PCs cannot speak CEC on their HDMI port. It needs a USB adapter
(Pulse-Eight's USB-CEC Adapter, which the kernel drives as `pulse8-cec`) or a
board whose HDMI transmitter has CEC wired up (some Intel NUC boards with a
Pulse-Eight chip on the board, Raspberry Pi, many ARM boards). Linux exposes
each one as `/dev/cecN` through the kernel CEC framework
(`Documentation/userspace-api/media/cec`). The reference TV box has no
`/dev/cec*`, so none of this can be tried on it.

## Options

| Option | For | Against |
|---|---|---|
| **Kernel CEC API directly**: `ioctl` on `/dev/cecN` through `golang.org/x/sys/unix` (already a dependency) | no cgo, no new package on the box, nothing to install; bounded calls (the kernel does the retries and waits for a reply up to a timeout we set); errors come back as data we can turn into reasons | we encode the structs ourselves (`struct cec_msg` 56 bytes, `struct cec_log_addrs` 92, `struct cec_caps` 76) and the message bytes |
| `cec-ctl` from v4l-utils with a fixed argv (like the audio backend's `pactl`) | the tool does the encoding | another system package to ask the owner to install; one process per key press on a 2-core box; parsing its text output for the power status; its own logical address claim per run unless kept running |
| libcec (`cec-client`, or cgo bindings) | the most widely used stack, handles vendor quirks | cgo or a long-running child process; a second, user-space CEC stack that competes with the kernel's for adapters the kernel already drives |

## Decision

1. **Kernel CEC API, no cgo.** `internal/platform/cec` opens the first
   `/dev/cec*`, reads `CEC_ADAP_G_CAPS`, and on first use claims one
   **playback device** logical address (`CEC_ADAP_S_LOG_ADDRS`, CEC 1.4,
   OSD name "Bear Den TV"); an adapter that is already configured (another
   program, or one that configures itself) is used as it is. It stays in the
   default mode (initiator, no follower): the kernel answers the core
   messages other devices ask of us (Give Physical Address, Give OSD Name).
2. **Five things only**, each one `CEC_TRANSMIT` in blocking mode:
   Image View On (`0x04`) and Standby (`0x36`) to the TV, Active Source
   (`0x82` + our physical address) broadcast, User Control Pressed/Released
   (`0x44`/`0x45`) with Volume Up, Volume Down, Mute Function and Restore
   Volume Function to the audio system when one acknowledges a poll, else to
   the TV, and Give Device Power Status (`0x8F`, the kernel waits for Report
   Power Status `0x90`, 1 s). The struct layouts and ioctl numbers are
   checked in tests against the values `linux/cec.h` produces.
3. **Bounded and off the coordinator's lock.** Each call runs with a context
   deadline (3 s from the coordinator); the ioctl runs on its own goroutine
   so a stuck adapter cannot hold a caller past its deadline, and calls are
   serialized so the bus sees one message at a time. Wake and Home send
   their CEC messages in the background: an action never waits on the bus.
4. **Off by default, fail closed with a reason.** Nothing is sent unless an
   adapter is present **and** the owner turned on `cec.enabled`
   ([`contracts/config.md`](../../contracts/config.md)). No device: "No
   HDMI-CEC device (/dev/cec*) — most PCs need a USB CEC adapter". A device
   the session user may not open says so and points at the `video` group;
   Bear Den never changes groups or udev rules itself.
5. **Volume stays the PC's unless the owner chooses the TV**
   (`cec.volume_target: pc | tv`) rather than separate TV volume actions:
   the phone keeps one volume group, labelled for what it drives. When the
   TV is chosen but CEC is not working, volume is unavailable with the
   reason; it never falls back to the PC silently.

## Consequences

- No new dependency and nothing to install beyond the adapter. A Pulse-Eight
  USB adapter still needs `inputattach --pulse8-cec` (from linuxconsoletools)
  to create `/dev/cecN` on most distributions; [`docs/operations.md`](../operations.md#tv-control-over-hdmi-cec)
  says how to check.
- Everything is tested against a fake ioctl layer and a fake TV, and **not
  seen on hardware**: the reference box has no CEC device. Vendor quirks
  (TVs that ignore Mute Function, TVs that do not answer power status in
  standby) are unknown until someone with an adapter tries it.
- The CEC bus is local wire only; nothing about it crosses the network
  ([`docs/security.md`](../security.md)).
