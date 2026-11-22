const MAX_INPUT_LENGTH = 2000;
const SCHEME_PATTERN = /^[a-z][a-z0-9+.-]*:\/\//i;
const IPV4_PATTERN = /^\d{1,3}(\.\d{1,3}){3}$/;
const LABEL_PATTERN = /^[a-z0-9_-]+$/;
const ALLOWED_PROTOCOLS = new Set(['http:', 'https:']);
const BLOCKED_SUFFIXES = ['local', 'internal', 'lan', 'home.arpa', 'test', 'localhost'];

/** Thrown for any input the demo refuses; `message` is user-facing (Korean). */
export class TargetError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'TargetError';
  }
}

const reject = (message: string): never => {
  throw new TargetError(message);
};

function parseUrl(raw: string): URL {
  const withScheme = SCHEME_PATTERN.test(raw) ? raw : `https://${raw}`;
  try {
    return new URL(withScheme);
  } catch {
    return reject('올바른 주소가 아닙니다. 예: example.com');
  }
}

function assertPublicHostname(hostname: string): void {
  const host = hostname.endsWith('.') ? hostname.slice(0, -1) : hostname;
  if (host.startsWith('[') || IPV4_PATTERN.test(host)) {
    reject('IP 주소는 검사할 수 없습니다. 도메인 이름을 입력해 주세요.');
  }
  if (!host.includes('.')) {
    reject('도메인 이름을 입력해 주세요. 예: example.com');
  }
  if (BLOCKED_SUFFIXES.some((s) => host === s || host.endsWith(`.${s}`))) {
    reject('내부 네트워크 주소는 검사할 수 없습니다.');
  }
  if (!host.split('.').every((label) => LABEL_PATTERN.test(label))) {
    reject('올바른 도메인 이름이 아닙니다.');
  }
}

/** Validates user input at the boundary and returns a safe, public http(s) URL. */
export function parseTarget(input: string): URL {
  const raw = input.trim();
  if (raw === '') reject('주소를 입력해 주세요.');
  if (raw.length > MAX_INPUT_LENGTH) reject('주소가 너무 깁니다.');
  const url = parseUrl(raw);
  if (!ALLOWED_PROTOCOLS.has(url.protocol)) reject('http 또는 https 주소만 검사할 수 있습니다.');
  if (url.username !== '' || url.password !== '') reject('계정 정보가 들어간 주소는 검사할 수 없습니다.');
  if (url.port !== '') reject('기본 포트(80, 443)만 사용할 수 있습니다.');
  assertPublicHostname(url.hostname);
  return url;
}
