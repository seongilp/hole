import { isBlocked } from './blocklist';

/** One loaded resource. `bytes` is 0 when the browser could not tell. */
export interface ResourceEntry {
  readonly url: string;
  readonly bytes: number;
}

export interface DomainStat {
  readonly domain: string;
  readonly count: number;
  readonly blocked: boolean;
  readonly rule: string | null;
  readonly bytes: number;
}

export interface Summary {
  readonly total: number;
  readonly blocked: number;
  readonly bytes: { readonly total: number; readonly blocked: number };
  readonly sizedRequests: number;
  readonly domains: readonly DomainStat[];
}

interface Tally {
  readonly count: number;
  readonly bytes: number;
}

function hostOf(raw: string): string | null {
  try {
    const u = new URL(raw);
    return u.protocol === 'http:' || u.protocol === 'https:' ? u.hostname.toLowerCase() : null;
  } catch {
    return null;
  }
}

const knownBytes = (n: number): number => (Number.isFinite(n) && n > 0 ? n : 0);

function tallyByHost(entries: readonly ResourceEntry[]): { byHost: Map<string, Tally>; sized: number } {
  const byHost = new Map<string, Tally>();
  let sized = 0;
  for (const { url, bytes } of entries) {
    const host = hostOf(url);
    if (host === null) continue;
    const b = knownBytes(bytes);
    if (b > 0) sized += 1;
    const prev = byHost.get(host) ?? { count: 0, bytes: 0 };
    byHost.set(host, { count: prev.count + 1, bytes: prev.bytes + b });
  }
  return { byHost, sized };
}

function compare(a: DomainStat, b: DomainStat): number {
  if (a.blocked !== b.blocked) return a.blocked ? -1 : 1;
  if (a.count !== b.count) return b.count - a.count;
  return a.domain < b.domain ? -1 : a.domain > b.domain ? 1 : 0;
}

const sum = (items: readonly DomainStat[], pick: (d: DomainStat) => number): number =>
  items.reduce((n, d) => n + pick(d), 0);

/**
 * Turns resource entries into per-domain stats. Non-http(s) URLs are skipped.
 * First-party requests are counted too: "N of M requests blocked" must be
 * measured against everything the page loaded. Byte totals are a lower bound
 * (cross-origin entries without Timing-Allow-Origin report 0).
 */
export function summarize(entries: readonly ResourceEntry[], list: string): Summary {
  const { byHost, sized } = tallyByHost(entries);
  const domains = [...byHost]
    .map(([domain, t]): DomainStat => {
      const rule = isBlocked(list, domain);
      return { domain, count: t.count, blocked: rule !== null, rule, bytes: t.bytes };
    })
    .sort(compare);
  return {
    total: sum(domains, (d) => d.count),
    blocked: sum(domains, (d) => (d.blocked ? d.count : 0)),
    bytes: { total: sum(domains, (d) => d.bytes), blocked: sum(domains, (d) => (d.blocked ? d.bytes : 0)) },
    sizedRequests: sized,
    domains,
  };
}
