# App performance: built for the floor, scaled to the box

Bear Den runs Plex HTPC, YouTube (VacuumTube) and Moonlight as they are and
never patches them. What it does is give each app **opinionated settings that
fit the box**. The optional apps (Spotify, Jellyfin Desktop, RetroArch) and
the web apps (Netflix, Disney+, Hulu in Google Chrome, the Browser in Brave) are listed too,
with nothing tuned yet ([below](#spotify-jellyfin-retroarch-optional-apps-no-tunable-settings)). The reference box is a 2-core Celeron 2955U. It is the **floor**
Bear Den is designed and tested on, and it gets the careful settings. Anything
faster gets more automatically: 4K, higher frame rates, full-quality scaling.

Code: [`internal/applications/tuning/`](../internal/applications/tuning/)
(`tuning.go` probe and tiers, `settings.go` the hand-adjustable catalog,
`plans.go` settings files, `detect.go` the detection test, expectations and
caveats), tests beside it. The coordinator's hook is
[`internal/session/tuning.go`](../internal/session/tuning.go).

## Principles

1. **Decode on the GPU/VPU.** Video decoding is the heaviest work a TV box
   does, and fixed-function decoders do it at almost no CPU cost. Ask for codecs
   this box decodes in hardware.
2. **When hardware can't, pick the cheapest codec that still fills the
   screen.** On a 1080p screen, H.264 is as sharp as VP9 and far cheaper to
   decode in software.
3. **Heavy work belongs to the big machines.** The Plex server transcodes. The
   gaming PC encodes games on its GPU (Sunshine/Apollo with NVENC, AMF or Quick
   Sync). The TV box only decodes and shows.
4. **Never more than the screen shows.** Pixels above the output resolution
   and frames above its refresh rate are decoded only to be thrown away.
5. **Restraint only where the hardware needs it.** Entry-level boxes get the
   careful settings. Capable boxes keep each app's full-quality defaults.
   Desktops can decode 4K in software.
6. **Hardware-agnostic.** Decisions come from what the machine *reports*
   (decoder elements, core count, clocks, display modes), never from GPU or CPU
   model tables. The same rules work on Intel, AMD, NVIDIA and ARM.
7. **The owner stays in charge.** Settings are written only while the app is
   closed. Every file is backed up. Tuning can be turned off, and the tier can
   be overridden.

## The detection test

| When | What happens |
|---|---|
| 20 s after the coordinator starts | Detect everything. Apply the plans of apps that are closed; mark running apps *pending*. |
| When a pending app closes | Re-read its settings file, apply the plan, and show "YouTube is now tuned for this TV". |
| `bear-den-tv apps detect` | The same test on demand. Dry run unless `--apply`. |

It reads three things:

1. **The box.** Core count, each core's top clock (cpufreq, else
   `/proc/cpuinfo`), memory. These give the **tier** (below).
2. **The display.** From `xrandr --current`: the output, its current mode and
   refresh, and whether the TV offers 4K, at what refresh.
3. **Each app's hardware decoding.** Its own Flatpak runtime is probed (next
   section).

Results appear on the TV in **Settings → Playback**: each app's status (Tuned,
Best settings applied, Will be tuned when … closes, Automatic tuning is off,
Could not apply), what it decodes on the GPU, what to expect, the caveats, and
the changes. The coordinator publishes this as `state.playback`
([`contracts/state.schema.json`](../contracts/state.schema.json)); the screen is
[`apps/tv-shell/qml/PlaybackScreen.qml`](../apps/tv-shell/qml/PlaybackScreen.qml).

## Box tiers

The tier covers what lands on the CPU and GPU when hardware decoding doesn't
take it. `Classify` in `tuning.go` computes it from the number of logical CPUs
and the **sum of their top clocks** (GHz):

| Tier | Rule | Examples (total GHz) | What it changes |
|---|---|---|---|
| `entry` | ≤ 2 logical CPUs, or < 8 GHz in total | **Celeron 2955U (2.8), the reference box**; Atom x5-Z8350 (7.7); Raspberry Pi 4 (6–7.2) | mpv's `fast` profile in Plex; no YouTube "Super resolution"; Moonlight at most 1080p; 120 Hz and CPU-decoding caveats |
| `standard` | everything between | Intel N100 (13.6), N5105 (11.6), J4125 (10.8); Raspberry Pi 5 (9.6); RK3588 (16.8) | each app's full-quality defaults; 4K wherever the hardware decodes it |
| `high` | ≥ 8 logical CPUs and ≥ 24 GHz | 4-core/8-thread laptop at 3.4 GHz (27); desktops | as standard, plus 4K YouTube decoded on the CPU when the GPU can't |

