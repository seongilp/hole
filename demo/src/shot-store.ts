import type { Env } from './env';
import { shotKey, type ShotKind } from './shot-route';

// A minute longer than the scan record so a live record never points at an expired image.
const SHOT_TTL_SECONDS = 24 * 60 * 60 + 60;

export interface ShotImages {
  readonly before: Uint8Array;
  readonly after: Uint8Array;
}

/** Stores both images; returns false (and logs) if either write fails. */
export async function saveShots(env: Env, id: string, shots: ShotImages): Promise<boolean> {
  try {
    const opts = { expirationTtl: SHOT_TTL_SECONDS };
    await Promise.all([
      env.SCANS.put(shotKey(id, 'before'), shots.before, opts),
      env.SCANS.put(shotKey(id, 'after'), shots.after, opts),
    ]);
    return true;
  } catch (err) {
    console.error('shot write failed', err);
    return false;
  }
}

export async function readShot(env: Env, id: string, kind: ShotKind): Promise<ArrayBuffer | null> {
  try {
    return await env.SCANS.get(shotKey(id, kind), 'arrayBuffer');
  } catch (err) {
    console.error('shot read failed', err);
    return null;
  }
}
