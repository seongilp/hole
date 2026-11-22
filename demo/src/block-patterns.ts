const MAX_PATTERNS = 300;
const SAFE_HOST = /^[a-z0-9._-]+$/;

export interface BlockPattern {
  readonly urlPattern: string;
  readonly block: boolean;
}

function safeHosts(hosts: readonly string[]): string[] {
  return [...new Set(hosts)].filter((h) => SAFE_HOST.test(h)).slice(0, MAX_PATTERNS);
}

/** Legacy `Network.setBlockedURLs({urls})` wildcard patterns, e.g. `*://host/*`. */
export function legacyBlockUrls(hosts: readonly string[]): string[] {
  return safeHosts(hosts).map((h) => `*://${h}/*`);
}

/** Current `Network.setBlockedURLs({urlPatterns})` entries (URLPattern syntax). */
export function blockPatterns(hosts: readonly string[]): BlockPattern[] {
  return legacyBlockUrls(hosts).map((urlPattern) => ({ urlPattern, block: true }));
}
