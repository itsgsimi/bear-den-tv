// All product copy for the phone remote, in English.
// Contract: views never contain literal user-facing strings; every label, notice,
// and error message is a member of `t`. Members that need values are functions.
// Server-provided `message` strings (action results, HTTP errors, capability
// reasons) are shown verbatim and are not translated here.

import type { NowPlayingStatus, Outcome, Permission, TargetKind, Transport } from './contract.ts';

const ACTION_NAMES: Record<string, string> = {
  'nav.up': 'Up',
  'nav.down': 'Down',
  'nav.left': 'Left',
  'nav.right': 'Right',
  select: 'Select',
  back: 'Back',
  home: 'Bear Den Home',
  'app.launch': 'Open app',
  'app.close': 'Close app',
  'media.play': 'Play',
  'media.pause': 'Pause',
  'media.seek_relative': 'Seek',
  'audio.volume_delta': 'PC volume',
  'audio.mute': 'PC mute',
  'text.submit': 'Text entry',
  'shell.restart': 'Restart shell',
  'power.sleep_timer': 'Sleep timer',
  'display.off': 'Screen off',
  'tv.power': 'TV power',
  'pointer.move': 'Touchpad',
  'pointer.click': 'Click',
  'pointer.scroll': 'Scroll',
  'app.install': 'Install app',
  'app.install_cancel': 'Cancel install',
  'app.uninstall': 'Remove app',
};

