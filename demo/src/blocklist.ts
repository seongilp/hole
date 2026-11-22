const NEWLINE = '\n';

/** Lowercases and strips a trailing root dot. */
function normalizeHost(host: string): string {
  const h = host.trim().toLowerCase();
  return h.endsWith('.') ? h.slice(0, -1) : h;
}

/**
 * Binary search over a newline-delimited, code-unit-sorted string.
 * Never splits the list: only the line under inspection is sliced.
 */
function contains(list: string, term: string): boolean {
  let lo = 0;
  let hi = list.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    const start = mid === 0 ? 0 : list.lastIndexOf(NEWLINE, mid - 1) + 1;
    const nl = list.indexOf(NEWLINE, mid);
    const end = nl === -1 ? list.length : nl;
    const line = list.slice(start, end);
    if (line === term) return true;
    if (term < line) hi = start;
    else lo = end + 1;
  }
  return false;
}

/**
 * Returns the rule (listed domain) that blocks `host`, or null. Like Hole,
 * the host itself and every parent suffix with >= 2 labels are checked.
 */
export function isBlocked(list: string, host: string): string | null {
  let candidate = normalizeHost(host);
  while (candidate.includes('.')) {
    if (candidate !== '' && contains(list, candidate)) return candidate;
    const dot = candidate.indexOf('.');
    if (dot < 0) return null;
    candidate = candidate.slice(dot + 1);
  }
  return null;
}
