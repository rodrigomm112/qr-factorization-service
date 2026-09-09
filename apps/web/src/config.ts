/**
 * Runtime `window.__APP_CONFIG__` (public/config.js, rewritten at container start by
 * docker-entrypoint.d/10-app-config.sh) wins over build-time `VITE_*`, then the compose
 * defaults: one image serves localhost and Cloud Run. The BROWSER resolves these URLs, so
 * inside compose they stay localhost and never the service names.
 */
export interface AppConfig {
  readonly qrApiBaseUrl: string;
  readonly statsApiBaseUrl: string;
}

declare global {
  interface Window {
    __APP_CONFIG__?: Partial<Record<keyof AppConfig, unknown>>;
  }
}

const DEFAULT_QR_API_BASE_URL = 'http://localhost:8080';
const DEFAULT_STATS_API_BASE_URL = 'http://localhost:3000';

/** Stored without a trailing slash; paths always start with one. */
const normalize = (value: string): string => value.trim().replace(/\/+$/, '');

function pick(candidates: readonly unknown[], fallback: string): string {
  for (const candidate of candidates) {
    if (typeof candidate === 'string' && candidate.trim() !== '') {
      const normalized = normalize(candidate);
      if (normalized !== '') return normalized;
    }
  }
  return fallback;
}

export function readConfig(runtime: Partial<Record<keyof AppConfig, unknown>> = {}): AppConfig {
  return {
    qrApiBaseUrl: pick(
      [runtime.qrApiBaseUrl, import.meta.env.VITE_QR_API_BASE_URL],
      DEFAULT_QR_API_BASE_URL,
    ),
    statsApiBaseUrl: pick(
      [runtime.statsApiBaseUrl, import.meta.env.VITE_STATS_API_BASE_URL],
      DEFAULT_STATS_API_BASE_URL,
    ),
  };
}

export const config: AppConfig = readConfig(
  typeof window === 'undefined' ? {} : (window.__APP_CONFIG__ ?? {}),
);
