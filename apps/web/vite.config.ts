import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [react()],
  build: {
    target: 'es2023',
    // The polyfill is injected inline, which the container's `script-src 'self'` blocks; every
    // targeted browser supports <link modulepreload> anyway.
    modulePreload: { polyfill: false },
    sourcemap: false,
    reportCompressedSize: true,
  },
  server: {
    port: 5173,
  },
  preview: {
    port: 4173,
  },
  test: {
    environment: 'jsdom',
    // On the 9p mount, one jsdom per file costs ~40 s; one reused worker keeps the suite at ~50 s.
    // Safe without isolation: every test stubs its own globals and `cleanup()` unmounts.
    pool: 'forks',
    maxWorkers: 1,
    isolate: false,
    testTimeout: 20_000,
    globals: false,
    setupFiles: ['./src/test/setup.ts'],
    css: false,
    restoreMocks: true,
    unstubGlobals: true,
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/main.tsx', 'src/test/**', 'src/**/*.test.{ts,tsx}'],
    },
  },
});
