# Real flatpak output for the installer tests

Captured 2026-09-28 with Flatpak 1.14.6 in an `ubuntu:24.04` Docker
container (`apt install flatpak dbus dbus-user-session` inside the
container), running as an unprivileged user with `LC_ALL=C.UTF-8`, a
private D-Bus session and a system bus (flatpak asks malcontent over it
before deploying an app). The app is Moonlight
(`com.moonlight_stream.Moonlight`, from the adapter table), with its KDE
6.11 runtime not installed. Nothing was installed on the development
machine itself.

| File | Command | Notes |
|---|---|---|
| `remote-info-moonlight.stdout` | `flatpak remote-info --user flathub com.moonlight_stream.Moonlight` | Download 7.7 MB, installed 18.6 MB, runtime `org.kde.Platform/x86_64/6.11` |
| `remote-info-kde-runtime.stdout` | `flatpak remote-info --user flathub runtime/org.kde.Platform/x86_64/6.11` | 409.9 MB, 1.1 GB |
| `remote-info-missing.stderr` | `flatpak remote-info --user flathub org.example.NotThere` | exit 1 |
| `info-runtime-missing.stderr` | `flatpak info --user runtime/org.kde.Platform/x86_64/6.11` | exit 1 (same with `--system`) |
| `remote-add-offline.stderr` | `flatpak remote-add --user --if-not-exists flathub <url>` with `--network none` | exit 1. With the network the first call and every later one print nothing and exit 0 (`--if-not-exists`). |
| `install-moonlight.stdout`, `.timing` | `flatpak install --user --noninteractive -y flathub com.moonlight_stream.Moonlight` | exit 0 in 16.5 s; `.timing` is when each line arrived (seconds). One line per operation, **no percentages**, also in a pty. Its stderr had only three `bwrap: Can't mount proc` lines (triggers need privileges a container lacks). `du` of the user installation afterwards: 2.5 GB. |
| `install-offline.stderr` | the same install with `--network none`, after the remote was added | exit 1 |
| `install-diskfull.stdout`, `.stderr` | the same install into a 150 MB tmpfs as `~/.local/share/flatpak` | exit 1; optional extensions fail with `Warning:`, the runtime with `Error:` |
| `install-cancel.stdout` | the same install, SIGTERM after 4 s | exit 1; ends with the cursor escape `ESC[?25h`; the three extensions it had finished stayed installed |
| `install-after-cancel.stdout` | the same install again | exit 0 in 12 s, carrying on where the cancelled one stopped |
| `install-already.stderr` | the same install once it is installed | exit 0 |
| `update-uptodate.stdout` | `flatpak update --user --noninteractive -y com.moonlight_stream.Moonlight` | exit 0, nothing to do |

The container recipe and the installer's own run through `bear-den-tv apps
install --here` are in [`docs/operations.md`](../../../../docs/operations.md#app-installs).
