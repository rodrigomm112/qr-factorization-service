import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

// `globals: false`, so Testing Library cannot install its own auto-cleanup.
afterEach(() => {
  cleanup();
});
