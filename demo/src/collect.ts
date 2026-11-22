import type { Frame, Page } from '@cloudflare/puppeteer';
import { withTimeout } from './browser-utils';
import type { ResourceEntry } from './summarize';

const FRAME_EVALUATE_TIMEOUT_MS = 3_000;
const MAX_FRAMES = 40;

// Evaluated as a string so the Worker does not need DOM typings. Cross-origin
// entries without Timing-Allow-Origin report 0 bytes; callers treat sizes as a lower bound.
const COLLECT_SCRIPT = `({
  entries: performance.getEntriesByType('resource').map((e) => ({ url: e.name, bytes: e.transferSize || e.encodedBodySize || 0 })),
  href: location.href
})`;

interface FrameSnapshot {
  readonly entries: readonly ResourceEntry[];
  readonly href: string;
}

function isSnapshot(value: unknown): value is FrameSnapshot {
  const v = value as FrameSnapshot | null;
  return typeof v?.href === 'string' && Array.isArray(v.entries);
}

async function snapshotFrame(frame: Frame): Promise<FrameSnapshot | null> {
  try {
    const value = await withTimeout(frame.evaluate(COLLECT_SCRIPT), FRAME_EVALUATE_TIMEOUT_MS, 'frame evaluate');
    return isSnapshot(value) ? value : null;
  } catch {
    return null; // detached, cross-process or slow frames are simply skipped
  }
}

/** One `evaluate` per frame (no per-request event handlers, to save Worker CPU). */
export async function collectEntries(page: Page): Promise<{ entries: ResourceEntry[]; finalUrl: string }> {
  const frames = page.frames().slice(0, MAX_FRAMES);
  const snapshots = (await Promise.all(frames.map(snapshotFrame))).filter((s): s is FrameSnapshot => s !== null);
  const entries = snapshots.flatMap((s) => [...s.entries, { url: s.href, bytes: 0 }]);
  return { entries, finalUrl: page.url() };
}
