// Downloads the block lists Hole uses by default and writes the deduped,
// sorted domain list the Worker searches with binary search.
import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseHosts } from './parse-hosts.mjs';

const SOURCES = [
  'https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts',
  'https://raw.githubusercontent.com/yous/YousList/master/hosts.txt',
];
const FETCH_TIMEOUT_MS = 60_000;
const MIN_DOMAINS = 50_000;
const outDir = resolve(dirname(fileURLToPath(import.meta.url)), '../src');

async function download(url) {
  const res = await fetch(url, { signal: AbortSignal.timeout(FETCH_TIMEOUT_MS) });
  if (!res.ok) throw new Error(`${url}: HTTP ${res.status}`);
  return res.text();
}

async function main() {
  const texts = await Promise.all(SOURCES.map(download));
  const domains = [...new Set(texts.flatMap(parseHosts))].sort();
  if (domains.length < MIN_DOMAINS) {
    throw new Error(`only ${domains.length} domains parsed (expected >= ${MIN_DOMAINS}); refusing to write`);
  }
  const meta = { builtAt: new Date().toISOString(), count: domains.length, sources: SOURCES };
  await mkdir(outDir, { recursive: true });
  await writeFile(resolve(outDir, 'blocklist.txt'), `${domains.join('\n')}\n`);
  await writeFile(resolve(outDir, 'blocklist-meta.json'), `${JSON.stringify(meta, null, 2)}\n`);
  console.log(`wrote ${domains.length} domains`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
