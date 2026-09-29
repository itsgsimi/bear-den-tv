// Production build of the navigation script: bundles src/main.ts into
// dist/nav.js (IIFE, ES2020, minified, no sourcemap, no banner) with esbuild.
// Deterministic for identical inputs; dist/ is committed and embedded by the
// Go binary (embed.go WebNav), and scripts/check-web-dist.sh checks it.
import { build } from 'esbuild';
import { mkdirSync, rmSync, statSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const dist = join(root, 'dist');

rmSync(dist, { recursive: true, force: true });
mkdirSync(dist, { recursive: true });

await build({
  entryPoints: [join(root, 'src/main.ts')],
  outfile: join(dist, 'nav.js'),
  bundle: true,
  minify: true,
  sourcemap: false,
  format: 'iife',
  target: ['es2020'],
  legalComments: 'none',
  charset: 'utf8',
  logLevel: 'info',
});

console.log(`nav.js ${statSync(join(dist, 'nav.js')).size} B`);
