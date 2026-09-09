import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'node',
    include: ['tests/**/*.test.ts'],
    restoreMocks: true,
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html', 'lcov'],
      include: ['src/**/*.ts'],
      // Composition root: wiring only, exercised by `docker run` rather than by unit tests.
      exclude: ['src/main.ts'],
      thresholds: {
        'src/domain/**': { statements: 85, branches: 85, functions: 85, lines: 85 },
        'src/application/**': { statements: 85, branches: 85, functions: 85, lines: 85 },
      },
    },
  },
});
