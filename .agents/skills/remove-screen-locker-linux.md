---
name: "remove-screen-locker-linux"
description: "Non-destructively remove/disable the screen lock and blanking on a Linux (Mint/Ubuntu Xfce, lightdm) box over ssh: identify mechanism, user-level overrides, PM display-sleep root cause, verify across reboot; includes auto-login distinction and revert."
---

# Remove the screen lock from a Linux desktop box (non-destructive)

Owner-requested removal of an idle screen lock. Default to the **user-level, no-sudo, reversible** path; `apt remove` is heavier and needs its own explicit OK.

## 1. Identify the mechanism first (read-only probe)
```sh
ssh -o BatchMode=yes "$BDTV_TARGET" '
  who; loginctl list-sessions --no-legend
  loginctl show-session <session-id> -p Active -p LockedHint
  pgrep -af "light-locker|screensaver|xdg-screensaver|swaylock|xautolock|xlock"
  ls ~/.config/autostart/; grep -E "^(Exec|Hidden|NoDisplay)" /etc/xdg/autostart/light-locker.desktop 2>/dev/null
  systemctl --user list-unit-files --no-legend | grep -i lock
  DISPLAY=:0 XAUTHORITY=$HOME/.Xauthority xset q | grep -A2 -iE "Screen Saver|DPMS"'
```
Key facts: locker binary, user- vs system-level autostart (`/etc/xdg/autostart`), generated unit name (like `app-light\x2dlocker@autostart.service`), X screensaver timeout.

## 2. Distinguish before changing anything
- **Idle lock** ≠ **greeter password** ≠ **auto-login**. Auto-login (`autologin-user=` in `/etc/lightdm/lightdm.conf`) only skips the boot greeter; it does nothing against idle locking. Check whether it's already set before proposing it.
- TV/appliance requests usually mean: locker off + blanking off, permanently.

## 3. Apply (user-level, reversible)
1. Override system autostart per XDG spec — `~/.config/autostart/light-locker.desktop`:
   ```ini
   [Desktop Entry]
   Name=Screen Locker
   Comment=Disabled by owner
   Exec=true
   Hidden=true
   X-GNOME-Autostart-enabled=false
   ```
2. Stop the running instance: `systemctl --user stop 'app-light\x2dlocker@autostart.service'` (in double-quoted remote strings write `"app-light\\x2d..."`). Generated autostart units have `Restart=no`; fallback `pkill -TERM -x light-locker`.
3. Blanking off — **root cause on Xfce: xfce4-power-manager applies its display-sleep setting during session init** (default `sleep-display-ac` = 10 min → writes `xset s 600`) and this happens *despite* `dpms-enabled=false`, landing after early autostart entries. Fix at config level, then race-proof:
   - `xfconf-query -c xfce4-power-manager -p /xfce4-power-manager/sleep-display-ac -s 0 --create -t int` (Never; needs `XDG_RUNTIME_DIR=/run/user/$UID` + `DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/$UID/bus`; older xfconf `-lp` fails — use `-l`)
   - plus a **delayed** autostart entry (`sleep 20` / `sleep 40` retries) running `xset s off -dpms`, with a debug tail such as `date >> ~/.cache/no-blank.log; xset q | grep -i timeout >> ~/.cache/no-blank.log 2>&1` so the next login is auditable.

## 4. Verify honestly
- `pgrep -x light-locker` — `-x` only; `pgrep -f` matches your own ssh command line and fakes "still running".
- After any blanking fix wait ~25 s and re-check `xset q` timeout (PM tick test); after a reboot, check it again *and* read the autostart debug log.
- `desktop-file-validate` every new/edited autostart file: **unescaped `$` inside a quoted Exec value is an error** — prefer `~` shell expansion over `$HOME`, or escape `\$`.
- A reboot is the real test of autostart suppression; only do one with explicit owner OK (network-critical boxes), and snapshot pre-reboot state first (Bear Den/home services such as DNS, what's playing).

## Pitfalls
- Never print or stash sudo passwords beyond immediate use (`sudo -S` from stdin + `shred -u` the temp file; note write tools may resolve a literal `~` relative to cwd — pass absolute paths).
- **Do not bounce xfce4-power-manager from ssh to test init behavior**: it applies its own xset settings on the way up, then crashes outside a full session environment (assert trap), leaving PM dead until next login. Infer from config values instead.
- Revert = delete user autostart files + `xfconf-query --unset` the two pm properties, re-login.
- If a "lock on suspend" hook exists and the box ever suspends, mention it as residual risk.
