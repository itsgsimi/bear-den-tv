// TypeScript representation of contracts/*.schema.json (protocol 1).
// Contract: these types mirror action.schema.json, state.schema.json, layout.schema.json,
// and http.md field for field. They add no fields and never widen enums; a schema
// change bumps `PROTOCOL` and this file together. Values received over the wire are
// typed here but only trusted after JSON parsing at the API boundary (api.ts).

export const PROTOCOL = 1 as const;

export type NavAction = 'nav.up' | 'nav.down' | 'nav.left' | 'nav.right';

export type ActionName =
  | NavAction
  | 'select'
  | 'back'
  | 'home'
  | 'app.launch'
  | 'app.close'
  | 'media.play'
  | 'media.pause'
  | 'media.seek_relative'
  | 'audio.volume_delta'
  | 'audio.mute'
  | 'text.submit'
  | 'shell.restart'
  | 'power.sleep_timer'
  | 'display.off';

/** power.sleep_timer minutes: 0 cancels, otherwise one of the fixed choices. */
export type SleepMinutes = 0 | 15 | 30 | 45 | 60 | 90 | 120;

/**
 * What a guest pass may send (contract.GuestActions in Go, contracts/actions.md).
 * The server enforces it; the phone only hides what a guest cannot use. Any
 * action not listed here is refused to guests.
 */
export const GUEST_ACTIONS: ReadonlySet<ActionName> = new Set<ActionName>([
  'nav.up',
  'nav.down',
  'nav.left',
  'nav.right',
  'select',
  'back',
  'home',
  'app.launch',
  'media.play',
  'media.pause',
  'media.seek_relative',
  'audio.volume_delta',
  'audio.mute',
  'text.submit',
]);

/** Argument object per action; `{}` for argument-free actions. */
export type ActionArgs = {
  'nav.up': Record<string, never>;
  'nav.down': Record<string, never>;
  'nav.left': Record<string, never>;
  'nav.right': Record<string, never>;
  select: Record<string, never>;
  back: Record<string, never>;
  home: Record<string, never>;
  'app.launch': { app_id: string };
  'app.close': { app_id: string; force?: boolean };
  'media.play': Record<string, never>;
  'media.pause': Record<string, never>;
  'media.seek_relative': { seconds: number };
  'audio.volume_delta': { delta: number };
  'audio.mute': { muted: boolean };
  'text.submit': { text: string };
  'shell.restart': Record<string, never>;
  'power.sleep_timer': { minutes: SleepMinutes };
  'display.off': Record<string, never>;
};

/** `"active"`, `"shell"`, or a registered application id. */
export type ActionTarget = 'active' | 'shell' | (string & { readonly __appId?: never });

export interface ActionRequest<A extends ActionName = ActionName> {
  protocol: typeof PROTOCOL;
  request_id: string;
  context_epoch: number;
  target: ActionTarget;
  action: A;
  args: ActionArgs[A];
}

export type Outcome = 'accepted' | 'delivered' | 'observed' | 'failed';

export type FailureCode =
  | 'invalid'
  | 'unsupported_protocol'
  | 'unauthorized'
  | 'forbidden'
  | 'rate_limited'
  | 'duplicate_mismatch'
  | 'stale_epoch'
  | 'no_target'
  | 'unknown_foreground'
  | 'locked'
  | 'unsupported'
  | 'target_unfocused'
  | 'busy'
  | 'launch_failed'
  | 'timeout'
  | 'internal'
  | 'display_off';

export type ResultCode = 'ok' | FailureCode;

export type TargetKind = 'shell' | 'app' | 'unknown' | 'locked' | 'none';

export interface TargetInfo {
  kind: TargetKind;
  app_id: string | null;
  label: string;
}

export interface ActionResult {
  protocol: typeof PROTOCOL;
  request_id: string;
  outcome: Outcome;
  code: ResultCode;
  message: string;
  context_epoch: number;
  target: TargetInfo;
  detail: Record<string, unknown>;
}

export interface HoldStartMessage {
  type: 'hold.start';
  hold_id: string;
  action: NavAction;
  context_epoch: number;
}
export interface HoldRenewMessage {
  type: 'hold.renew';
  hold_id: string;
}
export interface HoldStopMessage {
  type: 'hold.stop';
  hold_id: string;
}
export type HoldMessage = HoldStartMessage | HoldRenewMessage | HoldStopMessage;

export type HoldLeaseState = 'active' | 'expired' | 'cancelled' | 'busy';

export interface HoldEvent {
  type: 'hold';
  hold_id: string;
  state: HoldLeaseState;
  reason?: string;
}

export type ClientMessage =
  | { type: 'action'; request: ActionRequest }
  | HoldMessage
  | { type: 'visibility'; hidden: boolean }
  | { type: 'ping' };

export type ServerMessage =
  | { type: 'state'; state: StateSnapshot }
  | { type: 'action_result'; result: ActionResult }
  | HoldEvent
  | { type: 'revoked' }
  | { type: 'pong' };

