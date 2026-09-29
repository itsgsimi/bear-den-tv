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
    needSixDigits: 'Enter all six digits.',
    needName: 'Give this phone a name.',
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
    loggedOut: 'You have been logged out.',
    sessionExpired: 'The session ended. Pair again to continue.',
    httpNotice: 'Trusted-LAN HTTP: pairing prevents casual unpaired control but does not encrypt traffic.',
  },

  // Remote screen.
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
    notInstalled: (label: string) => `${label} is not installed on this TV.`,
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
    pcVolume: 'PC volume',
    volumeDown: 'Volume down',
    volumeUp: 'Volume up',
    mute: 'Mute',
    unmute: 'Unmute',
    text: 'Text entry',
    textTitle: 'Type on the TV',
    textPlaceholder: 'Text to send',
    textHint: 'Up to 256 characters. Sent only to a Bear Den TV text field.',
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

  errors: {
    network: 'Could not reach the TV.',
    csrf: 'The session token was refreshed. Try again.',
    forbidden: 'This phone is not allowed to do that.',
    generic: (message: string) => (message ? message : 'Something went wrong.'),
  },
} as const;
