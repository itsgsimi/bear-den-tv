// ESLint flat configuration for the navigation script: recommended JS +
// TypeScript rules over src/ and tests/; build output and dependencies are
// ignored.
import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import globals from 'globals';

export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**', 'test-results/**', 'playwright-report/**'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ['src/**/*.ts'],
    languageOptions: { globals: { ...globals.browser } },
  },
  {
    files: ['scripts/**/*.mjs', 'tests/**/*.ts', '*.ts', '*.js'],
    languageOptions: { globals: { ...globals.node } },
  },
  {
    rules: {
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      '@typescript-eslint/consistent-type-imports': 'error',
      'no-console': ['error', { allow: ['warn', 'error'] }],
    },
  },
  {
    files: ['scripts/**/*.mjs'],
    rules: { 'no-console': 'off' },
  },
);
