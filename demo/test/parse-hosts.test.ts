import { describe, expect, it } from 'vitest';
import { normalize, parseHosts, parseLine, validDomain } from '../scripts/parse-hosts.mjs';

describe('parseLine', () => {
  it.each([
    ['0.0.0.0 ads.example.com', 'ads.example.com'],
    ['127.0.0.1\ttracker.example.net', 'tracker.example.net'],
    ['::1 v6.example.com', 'v6.example.com'],
    ['plain.example.com', 'plain.example.com'],
    ['||adblock.example.com^', 'adblock.example.com'],
    ['0.0.0.0 UPPER.Example.COM.', 'upper.example.com'],
    ['0.0.0.0 a.example.com # trailing comment', 'a.example.com'],
    ['a_b.example.com', 'a_b.example.com'],
  ])('accepts %j', (line, want) => {
    expect(parseLine(line)).toBe(want);
  });

  it.each([
    [''],
    ['# comment'],
    ['! adblock comment'],
    ['0.0.0.0 localhost'],
    ['127.0.0.1 localhost.localdomain'],
    ['255.255.255.255 broadcasthost'],
    ['::1 ip6-localhost'],
    ['0.0.0.0 0.0.0.0'],
    ['0.0.0.0 singlelabel'],
    ['notanip bad.example.com'],
    ['0.0.0.0 a.example.com extra'],
    ['0.0.0.0 bad domain.com'],
    ['0.0.0.0 1.2.3.4'],
    ['0.0.0.0 a..com'],
    ['0.0.0.0 bad!char.com'],
  ])('rejects %j', (line) => {
    expect(parseLine(line)).toBeNull();
  });
});

describe('normalize / validDomain', () => {
  it('lowercases, trims and strips one trailing dot', () => {
    expect(normalize('  Foo.COM. ')).toBe('foo.com');
  });

  it('rejects over-long names and labels', () => {
    expect(validDomain(`${'a'.repeat(64)}.com`)).toBe(false);
    expect(validDomain(`${'a'.repeat(63)}.com`)).toBe(true);
    expect(validDomain(`${'a.'.repeat(130)}com`)).toBe(false);
  });
});

describe('parseHosts', () => {
  it('parses multi-line text and skips invalid lines', () => {
    const text = '# header\n127.0.0.1 localhost\n0.0.0.0 b.com\n0.0.0.0 a.com\n\nnot valid line here\n';
    expect(parseHosts(text)).toEqual(['b.com', 'a.com']);
  });

  it('handles CRLF line endings', () => {
    expect(parseHosts('0.0.0.0 a.com\r\n0.0.0.0 b.com\r\n')).toEqual(['a.com', 'b.com']);
  });
});
