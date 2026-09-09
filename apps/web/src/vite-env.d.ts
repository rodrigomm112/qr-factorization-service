/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Build-time override; runtime config.js wins. */
  readonly VITE_QR_API_BASE_URL?: string;
  /** Build-time override; runtime config.js wins. */
  readonly VITE_STATS_API_BASE_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
