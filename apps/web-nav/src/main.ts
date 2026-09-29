// Entry of dist/nav.js: exposes install() as globalThis.__bdtvNav in the
// isolated world the coordinator creates. The coordinator appends
// `__bdtvNav.install(<hints JSON or null>);` to this file's text
// (internal/applications/web script.go). Spec: nav.ts.
import { install } from './nav';

(globalThis as unknown as { __bdtvNav: { install: typeof install } }).__bdtvNav = { install };
