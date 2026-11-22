export interface Envelope<T> {
  readonly success: boolean;
  readonly data: T | null;
  readonly error: string | null;
}

const DEFAULT_ORIGINS = ['https://seongilp.github.io', 'http://localhost:8000'];
const PREFLIGHT_MAX_AGE_SECONDS = '86400';

export function allowedOrigins(raw: string | undefined): readonly string[] {
  if (raw === undefined) return DEFAULT_ORIGINS;
  return raw
    .split(',')
    .map((o) => o.trim())
    .filter((o) => o !== '');
}

export function corsHeaders(origin: string | null, allowed: readonly string[]): Record<string, string> {
  if (origin === null || !allowed.includes(origin)) return {};
  return {
    'Access-Control-Allow-Origin': origin,
    'Access-Control-Allow-Methods': 'GET, POST, OPTIONS',
    'Access-Control-Allow-Headers': 'Content-Type',
    'Access-Control-Max-Age': PREFLIGHT_MAX_AGE_SECONDS,
    Vary: 'Origin',
  };
}

export function json<T>(body: Envelope<T>, status: number, extra: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json; charset=utf-8', 'Cache-Control': 'no-store', ...extra },
  });
}

export const ok = <T>(data: T, extra?: Record<string, string>): Response =>
  json({ success: true, data, error: null }, 200, extra);

export const fail = (status: number, error: string, extra?: Record<string, string>): Response =>
  json({ success: false, data: null, error }, status, extra);
