// @ts-check

import js from '@eslint/js';
import { defineConfig, globalIgnores } from 'eslint/config';
import tseslint from 'typescript-eslint';
import globals from 'globals';
import reactHooks from 'eslint-plugin-react-hooks';

export default defineConfig(
  globalIgnores(['dist/', 'node_modules/', 'wailsjs/', 'bindings/', 'vite.config.ts']),

  {
    files: ['**/*.{ts,tsx}'],

    languageOptions: {
      parser: tseslint.parser,
      parserOptions: {
        ecmaFeatures: { jsx: true },
      },
      globals: {
        ...globals.browser,
        ...globals.es2021,
      },
    },

    plugins: {
      'react-hooks': reactHooks,
    },

    extends: [
      js.configs.recommended,
      ...tseslint.configs.recommended
    ],

    rules: {
      ...reactHooks.configs['recommended-latest'].rules,

      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_' }],
      '@typescript-eslint/consistent-type-imports': [
        'warn',
        { prefer: 'type-imports', fixStyle: 'inline-type-imports' },
      ],

      'no-console': 'off',
    },
  },

  {
    files: ['**/*.js'],
    extends: [tseslint.configs.disableTypeChecked],
  },

  {
    // Playwright fixtures (e2e/fixtures.ts) destructure a `use` callback per
    // the Playwright fixture API (`async ({ page }, use) => { await use(...) }`)
    // — react-hooks' rules-of-hooks sees `use(...)` and assumes it's React 19's
    // `use()` hook called outside a component/hook. This is plain test
    // infrastructure, not React code, so the react-hooks rules don't apply.
    files: ['e2e/**/*.{ts,tsx}'],
    rules: {
      'react-hooks/rules-of-hooks': 'off',
    },
  },
);
