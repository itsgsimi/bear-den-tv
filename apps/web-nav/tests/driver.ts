// Test driver that stands in for the coordinator (internal/applications/web):
// it injects dist/nav.js into the "bearden" isolated world over CDP, adds the
// __bdtvReport binding there, calls __bdtv.apply and performs the returned
// effect with trusted input (Playwright's mouse and keyboard, which are CDP
// Input events like the coordinator's). Fixtures are served from
// tests/fixtures through page.route under http://fixtures.test/.
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import type { CDPSession, Page } from '@playwright/test';

const here = dirname(fileURLToPath(import.meta.url));
export const NAV_JS = readFileSync(join(here, '../dist/nav.js'), 'utf8');
export const ORIGIN = 'http://fixtures.test';

export function loadHints(id: string): unknown {
  return JSON.parse(readFileSync(join(here, `../hints/${id}.json`), 'utf8'));
}

interface Effect {
  kind: 'click' | 'keys' | 'text';
  x?: number;
  y?: number;
  keys?: string[];
}

export interface Reply {
  ok: boolean;
  outcome: string;
  reason?: string;
  effect?: Effect;
  focus?: { role: string; text_field: boolean; index: number };
}

export class Driver {
  private ctx = 0;
  readonly reports: Record<string, unknown>[] = [];

  private constructor(readonly page: Page, private readonly cdp: CDPSession) {}

  static async attach(page: Page, hints: unknown = null): Promise<Driver> {
    await page.route(`${ORIGIN}/**`, (route) => {
      const name = new URL(route.request().url()).pathname.replace(/^\//, '') || 'grid.html';
      try {
        route.fulfill({ contentType: 'text/html', body: readFileSync(join(here, 'fixtures', name)) });
      } catch {
        route.fulfill({ status: 404, body: 'not found' });
      }
    });
    const cdp = await page.context().newCDPSession(page);
    const d = new Driver(page, cdp);
    cdp.on('Runtime.executionContextCreated', (e) => {
      if (e.context.name === 'bearden') d.ctx = e.context.id;
    });
    cdp.on('Runtime.executionContextsCleared', () => {
      d.ctx = 0;
    });
    cdp.on('Runtime.bindingCalled', (e) => {
      if (e.name === '__bdtvReport') d.reports.push(JSON.parse(e.payload));
    });
    await cdp.send('Runtime.enable');
    await cdp.send('Page.enable');
    await cdp.send('Runtime.addBinding', { name: '__bdtvReport', executionContextName: 'bearden' });
    await cdp.send('Page.addScriptToEvaluateOnNewDocument', {
      source: `${NAV_JS}\n;__bdtvNav.install(${JSON.stringify(hints)});`,
      worldName: 'bearden',
    });
    return d;
  }

  async open(name: string): Promise<void> {
    this.ctx = 0;
    await this.page.goto(`${ORIGIN}/${name}`);
    await this.ready();
  }

  async ready(): Promise<void> {
    for (let i = 0; i < 100 && !this.ctx; i++) await this.page.waitForTimeout(20);
    if (!this.ctx) throw new Error('the bearden world never appeared');
  }

  async evaluate<T>(expression: string): Promise<T> {
    await this.ready();
    const r = await this.cdp.send('Runtime.evaluate', { expression, contextId: this.ctx, returnByValue: true });
    if (r.exceptionDetails) throw new Error(r.exceptionDetails.text);
    return r.result.value as T;
  }

  /** What the coordinator does for one phone action. */
  async act(action: string, arg?: number, text?: string): Promise<Reply> {
    const call = arg === undefined ? `__bdtv.apply(${JSON.stringify(action)})` : `__bdtv.apply(${JSON.stringify(action)}, ${arg})`;
    const reply = await this.evaluate<Reply>(call);
    const e = reply.effect;
    if (e?.kind === 'click') await this.page.mouse.click(e.x!, e.y!);
    if (e?.kind === 'keys') for (const k of e.keys!) await this.page.keyboard.press(k === ' ' ? 'Space' : k);
    if (e?.kind === 'text' && text !== undefined) {
      await this.page.keyboard.insertText(text);
      await this.page.keyboard.press('Enter');
    }
    return reply;
  }

  /**
   * Moves the page's clock by ms of virtual time (CDP Emulation), for every
   * world, the script's timers included: deterministic, no sleeping. The
   * first call pauses real time for the page.
   */
  async advance(ms: number): Promise<void> {
    if (!this.virtual) {
      await this.cdp.send('Emulation.setVirtualTimePolicy', { policy: 'pause' });
      this.virtual = true;
    }
    const expired = new Promise<void>((resolve) => this.cdp.once('Emulation.virtualTimeBudgetExpired', () => resolve()));
    await this.cdp.send('Emulation.setVirtualTimePolicy', { policy: 'advance', budget: ms });
    await expired;
  }
  private virtual = false;

  /** The focus ring's opacity ("" before it is drawn), read in the script's world. */
  ringOpacity(): Promise<string> {
    return this.evaluate<string>(
      `(() => { const h = document.querySelector('[data-bdtv-overlay]'); const r = h && h.__root && h.__root.firstElementChild; return r && r.style.display !== 'none' ? (r.style.opacity || '1') : ''; })()`,
    );
  }

  /** Id of the element carrying the focus ring. */
  focused(): Promise<string | null> {
    return this.page.evaluate(() => document.querySelector('[data-bdtv-focused]')?.id ?? null);
  }

  body(key: string): Promise<string | undefined> {
    return this.page.evaluate((k) => document.body.dataset[k], key);
  }
}