export type Transport = 'local-only' | 'trusted-lan-http' | 'https';
/** `guest` is a time-limited guest pass and never comes with another permission. */
export type Permission = 'controller' | 'layout_editor' | 'owner' | 'guest';

export interface Capability {
  available: boolean;
  backend?: string;
  reason?: string;
  holdable?: boolean;
}

export type ShellState = 'stopped' | 'starting' | 'running' | 'crashed' | 'restarting' | 'circuit_open' | 'exited';

export interface SessionInfo {
  locked: boolean;
  display_session: 'x11' | 'wayland' | 'unknown';
  desktop_adapter: string;
  shell_connected: boolean;
  shell_state: ShellState;
}

export interface StateTarget extends TargetInfo {
  observed: boolean;
  window_title?: string | null;
}

export interface ShellFocus {
  screen: 'home' | 'settings' | 'pairing' | 'devices' | 'diagnostics' | 'dialog' | 'setup' | 'error' | 'unknown';
  focus: { section_id: string | null; item_id: string | null };
}

export type LaunchState = 'idle' | 'launching' | 'running' | 'failed' | 'exited' | 'crashed';

export interface Application {
  id: string;
  label: string;
  adapter: string;
  installed: boolean;
  version: string | null;
  installation: 'user' | 'system' | 'none' | 'unknown';
  running: boolean;
  foreground: boolean;
  launch_state: LaunchState;
  last_error: string | null;
}

export interface RemoteLimits {
  repeat_delay_ms: number;
  repeat_hz: number;
  hold_renew_ms: number;
  hold_expiry_ms: number;
  max_message_bytes: number;
}

export interface RemoteInfo {
  enabled: boolean;
  transport: Transport;
  listening: boolean;
  addresses: string[];
  https: boolean;
  http_layout_editing: boolean;
  paired_device_count: number;
  /** config remote.now_playing: whether phones may see state.now_playing (missing on older coordinators). */
  now_playing?: boolean;
  hold: { active: boolean; device_id: string | null; action: string | null };
  limits: RemoteLimits;
}

export interface Me {
  device_id: string;
  device_name: string;
  permissions: Permission[];
  transport_secure: boolean;
  /** Guest passes only: when the pass ends, Unix epoch ms (the TV's wall clock). */
  expires_at_ms?: number;
}

export interface Device {
  id: string;
  name: string;
  permissions: Permission[];
  connected: boolean;
  last_seen_ms: number;
  created_at: string;
  /** A guest pass (permissions is ['guest']); missing means false. */
  guest?: boolean;
  /** Guest passes: when the pass ends, Unix epoch ms; null or missing for family phones. */
  expires_at_ms?: number | null;
}

export interface LayoutPending {
  revision: number;
  previous_revision: number;
  expires_in_s: number;
  source: 'tv' | 'web';
}

export interface Notification {
  id: string;
  kind: 'info' | 'success' | 'warning' | 'error';
  text: string;
  created_ms: number;
}

/** A theme id (a theme package; contracts/theme.schema.json). Older values den-gradient/charcoal are aliases. */
export type BackgroundPreset = string;

export type FocusStyle = '' | 'vine' | 'fern' | 'stars' | 'embers';
export type ParticleKind = '' | 'none' | 'fireflies' | 'leaves' | 'stars' | 'embers' | 'snow';

/** The TV's look, mirrored by paired phones (state.appearance; resolved by internal/themes). */
export interface Appearance {
  background: BackgroundPreset;
  theme: 'den-dark' | 'plain-dark' | 'performance';
  accent: string;
  name?: string;
  /** The backdrop in use is pixel art: draw it without smoothing. */
  pixel?: boolean;
  /** The TV's art style (layout ui.art_style); missing means pixel. */
  art_style?: ArtStyle;
  palette?: { stem?: string; light?: string; dark?: string; bloom?: string; glow?: string };
  /** The tile decoration: style and a same-origin tip ornament URL. */
  focus?: { style: FocusStyle; tip?: string; tip_upright?: boolean };
  /** Backdrop URL (same origin), veil colour and particle kind. */
  phone?: { backdrop?: string; veil?: string; particles?: ParticleKind };
  /** Installed themes, for pickers. */
  themes?: { id: string; name: string; accent?: string }[];
}
export type TileDensity = 'comfortable' | 'large';
/** Pixel art on one grid, or smooth Classic art (layout ui.art_style). */
export type ArtStyle = 'pixel' | 'classic';

export interface LayoutUi {
  theme: 'den-dark' | 'plain-dark' | 'performance';
  accent: string;
  background: BackgroundPreset;
  text_scale: number;
  tile_density: TileDensity;
  safe_margin_percent: number;
  reduced_motion: boolean;
  high_contrast_focus: boolean;
  hero_enabled: boolean;
  clock_enabled: boolean;
  /** Optional; missing means pixel. */
  art_style?: ArtStyle;
}

