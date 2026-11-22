import meta from './blocklist-meta.json';
import type { Env } from './env';
import { MSG, apiError } from './errors';
import { scan, type ScanResult } from './scan';
import { shotUrls } from './shot-route';
import { saveShots } from './shot-store';
import { TargetError, parseTarget } from './validate';

export const MAX_BODY_BYTES = 4096;
const CACHE_TTL_SECONDS = 24 * 60 * 60;
const CACHE_PREFIX = 'scan:v2:';

type Scanned = Omit<ScanResult, 'shotImages'>;

/** What is cached in KV: the response minus per-request fields (`cached`, absolute shot URLs). */
export interface StoredScan extends Scanned {
  readonly url: string;
  readonly scannedAt: string;
  readonly listBuiltAt: string;
  readonly shotId: string | null;
}

export interface ScanData extends Scanned {
  readonly url: string;
  readonly scannedAt: string;
  readonly cached: boolean;
  readonly listBuiltAt: string;
  readonly shots: { readonly before: string; readonly after: string } | null;
}

function toResponseData(stored: StoredScan, origin: string, cached: boolean): ScanData {
  const { shotId, ...rest } = stored;
  return { ...rest, cached, shots: shotId === null ? null : shotUrls(origin, shotId) };
}

/** Reads the request body with a hard size limit and extracts `url`. */
export async function readTargetInput(request: Request): Promise<string> {
  const declared = Number(request.headers.get('Content-Length') ?? '0');
  if (declared > MAX_BODY_BYTES) throw apiError(413, MSG.tooLarge);
  const text = await request.text();
  if (new TextEncoder().encode(text).length > MAX_BODY_BYTES) throw apiError(413, MSG.tooLarge);
  try {
    const body: unknown = JSON.parse(text);
    const url = (body as { url?: unknown } | null)?.url;
    if (typeof url === 'string') return url;
  } catch {
    // fall through to the 400 below
  }
  throw apiError(400, MSG.badBody);
}

function parseOrThrow(input: string): URL {
  try {
    return parseTarget(input);
  } catch (err) {
    if (err instanceof TargetError) throw apiError(400, err.message);
    throw err;
  }
}

async function enforceRateLimit(env: Env, request: Request): Promise<void> {
  const key = request.headers.get('CF-Connecting-IP') ?? 'unknown';
  const { success } = await env.SCAN_LIMITER.limit({ key });
  if (!success) throw apiError(429, MSG.rateLimited);
}

async function readCache(env: Env, key: string): Promise<StoredScan | null> {
  try {
    return await env.SCANS.get<StoredScan>(key, 'json');
  } catch (err) {
    console.error('kv read failed', err);
    return null;
  }
}

async function writeCache(env: Env, key: string, data: StoredScan): Promise<void> {
  try {
    await env.SCANS.put(key, JSON.stringify(data), { expirationTtl: CACHE_TTL_SECONDS });
  } catch (err) {
    console.error('kv write failed', err);
  }
}

async function storeResult(env: Env, url: URL, result: ScanResult): Promise<StoredScan> {
  const { shotImages, ...scanned } = result;
  const id = crypto.randomUUID();
  const saved = shotImages !== null && (await saveShots(env, id, shotImages));
  return {
    ...scanned,
    url: `${url.origin}${url.pathname}`,
    scannedAt: new Date().toISOString(),
    listBuiltAt: meta.builtAt,
    shotId: saved ? id : null,
  };
}

/** Full POST /api/scan flow: validate, rate limit, cache, scan, store. */
export async function handleScan(request: Request, env: Env): Promise<ScanData> {
  const url = parseOrThrow(await readTargetInput(request));
  await enforceRateLimit(env, request);

  const origin = new URL(request.url).origin;
  const cacheKey = `${CACHE_PREFIX}${url.origin}${url.pathname}`;
  const hit = await readCache(env, cacheKey);
  if (hit !== null) return toResponseData(hit, origin, true);

  const stored = await storeResult(env, url, await scan(env, url));
  await writeCache(env, cacheKey, stored);
  return toResponseData(stored, origin, false);
}
