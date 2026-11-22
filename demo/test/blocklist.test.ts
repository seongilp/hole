import { describe, expect, it } from 'vitest';
import { isBlocked } from '../src/blocklist';

const LIST = ['ads.example.com', 'doubleclick.net', 'zzz.org', 'a.io', 'tracker.com'].sort().join('\n') + '\n';

describe('isBlocked', () => {
  it('matches the exact domain and returns the rule', () => {
    expect(isBlocked(LIST, 'doubleclick.net')).toBe('doubleclick.net');
  });

  it('matches subdomains of a listed domain', () => {
    expect(isBlocked(LIST, 'stats.g.doubleclick.net')).toBe('doubleclick.net');
  });

  it('returns null for non-matches', () => {
    expect(isBlocked(LIST, 'example.com')).toBeNull();
    expect(isBlocked(LIST, 'google.com')).toBeNull();
  });

  it('finds the first and last lines of the list', () => {
    expect(isBlocked(LIST, 'a.io')).toBe('a.io');
    expect(isBlocked(LIST, 'zzz.org')).toBe('zzz.org');
  });

  it('does not match a string-prefix or string-suffix of a listed name', () => {
    expect(isBlocked(LIST, 'doubleclick.ne')).toBeNull();
    expect(isBlocked(LIST, 'doubleclick.network')).toBeNull();
    expect(isBlocked(LIST, 'notdoubleclick.net')).toBeNull();
    expect(isBlocked(LIST, 'xtracker.com')).toBeNull();
  });

  it('requires at least two labels (a bare TLD never matches)', () => {
    expect(isBlocked('com\nnet\n', 'example.com')).toBeNull();
    expect(isBlocked('com\nnet\n', 'com')).toBeNull();
  });

  it('normalizes case and trailing dot', () => {
    expect(isBlocked(LIST, 'ADS.Example.COM.')).toBe('ads.example.com');
  });

  it('handles empty list and empty host', () => {
    expect(isBlocked('', 'a.com')).toBeNull();
    expect(isBlocked(LIST, '')).toBeNull();
  });

  it('works on a single-line list without trailing newline', () => {
    expect(isBlocked('only.com', 'x.only.com')).toBe('only.com');
  });

  it('finds every entry in a large sorted list', () => {
    const domains = Array.from({ length: 5000 }, (_, i) => `d${i}.test-${i % 7}.com`).sort();
    const big = domains.join('\n') + '\n';
    for (const d of domains) expect(isBlocked(big, d)).toBe(d);
    expect(isBlocked(big, 'd5000.test-0.com')).toBeNull();
  });
});
