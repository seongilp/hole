import puppeteer, { type Browser, type Page } from '@cloudflare/puppeteer';
import blocklist from './blocklist.txt';
import { load, openPage, sleep } from './browser-utils';
import { collectEntries } from './collect';
import type { Env } from './env';
import { MSG, apiError } from './errors';
import { shootAfter, shootBefore } from './shots';
import type { ShotImages } from './shot-store';
import { summarize, type Summary } from './summarize';

const NAVIGATION_TIMEOUT_MS = 12_000;
const LATE_SCRIPT_WAIT_MS = 4_000;
const POST_SCROLL_WAIT_MS = 1_500;
const TOTAL_BUDGET_MS = 32_000;
const MIN_SECOND_PASS_BUDGET_MS = 8_000;

const SCROLL_BOTTOM_SCRIPT = 'window.scrollTo(0, document.documentElement.scrollHeight)';

export interface ScanResult extends Summary {
  readonly finalUrl: string;
  /** null when nothing was blocked or the best-effort second pass failed. */
  readonly shotImages: ShotImages | null;
}

async function launch(env: Env): Promise<Browser> {
  try {
    return await puppeteer.launch(env.BROWSER);
  } catch (err) {
    console.error('browser launch failed', err);
    throw apiError(503, MSG.quota);
  }
}

async function firstPass(page: Page, url: URL) {
  await load(page, url, NAVIGATION_TIMEOUT_MS);
  await sleep(LATE_SCRIPT_WAIT_MS);
  await page.evaluate(SCROLL_BOTTOM_SCRIPT).catch(() => undefined);
  await sleep(POST_SCROLL_WAIT_MS);
  return collectEntries(page);
}

/** Best effort: any failure is logged and yields null, never failing the scan. */
async function captureShots(
  browser: Browser,
  page: Page,
  finalUrl: string,
  hosts: readonly string[],
  startedAt: number,
): Promise<ShotImages | null> {
  if (hosts.length === 0) return null;
  try {
    const target = new URL(finalUrl);
    const before = await shootBefore(page);
    const budget = TOTAL_BUDGET_MS - (Date.now() - startedAt);
    if (budget < MIN_SECOND_PASS_BUDGET_MS) return null;
    return { before, after: await shootAfter(browser, target, hosts, budget) };
  } catch (err) {
    console.error('screenshot pass failed', err);
    return null;
  }
}

/** Opens `url` in a headless browser and reports which loaded hosts Hole would block. */
export async function scan(env: Env, url: URL): Promise<ScanResult> {
  const startedAt = Date.now();
  const browser = await launch(env);
  try {
    const page = await openPage(browser);
    const { entries, finalUrl } = await firstPass(page, url);
    const summary = summarize(entries, blocklist);
    const hosts = summary.domains.filter((d) => d.blocked).map((d) => d.domain);
    const shotImages = await captureShots(browser, page, finalUrl, hosts, startedAt);
    return { finalUrl, ...summary, shotImages };
  } finally {
    await browser.close().catch((err: unknown) => console.error('browser close failed', err));
  }
}
