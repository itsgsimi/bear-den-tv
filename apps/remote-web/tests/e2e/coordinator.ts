// The real coordinator for the phone remote's browser tests: `bear-den-tv
// dev --dev-fixtures --no-shell` (fake desktop, DEMO apps and players, the
// phone remote on a free loopback port) in its own private dev root, started
// once per worker. Readiness and every check come from the coordinator's own
// log lines (waitForLine), never from a sleep. BDTV_BIN names the binary
// (`make test-web` passes build/bin/bear-den-tv); without it the tests fail
// with the reason instead of skipping.

import { spawn, execFile, type ChildProcess } from 'node:child_process';
import { mkdtempSync, rmSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

const run = promisify(execFile);

/** How long a coordinator line may take to appear (a process start, not a timing check). */
const LINE_TIMEOUT_MS = 15_000;

export class Coordinator {
  readonly lines: string[] = [];
  private waiters: { re: RegExp; from: number; resolve: (m: RegExpMatchArray) => void }[] = [];
  private exited: string | null = null;
  baseURL = '';

  private constructor(
    readonly bin: string,
    readonly root: string,
    private readonly proc: ChildProcess,
  ) {}

  static async start(): Promise<Coordinator> {
    const bin = resolve(process.env.BDTV_BIN ?? fileURLToPath(new URL('../../../../build/bin/bear-den-tv', import.meta.url)));
    if (!existsSync(bin)) {
      throw new Error(`no coordinator binary at ${bin}: run make go (make test-web builds it) or set BDTV_BIN`);
    }
    const root = mkdtempSync(join(tmpdir(), 'bdtv-e2e-'));
    const proc = spawn(bin, ['dev', '--dev-fixtures', '--no-shell', '--data-dir', root, '--dev-listen', '127.0.0.1:0'], {
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    const c = new Coordinator(bin, root, proc);
    const onData = (buf: Buffer) => {
      for (const line of buf.toString('utf8').split('\n')) if (line.trim()) c.push(line);
    };
    proc.stdout?.on('data', onData);
    proc.stderr?.on('data', onData);
    proc.on('exit', (code, signal) => {
      c.exited = `the coordinator exited (${code ?? signal})`;
    });
    const m = await c.waitForLine(/msg="remote: listening" urls=\[(http:\/\/127\.0\.0\.1:\d+)\/?/);
    c.baseURL = m[1] ?? '';
    return c;
  }

  private push(line: string) {
    this.lines.push(line);
    for (const w of [...this.waiters]) {
      const m = line.match(w.re);
      if (m) {
        this.waiters = this.waiters.filter((x) => x !== w);
        w.resolve(m);
      }
    }
  }

  /** Index of the next line: pass it to waitForLine to look only at later lines. */
  mark(): number {
    return this.lines.length;
  }

  /** Resolves with the first line (from index `from` on) matching re. */
  waitForLine(re: RegExp, from = 0): Promise<RegExpMatchArray> {
    for (const line of this.lines.slice(from)) {
      const m = line.match(re);
      if (m) return Promise.resolve(m);
    }
    return new Promise((resolveLine, reject) => {
      if (this.exited) {
        reject(new Error(`${this.exited} before ${re}:\n${this.lines.join('\n')}`));
        return;
      }
      const timer = setTimeout(() => {
        this.waiters = this.waiters.filter((x) => x.resolve !== done);
        reject(new Error(`no coordinator line ${re} within ${LINE_TIMEOUT_MS} ms:\n${this.lines.slice(-30).join('\n')}`));
      }, LINE_TIMEOUT_MS);
      const done = (m: RegExpMatchArray) => {
        clearTimeout(timer);
        resolveLine(m);
      };
      this.waiters.push({ re, from, resolve: done });
    });
  }

  /** A fresh six-digit pairing code from the running coordinator (`bear-den-tv pair`). */
  async pairingCode(): Promise<string> {
    const { stdout } = await run(this.bin, ['pair', '--socket', join(this.root, 'run', 'shell.sock')]);
    const m = stdout.match(/Pairing code: (\d{6})/);
    if (!m?.[1]) throw new Error(`no pairing code in: ${stdout}`);
    return m[1];
  }

  async stop(): Promise<void> {
    if (!this.exited) {
      const gone = new Promise((r) => this.proc.once('exit', r));
      this.proc.kill('SIGINT');
      await gone;
    }
    rmSync(this.root, { recursive: true, force: true });
  }
}