export const t = {
  productName: 'Bear Den TV',
  tagline: 'Phone remote',

  // Connection status.
  status: {
    idle: 'Not connected',
    connecting: 'Connecting…',
    online: 'Connected',
    reconnecting: 'Reconnecting…',
    offline: 'Offline',
  },

  // Tabs.
  tabs: {
    remote: 'Remote',
    editor: 'Layout',
    devices: 'Devices',
    badges: 'Badges',
    about: 'About',
  },

  // Pair screen.
  pair: {
    heading: (tvName: string) => `Pair with ${tvName}`,
    intro: 'Enter the six-digit code shown on the TV, or scan its QR code.',
    codeLabel: 'Pairing code',
    codePlaceholder: '000000',
    deviceNameLabel: 'This phone’s name',
    deviceNamePlaceholder: 'Name shown on the TV',
    connect: 'Connect',
    connecting: 'Connecting…',
    redeeming: 'Redeeming the invitation from the QR code…',
    needSixDigits: 'Type the six-digit code the TV shows.',
    loadingInfo: 'Looking for the TV…',
    paired: (tvName: string) => `Paired with ${tvName}`,
    infoFailed: 'Could not reach the TV. Check that you are on the same Wi‑Fi and try again.',
    errors: {
      invalid_invitation: 'That code was not accepted. Check the code on the TV and try again.',
      invitation_expired: 'That invitation has expired. Ask the TV for a new code.',
      too_many_attempts: 'Too many attempts. Wait a minute, then ask the TV for a new code.',
      network: 'Could not reach the TV. Check the Wi‑Fi connection and try again.',
      generic: (message: string) => (message ? message : 'Pairing failed.'),
    },
    revoked: 'This phone’s access was revoked on the TV. Pair again to continue.',
    passEnded: 'Your guest pass has ended',
    passEndedBody: 'Thanks for visiting. To use this phone as a remote again, ask for a new code on the TV.',
    loggedOut: 'You have been logged out.',
    sessionExpired: 'The session ended. Pair again to continue.',
    httpNotice: 'Trusted-LAN HTTP: pairing prevents casual unpaired control but does not encrypt traffic.',
  },

  // Remote screen.
  touchpad: {
    title: 'Touchpad',
    area: 'Touchpad for the web page on the TV',
    hint: 'Drag to move the pointer · tap to click · two fingers to scroll',
    click: 'Click',
    rightClick: 'Right-click',
  },

  remote: {
    controlling: 'Controlling',
    unverified: '(unverified)',
    targetLabel: (kind: TargetKind, label: string): string => {
      switch (kind) {
        case 'shell':
          return label || 'Bear Den TV';
        case 'app':
          return label;
        case 'unknown':
          return 'Unknown window (input paused)';
        case 'locked':
          return 'Locked';
        case 'none':
          return 'Nothing';
        default:
          return label;
      }
    },
    up: 'Up',
    down: 'Down',
    left: 'Left',
    right: 'Right',
    select: 'Select',
    back: 'Back',
    home: 'Bear Den Home',
    closeApp: (label: string) => `Close ${label}`,
    closeNothing: 'Close app',
    closeConfirm: (label: string) => `Tap again to close ${label}`,
    apps: 'Apps',
    launch: (label: string) => `Open ${label}`,
    installFromAddApps: (label: string) => `${label} is not installed on this TV. Install it under Add apps below.`,
    onlyOwnerInstalls: (label: string) => `${label} is not installed on this TV. Only the TV's owner can install apps.`,
    installingNow: (label: string) => `${label} is installing on the TV.`,
    ownerInstalls: "Apps marked Not installed can be installed from the TV, or from the owner's phone.",
    tileNotInstalled: 'Not installed',
    tileInstalling: (percent: number) => `Installing ${percent}%`,
    tileReady: 'Ready',
    launching: 'Launching…',
    playback: 'Playback',
    play: 'Play',
    pause: 'Pause',
    seekBack: '−30 s',
    seekForward: '+30 s',
    seekBackLabel: 'Seek back 30 seconds',
    seekForwardLabel: 'Seek forward 30 seconds',
    nowPlaying: 'Now playing',
    nowPlayingIn: (app: string) => `Now playing in ${app}`,
    nowPlayingStatus: (status: NowPlayingStatus): string => (status === 'playing' ? 'Playing' : status === 'paused' ? 'Paused' : 'Stopped'),
    nowPlayingProgress: (position: string, length: string) => `${position} of ${length}`,
    /** The card of an app still playing (or paused) with Bear Den in front: "Playing in YouTube · behind Home". */
    /** A reading from the owner's Plex server (Plex HTPC has no player Bear Den can control). */
    nowPlayingFromPlex: 'From your Plex server · read-only',
    nowPlayingBehind: (status: NowPlayingStatus, app: string): string => `${status === 'playing' ? 'Playing' : 'Paused'} in ${app} · behind Home`,
    pcVolume: 'PC volume',
    tvVolume: 'TV volume',
    volumeDown: 'Volume down',
    volumeUp: 'Volume up',
    mute: 'Mute',
    unmute: 'Unmute',
    text: 'Text entry',
    textTitle: 'Type on the TV',
    textWaiting: 'A text box is open on the TV. Type here and press Send.',
    textPlaceholder: 'Text to send',
    textHint: 'Up to 256 characters. Sent only to the text box that is open on the TV.',
    textSend: 'Send',
    textCancel: 'Cancel',
    textEmpty: 'Type something first.',
    textTooLong: 'Keep it under 256 characters.',
    textControlChars: 'Line breaks and control characters cannot be sent.',
    unavailable: (target: string) => `Not available while controlling ${target}.`,
    capabilityUnknown: 'This control is not available for the current target.',
    holdBusy: (device: string | null) =>
      device ? `Another phone (${device}) is holding a direction. Wait for it to finish.` : 'Another phone is holding a direction. Wait for it to finish.',
    holdOffline: 'Reconnecting to the TV — press again to repeat.',
    lockedTitle: 'The TV session is locked',
    lockedBody: 'Unlock it on the TV to continue. Every control is paused until then.',
    unknownTitle: 'Unknown window in front',
    unknownBody: 'Something Bear Den TV does not recognise is in the foreground. Input is paused so your presses do not go astray. Bear Den Home brings the TV back.',
    shellCrashedTitle: 'Bear Den TV shell is not running',
    shellCrashedBody: (state: string) =>
      state === 'circuit_open'
        ? 'The shell crashed repeatedly and automatic restarts were stopped.'
        : 'The shell stopped unexpectedly and is being restarted.',
    restartShell: 'Restart Bear Den shell',
    restartShellOwnerOnly: 'An owner phone can restart the shell.',
    staleRefreshed: 'The TV changed target; the remote refreshed itself.',
    ack: (outcome: Outcome): string => {
      switch (outcome) {
        case 'accepted':
          return 'Accepted';
        case 'delivered':
          return 'Delivered';
        case 'observed':
          return 'Done';
        case 'failed':
          return 'Failed';
        default:
          return outcome;
      }
    },
    sendFailed: 'Could not reach the TV. The press was not sent.',
    dpadHint: 'Press and hold a direction to repeat.',
    sending: 'Sending…',
    lastResult: 'Last press',
    running: 'Running',
    inFront: 'In front',
    demo: 'DEMO',
    demoBody: 'Development fixtures are active. Content and applications on the TV are demo data.',
    actionName: (action: string): string => ACTION_NAMES[action] ?? action,
  },

  // Sleep timer and screen off (views/sleep.tsx).
  sleep: {
    heading: 'Sleep',
    noTimer: 'No sleep timer. The TV pauses, goes Home and turns the screen off when it runs out.',
    sleepingIn: (left: string) => `Going to sleep in ${left}`,
    warning: (left: string) => `Going to sleep in ${left}. Press anything to stay awake.`,
    screenIsOff: 'The screen is off. Any button turns it back on.',
    choicesLabel: 'Sleep timer',
    chip: (minutes: number) => (minutes >= 60 && minutes % 60 === 0 ? `${minutes / 60} h` : minutes > 60 ? `${Math.floor(minutes / 60)} h ${minutes % 60}` : `${minutes} min`),
    chipLabel: (minutes: number) => `Sleep in ${minutes} minutes`,
    cancel: 'Cancel timer',
    screenOff: 'Screen off',
  },

  // Add apps (views/install.tsx): owner phones only.
  install: {
    heading: 'Add apps',
    note: 'Installs on the TV for its user, from Flathub. Nothing installs until you tap Install.',
    from: 'From Flathub',
    size: (download: string): string => `About ${download} to download, from Flathub`,
    install: 'Install',
    tryAgain: 'Try again',
    cancel: 'Cancel',
    preparing: 'Getting ready…',
    installing: (percent: number): string => `Installing… ${percent}%`,
    finishing: 'Finishing…',
    ready: 'Ready',
    failed: 'The install stopped.',
    /** A web apps' browser row when the TV did not name the browser (older coordinators). */
    browserFallback: 'Web browser',
    /** Why a browser row is there: the web apps it runs. */
    neededFor: (labels: readonly string[]): string =>
      `Needed for ${labels.length > 1 ? `${labels.slice(0, -1).join(', ')} and ${labels[labels.length - 1]}` : (labels[0] ?? '')}`,
    // The "+ Add apps" tile in the Apps grid (install.tsx AddAppsTile).
    tile: 'Add apps',
    tileSub: (labels: readonly string[], more: boolean): string => (more ? `${labels.join(', ')} and more` : labels.join(' and ')),
    tileLabel: (sub: string): string => `Add apps: ${sub}`,
  },

  // Remove apps (views/remove.tsx): owner phones only.
  remove: {
    heading: 'Remove apps',
    note: 'Removes an app from the TV, for its user only. Nothing is removed until you confirm.',
    remove: 'Remove',
    removeData: 'Remove and delete its data',
    cancel: 'Cancel',
    removing: 'Removing…',
    system: "Installed for everyone on this PC, so only the PC's own software tool can remove it.",
    frees: (size: string): string => `Frees about ${size} on the TV`,
    alsoOff: (labels: readonly string[], name: string): string =>
      labels.length === 1
        ? `${labels[0]} uses ${name}, so removing it turns ${labels[0]} off.`
        : `${labels.slice(0, -1).join(', ')} and ${labels[labels.length - 1]} use ${name}, so removing it turns them off.`,
    ask: (name: string): string => `Remove ${name} from the TV? Its sign-ins and settings stay unless you also delete its data.`,
  },

  // TV over HDMI-CEC (views/tv.tsx).
  tv: {
    heading: 'TV',
    isOn: 'The TV is on.',
    isStandby: 'The TV is in standby.',
    isUnknown: 'The TV did not say whether it is on.',
    on: 'TV on',
    standby: 'TV standby',
  },

  // Layout editor.
  editor: {
    heading: 'Home screen layout',
    intro: 'Changes preview on the TV first. Apply saves them; risky changes ask the TV for a timed confirmation.',
    loading: 'Loading layout…',
    loadFailed: (message: string) => `Could not load the layout: ${message}`,
    retry: 'Try again',
    httpReadOnly: 'This TV allows layout edits only over HTTPS or on the TV itself. You can preview here but not apply.',
    sections: 'Sections',
    sectionKind: (kind: string): string => {
      switch (kind) {
        case 'applications':
          return 'Applications';
        case 'plex-continue-watching':
          return 'Plex: Continue Watching';
        case 'plex-recently-added':
          return 'Plex: Recently Added';
        case 'plex-collection':
          return 'Plex collection';
        default:
          return kind;
      }
    },
    moveUp: 'Move up',
    moveDown: 'Move down',
    enabled: 'Shown',
    title: 'Title',
    hideWhenEmpty: 'Hide when empty',
    resetSection: 'Reset section',
    appearance: 'Appearance',
    textScale: 'Text size',
    density: 'Tile density',
    densityComfortable: 'Comfortable',
    densityLarge: 'Large',
    artStyle: 'Art style',
    artPixel: 'Pixel',
    artClassic: 'Classic',
    appIcons: 'App icons',
    appIconsApp: "App's own",
    appIconsBearDen: 'Bear Den style',
    safeMargin: 'Safe margins',
    reducedMotion: 'Reduced motion',
    highContrast: 'High-contrast focus',
    hero: 'Featured panel',
    clock: 'Clock',
    background: 'Theme',
    accent: 'Accent colour',
    accentCustom: 'Custom hex',
    preview: 'Preview on TV',
    previewing: 'Previewing on the TV',
    previewingBody: 'The TV shows this draft for about a minute without saving it.',
    endPreview: 'End preview',
    apply: 'Apply',
    applying: 'Applying…',
    undo: 'Undo last apply',
    resetAll: 'Reset everything',
    confirmResetAll: 'Reset every section and appearance setting to the defaults?',
    confirmResetSection: (title: string) => `Reset “${title}” to its default?`,
    confirmYes: 'Yes, reset',
    confirmNo: 'Keep',
    unsaved: 'Unsaved changes',
    noChanges: 'No changes to apply.',
    applied: 'Layout applied.',
    pendingTitle: (seconds: number) => `Keep this layout? Reverting in ${seconds} s`,
    pendingTitleUnknown: 'Keep this layout?',
    titleRequired: 'Every section needs a title.',
    pendingBody: 'The TV asks for confirmation because the change could make navigation harder.',
    keep: 'Keep',
    revert: 'Revert',
    conflict: 'Someone else changed the layout first. It was reloaded; review and apply again.',
    invalid: (errors: string[]) => `The TV rejected the layout: ${errors.join('; ')}`,
    undone: 'Restored the previous layout.',
    reset: 'Reset to defaults.',
    forbidden: 'This phone may not change the layout.',
    revision: (n: number) => `Revision ${n}`,
  },

  // App notes (views/notes.tsx): the TV's sentences are shown as sent.
  notes: {
    show: (label: string) => `Good to know about ${label}`,
    panel: (label: string) => `Good to know: ${label}`,
    close: 'Close',
  },

  // Den badges (views/badges.tsx): read-only; names and hints keyed by the
  // badge ids of internal/achievements (the TV has the same copy in Badges.qml).
  badges: {
    heading: 'Den badges',
    summary: (earned: number, total: number) => `${earned} of ${total} earned on your TV`,
    off: 'Badges are off on the TV: nothing is being counted.',
    hidden: 'Badges show on family phones while the TV is unlocked.',
    earnedOn: (day: string) => `Earned ${day}`,
    progress: (count: number, goal: number) => `${count} of ${goal}`,
    privacy: 'Counted on the TV only: just counts and days, never what was watched. Turn badges off or reset them in Themes → Den badges on the TV.',
    names: {
      'first-night-in': ['First Night In', 'Open any app from Home.'],
      'movie-night': ['Movie Night', 'Open Plex ten times.'],
      'couch-explorer': ['Couch Explorer', 'Open every installed app at least once.'],
      'night-owl': ['Night Owl', 'Visit Home after 11 pm on five nights.'],
      'early-cub': ['Early Cub', 'Visit Home before 7 am on five mornings.'],
      'rainy-day': ['Rainy Day Den', 'Visit Home while it rains, on three days. Needs Weather on.'],
      'snow-day': ['Snow Day', 'Visit Home while it snows. Needs Weather on.'],
      'thunder-buddy': ['Thunder Buddy', 'Keep the bears company in a thunderstorm. Needs Weather on.'],
      'all-seasons': ['All Seasons', 'Visit Home in spring, summer, autumn and winter.'],
      'style-switcher': ['Style Switcher', 'Try both art styles: Pixel and Classic.'],
      'theme-tourist': ['Theme Tourist', 'Visit every built-in theme.'],
      'family-den': ['Family Den', 'Pair two family phones.'],
      'good-host': ['Good Host', 'Give a visitor a guest pass.'],
      'sleepy-bear': ['Sleepy Bear', 'Set the sleep timer five times.'],
      'parade-spotter': ['Parade Spotter', 'The bears march for an old, old code…'],
      'loyal-den': ['Loyal Den', 'Spend time in the den on thirty different days.'],
    } as Record<string, readonly [string, string]>,
  },

  // Devices.
  devices: {
    heading: 'Paired phones',
    intro: 'Every phone that can control this TV. Revoking ends its control immediately.',
    hiddenOnHttp: 'Managing paired phones needs HTTPS or the TV itself. Over trusted-LAN HTTP this list is read-only on the TV.',
    loading: 'Loading…',
    empty: 'No phones are paired.',
    you: 'this phone',
    connected: 'Connected',
    disconnected: 'Not connected',
    revoke: 'Revoke',
    revokeAll: 'Revoke all',
    confirmRevoke: (name: string) => `Revoke “${name}”? It will lose control right away.`,
    confirmRevokeAll: 'Revoke every phone, including this one? You will need to pair again.',
    confirm: 'Yes, revoke',
    cancel: 'Cancel',
    revoked: (name: string) => `Revoked ${name}.`,
    failed: (message: string) => `Could not revoke: ${message}`,
    permission: (p: Permission): string => {
      switch (p) {
        case 'controller':
          return 'Controller';
        case 'layout_editor':
          return 'Layout editor';
        case 'owner':
          return 'Owner';
        case 'guest':
          return 'Guest pass';
        default:
          return p;
      }
    },
  },

  // About.
  about: {
    heading: 'About this remote',
    tv: 'TV',
    transport: 'Connection',
    transportText: (transport: Transport, https: boolean): string => {
      if (https || transport === 'https') {
        return 'HTTPS — encrypted. Secure cookies and every remote feature are available.';
      }
      if (transport === 'trusted-lan-http') {
        return 'Trusted home LAN over HTTP — not encrypted. Pairing stops casual unpaired control but not interception on this network. Sensitive changes (paired phones, admin settings) need HTTPS or the TV itself.';
      }
      return 'Local only — the TV is not exposing the remote on the network.';
    },
    deviceName: 'This phone',
    deviceId: 'Device id',
    permissions: 'Permissions',
    protocol: 'Protocol',
    logout: 'Log out',
    logoutHint: 'Ends this session. The TV keeps the device until you revoke it.',
    noPwa: 'Over plain HTTP the browser cannot install this page as an app; open it from the address or the QR code each time.',
  },

  toast: {
    dismiss: 'Dismiss',
  },

  // Guest passes (contracts/http.md#guest-passes).
  guest: {
    chip: (ends: string) => `Guest · ends ${ends}`,
    chipLabel: (ends: string) => `This phone has a guest pass that ends ${ends}.`,
  },

  errors: {
    network: 'Could not reach the TV.',
    csrf: 'The session token was refreshed. Try again.',
    forbidden: 'This phone is not allowed to do that.',
    generic: (message: string) => (message ? message : 'Something went wrong.'),
  },
} as const;

