export type ShotKind = 'before' | 'after';

const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const SHOT_PATH = /^\/api\/shot\/([^/]+)\/(before|after)\.jpg$/;

export const isUuid = (value: string): boolean => UUID_PATTERN.test(value);

export function shotKey(id: string, kind: ShotKind): string {
  return `shot:v1:${id}:${kind}`;
}

/** Parses `/api/shot/<uuid>/before.jpg`; returns null for anything else. */
export function parseShotPath(pathname: string): { id: string; kind: ShotKind } | null {
  const m = SHOT_PATH.exec(pathname);
  const id = m?.[1]?.toLowerCase();
  const kind = m?.[2] as ShotKind | undefined;
  return id !== undefined && kind !== undefined && isUuid(id) ? { id, kind } : null;
}

export function shotUrls(origin: string, id: string): { before: string; after: string } {
  return { before: `${origin}/api/shot/${id}/before.jpg`, after: `${origin}/api/shot/${id}/after.jpg` };
}
