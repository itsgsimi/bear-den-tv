// Production build: bundles src/main.tsx into dist/app.js and src/app.css into
// dist/app.css with esbuild (ES2020, minified, no sourcemap), then copies static/
// verbatim. Output is deterministic for identical inputs: no hashes, timestamps,
// or legal-comment banners. dist/ is committed and embedded by the Go binary.
import { build } from 'esbuild';
import { cpSync, mkdirSync, readdirSync, rmSync, statSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const dist = join(root, 'dist');

rmSync(dist, { recursive: true, force: true });
mkdirSync(dist, { recursive: true });

const shared = {
  bundle: true,
  minify: true,
  sourcemap: false,
  target: ['es2020'],
  legalComments: 'none',
  logLevel: 'info',
  charset: 'utf8',
};

await build({
  ...shared,
  entryPoints: [join(root, 'src/main.tsx')],
  outfile: join(root, 'dist/app.js'),
  format: 'esm',
  jsx: 'automatic',
  jsxImportSource: 'preact',
  define: { 'process.env.NODE_ENV': '"production"' },
});

await build({
  ...shared,
  entryPoints: [join(root, 'src/app.css')],
  outfile: join(root, 'dist/app.css'),
  // Artwork referenced from CSS (url('art/…')) is copied verbatim from static/art
  // and served same-origin; keep the relative URLs instead of bundling them.
  external: ['art/*'],
});

cpSync(join(root, 'static'), dist, { recursive: true });

const sizes = [];
for (const name of readdirSync(dist)) {
  const stat = statSync(join(dist, name));
  if (stat.isFile()) sizes.push(`${name} ${stat.size} B`);
}
console.log(sizes.join('\n'));
