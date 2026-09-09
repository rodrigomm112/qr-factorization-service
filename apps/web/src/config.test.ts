import { describe, expect, it } from 'vitest';
import { readConfig } from './config';

describe('readConfig', () => {
  it('takes the runtime values injected by config.js and strips trailing slashes', () => {
    expect(
      readConfig({
        qrApiBaseUrl: 'https://qr-api-abc.a.run.app/',
        statsApiBaseUrl: 'https://stats-api-abc.a.run.app///',
      }),
    ).toEqual({
      qrApiBaseUrl: 'https://qr-api-abc.a.run.app',
      statsApiBaseUrl: 'https://stats-api-abc.a.run.app',
    });
  });

  it('falls back to the compose defaults when the injected value is missing or unusable', () => {
    const defaults = {
      qrApiBaseUrl: 'http://localhost:8080',
      statsApiBaseUrl: 'http://localhost:3000',
    };

    expect(readConfig({})).toEqual(defaults);
    // An entrypoint run without env, or a hand-edited config.js.
    expect(readConfig({ qrApiBaseUrl: '   ', statsApiBaseUrl: 42 })).toEqual(defaults);
    expect(readConfig({ qrApiBaseUrl: '/' })).toEqual(defaults);
  });
});