export type SectionKind = 'applications' | 'plex-continue-watching' | 'plex-recently-added' | 'plex-collection';

export interface LayoutSection {
  id: string;
  title: string;
  kind: SectionKind;
  enabled: boolean;
  application_ids?: string[];
  hide_when_empty: boolean;
}

export interface Layout {
  ui: LayoutUi;
  sections: LayoutSection[];
}

/** state.schema.json. Optional members are omitted for viewers that may not see them. */
export interface StateSnapshot {
  protocol: typeof PROTOCOL;
  context_epoch: number;
  generated_at_ms: number;
  device_name: string;
  dev_mode: boolean;
  config_revision: number;
  session: SessionInfo;
  target: StateTarget;
  capabilities: Record<string, Capability>;
  shell: ShellFocus;
  applications: Application[];
  remote: RemoteInfo;
  me?: Me;
  appearance?: Appearance;
  devices?: Device[];
  layout?: Layout;
  layout_pending?: LayoutPending | null;
  notifications: Notification[];
  playback?: Playback;
  content?: unknown;
  weather?: Weather;
  /** Controller phones only; absent (or null) while locked, when the owner turned it off, or when nothing is known. */
  now_playing?: NowPlaying | null;
  /** The sleep timer and the display; absent for anonymous viewers and from older coordinators. */
  power?: Power;
}

/**
 * state.power (contracts/http.md, "Sleep timer and screen off"). sleep_at_ms is
 * in the coordinator's clock (the one generated_at_ms uses), never the phone's.
 */
export interface Power {
  sleep_at_ms: number | null;
  sleep_minutes?: Exclude<SleepMinutes, 0>;
  warning: boolean;
  display: 'on' | 'off';
  suspend?: { available: boolean; reason?: string };
}

export type NowPlayingStatus = 'playing' | 'paused' | 'stopped';

/**
 * state.now_playing: what the foreground app's own MPRIS player reports
 * (contracts/http.md, "Now playing"). position_at is in the coordinator's
 * clock (the one generated_at_ms uses), never the phone's.
 */
export interface NowPlaying {
  app_id: string;
  title: string;
  subtitle?: string;
  status: NowPlayingStatus;
  length_ms?: number;
  position_ms?: number;
  position_at: number;
  rate: number;
}

/** state.weather: local weather from Open-Meteo (shell view only; phones never receive it). */
export type WeatherCondition = 'clear' | 'partly-cloudy' | 'cloudy' | 'fog' | 'drizzle' | 'rain' | 'snow' | 'thunder';

export interface WeatherCurrent {
  temperature: number;
  condition: WeatherCondition;
  intensity: 'light' | 'moderate' | 'heavy';
  is_day: boolean;
  observed_at: string;
}

export interface Weather {
  status: 'disabled' | 'connecting' | 'ready' | 'stale' | 'error';
  message: string;
  place: string;
  units: 'celsius' | 'fahrenheit';
  scene: boolean;
  current: WeatherCurrent | null;
}

/** state.playback: the playback detection test (shell view only; phones never receive it). */
export interface PlaybackNote {
  level: 'info' | 'warn';
  text: string;
}

export interface PlaybackOption {
  value: string;
  label: string;
  note?: string;
}

/** A setting the owner may adjust on the TV; `value` is in effect, `auto` is detection's choice. */
export interface PlaybackSetting {
  id: string;
  label: string;
  auto: string;
  value: string;
  overridden: boolean;
  options: PlaybackOption[];
}

export type PlaybackStatus = 'tuned' | 'applied' | 'pending' | 'off' | 'error' | 'suggested';

export interface PlaybackApp {
  label: string;
  adapter: string;
  hardware: string[];
  expect: string[];
  notes: PlaybackNote[];
  status: PlaybackStatus;
  changes: string[];
  settings?: PlaybackSetting[];
}

export interface Playback {
  at_ms: number;
  summary: string;
  display?: string;
  notes: PlaybackNote[];
  apps: PlaybackApp[];
}

// http.md response bodies.

export interface Info {
  protocol: typeof PROTOCOL;
  device_name: string;
  transport: Transport;
  https: boolean;
  pairing_required: boolean;
}

export interface ClaimRequest {
  invitation: string | null;
  code: string | null;
  device_name: string;
}

export interface Session {
  device_id: string;
  device_name: string;
  permissions: Permission[];
  csrf_token: string;
  transport_secure: boolean;
}

export interface CapabilitiesResponse {
  context_epoch: number;
  target: StateTarget;
  capabilities: Record<string, Capability>;
}

export interface LayoutDocument {
  revision: number;
  layout: Layout;
  defaults: Layout;
  pending: LayoutPending | null;
}

export interface LayoutPutResponse {
  revision: number;
  pending: boolean;
}

export interface ErrorBody {
  error: string;
  message: string;
}
