import js from '@eslint/js';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  { ignores: ['dist/**', 'coverage/**', 'node_modules/**'] },
  js.configs.recommended,
  tseslint.configs.recommendedTypeChecked,
  {
    languageOptions: {
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      '@typescript-eslint/consistent-type-imports': ['error', { fixStyle: 'separate-type-imports' }],
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
    },
  },
  {
    // Layering enforced, not merely documented: the domain is pure TypeScript.
    files: ['src/domain/**/*.ts'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['express', 'express/*', 'zod', 'zod/*', 'jsonwebtoken', 'jsonwebtoken/*', 'pino', 'pino/*'],
              message:
                'src/domain must stay free of frameworks and libraries: keep HTTP, validation and logging in the adapters.',
            },
            {
              group: ['**/adapters/**', '**/platform/**'],
              message:
                'src/domain is the innermost ring: it may not import from src/adapters or src/platform (the dependency points inwards).',
            },
          ],
        },
      ],
    },
  },
  {
    // The use case orchestrates the domain; it knows nothing about HTTP.
    files: ['src/application/**/*.ts'],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['express', 'express/*', '**/adapters/**'],
              message:
                'src/application must not depend on the transport: keep Express and the adapters outside the use case.',
            },
          ],
        },
      ],
    },
  },
  {
    // supertest types `response.body` as `any`; asserting on it is the point. `src/**` keeps them on.
    files: ['tests/**/*.ts'],
    rules: {
      '@typescript-eslint/no-unsafe-argument': 'off',
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
    },
  },
  {
    files: ['**/*.js'],
    extends: [tseslint.configs.disableTypeChecked],
  },
);
