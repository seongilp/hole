// Mirror of internal/blocklist/blocklist.go (Parse, parseLine, Normalize,
// ValidDomain, ignoredHosts). Keep the two in sync.
import { isIP } from 'node:net';

const IGNORED_HOSTS = new Set([
  'localhost',
  'localhost.localdomain',
  'local',
  'broadcasthost',
  'ip6-localhost',
  'ip6-loopback',
  '0.0.0.0',
]);

const MAX_DOMAIN_LENGTH = 253;
const MAX_LABEL_LENGTH = 63;
const LABEL_PATTERN = /^[a-z0-9_-]+$/;

/** Lowercases, trims and strips a single trailing root dot. */
export function normalize(domain) {
  const d = domain.trim().toLowerCase();
  return d.endsWith('.') ? d.slice(0, -1) : d;
}

/** True for a syntactically valid, non-IP domain with at least two labels. */
export function validDomain(d) {
  if (d === '' || d.length > MAX_DOMAIN_LENGTH || !d.includes('.') || isIP(d) !== 0) {
    return false;
  }
  return d.split('.').every((label) => label !== '' && label.length <= MAX_LABEL_LENGTH && LABEL_PATTERN.test(label));
}

function candidateFrom(fields) {
  if (fields.length === 1) {
    const f = fields[0].startsWith('||') ? fields[0].slice(2) : fields[0];
    return f.endsWith('^') ? f.slice(0, -1) : f;
  }
  if (fields.length === 2 && isIP(fields[0]) !== 0) {
    return fields[1];
  }
  return null;
}

/** Parses one list line; returns the normalized domain or null. */
export function parseLine(line) {
  const cut = line.search(/[#!]/);
  const body = cut >= 0 ? line.slice(0, cut) : line;
  const fields = body.split(/\s+/).filter((f) => f !== '');
  const candidate = candidateFrom(fields);
  if (candidate === null) return null;
  const d = normalize(candidate);
  return IGNORED_HOSTS.has(d) || !validDomain(d) ? null : d;
}

/** Parses hosts / plain / adblock-style text into domains in order of appearance. */
export function parseHosts(text) {
  return text
    .split('\n')
    .map(parseLine)
    .filter((d) => d !== null);
}
