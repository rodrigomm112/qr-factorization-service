import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

/** Package root; `src/platform` (tsx) and `dist/platform` (built) both sit one level below. */
export const PACKAGE_ROOT: string = resolve(fileURLToPath(new URL('.', import.meta.url)), '..', '..');

export const OPENAPI_PATH: string = resolve(PACKAGE_ROOT, 'openapi.yaml');

export const SERVICE_NAME = 'stats-api';

function readVersion(): string {
  // APP_VERSION is stamped by `--build-arg VERSION=<git sha>`, so /health/live reports the deployed
  // commit; outside a build package.json is the fallback.
  const fromEnv = process.env['APP_VERSION'];
  if (fromEnv !== undefined && fromEnv.trim() !== '') return fromEnv.trim();
  try {
    const parsed: unknown = JSON.parse(readFileSync(resolve(PACKAGE_ROOT, 'package.json'), 'utf8'));
    if (typeof parsed === 'object' && parsed !== null) {
      const version: unknown = (parsed as Record<string, unknown>)['version'];
      if (typeof version === 'string') return version;
    }
  } catch {
    // The version is cosmetic: a missing package.json must not stop the service from starting.
  }
  return '0.0.0';
}

export const SERVICE_VERSION: string = readVersion();