When the clock is unknown (some virtual machines), cores decide alone: ≤ 2 is
entry, ≥ 8 is high, anything else standard.

- **Why cores × clock.** It is stable, so the same box always lands in the same
  tier and settings never flip between runs. It is also vendor-agnostic. It
  can't see per-clock differences: an old 4-core Atom such as the J1900 (9.7)
  lands in *standard*.
- **Override.** Set `BDTV_TIER=entry|standard|high` in the coordinator's
  environment. `bear-den-tv apps detect --tier standard` previews what another
  tier would get.
- **Validation status.** The rule is live-validated on the reference box only.
  The other examples are computed from published core counts and clocks, not
  measured.

## How Bear Den knows what the box can decode

Each Flatpak app runs on its own runtime, which ships its own video drivers,
so the same GPU can decode different codecs in different apps. On the
reference box (Intel Haswell), Plex and Moonlight get H.264 in hardware.
VacuumTube's newer runtime only has Intel's `iHD` driver, which does not support
Haswell, so there YouTube decodes on the CPU.

The probe asks each app's runtime directly. It runs
`nice -n 10 flatpak run --command=gst-inspect-1.0 <app>` (the app itself does
not start) and reads which **hardware** decoder elements GStreamer can build:

| Family | Hardware | Elements (examples) |
|---|---|---|
| VA-API | Intel, AMD (Mesa) | `vah264dec`, `vah265dec`, `vavp9dec`, `vaav1dec`; legacy `vaapih264dec` |
| NVDEC | NVIDIA | `nvh264dec`, `nvh265dec`, `nvvp9dec`, `nvav1dec` |
| V4L2 | ARM SoCs (Raspberry Pi, Rockchip, …) | `v4l2slh264dec`, `v4l2h264dec`, … |
| Quick Sync | Intel (oneVPL/MSDK) | `qsvh264dec`, `msdkh265dec`, … |

These families register a decoder only for codecs the device's driver supports,
so the list is a faithful capability report. Software decoders (`avdec_h264`,
`vp9dec`, `dav1ddec`, …) never count.

**Vulkan decoders are ignored.** `vulkanh265dec` and friends are registered even
when the GPU can't decode (seen on the Haswell box).

## The settings

### Moonlight (`…/Moonlight Game Streaming Project/Moonlight.conf`, `[General]`)

| Setting | Value | Why |
|---|---|---|
| `videocfg` (codec) | `1` (H.264) when H.264 is the only hardware codec, or when unknown; `0` (auto) when HEVC/AV1 decode in hardware | The PC encodes what this box's GPU decodes; HEVC or AV1 would fall to the CPU here. |
| `videodec` | `0` (auto) | Hardware first, software only as a fallback, never forced to software. |
| `width` × `height` | the largest of 720p, 1080p, 1440p and 4K that fits the TV output's current mode; at most 1920×1080 on an entry box or when only H.264 decodes in hardware (1080p when the mode can't be read) | Extra pixels are decoded only to be scaled away. 4K H.264 at game frame rates is beyond older decoders. |
| `fps` | the output's refresh rate (120 on a 120 Hz TV; 60 when it can't be read) | The smoothest motion the screen can show; entry boxes get a note to lower it if games stutter. |
| `bitrate` | Moonlight's own default for the new size and frame rate, only while the file still holds the default for the old ones | Each frame keeps its quality; a bitrate the owner set is kept. |
| `vsync`, `framepacing` | `true` | No tearing. Frame pacing gives smooth motion on a TV, at the cost of up to one frame of latency. |
| `windowmode` | `0` (fullscreen) | Nothing else is composited over the stream. |

Values come from moonlight-qt's `StreamingPreferences`: `VCC_AUTO=0`,
`VCC_FORCE_H264=1`, `VCC_FORCE_HEVC=2`, `VCC_FORCE_AV1=4`; `VDS_AUTO=0`;
`WM_FULLSCREEN=0`.

Advice it can't apply: keep the gaming PC's Sunshine/Apollo on a **hardware**
encoder (NVENC, AMF or Quick Sync).

### Plex HTPC (`…/tv.plex.PlexHTPC/data/plex/mpv.conf`)

Plex HTPC plays through mpv and reads `mpv.conf` before every playback. Its own
`mpv.conf.md` says `hwdec`, scalers, dithering and debanding set by Plex can be
overridden there. Bear Den adds one marked block and leaves everything else in
the file alone:

