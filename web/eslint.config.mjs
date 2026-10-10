/** @type {import('eslint').Linter.Config[]} */
import js from '@eslint/js';
import typescriptParser from '@typescript-eslint/parser';
import react from 'eslint-plugin-react';
import reactHooks from 'eslint-plugin-react-hooks';
import tseslint from 'typescript-eslint';

const config = [
  // Global ignores
  {
    ignores: [
      '.next/**',
      'node_modules/**',
      'out/**',
      '.env*',
      'public/**',
      'dist/**',
      'build/**',
      '*.config.js',
      '*.config.mjs',
      'tailwind.config.ts',
      'next.config.js',
      'postcss.config.mjs',
    ],
  },

  // Node.js server files configuration
  {
    files: [
      'server.js',
      'start-pm2.js',
      'ecosystem.config.js',
      'ecosystem.config.template.js',
    ],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'commonjs',
      globals: {
        console: 'readonly',
        process: 'readonly',
        Buffer: 'readonly',
        __dirname: 'readonly',
        __filename: 'readonly',
        module: 'readonly',
        require: 'readonly',
        exports: 'readonly',
        global: 'readonly',
      },
    },
    rules: {
      'no-console': 'off',
      'no-undef': 'off',
    },
  },

  // Node.js module scripts
  {
    files: ['scripts/**/*.mjs'],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      globals: {
        console: 'readonly',
        process: 'readonly',
        Buffer: 'readonly',
        __dirname: 'readonly',
        __filename: 'readonly',
        global: 'readonly',
      },
    },
    rules: {
      'no-console': 'off',
      'no-undef': 'off',
    },
  },

  // Recommended base configs
  js.configs.recommended,

  // Base configuration for all JavaScript/TypeScript files
  {
    files: ['**/*.{js,jsx,ts,tsx}'],
    plugins: {
      react,
      'react-hooks': reactHooks,
    },
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      parserOptions: {
        ecmaFeatures: {
          jsx: true,
        },
      },
      globals: {
        React: 'readonly',
        JSX: 'readonly',
        console: 'readonly',
        process: 'readonly',
        Buffer: 'readonly',
        __dirname: 'readonly',
        __filename: 'readonly',
        global: 'readonly',
        window: 'readonly',
        document: 'readonly',
      },
    },
    settings: {
      react: {
        version: 'detect',
      },
    },
    rules: {
      // === Code Quality ===
      'no-unused-vars': 'off', // TypeScript handles this
      'no-console': ['warn', { allow: ['warn', 'error'] }],
      'no-debugger': 'warn',
      'prefer-const': 'error',
      'no-var': 'error',

      // === Code Style ===
      quotes: ['error', 'single', { avoidEscape: true, allowTemplateLiterals: true }],
      semi: ['error', 'always'],
      'object-curly-spacing': ['error', 'always'],
      'array-bracket-spacing': ['error', 'never'],
      indent: 'off',

      // === Best Practices ===
      eqeqeq: ['error', 'always', { null: 'ignore' }],
      curly: ['error', 'multi-line'],
      'no-eval': 'error',
      'no-implied-eval': 'error',

      // === React Rules ===
      'react/jsx-uses-react': 'off', // Not needed with new JSX transform
      'react/react-in-jsx-scope': 'off', // Not needed with new JSX transform
      'react/jsx-uses-vars': 'error',
      'react/jsx-key': 'error',
      'react/prop-types': 'off', // TypeScript handles this
      'react/display-name': 'warn',
      'react/self-closing-comp': 'error',
      'react/jsx-boolean-value': ['error', 'never'],
      'react/jsx-curly-brace-presence': ['error', { props: 'never', children: 'never' }],

      // === React Hooks ===
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
    },
  },

  // TypeScript specific configuration
  {
    files: ['**/*.{ts,tsx}'],
    // Ensure TS plugin is available in flat config
    plugins: {
      '@typescript-eslint': tseslint.plugin,
    },
    rules: {
      // Disable base rule which doesn't understand TS declaration merging
      'no-redeclare': 'off',
      // Enable TS-aware rule and allow declaration merging (function overloads, interface merging)
      '@typescript-eslint/no-redeclare': ['error', { ignoreDeclarationMerge: true }],
    },
    languageOptions: {
      parser: typescriptParser,
      parserOptions: {
        ecmaVersion: 'latest',
        sourceType: 'module',
        ecmaFeatures: {
          jsx: true,
        },
      },
    },
    rules: {
      // Disable base rules that are covered by TypeScript
      'no-unused-vars': 'off',
      'no-undef': 'off',

      // === TypeScript Rules ===
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          caughtErrorsIgnorePattern: '^_',
        },
      ],
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/prefer-as-const': 'error',
      '@typescript-eslint/no-non-null-assertion': 'warn',
      '@typescript-eslint/consistent-type-imports': ['error', { prefer: 'type-imports' }],
      '@typescript-eslint/no-empty-interface': 'error',
      '@typescript-eslint/array-type': ['error', { default: 'array-simple' }],
      '@typescript-eslint/consistent-type-definitions': ['error', 'interface'],
    },
  },

  // Money safety: the billing and model-price presentation layer must not run
  // money arithmetic on binary floats. Money is canonical USD handled through
  // decimal.js, so parseFloat / Number() arithmetic / Math.round are rejected.
  {
    files: [
      'src/utils/money.ts',
      'src/utils/billing-display.ts',
      'src/utils/model-price.ts',
      'src/utils/build-custom-model-pricing.ts',
      'src/components/providers/model-price-dialog.tsx',
      'src/components/providers/custom-model-dialog.tsx',
    ],
    rules: {
      'no-restricted-syntax': [
        'error',
        {
          selector: "CallExpression[callee.name='parseFloat']",
          message: 'Money must use decimal.js, not parseFloat().',
        },
        {
          selector: "MemberExpression[object.name='Number'][property.name='parseFloat']",
          message: 'Money must use decimal.js, not Number.parseFloat().',
        },
        {
          selector: "BinaryExpression[operator='+']:has(CallExpression[callee.name='Number'])",
          message: 'Money must use decimal.js, not Number() arithmetic.',
        },
        {
          selector: "BinaryExpression[operator='-']:has(CallExpression[callee.name='Number'])",
          message: 'Money must use decimal.js, not Number() arithmetic.',
        },
        {
          selector: "BinaryExpression[operator='*']:has(CallExpression[callee.name='Number'])",
          message: 'Money must use decimal.js, not Number() arithmetic.',
        },
        {
          selector: "BinaryExpression[operator='/']:has(CallExpression[callee.name='Number'])",
          message: 'Money must use decimal.js, not Number() arithmetic.',
        },
        {
          selector: "BinaryExpression[operator='%']:has(CallExpression[callee.name='Number'])",
          message: 'Money must use decimal.js, not Number() arithmetic.',
        },
      ],
    },
  },

  // Rounding of money values must happen through decimal.js only. The dialogs
  // are excluded because they round non-money config parameters with Math.round.
  {
    files: [
      'src/utils/money.ts',
      'src/utils/billing-display.ts',
      'src/utils/model-price.ts',
      'src/utils/build-custom-model-pricing.ts',
    ],
    rules: {
      'no-restricted-syntax': [
        'error',
        {
          selector: "CallExpression[callee.name='parseFloat']",
          message: 'Money must use decimal.js, not parseFloat().',
        },
        {
          selector: "MemberExpression[object.name='Number'][property.name='parseFloat']",
          message: 'Money must use decimal.js, not Number.parseFloat().',
        },
        {
          selector: "BinaryExpression[operator='+']:has(CallExpression[callee.name='Number'])",
          message: 'Money must use decimal.js, not Number() arithmetic.',
        },
        {
          selector: "BinaryExpression[operator='-']:has(CallExpression[callee.name='Number'])",
          message: 'Money must use decimal.js, not Number() arithmetic.',
        },
        {
          selector: "BinaryExpression[operator='*']:has(CallExpression[callee.name='Number'])",
          message: 'Money must use decimal.js, not Number() arithmetic.',
        },
        {
          selector: "BinaryExpression[operator='/']:has(CallExpression[callee.name='Number'])",
          message: 'Money must use decimal.js, not Number() arithmetic.',
        },
        {
          selector: "BinaryExpression[operator='%']:has(CallExpression[callee.name='Number'])",
          message: 'Money must use decimal.js, not Number() arithmetic.',
        },
        {
          selector: "CallExpression[callee.object.name='Math'][callee.property.name='round']",
          message: 'Money must be rounded with decimal.js, not Math.round().',
        },
      ],
    },
  },

  // Configuration files exceptions
  {
    files: ['*.config.{js,mjs,ts}', 'tailwind.config.ts', 'next.config.js', 'postcss.config.mjs'],
    rules: {
      'no-console': 'off',
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-var-requires': 'off',
    },
  },

  // Test files exceptions
  {
    files: [
      '**/*.test.{js,jsx,ts,tsx}',
      '**/*.spec.{js,jsx,ts,tsx}',
      '**/__tests__/**/*.{js,jsx,ts,tsx}',
    ],
    rules: {
      'no-console': 'off',
      '@typescript-eslint/no-explicit-any': 'off',
      indent: 'off',
      'react/jsx-indent': 'off',
      'react/jsx-indent-props': 'off',
    },
  },
];

export default config;
