import type { Browser, CDPSession, Page } from '@cloudflare/puppeteer';
import { blockPatterns, legacyBlockUrls } from './block-patterns';
import { load, openPage, sleep } from './browser-utils';

const JPEG_QUALITY = 60;
const SCROLL_TOP_SETTLE_MS = 300;
const SECOND_PASS_SETTLE_MS = 3_000;
const SECOND_PASS_NAV_TIMEOUT_MS = 10_000;
const SCREENSHOT_RESERVE_MS = 1_500;

const SCROLL_TOP_SCRIPT = 'window.scrollTo(0, 0)';

export async function screenshot(page: Page): Promise<Uint8Array> {
  return page.screenshot({ type: 'jpeg', quality: JPEG_QUALITY, fullPage: false });
}

/** Screenshot A: scroll back to the top of the already-loaded page. */
export async function shootBefore(page: Page): Promise<Uint8Array> {
  await page.evaluate(SCROLL_TOP_SCRIPT);
  await sleep(SCROLL_TOP_SETTLE_MS);
  return screenshot(page);
}

/**
 * Blocks hosts with CDP `Network.setBlockedURLs` (no request interception, so
 * no per-request handler runs in the Worker). Tries the current `urlPatterns`
 * form together with the deprecated `urls` form, then `urls` alone for older Chromes.
 */
async function blockHosts(session: CDPSession, hosts: readonly string[]): Promise<void> {
  await session.send('Network.enable');
  try {
    await session.send('Network.setBlockedURLs', { urls: legacyBlockUrls(hosts), urlPatterns: blockPatterns(hosts) });
  } catch (err) {
    console.error('setBlockedURLs with urlPatterns failed, retrying with urls only', err);
    await session.send('Network.setBlockedURLs', { urls: legacyBlockUrls(hosts) });
  }
}

/** Screenshot B: same page with the blocked hosts unreachable. */
export async function shootAfter(browser: Browser, url: URL, hosts: readonly string[], budgetMs: number): Promise<Uint8Array> {
  const page = await openPage(browser);
  const session = await page.createCDPSession();
  await blockHosts(session, hosts);
  const navTimeout = Math.min(SECOND_PASS_NAV_TIMEOUT_MS, budgetMs - SECOND_PASS_SETTLE_MS - SCREENSHOT_RESERVE_MS);
  await load(page, url, navTimeout);
  await sleep(SECOND_PASS_SETTLE_MS);
  return screenshot(page);
}
