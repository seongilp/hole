import { describe, expect, it } from 'vitest';
import { isUuid, parseShotPath, shotKey, shotUrls } from '../src/shot-route';

const ID = '123e4567-e89b-42d3-a456-426614174000';

describe('isUuid', () => {
  it.each([[ID, true], ['not-a-uuid', false], ['', false], [`${ID}x`, false], [ID.toUpperCase(), false], ['../../etc', false]])(
    '%j -> %s',
    (v, want) => expect(isUuid(v)).toBe(want),
  );
});

describe('parseShotPath', () => {
  it('parses before and after', () => {
    expect(parseShotPath(`/api/shot/${ID}/before.jpg`)).toEqual({ id: ID, kind: 'before' });
    expect(parseShotPath(`/api/shot/${ID}/after.jpg`)).toEqual({ id: ID, kind: 'after' });
  });

  it('lowercases an uppercase id', () => {
    expect(parseShotPath(`/api/shot/${ID.toUpperCase()}/after.jpg`)?.id).toBe(ID);
  });

  it.each([
    ['/api/shot/nope/before.jpg'],
    [`/api/shot/${ID}/middle.jpg`],
    [`/api/shot/${ID}/before.png`],
    [`/api/shot/${ID}/before.jpg/`],
    [`/api/shot/${ID}/../before.jpg`],
    [`/api/shot//before.jpg`],
    [`/shot/${ID}/before.jpg`],
    ['/api/health'],
  ])('rejects %j', (p) => expect(parseShotPath(p)).toBeNull());
});

describe('shot helpers', () => {
  it('builds the KV key', () => {
    expect(shotKey(ID, 'before')).toBe(`shot:v1:${ID}:before`);
  });

  it('builds absolute urls', () => {
    expect(shotUrls('https://hole-demo.example.workers.dev', ID)).toEqual({
      before: `https://hole-demo.example.workers.dev/api/shot/${ID}/before.jpg`,
      after: `https://hole-demo.example.workers.dev/api/shot/${ID}/after.jpg`,
    });
  });
});
