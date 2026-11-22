import type { Browser, Page } from '@cloudflare/puppeteer';
import { MSG, apiError } from './errors';

export const USER_AGENT =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36';
export const VIEWPORT = { width: 1280, height: 800 } as const;

export const sleep = (ms: number): Promise<void> => new Promise((resolve) => setTimeout(resolve, ms));

export function withTimeout<T>(work: Promise<T>, ms: number, label: string): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => reject(new Error(`${label} timeout`)), ms);
  });
  return Promise.race([work, timeout]).finally(() => clearTimeout(timer));
}

export async function openPage(browser: Browser): Promise<Page> {
  const page = await browser.newPage();
  await page.setUserAgent(USER_AGENT);
  await page.setViewport(VIEWPORT);
  return page;
}

/**
 * Ad-heavy pages often never fire `load`. A timeout after the document has
 * started loading is not a failure: whatever was requested so far is the result.
 */
export async function load(page: Page, url: URL, timeoutMs: number): Promise<void> {
  try {
    await page.goto(url.href, { waitUntil: 'load', timeout: timeoutMs });
  } catch (err) {
    const timedOut = err instanceof Error && (err.name === 'TimeoutError' || /timeout/i.test(err.message));
    if (timedOut && page.url().startsWith('http')) return;
    console.error('navigation failed', url.href, err);
    throw timedOut ? apiError(504, MSG.timeout) : apiError(502, MSG.loadFailed);
  }
}
