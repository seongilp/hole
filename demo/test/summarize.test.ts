import { describe, expect, it } from 'vitest';
import { summarize, type ResourceEntry } from '../src/summarize';

const LIST = ['ads.net', 'tracker.io'].join('\n') + '\n';
const e = (url: string, bytes = 0): ResourceEntry => ({ url, bytes });

describe('summarize', () => {
  it('counts requests and sums bytes per domain', () => {
    const r = summarize(
      [
        e('https://cdn.site.com/a.js', 1000),
        e('https://cdn.site.com/b.js', 500),
        e('https://x.ads.net/p.gif', 43),
        e('https://tracker.io/t', 2000),
        e('https://tracker.io/t2', 0),
        e('https://tracker.io/t3', 100),
      ],
      LIST,
    );
    expect(r.total).toBe(6);
    expect(r.blocked).toBe(4);
    expect(r.bytes).toEqual({ total: 3643, blocked: 2143 });
    expect(r.sizedRequests).toBe(5);
    expect(r.domains).toEqual([
      { domain: 'tracker.io', count: 3, blocked: true, rule: 'tracker.io', bytes: 2100 },
      { domain: 'x.ads.net', count: 1, blocked: true, rule: 'ads.net', bytes: 43 },
      { domain: 'cdn.site.com', count: 2, blocked: false, rule: null, bytes: 1500 },
    ]);
  });

  it('counts zero-size entries as requests but not as sized', () => {
    const r = summarize([e('https://a.com/x', 0), e('https://a.com/y', 0)], LIST);
    expect(r.total).toBe(2);
    expect(r.sizedRequests).toBe(0);
    expect(r.bytes).toEqual({ total: 0, blocked: 0 });
  });

  it('ignores negative and non-finite sizes', () => {
    const r = summarize([e('https://a.com/x', -5), e('https://a.com/y', Number.NaN), e('https://a.com/z', 10)], LIST);
    expect(r.bytes.total).toBe(10);
    expect(r.sizedRequests).toBe(1);
  });

  it('skips non-http(s) and malformed URLs', () => {
    const r = summarize(
      [e('data:image/png;base64,AAAA', 99), e('blob:https://a.com/1'), e('about:blank'), e('not a url'), e('https://ok.com/x', 7)],
      LIST,
    );
    expect(r.total).toBe(1);
    expect(r.bytes.total).toBe(7);
    expect(r.domains.map((d) => d.domain)).toEqual(['ok.com']);
  });

  it('counts first-party requests too', () => {
    const r = summarize([e('https://site.com/a'), e('https://other.com/b')], LIST);
    expect(r.domains.map((d) => d.domain)).toEqual(['other.com', 'site.com']);
  });

  it('breaks ties by domain and does not mutate input', () => {
    const input = Object.freeze([e('https://b.com/'), e('https://a.com/')]) as ResourceEntry[];
    const r = summarize(input, LIST);
    expect(r.domains.map((d) => d.domain)).toEqual(['a.com', 'b.com']);
    expect(input.map((x) => x.url)).toEqual(['https://b.com/', 'https://a.com/']);
  });

  it('handles empty input', () => {
    expect(summarize([], LIST)).toEqual({
      total: 0,
      blocked: 0,
      bytes: { total: 0, blocked: 0 },
      sizedRequests: 0,
      domains: [],
    });
  });

  it('is case-insensitive on hostnames', () => {
    const r = summarize([e('https://CDN.Site.com/a', 1), e('https://cdn.site.com/b', 2)], LIST);
    expect(r.domains).toEqual([{ domain: 'cdn.site.com', count: 2, blocked: false, rule: null, bytes: 3 }]);
  });
});
