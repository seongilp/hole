import type { Env } from './env';
import { ApiError, MSG } from './errors';
import { allowedOrigins, corsHeaders, fail, ok } from './response';
import { handleScan } from './scan-handler';
import { parseShotPath } from './shot-route';
import { readShot } from './shot-store';

type Cors = Record<string, string>;

function errorResponse(err: unknown, cors: Cors): Response {
  if (err instanceof ApiError) return fail(err.status, err.message, cors);
  console.error('unhandled error', err);
  return fail(500, MSG.internal, cors);
}

async function route(request: Request, env: Env, cors: Cors): Promise<Response> {
  const { pathname } = new URL(request.url);
  if (pathname === '/api/health' && request.method === 'GET') {
    return ok({ status: 'ok' }, cors);
  }
  if (pathname === '/api/scan' && request.method === 'POST') {
    return ok(await handleScan(request, env), cors);
  }
  return fail(404, MSG.notFound, cors);
}

const SHOT_HEADERS = {
  'Content-Type': 'image/jpeg',
  'Cache-Control': 'public, max-age=86400',
  'X-Content-Type-Options': 'nosniff',
} as const;

/** GET /api/shot/<uuid>/before|after.jpg. Loaded by <img>, so no Origin restriction. */
async function serveShot(request: Request, env: Env): Promise<Response | null> {
  if (request.method !== 'GET') return null;
  const target = parseShotPath(new URL(request.url).pathname);
  if (target === null) return null;
  const image = await readShot(env, target.id, target.kind);
  return image === null ? fail(404, MSG.notFound) : new Response(image, { headers: SHOT_HEADERS });
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const shot = await serveShot(request, env);
    if (shot !== null) return shot;

    const origin = request.headers.get('Origin');
    const allowed = allowedOrigins(env.ALLOWED_ORIGINS);
    const cors = corsHeaders(origin, allowed);

    if (request.method === 'OPTIONS') {
      return new Response(null, { status: Object.keys(cors).length > 0 ? 204 : 403, headers: cors });
    }
    // Browsers always send Origin on cross-site POST; refuse origins we don't know.
    if (origin !== null && !allowed.includes(origin)) {
      return fail(403, MSG.forbiddenOrigin);
    }
    try {
      return await route(request, env, cors);
    } catch (err) {
      return errorResponse(err, cors);
    }
  },
} satisfies ExportedHandler<Env>;