/**
 * One-line help under the settings the phone shares with the TV, keyed by the
 * TV's Settings row ids (apps/tv-shell/qml/SettingsScreen.qml) so both say
 * the same thing; keep the words in sync with the TV. Views draw them with
 * `SettingHelp` (views/help.tsx). Rows the phone does not show yet (style,
 * now-playing, auto-update) are here so the table stays the TV's.
 */
export const SETTING_HELP = {
  background: 'The world Bear Den lives in: colours, wallpaper and the corner scene.',
  style: 'How much decoration: Bear Den adds bears and ornaments, Plain keeps it simple, Performance turns animation off.',
  art: 'Pixel draws everything in pixel art; Classic uses smooth drawings.',
  'app-icons': "App's own uses each installed app's icon; Bear Den style draws every app in Bear Den's look.",
  text: 'Makes all text on the TV bigger or smaller.',
  density: 'Bigger tiles are easier to see from the couch; smaller fit more on a row.',
  margin: "For TVs that cut off the picture's edges: raise it until nothing is cut off.",
  motion: 'Stops the moving decorations and slides; everything still works.',
  contrast: 'A thicker, brighter outline around whatever is selected.',
  hero: 'The big panel above your apps that describes what is selected.',
  clock: 'Shows the time in the top bar.',
  cec: "Lets Bear Den turn the TV on and off and switch its input, over HDMI. Needs a CEC adapter; most PCs don't have one.",
  'cec-volume': "Which volume your phone's volume buttons change: this PC's, or the TV's over HDMI.",
  sleep: 'Pauses what is playing where it can, goes Home and turns the screen off after the time you pick.',
  'screen-off': 'Turns the picture off now. Any button wakes it; that first press only wakes it.',
  'now-playing': 'Paired phones see the title of what is playing. Never while the TV is locked.',
  'auto-update': "Updates the apps installed for the TV's user (not system-wide ones) while the TV is idle, about once a day.",
} as const;

/** A TV Settings row id with help text. */
export type SettingHelpId = keyof typeof SETTING_HELP;

const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'] as const;
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'] as const;

/**
 * @param day A badge's earned day, YYYY-MM-DD (the TV's local calendar day).
 * @returns "2 Sep 2026", or the text as sent when it is not a day.
 */
export function badgeDay(day: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(day);
  const month = m ? MONTHS[Number(m[2]) - 1] : undefined;
  return m && month ? `${Number(m[3])} ${month} ${m[1]}` : day;
}

/**
 * @param endsAt When a guest pass ends, Unix epoch ms.
 * @param now The phone's clock, Unix epoch ms.
 * @returns "04:00" when it ends within 20 hours, else "Sat 21:30" (the phone's
 *   local time; 24-hour clock like the TV).
 */
export function passEndLabel(endsAt: number, now: number): string {
  const d = new Date(endsAt);
  const time = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
  return endsAt - now < 20 * 3600 * 1000 ? time : `${WEEKDAYS[d.getDay()]} ${time}`;
}
