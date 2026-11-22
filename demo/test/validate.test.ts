import { describe, expect, it } from 'vitest';
import { TargetError, parseTarget } from '../src/validate';

describe('parseTarget accepts', () => {
  it.each([
    ['example.com', 'https://example.com/'],
    ['  example.com  ', 'https://example.com/'],
    ['https://example.com/a?b=1', 'https://example.com/a?b=1'],
    ['http://example.com', 'http://example.com/'],
    ['https://example.com:443/x', 'https://example.com/x'],
    ['http://example.com:80', 'http://example.com/'],
    ['news.example.co.kr/path', 'https://news.example.co.kr/path'],
    ['EXAMPLE.com.', 'https://example.com./'],
    ['https://xn--bj0bj06e.kr', 'https://xn--bj0bj06e.kr/'],
    ['한국.kr', 'https://xn--3e0b707e.kr/'],
  ])('%j', (input, href) => {
    expect(parseTarget(input).href).toBe(href);
  });
});

describe('parseTarget rejects', () => {
  it.each([
    [''],
    ['   '],
    ['ftp://example.com'],
    ['file:///etc/passwd'],
    ['javascript:alert(1)'],
    ['data:text/html,hi'],
    ['https://user:pw@example.com'],
    ['https://user@example.com'],
    ['https://example.com:8443'],
    ['http://example.com:443'],
    ['http://127.0.0.1'],
    ['http://2130706433'],
    ['http://0x7f.1'],
    ['http://10.0.0.1/admin'],
    ['https://[::1]/'],
    ['https://[2001:db8::1]/'],
    ['localhost'],
    ['http://localhost'],
    ['intranet'],
    ['printer.local'],
    ['db.internal'],
    ['router.lan'],
    ['nas.home.arpa'],
    ['site.test'],
    ['foo.localhost'],
    ['bad host.com'],
    ['a..com'],
    ['http://'],
  ])('%j', (input) => {
    expect(() => parseTarget(input)).toThrow(TargetError);
  });

  it('rejects input longer than 2000 chars', () => {
    expect(() => parseTarget(`example.com/${'a'.repeat(2000)}`)).toThrow(TargetError);
  });

  it('gives a Korean message', () => {
    try {
      parseTarget('localhost');
      expect.unreachable();
    } catch (e) {
      expect((e as TargetError).message).toMatch(/[가-힣]/);
    }
  });
});