```
# >>> bear-den-tv: playback settings (managed by `bear-den-tv apps detect`)
hwdec=auto-safe          # decode on the GPU/VPU when it can (VA-API, NVDEC, V4L2, …), else on the CPU
video-sync=audio
interpolation=no
profile=fast             # entry-level box: bilinear scaling, no dithering or peak detection
# <<< bear-den-tv
```

- `hwdec=auto-safe` works with any vendor and falls back safely.
- `profile=fast` is written on **entry** boxes only; everywhere else mpv keeps
  its full-quality scaling. mpv has had the profile since 0.37; Plex HTPC ships
  mpv 0.38 (checked on the reference box).
- The block is found by its `# >>> bear-den-tv` prefix, so blocks written by
  older releases are replaced in place, never duplicated.

Advice it can't apply: for files in codecs the box can't decode in hardware (on
the reference box, anything but H.264), let the **server** do the work. Play
them at a quality below *Original* (the server transcodes to H.264), or create
H.264 **Optimized Versions** on the server.

### YouTube / VacuumTube (`…/rocks.shy.VacuumTube/config/VacuumTube/config.json`)

| Setting | Value | Why |
|---|---|---|
| `hardware_decoding` | `true` | Use the GPU whenever the runtime can. |
| codec filter (`h264ify`, `h264ify_disable_*`) | AV1 in hardware: filter off. VP9 only: block AV1 (and VP8). Neither: H.264 only (block WebM, VP8, VP9, AV1), except on a **high** box driving a 4K screen, where the filter is off and the CPU decodes 4K VP9/AV1. | YouTube picks another codec when one is blocked, so it only sends what this box decodes cheaply, or what a desktop can afford. |
| `remove_super_resolution` | `true` on **entry** boxes; elsewhere left out (off), or set back to `false` if it was on | Skips YouTube's AI-upscaled "Super resolution" streams, which are heavier to decode. |
| `low_memory_mode` | `true` with ≤ 4.5 GiB RAM; elsewhere left out (off), or set back to `false` if it was on | YouTube's own low-memory mode. |
| `pause_on_blur` | `true` | Pauses when Bear Den's Home is in front instead of decoding video nobody sees. Resume with Play. **Background audio stops too.** |
| `fullscreen` | `true` | Starts fullscreen (Bear Den also enforces it). |

VacuumTube also reads extra Chromium flags from `flags.txt` in the same folder.
Bear Den doesn't need any today.

### Spotify, Jellyfin, RetroArch (optional apps): no tunable settings

These are rows in `Apps` with `NoTuning` plans: no settings file, no changes,
nothing to adjust by hand, and one note that says why. Detection still lists
them with what to expect.

| App | Why Bear Den tunes nothing |
|---|---|
| Spotify | It streams compressed audio, which any box plays without help. |
| Jellyfin Desktop | Its playback settings live on the Jellyfin server and in its own profile; Bear Den does not edit them yet. If a file stutters, lower its quality in the player so the server converts it. |
| RetroArch | Video and audio settings belong to each core and game; `retroarch.cfg` is the owner's. On an entry box: lightweight cores, shaders off. |

### Web apps (Netflix, Disney+, Hulu, Browser): no tunable settings

Each has a `NoTuning` row (`webRow` in `detect.go`) on its default browser's Flatpak id (Google Chrome's for the streaming sites, Brave's for the Browser):
the site chooses its own quality, and Bear Den leaves the browser's settings
alone. What to expect: in a Linux browser Netflix and Disney+ stop at about
720p and Hulu may go lower ("Up to 720p" on the tiles), and playback needs
Widevine (bundled with Google Chrome; [ADR 0014](decisions/0014-google-chrome-for-streaming-brave-for-browser.md)). How
Chrome decodes video on an entry box, and what a 720p stream costs on the
reference box's two cores, is **not measured**.

## What to expect, and the caveats

Each app gets plain "expect" lines: what plays, up to what resolution, and
whether it decodes on the GPU or the CPU. It also gets notes: **⚠ warn** for
something the owner will notice, **• info** for advice.

