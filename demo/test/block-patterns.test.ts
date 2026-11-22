import { describe, expect, it } from 'vitest';
import { blockPatterns, legacyBlockUrls } from '../src/block-patterns';

describe('block patterns', () => {
  it('builds wildcard patterns per host', () => {
    expect(legacyBlockUrls(['ads.net', 'x.tracker.io'])).toEqual(['*://ads.net/*', '*://x.tracker.io/*']);
    expect(blockPatterns(['ads.net'])).toEqual([{ urlPattern: '*://ads.net/*', block: true }]);
  });

  it('dedupes and drops hosts with unsafe characters', () => {
    expect(legacyBlockUrls(['a.com', 'a.com', 'b.com/evil', '*.com', 'c d.com', ''])).toEqual(['*://a.com/*']);
  });

  it('caps the number of patterns', () => {
    const hosts = Array.from({ length: 1000 }, (_, i) => `h${i}.com`);
    expect(legacyBlockUrls(hosts)).toHaveLength(300);
  });

  it('handles empty input without mutating it', () => {
    const hosts = Object.freeze(['a.com']) as string[];
    expect(legacyBlockUrls([])).toEqual([]);
    legacyBlockUrls(hosts);
    expect(hosts).toEqual(['a.com']);
  });
});
