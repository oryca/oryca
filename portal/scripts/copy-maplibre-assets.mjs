import { copyFileSync, mkdirSync, readFileSync, rmSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);

const pkgPath = require.resolve('maplibre-gl/package.json');
const pkg = JSON.parse(readFileSync(pkgPath, 'utf8'));
const version = pkg.version;
const distDir = join(dirname(pkgPath), 'dist');

const FILES = ['maplibre-gl-worker.mjs', 'maplibre-gl-shared.mjs'];

const outRoot = resolve(fileURLToPath(import.meta.url), '..', '..', 'public', 'maplibre');
rmSync(outRoot, { recursive: true, force: true });
const outDir = join(outRoot, version);
mkdirSync(outDir, { recursive: true });

for (const file of FILES) {
  copyFileSync(join(distDir, file), join(outDir, file));
}

console.log(`copy-maplibre-assets: maplibre-gl ${version} -> public/maplibre/${version}/`);