| Where | Note | When |
|---|---|---|
| Box | The TV output runs at 120 Hz: menus draw twice the frames of 60 Hz (info) | entry box, refresh > 61 Hz |
| Box | 4K TV at 1080p: "4K output here tops out at 30 Hz — keep 1080p" | the TV offers 4K only below 50 Hz |
| Box | 4K TV at 1080p: keep 1080p (4× the pixels for an entry GPU) / "this box can drive it" | 4K ≥ 50 Hz offered; entry / standard or high |
| Box | 4 GiB of RAM or less: YouTube's low-memory mode | ≤ 4.5 GiB |
| YouTube | No hardware decoding: 4K and HDR unavailable (warn) | the runtime has no hardware decoder |
| YouTube | 1080p60 keeps an entry CPU busy; pick 1080p30/720p if it stutters (warn) | entry box, CPU decoding |
| YouTube | 4K needs VP9/AV1 hardware decoding; YouTube stops at 1080p | 4K screen, H.264-only hardware |
| Plex | HEVC and 4K HDR aren't hardware-decoded: play below Original (warn; info on a high box) | no HEVC in hardware |
| Moonlight | Set the gaming PC to 16:9 for a full-screen picture | always |
| Moonlight | The TV runs at 120 Hz: raise Moonlight to 120 fps for games; on an entry box, watch the statistics overlay (Ctrl+Alt+Shift+S) for dropped frames | refresh > 61 Hz, hardware decoding |

## On the reference box (Celeron 2955U, 2 × 1.4 GHz, 7.6 GiB, Haswell GPU)

Tier **entry**. Display: HDMI-1 at 1920×1080, 120 Hz, on an LG 4K TV that
offers 4K only at 30 Hz. The probe reports:

| App | Runtime | Hardware decoding |
|---|---|---|
| Plex HTPC | freedesktop 23.08 (Intel i965 + iHD drivers) | H.264 |
| YouTube (VacuumTube) | freedesktop 25.08 (iHD only; no Haswell support) | none: CPU; H.264 keeps 1080p affordable |
| Moonlight | KDE 6.8 | H.264 |

The plan (dry run, 2026-09-22):
- **Moonlight:** H.264 and frame pacing.
- **Plex:** the mpv block, with `profile=fast`.
- **YouTube:** skip Super resolution, pause when Home is in front, start
  fullscreen. The H.264-only filter and hardware decoding were already on.

The caveats these rules give it are checked by `TestCaveatsForTheReferenceBox`
against the box's recorded `xrandr` output ([`testdata/xrandr-reference-box.txt`](../internal/applications/tuning/testdata/xrandr-reference-box.txt)):
- 120 Hz output; 4K only at 30 Hz, so keep 1080p.
- YouTube has no hardware decoding: 4K and HDR are unavailable; choose 720p
  or 1080p30 if 1080p60 stutters.
- Plex should convert HEVC.
- Moonlight: set the gaming PC to 16:9; 120 fps is worth trying while watching
  for dropped frames.

## Machine-level advice (not applied by Bear Den)

- **120 Hz output.** On an entry box, 60 Hz halves the frames that menus and
  animations draw. Video is 24–60 fps either way. On capable boxes 120 Hz costs
  little and makes Moonlight's 120 fps streams possible.
- **4K output.** Worth it when the TV offers 4K at 50 Hz or more and the box is
  standard or high. Keep 1080p when 4K tops out at 30 Hz.
- **Wired Ethernet** for Moonlight and high-bitrate Plex.

## Safety and undo

- **Automatic by default.** Settings are applied after startup and when a
  pending app closes. To turn that off, set `"startup": {"tune_apps": false}`
  in `config.json` ([`contracts/config.md`](../contracts/config.md)). The report stays available as
  advice (status *Automatic tuning is off*), and `apps detect --apply` still
  works.
- **Apps must be closed.** Moonlight and Plex rewrite their settings when they
  exit, so a running app is marked *pending* and tuned when it closes.
- **Backups.** Every changed file is backed up next to the original as
  `<file>.bak-bear-den-<date>-<time>` and replaced atomically. To undo, turn
  automatic tuning off, then copy the backup over the file while the app is
  closed. For Plex, deleting the marked block is enough.
- **Idempotent.** Once applied, the next run proposes no changes and the
  status reads *Tuned for this TV*.
- **Per setting.** To keep one setting your way (for example YouTube audio
  playing behind Home), choose it in Settings → Advanced playback (next
  section) instead of turning automatic tuning off.
