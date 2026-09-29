// Shapes shared by the navigation script and the coordinator that drives it
// (internal/applications/web: Reply, Status, Effect in page.go). Hints follow
// contracts/web-hints.schema.json. Keep both sides in step: the coordinator
// decodes replies into fixed structs and refuses anything else.

/** A key the coordinator may press for the page (contracts/web-hints.schema.json $defs/key). */
export type Key = ' ' | 'k' | 'j' | 'l' | 'f' | 'ArrowLeft' | 'ArrowRight' | 'Escape' | 'Enter';

export type BackStep = 'field' | 'fullscreen' | 'overlay' | 'history';

export interface MediaKeys {
  toggle?: Key;
  seek_back?: Key;
  seek_forward?: Key;
  seek_step_s?: number;
  fullscreen?: Key;
}

/** Per-site hints (contracts/web-hints.schema.json). */
export interface Hints {
  id: string;
  verified: boolean;
  note?: string;
  prefer?: string[];
  skip?: string[];
  initial?: string[];
  modal?: string[];
  close?: string[];
  player?: string;
  back?: BackStep[];
  media?: MediaKeys;
}

/**
 * What the coordinator should do with trusted input after the script has
 * prepared the page: click at a viewport point (CSS pixels), press keys from
 * the closed set, or type the phone's text into the focused field.
 */
export type Effect =
  | { kind: 'click'; x: number; y: number }
  | { kind: 'keys'; keys: Key[] }
  | { kind: 'text' };

/** Where focus is, without its label (poster titles name private media). */
export interface FocusInfo {
  role: string;
  text_field: boolean;
  /** 0-based index among the page's focus targets at the time of the move. */
  index: number;
}

export interface Reply {
  ok: boolean;
  /** moved, edge, scrolled, click, text_field, left_field, exited_fullscreen,
   * closing_overlay, history_back, at_root, keys, already_playing,
   * already_paused, text, cursor, none. */
  outcome: string;
  reason?: string;
  effect?: Effect;
  focus?: FocusInfo;
}

export type VideoState = 'none' | 'playing' | 'paused';

/** Pushed to the coordinator through the __bdtvReport binding. */
export interface Status {
  v: 1;
  visible: boolean;
  video: VideoState;
  text_field: boolean;
  fullscreen: boolean;
  viewport: { w: number; h: number };
  focus?: FocusInfo;
}