- **Machine-readable.** `apps detect --json` prints the host (with its tier),
  the display, the probes, the plans (with each app's adjustable settings),
  the expectations and the notes.

## Adjusting settings by hand

**Settings → Advanced playback** on the TV lists every setting Bear Den
changes, app by app. Each row shows the value in effect; a small **Auto**
marker means Bear Den chose it. Left/Right picks another option; the
option's note (for example "decodes on the CPU here") shows under the
focused row. OK returns the row to Auto. The Settings row itself reads
*Auto* or how many settings were chosen by hand.

Only choices this box can handle are offered, from the same detection as
the automatic rules:

| App | Setting | Options | Filtered by |
|---|---|---|---|
| Moonlight | Video codec | Automatic, H.264; HEVC and AV1 | HEVC/AV1 only when Moonlight's runtime decodes them in hardware |
| Moonlight | Frame rate | 30, 60; the TV output's refresh rate | the refresh rate is offered when it is not 30 or 60 (120 on a 120 Hz TV, 50 on a 50 Hz one) |
| Moonlight | Resolution | 720p, 1080p; 1440p, 4K | 1440p/4K only when the output mode is that large, the box is not **entry**, and HEVC or AV1 decodes in hardware |
| Moonlight | Frame pacing | On, Off | — |
| YouTube | Video codecs | H.264 only; VP9 (no AV1); All (AV1, VP9) | VP9/AV1 only when decoded in hardware, or on a **high** box (noted as decoding on the CPU) |
| YouTube | AI-upscaled streams, Pause behind Home, Low-memory mode | On, Off | — |
| Plex | Video scaling | Fast, Standard; High quality | High quality (mpv's `profile=high-quality`) not on **entry** boxes |
| Plex | Hardware decoding | GPU when possible (`auto-safe`), CPU only (`no`) | — |

Changing a Moonlight frame rate or resolution also updates the bitrate while
it is still Moonlight's default for the old values.

**How choices persist.** The TV sends `playback.set` ([`contracts/ipc.md`](../contracts/ipc.md));
the coordinator checks the value against the options offered now (anything
else is refused with a reason), stores it in `config.json`:

```json
"playback": {"overrides": {"moonlight": {"fps": "60"}, "vacuumtube": {"pause_on_blur": "off"}}}
```

and re-plans that app. A closed app's file is written at once (with a
backup, as always); an open one is marked *pending* and written when it
closes. Automatic tuning after every start, `apps detect --apply`, and the
Playback screen all use the stored choices, so they survive restarts and are
never undone by detection. If a stored choice stops being offered (new
hardware, another tier, a hand-edited file), Bear Den uses Auto for it and the
Playback screen says so; the choice stays in the file and applies again if
the option comes back.

**Back to Auto.** Press OK on the row, or delete the setting from
`playback.overrides` (the coordinator removes empty objects itself). With
`startup.tune_apps: false` choices are still stored and shown, but nothing is
written to the apps' files; `apps detect --apply` applies them.

## Adding an app

1. Find where the app keeps its settings (inside its Flatpak data directory)
   and the documented meaning of each setting.
2. List the settings the owner may change by hand in `Catalog`
   ([`settings.go`](../internal/applications/tuning/settings.go)): an id, a label, the options this box can handle (with a
   note where one costs more here), and detection's choice as `Auto`.
3. Write `Plan<App>(path, caps, host, disp, overrides)` in [`plans.go`](../internal/applications/tuning/plans.go):
   - resolve the catalog with `Settings(...)` and write each setting's `Value`
     (an override when it is offered, else Auto), including returning a
     switch to its Auto value after an override is removed;
   - use `caps.Has(codec)` for hardware decoding; `host.Entry()`,
     `host.High()` and `host.LowMemory()` for the box; `disp` for the screen;
   - give every change a one-line reason;
   - keep unknown settings and the file's formatting as they were;
   - with no settings file yet, return a note and no changes.
4. Add a row to `Apps` in [`detect.go`](../internal/applications/tuning/detect.go) (Flatpak id, adapter name, label, plan)
   and a case in `Expectations` for what to expect and its caveats. Every
   adapter needs a row (`TestEveryAdapterHasATuningRow`); an app with nothing
   to tune gets `NoTuning(adapter, flatpakID, why)` and skips steps 1–3.
5. Test it in [`tuning_test.go`](../internal/applications/tuning/tuning_test.go) and [`settings_test.go`](../internal/applications/tuning/settings_test.go) against the reference
   box (`small`, the `haswellInspect` decoders, the `refDisplay` display) and a
   capable one (`standard` or `big`, `modernInspect`, `uhd60`): the catalog's
   options per tier, overrides honoured and refused, and idempotency (apply,
   plan again, expect no changes). Break the rule on purpose and check that
   the test fails.
6. Document the settings table and the hand-adjustable options here.

Done when:
- `go test -race ./internal/applications/tuning/` passes, and failed while the
  rule was broken on purpose;
- `make build && build/bin/bear-den-tv apps detect` lists the new app with its
  plan (a dry run; nothing is written).

The app itself must already be supported by Bear Den (adapter, launcher, window
matching); see [`internal/AGENTS.md`](../internal/AGENTS.md#add-an-app).
